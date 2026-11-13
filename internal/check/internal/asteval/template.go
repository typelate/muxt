package asteval

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/typelate/muxt/internal/check/internal/astgen"
	"github.com/typelate/muxt/template"
	"github.com/typelate/muxt/template/lex"
)

// TemplateMetadata accumulates metadata during template evaluation.
type TemplateMetadata struct {
	EmbedFilePaths []string
	ParseCalls     []*ast.BasicLit
	Sources        []ParsedText
}

// ParsedText records the source one Parse or ParseFS call consumed, in
// call order, so that template definitions can be located afterward.
//
// The delimiters are recorded because they are only known here: they are
// set by Delims calls earlier in the chain, and neither the template nor
// its trees report them afterward.
type ParsedText struct {
	// RootName is the name of the template that owns the text.
	RootName string

	// Filename is the template file the text was read from and Text is
	// its content. Both are empty when the text came from a Go string
	// literal, which Literal holds instead.
	Filename string
	Text     string

	// Literal is the Go string literal the text was written as, nil when
	// the text was read from a file.
	Literal *ast.BasicLit

	// LeftDelim and RightDelim are the delimiters in effect for this
	// call. Empty means the template defaults.
	LeftDelim  string
	RightDelim string

	// Options are the lexer options the text was parsed with.
	Options lex.Options

	// Package is the import path of the package whose files held the text,
	// empty for the analyzed package and for Go string literals. It is how
	// a definition is traced back to the package that owns it.
	Package string
}

// recordParsedLiteral notes a Parse call whose argument is a string
// literal. A call built any other way, such as from concatenation, has no
// single literal to resolve positions against and is not recorded.
func recordParsedLiteral(meta *TemplateMetadata, arg ast.Expr, rootName, leftDelim, rightDelim string, options lex.Options) {
	if meta == nil {
		return
	}
	lit, ok := arg.(*ast.BasicLit)
	if !ok {
		return
	}
	meta.ParseCalls = append(meta.ParseCalls, lit)
	meta.Sources = append(meta.Sources, ParsedText{
		RootName:   rootName,
		Literal:    lit,
		LeftDelim:  leftDelim,
		RightDelim: rightDelim,
		Options:    withDelimiters(options, leftDelim, rightDelim),
	})
}

// EvaluateTemplateSelector evaluates one call of a template construction
// chain, such as New("x"), Parse("...") or ParseFS(fsys, "*.gohtml"). chain is
// what the calls before it built; the result is the chain after this call, so
// each step sees the delimiters an earlier Delims call set. A call muxt cannot
// read is an error that names its file and line.
func EvaluateTemplateSelector(scope Scope, chain Chain, expression ast.Expr, funcs TemplateFunctions, fm map[string]any, meta *TemplateMetadata) (Chain, error) {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return chain.fail(wrapWithFilename(scope.Dir, scope.Fset, expression.Pos(), fmt.Errorf("expected call expression")))
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return chain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Fun.Pos(), fmt.Errorf("unexpected expression %T: %s", call.Fun, astgen.Format(call.Fun))))
	}
	switch x := sel.X.(type) {
	default:
		return chain.fail(wrapWithFilename(scope.Dir, scope.Fset, sel.X.Pos(), fmt.Errorf("expected exactly one argument %s got %d", astgen.Format(sel.X), len(call.Args))))
	case *ast.Ident:
		if !IsTemplatePkgIdent(scope.Info, x) {
			if chain.Templates == nil {
				return chain.fail(wrapWithFilename(scope.Dir, scope.Fset, sel.X.Pos(), fmt.Errorf("expected template package got %s", astgen.Format(sel.X))))
			}
			// Variable receiver — apply method to existing template.
			switch sel.Sel.Name {
			case "ParseFS":
				filePaths, pkgPath, err := evaluateCallParseFilesArgs(scope, call)
				if err != nil {
					return chain.fail(err)
				}
				if meta != nil {
					meta.EmbedFilePaths = append(meta.EmbedFilePaths, filePaths...)
				}
				t, err := parseFiles(chain.Templates, false, fm, meta, pkgPath, chain.LeftDelim, chain.RightDelim, filePaths...)
				return chain.next(t, err)
			case "Parse":
				if len(call.Args) != 1 {
					return chain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Lparen, fmt.Errorf("expected exactly one string literal argument")))
				}
				sl, err := StringLiteralExpression(scope.Dir, scope.Fset, call.Args[0])
				if err != nil {
					return chain.fail(err)
				}
				recordParsedLiteral(meta, call.Args[0], chain.Templates.Name(), chain.LeftDelim, chain.RightDelim, chain.Templates.ParseOptions())
				t, err := chain.Templates.Parse(sl)
				return chain.next(t, err)
			case "Funcs", "Functions":
				if err := evaluateFuncMap(scope, call, fm, funcs); err != nil {
					return chain.fail(err)
				}
				sigs := make(map[string]*types.Signature)
				for name, sig := range funcs {
					sigs[name] = sig
				}
				return chain.next(chain.Templates.Functions(sigs), nil)
			case "Option":
				list, err := StringLiteralExpressionList(scope.Dir, scope.Fset, scope.Files, call.Args)
				if err != nil {
					return chain.fail(err)
				}
				t, err := chain.Templates.Option(list...)
				if err != nil {
					return chain.fail(err)
				}
				return chain.next(t, nil)
			case "Delimiters", "Delims":
				if len(call.Args) != 2 {
					return chain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Lparen, fmt.Errorf("expected exactly two string literal arguments")))
				}
				list, err := StringLiteralExpressionList(scope.Dir, scope.Fset, scope.Files, call.Args)
				if err != nil {
					return chain.fail(err)
				}
				return chain.delimited(chain.Templates.Delimiters(list[0], list[1]), list[0], list[1])
			default:
				return chain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Fun.Pos(), fmt.Errorf("unsupported method %s on variable receiver", sel.Sel.Name)))
			}
		}
		switch sel.Sel.Name {
		case "Must":
			if len(call.Args) != 1 {
				return chain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Lparen, fmt.Errorf("expected exactly one argument %s got %d", astgen.Format(sel.X), len(call.Args))))
			}
			return EvaluateTemplateSelector(scope, chain, call.Args[0], funcs, fm, meta)
		case "New":
			if len(call.Args) != 1 {
				return chain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Lparen, fmt.Errorf("expected exactly one string literal argument")))
			}
			templateNames, err := StringLiteralExpressionList(scope.Dir, scope.Fset, scope.Files, call.Args)
			if err != nil {
				return chain.fail(err)
			}
			if isTextTemplatePkgIdent(scope.Info, x) {
				return chain.next(template.NewText[*types.Signature](templateNames[0]), nil)
			}
			return chain.next(template.New[*types.Signature](templateNames[0]), nil)
		case "ParseFS":
			filePaths, pkgPath, err := evaluateCallParseFilesArgs(scope, call)
			if err != nil {
				return chain.fail(err)
			}
			if meta != nil {
				meta.EmbedFilePaths = append(meta.EmbedFilePaths, filePaths...)
			}
			t, err := parseFiles(nil, isTextTemplatePkgIdent(scope.Info, x), fm, meta, pkgPath, chain.LeftDelim, chain.RightDelim, filePaths...)
			return chain.next(t, err)
		default:
			return chain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Fun.Pos(), fmt.Errorf("unsupported function %s%s", sel.Sel.Name, unsupportedFunctionHint(sel.Sel.Name))))
		}
	case *ast.CallExpr:
		upChain, err := EvaluateTemplateSelector(scope, chain, sel.X, funcs, fm, meta)
		if err != nil {
			return chain.fail(err)
		}
		up := upChain.Templates
		upLDelim, upRDelim := upChain.LeftDelim, upChain.RightDelim
		switch sel.Sel.Name {
		case "Delimiters", "Delims":
			if len(call.Args) != 2 {
				return upChain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Lparen, fmt.Errorf("expected exactly two string literal arguments")))
			}
			list, err := StringLiteralExpressionList(scope.Dir, scope.Fset, scope.Files, call.Args)
			if err != nil {
				return upChain.fail(err)
			}
			return chain.delimited(up.Delimiters(list[0], list[1]), list[0], list[1])
		case "Parse":
			if len(call.Args) != 1 {
				return upChain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Lparen, fmt.Errorf("expected exactly one string literal argument")))
			}
			sl, err := StringLiteralExpression(scope.Dir, scope.Fset, call.Args[0])
			if err != nil {
				return upChain.fail(err)
			}
			recordParsedLiteral(meta, call.Args[0], up.Name(), upLDelim, upRDelim, up.ParseOptions())
			t, err := up.Parse(sl)
			return upChain.next(t, err)
		case "New":
			if len(call.Args) != 1 {
				return upChain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Lparen, fmt.Errorf("expected exactly one string literal argument")))
			}
			templateNames, err := StringLiteralExpressionList(scope.Dir, scope.Fset, scope.Files, call.Args)
			if err != nil {
				return upChain.fail(err)
			}
			// Create a new associated template
			// htt.Template doesn't have a New method on instances, so we use AddParseTree
			// to add a named tree to the existing template set.
			options := withDelimiters(up.ParseOptions(), upLDelim, upRDelim)
			newTree, _ := template.ParseTree(templateNames[0], templateNames[0], "", nil, options)
			t, err := up.AddParseTree(templateNames[0], newTree)
			if err != nil {
				return upChain.fail(err)
			}
			return upChain.next(t, nil)
		case "ParseFS":
			filePaths, pkgPath, err := evaluateCallParseFilesArgs(scope, call)
			if err != nil {
				return upChain.fail(err)
			}
			if meta != nil {
				meta.EmbedFilePaths = append(meta.EmbedFilePaths, filePaths...)
			}
			t, err := parseFiles(up, false, fm, meta, pkgPath, upLDelim, upRDelim, filePaths...)
			return upChain.next(t, err)
		case "Option":
			list, err := StringLiteralExpressionList(scope.Dir, scope.Fset, scope.Files, call.Args)
			if err != nil {
				return upChain.fail(err)
			}
			t, err := up.Option(list...)
			if err != nil {
				return upChain.fail(err)
			}
			return upChain.next(t, nil)
		case "Funcs", "Functions":
			if err := evaluateFuncMap(scope, call, fm, funcs); err != nil {
				return upChain.fail(err)
			}
			sigs := make(map[string]*types.Signature)
			for name, sig := range funcs {
				sigs[name] = sig
			}
			return upChain.next(up.Functions(sigs), nil)
		default:
			return upChain.fail(wrapWithFilename(scope.Dir, scope.Fset, call.Fun.Pos(), fmt.Errorf("unsupported method %s", sel.Sel.Name)))
		}
	}
}

// IsTemplatePkgIdent reports whether ident refers to the "html/template"
// or "text/template" package via the type checker.
// isTemplateFuncMapLiteral reports whether lit is spelled as a
// template.FuncMap composite literal (an html/template or text/template
// FuncMap), used to validate the spelling when the literal's type does not
// resolve.
func isTemplateFuncMapLiteral(typesInfo *types.Info, lit *ast.CompositeLit) bool {
	sel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "FuncMap" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && IsTemplatePkgIdent(typesInfo, x)
}

// isAcceptedTemplatePkg reports whether path is an accepted template package.
func isAcceptedTemplatePkg(path string) bool {
	return path == "html/template" ||
		path == "text/template"
}

// withDelimiters returns options with the delimiters in effect for a
// call; empty ones are the template defaults.
func withDelimiters(options lex.Options, leftDelim, rightDelim string) lex.Options {
	options.LeftDelim, options.RightDelim = cmp.Or(leftDelim, "{{"), cmp.Or(rightDelim, "}}")
	return options
}

// isTextTemplatePkgIdent reports whether ident refers to the
// "text/template" package, whose sets parse as text rather than HTML.
func isTextTemplatePkgIdent(info *types.Info, ident *ast.Ident) bool {
	if info == nil {
		return false
	}
	pkgName, ok := info.Uses[ident].(*types.PkgName)
	return ok && pkgName.Imported().Path() == "text/template"
}

func IsTemplatePkgIdent(info *types.Info, ident *ast.Ident) bool {
	if info == nil {
		return false
	}
	obj := info.Uses[ident]
	pkgName, ok := obj.(*types.PkgName)
	if !ok {
		return false
	}
	return isAcceptedTemplatePkg(pkgName.Imported().Path())
}

// IsTemplateMethod reports whether sel refers to a method on
// *html/template.Template or *text/template.Template.
func IsTemplateMethod(typesInfo *types.Info, sel *ast.SelectorExpr) bool {
	if typesInfo == nil {
		return false
	}
	selection, ok := typesInfo.Selections[sel]
	if !ok {
		return false
	}
	fn, ok := selection.Obj().(*types.Func)
	if !ok {
		return false
	}
	fnPkg := fn.Pkg()
	if fnPkg == nil {
		return false
	}
	return isAcceptedTemplatePkg(fnPkg.Path())
}

// FindModificationReceiver unwraps template.Must and returns the types.Object
// of the variable receiver for a method call like ts.ParseFS(...) or
// template.Must(ts.ParseFS(...)). Returns nil if no variable receiver is found.
func FindModificationReceiver(expr *ast.CallExpr, typesInfo *types.Info) types.Object {
	sel, ok := expr.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	switch x := sel.X.(type) {
	case *ast.Ident:
		if IsTemplatePkgIdent(typesInfo, x) && sel.Sel.Name == "Must" && len(expr.Args) == 1 {
			inner, ok := expr.Args[0].(*ast.CallExpr)
			if !ok {
				return nil
			}
			return FindModificationReceiver(inner, typesInfo)
		}
		if IsTemplatePkgIdent(typesInfo, x) {
			return nil
		}
		return typesInfo.Uses[x]
	}
	return nil
}

// parseFiles parses each file into t, as ParseFiles does. A nil t is
// created from the first file's name, as a text/template set when text
// is true.
//
// pkgPath is the import path of the package whose files these are, recorded
// on each ParsedText; empty means the analyzed package.
func parseFiles(t *template.Template[*types.Signature], text bool, fm map[string]any, meta *TemplateMetadata, pkgPath, leftDelim, rightDelim string, filenames ...string) (*template.Template[*types.Signature], error) {
	if len(filenames) == 0 {
		return nil, fmt.Errorf("template: no files named in call to ParseFiles")
	}
	for _, filename := range filenames {
		templateName := filepath.Base(filename)
		b, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		s := string(b)
		if t == nil && text {
			t = template.NewText[*types.Signature](templateName)
		}
		if t == nil {
			t = template.New[*types.Signature](templateName)
		}
		if leftDelim != "" || rightDelim != "" {
			t = t.Delimiters(leftDelim, rightDelim)
		}

		// Register function names so the parser doesn't reject them.
		// The actual type signatures are tracked separately.
		funcNames := make(map[string]*types.Signature)
		for name := range fm {
			funcNames[name] = nil
		}
		for name := range builtins() {
			funcNames[name] = nil
		}
		t = t.Functions(funcNames)

		// The set's own options, as its Parse would use: HTML structure
		// unless it is a text/template set, and break and continue as
		// keywords unless it registers functions of those names.
		options := withDelimiters(t.ParseOptions(), leftDelim, rightDelim)

		// Parse with the absolute path as the file name so the tree — and,
		// crucially, a *htt.ParseError on a syntax failure — carries the
		// absolute Filepath, not just the base name. Tooling routes
		// diagnostics by that path. ParseTree expects a slash-separated path
		// (it derives ParseName via path.Base), so feed it ToSlash form;
		// ParseName stays the base name on every platform, keeping error
		// strings unchanged.
		absoluteFilename, err := filepath.Abs(filename)
		if err != nil {
			return nil, err
		}
		// treeSet collects {{block}}-defined sub-templates as a side effect
		// of parsing (htt.ParseTree registers them into whatever map is
		// passed in); htt.Extract, below, only pulls out {{define}} blocks.
		// A nil treeSet here would still let ParseTree run (it allocates its
		// own local map when treeSet is nil), but that map is never
		// returned, so every {{block "name" ...}} in the file would parse
		// successfully and then vanish - unlike html/template, where a block
		// literally is a define, htt keeps them as a distinct parser
		// side-channel that callers must capture. The template/ shim's own
		// Parse does exactly this merge; this mirrors it so form-field
		// validation lookups (template:"..." struct tags naming a {{block}})
		// resolve the same way here as they do at runtime.
		treeSet := make(map[string]*template.Tree)
		tree, err := template.ParseTree(filepath.ToSlash(absoluteFilename), templateName, s, treeSet, options)
		if err != nil {
			var parseErr *template.ParseError
			if pkgPath != "" && errors.As(err, &parseErr) {
				return nil, SharedParseError{Filename: absoluteFilename, Line: parseErr.Position().Line, Err: err}
			}
			return nil, err
		}
		if meta != nil {
			meta.Sources = append(meta.Sources, ParsedText{
				RootName:   templateName,
				Package:    pkgPath,
				Filename:   absoluteFilename,
				Text:       s,
				LeftDelim:  leftDelim,
				RightDelim: rightDelim,
				Options:    options,
			})
		}
		// The file's own tree, its defines and its blocks, with
		// text/template's precedence between definitions of one name.
		for name, dt := range template.DefinedTrees(tree, treeSet) {
			if _, err = t.AddParseTree(name, dt); err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

func builtins() map[string]any {
	type nothing struct{}
	return map[string]any{
		"and":      func() (_ nothing) { return },
		"call":     func() (_ nothing) { return },
		"html":     func() (_ nothing) { return },
		"index":    func() (_ nothing) { return },
		"slice":    func() (_ nothing) { return },
		"js":       func() (_ nothing) { return },
		"len":      func() (_ nothing) { return },
		"not":      func() (_ nothing) { return },
		"or":       func() (_ nothing) { return },
		"print":    func() (_ nothing) { return },
		"printf":   func() (_ nothing) { return },
		"println":  func() (_ nothing) { return },
		"urlquery": func() (_ nothing) { return },

		// Comparisons
		"eq": func() (_ nothing) { return },
		"ge": func() (_ nothing) { return },
		"gt": func() (_ nothing) { return },
		"le": func() (_ nothing) { return },
		"lt": func() (_ nothing) { return },
		"ne": func() (_ nothing) { return },
	}
}

// unreadableFuncsMessage is the error for a Funcs argument muxt cannot trace
// to a template.FuncMap literal. It opens with the method name, so it is not
// written as an error string literal.
const unreadableFuncsMessage = "Funcs: cannot read funcs: muxt reads a template.FuncMap literal, or a package-level var or const holding one"

// sharedFuncsHint tells a caller who passes another package's FuncMap var how
// to share functions. Muxt reads a FuncMap only where it is written, so the
// functions must be exported and listed in a literal in the package that
// parses. It is empty unless arg is a qualified name such as fragments.Funcs,
// since only then is there a package to name in the hint.
func sharedFuncsHint(arg ast.Expr) string {
	sel, ok := arg.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	qualifier, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return fmt.Sprintf("; to share functions across packages, export them and write template.FuncMap{\"name\": %s.Func}", qualifier.Name)
}

// firstArgumentToParseFSMessage is the error for a ParseFS first argument that
// is neither a package-level variable nor a variable of an imported package.
// It names both forms, so the caller sees the shared one.
const firstArgumentToParseFSMessage = "first argument to ParseFS must be an identifier or an exported variable of an imported package"

// unsupportedFunctionHint names the supported alternative for template
// package functions muxt cannot evaluate.
func unsupportedFunctionHint(name string) string {
	if name == "ParseGlob" {
		return ": use ParseFS with an embed.FS"
	}
	return ""
}

func evaluateFuncMap(scope Scope, call *ast.CallExpr, fm map[string]any, funcTypesMap TemplateFunctions) error {
	workingDirectory, typesInfo, pkg, fileSet, files := scope.Dir, scope.Info, scope.Types, scope.Fset, scope.Files
	if len(call.Args) != 1 {
		return wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly 1 template.FuncMap composite literal argument"))
	}
	arg := call.Args[0]
	lit, ok := resolvePackageLevel(files, arg).(*ast.CompositeLit)
	if !ok {
		return wrapWithFilename(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("%s (got %s)%s", unreadableFuncsMessage, astgen.Format(arg), sharedFuncsHint(arg)))
	}
	if typesInfo != nil {
		litType := typesInfo.TypeOf(lit)
		if named, ok := litType.(*types.Named); ok {
			obj := named.Obj()
			if obj.Pkg() == nil || obj.Name() != "FuncMap" {
				return wrapWithFilename(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("expected template.FuncMap got %s", litType))
			}
			path := obj.Pkg().Path()
			if !isAcceptedTemplatePkg(path) {
				return wrapWithFilename(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("expected template.FuncMap got %s", litType))
			}
		} else if !isTemplateFuncMapLiteral(typesInfo, lit) {
			// The literal's type did not resolve — the package may not fully
			// type check — so validate the spelling instead of silently
			// skipping the function registrations.
			return wrapWithFilename(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("expected a template.FuncMap composite literal got %s", astgen.Format(lit)))
		}
	}
	var buf bytes.Buffer
	for i, exp := range lit.Elts {
		el, ok := exp.(*ast.KeyValueExpr)
		if !ok {
			return wrapWithFilename(workingDirectory, fileSet, exp.Pos(), fmt.Errorf("expected element at index %d to be a key value pair got %s", i, astgen.Format(exp)))
		}
		funcName, err := StringLiteralExpression(workingDirectory, fileSet, el.Key)
		if err != nil {
			return err
		}

		fm[funcName] = fmt.Sprintln

		if pkg == nil {
			continue
		}
		buf.Reset()
		if err := format.Node(&buf, fileSet, el.Value); err != nil {
			return err
		}
		tv, err := types.Eval(fileSet, pkg, lit.Pos(), buf.String())
		if err != nil {
			return err
		}
		sig, ok := tv.Type.(*types.Signature)
		if !ok {
			return wrapWithFilename(workingDirectory, fileSet, el.Value.Pos(), fmt.Errorf("FuncMap value %q is not a function type: %s", funcName, tv.Type))
		}
		funcTypesMap[funcName] = sig
	}
	return nil
}

// evaluateCallParseFilesArgs resolves the files a ParseFS call reads and the
// import path of the package that owns them, empty for the analyzed package.
// Matched paths are joined onto base, the directory the embedded paths are
// relative to, which is not always the working directory used to render
// errors: it is the package directory of whichever package declares the
// embed.FS variable.
func evaluateCallParseFilesArgs(scope Scope, call *ast.CallExpr) (paths []string, importPath string, err error) {
	workingDirectory, fileSet, files := scope.Dir, scope.Fset, scope.Files
	if len(call.Args) < 1 {
		return nil, "", wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("missing required arguments"))
	}
	base := scope.Dir
	var matches []string
	if shared, ok := call.Args[0].(*ast.SelectorExpr); ok {
		owned, owner, err := sharedFiles(scope, shared)
		if err != nil {
			return nil, "", err
		}
		importPath, base, matches = owner, scope.Imports[owner].Dir, owned
	} else {
		var decls []*ast.GenDecl
		for _, decl := range astgen.IterateGenDecl(files, token.VAR) {
			decls = append(decls, decl)
		}
		if matches, err = embedFSFilePaths(workingDirectory, fileSet, decls, call.Args[0], scope.Embedded); err != nil {
			return nil, "", err
		}
	}
	templateNames, err := StringLiteralExpressionList(workingDirectory, fileSet, files, call.Args[1:])
	if err != nil {
		return nil, "", err
	}
	filtered := matches[:0]
	for _, ef := range matches {
		for j, pattern := range templateNames {
			match, err := filepath.Match(pattern, ef)
			if err != nil {
				return nil, "", wrapWithFilename(workingDirectory, fileSet, call.Args[j+1].Pos(), fmt.Errorf("bad pattern %q: %w", pattern, err))
			}
			if !match {
				continue
			}
			filtered = append(filtered, ef)
			break
		}
	}
	return joinFilePaths(base, filtered...), importPath, nil
}

// embedFSFilePaths finds the //go:embed declaration of the variable exp names
// among decls and returns the embedded files it matches. It takes the
// declarations, not the files, so that it can read those of a package whose
// files are not the ones being analyzed.
func embedFSFilePaths(dir string, fileSet *token.FileSet, decls []*ast.GenDecl, exp ast.Expr, embeddedFiles []string) ([]string, error) {
	varIdent, ok := exp.(*ast.Ident)
	if !ok {
		return nil, wrapWithFilename(dir, fileSet, exp.Pos(), fmt.Errorf("%s (got %s)", firstArgumentToParseFSMessage, astgen.Format(exp)))
	}
	commentNode, templateNames, found := embedDirective(decls, varIdent.Name)
	if !found {
		return nil, wrapWithFilename(dir, fileSet, exp.Pos(), fmt.Errorf("variable %s not found", varIdent))
	}
	return embeddedFilesMatchingTemplateNameList(dir, fileSet, commentNode, templateNames, embeddedFiles)
}

// embedDirective finds the declaration of the variable name among decls and
// returns the comment holding its //go:embed lines and the patterns on them.
// found is false when no declaration names it; the patterns are empty when it
// has no directive.
func embedDirective(decls []*ast.GenDecl, name string) (comment ast.Node, patterns []string, found bool) {
	for _, decl := range decls {
		for _, s := range decl.Specs {
			spec, ok := s.(*ast.ValueSpec)
			if !ok || !slices.ContainsFunc(spec.Names, func(e *ast.Ident) bool { return e.Name == name }) {
				continue
			}
			var text strings.Builder
			comment = readComments(&text, decl.Doc, spec.Doc)
			return comment, ParseEmbedPatterns(text.String()), true
		}
	}
	return nil, nil, false
}

func embeddedFilesMatchingTemplateNameList(dir string, set *token.FileSet, comment ast.Node, templateNames, embeddedFiles []string) ([]string, error) {
	var matches []string
	for _, fp := range embeddedFiles {
		for _, pattern := range templateNames {
			matched, err := embedPatternMatches(pattern, filepath.ToSlash(fp))
			if err != nil {
				return nil, wrapWithFilename(dir, set, comment.Pos(), fmt.Errorf("embed comment malformed: %w", err))
			}
			if matched {
				matches = append(matches, fp)
				break
			}
		}
	}
	return slices.Clip(matches), nil
}

// embedPatternMatches reports whether a go:embed pattern matches the
// slash-separated path fp, with cmd/go semantics: a pattern that names a
// directory embeds its whole subtree, excluding path elements that begin with
// a dot or an underscore unless the pattern carries the all: prefix.
func embedPatternMatches(pattern, fp string) (bool, error) {
	pat, all := strings.CutPrefix(pattern, "all:")
	if matched, err := path.Match(pat, fp); err != nil || matched {
		return matched, err
	}
	// path.Match patterns match a fixed number of path elements, so at most
	// one ancestor directory of fp can match pat.
	for parent := path.Dir(fp); parent != "." && parent != "/"; parent = path.Dir(parent) {
		matched, err := path.Match(pat, parent)
		if err != nil {
			return false, err
		}
		if !matched {
			continue
		}
		if all {
			return true, nil
		}
		for elem := range strings.SplitSeq(strings.TrimPrefix(fp, parent+"/"), "/") {
			if strings.HasPrefix(elem, ".") || strings.HasPrefix(elem, "_") {
				return false, nil
			}
		}
		return true, nil
	}
	return false, nil
}

const goEmbedCommentPrefix = "//go:embed"

func readComments(s *strings.Builder, groups ...*ast.CommentGroup) ast.Node {
	var n ast.Node
	for _, c := range groups {
		if c == nil {
			continue
		}
		for _, line := range c.List {
			if !strings.HasPrefix(line.Text, goEmbedCommentPrefix) {
				continue
			}
			s.WriteString(strings.TrimSpace(strings.TrimPrefix(line.Text, goEmbedCommentPrefix)))
			s.WriteByte(' ')
		}
		n = c
		break
	}
	return n
}

// ParseEmbedPatterns splits the text of a //go:embed line, after the
// directive, into its patterns. A pattern in double quotes or backquotes may
// hold a space.
func ParseEmbedPatterns(input string) []string {
	var (
		templateNames       []string
		currentTemplateName strings.Builder
		inQuote             = false
		quoteChar           rune
	)

	for _, r := range input {
		switch {
		case r == '"' || r == '`':
			if !inQuote {
				inQuote = true
				quoteChar = r
				continue
			}
			if r != quoteChar {
				currentTemplateName.WriteRune(r)
				continue
			}
			templateNames = append(templateNames, currentTemplateName.String())
			currentTemplateName.Reset()
			inQuote = false
		case unicode.IsSpace(r):
			if inQuote {
				currentTemplateName.WriteRune(r)
				continue
			}
			if currentTemplateName.Len() > 0 {
				templateNames = append(templateNames, currentTemplateName.String())
				currentTemplateName.Reset()
			}
		default:
			currentTemplateName.WriteRune(r)
		}
	}

	if currentTemplateName.Len() > 0 {
		templateNames = append(templateNames, currentTemplateName.String())
	}

	return templateNames
}

func joinFilePaths(wd string, rel ...string) []string {
	result := slices.Clone(rel)
	for i := range result {
		result[i] = filepath.Join(wd, result[i])
	}
	return result
}

func RelativeFilePaths(wd string, abs ...string) ([]string, error) {
	result := slices.Clone(abs)
	for i, p := range result {
		r, err := filepath.Rel(wd, p)
		if err != nil {
			return nil, err
		}
		result[i] = r
	}
	return result, nil
}

type TemplateFunctions map[string]*types.Signature

func DefaultFunctions(pkg *types.Package) TemplateFunctions {
	funcTypeMap := make(TemplateFunctions)
	for _, entry := range []struct {
		pkgPath   string
		funcName  string
		aliasName string
	}{
		{"fmt", "Sprintf", "printf"},
		{"fmt", "Sprint", "print"},
		{"fmt", "Sprintln", "println"},
	} {
		p, ok := findPackage(pkg, entry.pkgPath)
		if !ok || p == nil {
			continue
		}
		obj := p.Scope().Lookup(entry.funcName)
		if obj == nil {
			continue
		}
		sig, ok := obj.Type().(*types.Signature)
		if !ok {
			continue
		}
		funcTypeMap[entry.aliasName] = sig
	}
	return funcTypeMap
}

func findPackage(pkg *types.Package, path string) (*types.Package, bool) {
	return findPackageVisited(pkg, path, nil)
}

func findPackageVisited(pkg *types.Package, path string, visited map[*types.Package]bool) (*types.Package, bool) {
	if pkg == nil {
		return nil, false
	}
	if pkg.Path() == path {
		return pkg, true
	}
	if visited == nil {
		visited = make(map[*types.Package]bool)
	}
	if visited[pkg] {
		return nil, false
	}
	visited[pkg] = true
	for _, im := range pkg.Imports() {
		if p, ok := findPackageVisited(im, path, visited); ok {
			return p, true
		}
	}
	return nil, false
}

func (functions TemplateFunctions) FindFunction(name string) (*types.Signature, bool) {
	m := (map[string]*types.Signature)(functions)
	fn, ok := m[name]
	if !ok {
		return nil, false
	}
	return fn, true
}

func BasicLiteralString(node ast.Node) (string, bool) {
	name, ok := node.(*ast.BasicLit)
	if !ok {
		return "", false
	}
	if name.Kind != token.STRING {
		return "", false
	}
	templateName, err := strconv.Unquote(name.Value)
	if err != nil {
		return "", false
	}
	return templateName, true
}
