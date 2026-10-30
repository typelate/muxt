package check

import (
	"fmt"
	"go/ast"
	"go/types"
	"iter"
	"slices"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/check/internal/asteval"
	"github.com/typelate/muxt/internal/check/internal/astgen"
	"github.com/typelate/muxt/internal/template"
	htttemplate "github.com/typelate/muxt/internal/template/html"
)

// LoadTemplates resolves the construction chain of the package-level
// template variable named templatesVariable and returns its evaluated
// template set. It is the variable-first counterpart to AnalyzePackage's
// per-call-site discovery: a consumer that already knows which variable
// holds its templates loads it once and enumerates the ExecuteTemplate
// calls made on it.
//
// The package must be loaded with syntax, type information, and embed
// files (packages.NeedSyntax, NeedTypes, NeedTypesInfo, and
// NeedEmbedFiles).
func LoadTemplates(pkg *packages.Package, templatesVariable string) (*Templates, error) {
	if pkg.TypesInfo == nil || pkg.Types == nil {
		return nil, fmt.Errorf("package %s was loaded without type information; load it with packages.NeedTypes and packages.NeedTypesInfo", pkg.PkgPath)
	}

	var (
		variable types.Object
		value    ast.Expr
	)
	for _, tv := range astgen.IterateValueSpecs(pkg.Syntax) {
		for i, ident := range tv.Names {
			if ident.Name != templatesVariable || i >= len(tv.Values) {
				continue
			}
			if obj := pkg.TypesInfo.Defs[ident]; obj != nil {
				variable = obj
				value = tv.Values[i]
			}
		}
	}
	if variable == nil {
		return nil, fmt.Errorf("variable %s not found in package %s", templatesVariable, pkg.PkgPath)
	}

	workingDirectory := packageDirectory(pkg)
	embeddedPaths, err := asteval.RelativeFilePaths(workingDirectory, pkg.EmbedFiles...)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate relative path for embedded files: %w", err)
	}

	// EvaluateTemplateSelector resolves the chain and augments functions in
	// place with every Funcs-registered function.
	functions := asteval.DefaultFunctions(pkg.Types)
	baseline := make(map[string]*types.Signature, len(functions))
	for name, sig := range functions {
		baseline[name] = sig
	}
	meta := new(asteval.TemplateMetadata)
	ts, _, _, err := asteval.EvaluateTemplateSelector(nil, pkg.Types, pkg.TypesInfo, value, workingDirectory, templatesVariable, "", "", pkg.Fset, pkg.Syntax, embeddedPaths, functions, make(map[string]any), meta)
	if err != nil {
		return nil, err
	}
	collected := make(Functions)
	for name, sig := range functions {
		if base, ok := baseline[name]; !ok || base != sig {
			collected[name] = sig
		}
	}

	// The callable set is htt's builtin signatures — the same ones
	// AnalyzePackage reports per Entry — overlaid with the chain-registered
	// functions.
	merged := DefaultFunctions(pkg.Types)
	for name, sig := range functions {
		merged[name] = sig
	}

	result := &Templates{
		pkg:         pkg,
		variable:    variable,
		templates:   ts,
		functions:   merged,
		collected:   collected,
		definitions: newDefinitionSet(definitionsFor(pkg.Fset, meta.Sources)),
	}
	result.definitions.adoptTrees(result)
	return result, nil
}

// Templates is one template variable's evaluated construction chain,
// together with the package it was loaded from so the ExecuteTemplate
// calls made on that variable can be enumerated.
type Templates struct {
	pkg         *packages.Package
	variable    types.Object
	templates   *template.Template[*types.Signature]
	functions   Functions
	collected   Functions
	definitions definitionSet
}

// Set returns the evaluated template set; use its Names, Lookup, and
// FindTree to inspect the templates the chain parsed.
func (t *Templates) Set() *template.Template[*types.Signature] {
	return t.templates
}

// Functions are the callable functions available to the template set:
// htt's builtin signatures merged with the functions the construction
// chain registered through Funcs calls.
func (t *Templates) Functions() Functions {
	return t.functions
}

// CollectedFunctions are only the functions the construction chain
// registered through Funcs calls, without the builtin defaults.
func (t *Templates) CollectedFunctions() Functions {
	return t.collected
}

// FindTree returns the parse tree for the named template in the set.
func (t *Templates) FindTree(name string) (*template.Tree, bool) {
	return t.templates.FindTree(name)
}

// FindDefinition implements DefinitionFinder: it reports where name was
// defined, with Definition.Define spanning the define or block clause.
func (t *Templates) FindDefinition(name string) (Definition, bool) {
	return t.definitions.FindDefinition(name)
}

// Template reconstructs a runtime *github.com/typelate/htt/template.Template
// from the resolved set, so callers can inspect it with the html/template-
// shaped API (Lookup, Templates, Tree). ok is false when the set is empty.
//
// Reconstructing the template set calls AddParseTree, which sets HTMLMode and
// annotates HTML contexts on the underlying *htt.Tree values IN PLACE. Since
// the trees are shared with the check set (Set()), calling Template() has the
// side effect of annotating those shared trees. This is intended for the
// structural/HTML-mode use the bridge exists for; callers needing pristine
// trees should clone them first.
func (t *Templates) Template() (*htttemplate.Template, bool) {
	if t.templates == nil {
		return nil, false
	}
	names := t.templates.Names()
	if len(names) == 0 {
		return nil, false
	}
	out := htttemplate.New(names[0])
	for _, name := range names {
		tree, ok := t.templates.FindTree(name)
		if !ok || tree == nil {
			continue
		}
		if _, err := out.AddParseTree(name, tree); err != nil {
			return nil, false
		}
	}
	return out, true
}

// HTML returns the template set as an html/template-shaped value, as
// Template does; ok is false when the variable holds a text/template
// value, so a caller can tell which package built it.
func (t *Templates) HTML() (*htttemplate.Template, bool) {
	if t.templatePackagePath() == "text/template" {
		return nil, false
	}
	return t.Template()
}

// Text returns the evaluated template set when the variable holds a
// text/template value; ok is false when it holds an html/template value.
// The fork parses both kinds into the same set type, so this is the set
// Set returns, gated on the variable's type.
func (t *Templates) Text() (*template.Template[*types.Signature], bool) {
	if t.templates == nil || t.templatePackagePath() != "text/template" {
		return nil, false
	}
	return t.templates, true
}

// templatePackagePath is the import path of the package declaring the
// variable's template type: "html/template" or "text/template".
func (t *Templates) templatePackagePath() string {
	if t.variable == nil {
		return ""
	}
	named, ok := dereference(t.variable.Type()).(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return ""
	}
	return named.Obj().Pkg().Path()
}

// ExecuteTemplateCalls reports, in file order, each ExecuteTemplate call
// in the loaded package whose receiver resolves to this variable's object
// — resolution is by object, so a shadowed or renamed identifier does not
// match — and whose template name is a string literal. Each Entry carries
// the invoked template's tree (nil when the set does not know the name)
// and the resolved set, so callers can locate the definition via
// Tree.Position or Set().Lookup.
func (t *Templates) ExecuteTemplateCalls() iter.Seq[Entry] {
	return func(yield func(Entry) bool) {
		if t.pkg == nil || t.variable == nil {
			return
		}
		functionNames := make([]string, 0, len(t.functions))
		for name := range t.functions {
			functionNames = append(functionNames, name)
		}
		slices.Sort(functionNames)

		for _, file := range t.pkg.Syntax {
			keepWalking := true
			ast.Inspect(file, func(node ast.Node) bool {
				if !keepWalking {
					return false
				}
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) != 3 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "ExecuteTemplate" {
					return true
				}
				if !asteval.IsTemplateMethod(t.pkg.TypesInfo, sel) {
					return true
				}
				receiverIdent, ok := sel.X.(*ast.Ident)
				if !ok || t.pkg.TypesInfo.Uses[receiverIdent] != t.variable {
					return true
				}
				templateName, ok := asteval.BasicLiteralString(call.Args[1])
				if !ok {
					return true
				}
				var tree *template.Tree
				if looked := t.templates.Lookup(templateName); looked != nil {
					tree = looked.Tree()
				}
				keepWalking = yield(Entry{
					Call:         call,
					TemplateName: templateName,
					DataType:     t.pkg.TypesInfo.TypeOf(call.Args[2]),
					Templates:    t.templates,
					Tree:         tree,
					Functions:    functionNames,
				})
				return keepWalking
			})
			if !keepWalking {
				return
			}
		}
	}
}
