package check

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/check/internal/asteval"
	"github.com/typelate/muxt/internal/check/internal/astgen"
	"github.com/typelate/muxt/internal/template"
)

type pendingCall struct {
	call         *ast.CallExpr
	receiverObj  types.Object
	templateName string
	dataType     types.Type
}

type resolvedTemplate struct {
	templates   *template.Template[*types.Signature]
	functions   asteval.TemplateFunctions
	metadata    *asteval.TemplateMetadata
	definitions definitionSet
}

// definitionsFor locates every template definition in the sources that
// were parsed while resolving a template variable, in parse order.
func definitionsFor(fset *token.FileSet, sources []asteval.ParsedText) []Definition {
	var ordered []Definition
	for _, src := range sources {
		if src.Literal != nil {
			ordered = append(ordered, definitionsInLiteral(fset, src.Literal, src.RootName, src.LeftDelim, src.RightDelim)...)
			continue
		}
		ordered = append(ordered, definitionsIn(src.RootName, src.Filename, src.Text, src.LeftDelim, src.RightDelim)...)
	}
	return ordered
}

// ExecuteTemplateNodeInspectorFunc is called for each ExecuteTemplate
// call that resolves to a known template.
//
// The tree is the invoked template's own tree, the same one def.Tree
// holds. The type is the data argument's type, and def locates where the
// template was defined.
type ExecuteTemplateNodeInspectorFunc func(node *ast.CallExpr, t *template.Tree, tp types.Type, def Definition)

// Package discovers all .ExecuteTemplate calls in the given package,
// resolves receiver variables to their template construction chains,
// and type-checks each call.
//
// ExecuteTemplate must be called with a string literal for the second parameter.
func Package(pkg *packages.Package, inspectCall ExecuteTemplateNodeInspectorFunc, inspectTemplate TemplateNodeInspectorFunc) error {
	pending, receivers, _ := findExecuteCalls(pkg)
	resolved, resolveErrs := resolveTemplates(pkg, receivers)
	callErr := checkCalls(pkg, pending, resolved, inspectCall, inspectTemplate)
	return errors.Join(append(resolveErrs, callErr)...)
}

// Entry is one resolved ExecuteTemplate call site: the template the call
// names, the data type it passes, and the resolved template set that name
// was found in (which also resolves any {{template}} names reachable from
// it). Tooling receives one Entry per checked call via AnalyzePackage.
type Entry struct {
	Call         *ast.CallExpr                        // the ExecuteTemplate(...) call
	TemplateName string                               // the invoked template's name
	DataType     types.Type                           // type of the third (data) argument
	Templates    *template.Template[*types.Signature] // resolved set; use Lookup/FindTree for names
	Tree         *template.Tree                       // the invoked template's tree (Entry point)
	Functions    []string                             // names of callable functions (builtins + funcmap), sorted
}

// AnalyzePackage runs the same ExecuteTemplate discovery, template
// resolution, and type-check pipeline as Package, but for each resolved
// call it invokes onEntry(entry) and installs the returned
// NodeInspectorFunc (which may be nil) as the per-node type hook while
// checking that entry. It lets tooling such as the LSP build a per-template
// offset->type index and resolve {{template}} names to their definitions in
// one workspace pass. It returns the joined resolution and check errors.
//
// Like Package, it type-checks each entry by walking its template, which
// recurses through {{template}} invocations; a directly or indirectly
// self-recursive template set is not supported (same limitation as Package).
func AnalyzePackage(pkg *packages.Package, onEntry func(Entry) NodeInspectorFunc) ([]Warning, error) {
	pending, receivers, nonStaticNames := findExecuteCalls(pkg)
	resolved, resolveErrs := resolveTemplates(pkg, receivers)

	var warnings []Warning
	for _, pos := range nonStaticNames {
		warnings = append(warnings, Warning{
			Code:    WarnNonStaticName,
			Message: "ExecuteTemplate is called with a non-literal template name; it cannot be checked statically",
			Pos:     pos,
		})
	}
	warnings = append(warnings, unusedTemplateWarnings(resolved, pending)...)

	mergedFunctions := make(Functions)
	if pkg.Types != nil {
		mergedFunctions = DefaultFunctions(pkg.Types)
	}
	for _, rt := range resolved {
		for name, sig := range rt.functions {
			mergedFunctions[name] = sig
		}
	}
	functionNames := make([]string, 0, len(mergedFunctions))
	for name := range mergedFunctions {
		functionNames = append(functionNames, name)
	}
	slices.Sort(functionNames)

	var errs []error
	var invocations []tmplInvocation
	for _, p := range pending {
		rt, ok := resolved[p.receiverObj]
		if !ok {
			continue
		}
		looked := rt.templates.Lookup(p.templateName)
		if looked == nil {
			continue
		}
		tree := looked.Tree()
		if tree == nil {
			continue
		}
		global := NewGlobal(pkg.Types, pkg.Fset, rt.templates, mergedFunctions)
		global.InspectWarning = func(w Warning) { warnings = append(warnings, w) }
		global.InspectTemplateNode = func(n *template.TemplateNode, t *template.Tree, dot types.Type, _ Definition) {
			invocations = append(invocations, tmplInvocation{name: n.Name, dot: dot, tree: t, node: n})
		}
		if onEntry != nil {
			global.InspectNode = onEntry(Entry{
				Call:         p.call,
				TemplateName: p.templateName,
				DataType:     p.dataType,
				Templates:    rt.templates,
				Tree:         tree,
				Functions:    functionNames,
			})
		}
		if err := Execute(global, tree, p.dataType); err != nil {
			errs = append(errs, err)
		}
	}

	warnings = append(warnings, inconsistentTemplateWarnings(invocations)...)
	return warnings, errors.Join(append(resolveErrs, errs...)...)
}

// tmplInvocation records one {{template "name" pipe}} call site and the dot
// type it passes, for the W007 cross-call-site consistency check.
type tmplInvocation struct {
	name string
	dot  types.Type
	tree *template.Tree
	node *template.TemplateNode
}

// inconsistentTemplateWarnings reports W007 when a sub-template is invoked
// with data types that are mutually incompatible across its call sites (e.g.
// one site passes a struct, another an int). Compatible variations — the same
// type, a pointer vs. value, or an interface on either side — do not warn.
func inconsistentTemplateWarnings(invocations []tmplInvocation) []Warning {
	byName := make(map[string][]tmplInvocation)
	for _, inv := range invocations {
		if inv.dot != nil {
			byName[inv.name] = append(byName[inv.name], inv)
		}
	}
	var warnings []Warning
	for name, invs := range byName {
		var a, b types.Type
		conflict := false
		for i := 0; i < len(invs) && !conflict; i++ {
			for j := i + 1; j < len(invs); j++ {
				if !templateTypesCompatible(invs[i].dot, invs[j].dot) {
					a, b, conflict = invs[i].dot, invs[j].dot, true
					break
				}
			}
		}
		if !conflict {
			continue
		}
		msg := fmt.Sprintf("template %q is invoked with inconsistent data types (%s and %s)",
			name, types.TypeString(dereference(a), nil), types.TypeString(dereference(b), nil))
		seen := make(map[*template.TemplateNode]bool)
		for _, inv := range invs {
			if seen[inv.node] {
				continue
			}
			seen[inv.node] = true
			warnings = append(warnings, Warning{Code: WarnInconsistentTemplate, Message: msg, Tree: inv.tree, Node: inv.node})
		}
	}
	return warnings
}

// templateTypesCompatible reports whether two dot types passed to the same
// sub-template are compatible: identical (after pointer deref), assignable
// either way, or with an interface on either side (unknowable).
func templateTypesCompatible(a, b types.Type) bool {
	a, b = dereference(a), dereference(b)
	if types.Identical(a, b) || types.IsInterface(a) || types.IsInterface(b) {
		return true
	}
	return types.AssignableTo(a, b) || types.AssignableTo(b, a)
}

// unusedTemplateWarnings reports W002 for each file-backed template that no
// ExecuteTemplate call and no {{template}} action references. It considers
// only file templates (a template whose name is the base of its file path),
// so {{define}} blocks and the root container template are not flagged, and
// it warns once per file.
func unusedTemplateWarnings(resolved map[types.Object]*resolvedTemplate, pending []pendingCall) []Warning {
	referenced := make(map[string]bool)
	for _, p := range pending {
		referenced[p.templateName] = true
	}
	sets := make(map[*template.Template[*types.Signature]]bool)
	for _, rt := range resolved {
		set := rt.templates
		if set == nil || sets[set] {
			continue
		}
		sets[set] = true
		for _, name := range set.Names() {
			tree, ok := set.FindTree(name)
			if !ok || tree == nil {
				continue
			}
			template.Walk(tree, func(_ *template.Tree, n template.Node) {
				if tn, ok := n.(*template.TemplateNode); ok {
					referenced[tn.Name] = true
				}
			})
		}
	}

	var warnings []Warning
	warned := make(map[string]bool)
	for _, rt := range resolved {
		set := rt.templates
		if set == nil {
			continue
		}
		for _, name := range set.Names() {
			if referenced[name] {
				continue
			}
			tree, ok := set.FindTree(name)
			if !ok || tree == nil || tree.Filepath == "" || name != path.Base(tree.Filepath) {
				continue // skip defines, the root, and referenced files
			}
			if !hasRenderableContent(tree) {
				continue // a partials-only file (just {{define}} blocks) is not "unused"
			}
			if warned[tree.Filepath] {
				continue
			}
			warned[tree.Filepath] = true
			warnings = append(warnings, Warning{
				Code:    WarnUnusedTemplate,
				Message: fmt.Sprintf("template %q is parsed but never referenced", name),
				Tree:    tree,
				Node:    tree.Root,
			})
		}
	}
	return warnings
}

// hasRenderableContent reports whether a template tree renders anything of
// its own — non-whitespace text, an element, an action, or a control
// structure — as opposed to being only {{define}} blocks and whitespace.
//
// htt keeps {{define}} inline (as a DefineNode wrapped in the file's
// HTMLDocumentFragmentNode) rather than extracting it, and htt.Walk descends
// into a DefineNode's body — so this must NOT use htt.Walk: a define-only
// file (the muxt/htt-generate route-file shape) would otherwise be reported
// as having renderable content and W002-flagged as unused, even though its
// defines are used via their route names. A DefineNode is therefore opaque
// here and its body is not descended into.
func hasRenderableContent(tree *template.Tree) bool {
	if tree == nil {
		return false
	}
	return nodeRenders(tree.Root)
}

func nodeRenders(n template.Node) bool {
	switch v := n.(type) {
	case nil:
		return false
	case *template.ListNode:
		if v == nil {
			return false
		}
		for _, c := range v.Nodes {
			if nodeRenders(c) {
				return true
			}
		}
		return false
	case *template.HTMLDocumentFragmentNode:
		for _, c := range v.Children {
			if nodeRenders(c) {
				return true
			}
		}
		return false
	case *template.TextNode:
		return strings.TrimSpace(string(v.Text)) != ""
	case *template.CommentNode, *template.HTMLCommentNode, *template.DefineNode:
		// A {{define}} renders nothing in place; comments and whitespace
		// are not renderable content.
		return false
	default:
		// ActionNode, HTMLElementNode, IfNode/RangeNode/WithNode,
		// TemplateNode, BlockNode, etc. render content in place.
		return true
	}
}

// findExecuteCalls walks the package syntax looking for ExecuteTemplate calls
// and returns the pending calls, the set of receiver objects that need
// template resolution, and the positions of calls whose template name is not
// a string literal (which cannot be checked statically — W001).
func findExecuteCalls(pkg *packages.Package) ([]pendingCall, map[types.Object]struct{}, []token.Pos) {
	var pending []pendingCall
	var nonStaticNames []token.Pos
	receiverSet := make(map[types.Object]struct{})

	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "ExecuteTemplate" {
				return true
			}
			// Verify the method belongs to html/template or text/template.
			if !asteval.IsTemplateMethod(pkg.TypesInfo, sel) {
				return true
			}
			receiverIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			obj := pkg.TypesInfo.Uses[receiverIdent]
			if obj == nil {
				return true
			}
			templateName, ok := asteval.BasicLiteralString(call.Args[1])
			if !ok {
				nonStaticNames = append(nonStaticNames, call.Args[1].Pos())
				return true
			}
			dataType := pkg.TypesInfo.TypeOf(call.Args[2])
			pending = append(pending, pendingCall{
				call:         call,
				receiverObj:  obj,
				templateName: templateName,
				dataType:     dataType,
			})
			receiverSet[obj] = struct{}{}
			return true
		})
	}

	return pending, receiverSet, nonStaticNames
}

// resolveTemplates resolves each unique receiver object to its template
// construction chain, including additional ParseFS/Parse modifications.
func resolveTemplates(pkg *packages.Package, receivers map[types.Object]struct{}) (map[types.Object]*resolvedTemplate, []error) {
	resolved := make(map[types.Object]*resolvedTemplate)

	workingDirectory := packageDirectory(pkg)
	embeddedPaths, err := asteval.RelativeFilePaths(workingDirectory, pkg.EmbedFiles...)
	if err != nil {
		return nil, []error{fmt.Errorf("failed to calculate relative path for embedded files: %w", err)}
	}

	var resolveErrs []error

	resolveExpr := func(obj types.Object, name string, expr ast.Expr) {
		if _, needed := receivers[obj]; !needed {
			return
		}
		funcTypeMap := asteval.DefaultFunctions(pkg.Types)
		meta := &asteval.TemplateMetadata{}
		ts, _, _, err := asteval.EvaluateTemplateSelector(nil, pkg.Types, pkg.TypesInfo, expr, workingDirectory, name, "", "", pkg.Fset, pkg.Syntax, embeddedPaths, funcTypeMap, make(map[string]any), meta)
		if err != nil {
			resolveErrs = append(resolveErrs, err)
			return
		}
		resolved[obj] = &resolvedTemplate{
			templates: ts,
			functions: funcTypeMap,
			metadata:  meta,
		}
	}

	// Resolve top-level var declarations.
	for _, tv := range astgen.IterateValueSpecs(pkg.Syntax) {
		for i, ident := range tv.Names {
			if i >= len(tv.Values) {
				continue
			}
			obj := pkg.TypesInfo.Defs[ident]
			if obj == nil {
				continue
			}
			resolveExpr(obj, ident.Name, tv.Values[i])
		}
	}

	// Resolve function-local var declarations and short variable declarations.
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.DeclStmt:
				gd, ok := n.Decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.VAR {
					return true
				}
				for _, spec := range gd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, ident := range vs.Names {
						if i >= len(vs.Values) {
							continue
						}
						obj := pkg.TypesInfo.Defs[ident]
						if obj == nil {
							continue
						}
						resolveExpr(obj, ident.Name, vs.Values[i])
					}
				}
			case *ast.AssignStmt:
				if n.Tok != token.DEFINE {
					return true
				}
				for i, lhs := range n.Lhs {
					if i >= len(n.Rhs) {
						continue
					}
					ident, ok := lhs.(*ast.Ident)
					if !ok {
						continue
					}
					obj := pkg.TypesInfo.Defs[ident]
					if obj == nil {
						continue
					}
					resolveExpr(obj, ident.Name, n.Rhs[i])
				}
			}
			return true
		})
	}

	// Find additional ParseFS/Parse calls on resolved template variables.
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			obj := asteval.FindModificationReceiver(call, pkg.TypesInfo)
			if obj == nil {
				return true
			}
			rt, ok := resolved[obj]
			if !ok {
				return true
			}
			meta := &asteval.TemplateMetadata{}
			ts, _, _, err := asteval.EvaluateTemplateSelector(rt.templates, pkg.Types, pkg.TypesInfo, call, workingDirectory, "", "", "", pkg.Fset, pkg.Syntax, embeddedPaths, rt.functions, make(map[string]any), meta)
			if err != nil {
				return true
			}
			rt.templates = ts
			rt.metadata.EmbedFilePaths = append(rt.metadata.EmbedFilePaths, meta.EmbedFilePaths...)
			rt.metadata.ParseCalls = append(rt.metadata.ParseCalls, meta.ParseCalls...)
			rt.metadata.Sources = append(rt.metadata.Sources, meta.Sources...)
			return true
		})
	}

	return resolved, resolveErrs
}

// checkCalls type-checks each pending ExecuteTemplate call against its
// resolved template.
func checkCalls(pkg *packages.Package, pending []pendingCall, resolved map[types.Object]*resolvedTemplate, inspectCall ExecuteTemplateNodeInspectorFunc, inspectTemplate TemplateNodeInspectorFunc) error {
	mergedFunctions := make(Functions)
	if pkg.Types != nil {
		mergedFunctions = DefaultFunctions(pkg.Types)
	}
	for _, rt := range resolved {
		for name, sig := range rt.functions {
			mergedFunctions[name] = sig
		}
		rt.definitions = newDefinitionSet(definitionsFor(pkg.Fset, rt.metadata.Sources))
		rt.definitions.adoptTrees(rt.templates)
	}

	var errs []error
	for _, p := range pending {
		rt, ok := resolved[p.receiverObj]
		if !ok {
			continue
		}
		looked := rt.templates.Lookup(p.templateName)
		if looked == nil {
			continue
		}
		tree := looked.Tree()
		if tree == nil {
			// Hardening over upstream typelate/check, which passes a nil
			// tree straight to Execute and panics on tree.Root. TODO: port
			// this guard upstream so the two stay parallel.
			continue
		}
		global := NewGlobal(pkg.Types, pkg.Fset, rt.templates, mergedFunctions)
		global.InspectTemplateNode = inspectTemplate
		global.Definitions = rt.definitions
		if inspectCall != nil {
			inspectCall(p.call, tree, p.dataType, global.definition(p.templateName))
		}
		if err := Execute(global, tree, p.dataType); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func packageDirectory(pkg *packages.Package) string {
	if len(pkg.GoFiles) > 0 {
		return filepath.Dir(pkg.GoFiles[0])
	}
	return "."
}
