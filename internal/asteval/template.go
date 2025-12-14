package asteval

import (
	"bytes"
	"cmp"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template/parse"
	"unicode"

	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/asterr"
	"github.com/typelate/muxt/internal/astgen"
)

const (
	templateExecuteFunc = "ExecuteTemplate"
)

func Templates(workingDirectory, templatesVariable string, pkg *packages.Package) (*template.Template, TemplateFunctions, error) {
	funcTypeMap := DefaultFunctions(pkg.Types)
	for file, tv := range astgen.IterateValueSpecs(pkg.Syntax) {
		i := slices.IndexFunc(tv.Names, func(e *ast.Ident) bool {
			return e.Name == templatesVariable
		})
		if i < 0 || i >= len(tv.Values) {
			continue
		}
		embeddedPaths, err := relativeFilePaths(workingDirectory, pkg.EmbedFiles...)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to calculate relative path for embedded files: %w", err)
		}
		templatePackageIdent := "template"
		for _, im := range file.Imports {
			path, _ := strconv.Unquote(im.Path.Value)
			switch path {
			case "html/template", "text/template":
				if im.Name != nil {
					templatePackageIdent = cmp.Or(im.Name.Name, templatePackageIdent)
				}
			}
		}
		ts, _, _, err := evaluateTemplateSelector(nil, pkg.Types, tv.Values[i], workingDirectory, templatesVariable, templatePackageIdent, "", "", pkg.Fset, pkg.Syntax, embeddedPaths, funcTypeMap, make(template.FuncMap))
		if err != nil {
			return nil, nil, fmt.Errorf("run template %s failed at %w", templatesVariable, err)
		}
		return ts, funcTypeMap, nil
	}
	return nil, nil, fmt.Errorf("variable %s not found", templatesVariable)
}

func findPackage(pkg *types.Package, path string) (*types.Package, bool) {
	if pkg == nil || pkg.Path() == path {
		return pkg, true
	}
	for _, im := range pkg.Imports() {
		if p, ok := findPackage(im, path); ok {
			return p, true
		}
	}
	return nil, false
}

func evaluateTemplateSelector(ts *template.Template, pkg *types.Package, expression ast.Expr, workingDirectory, templatesVariable, templatePackageIdent, rDelim, lDelim string, fileSet *token.FileSet, files []*ast.File, embeddedPaths []string, funcTypeMaps TemplateFunctions, fm template.FuncMap) (*template.Template, string, string, error) {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return nil, lDelim, rDelim, asterr.WrapWithFilename(workingDirectory, fileSet, expression.Pos(), fmt.Errorf("expected call expression"))
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, lDelim, rDelim, asterr.WrapWithFilename(workingDirectory, fileSet, call.Fun.Pos(), fmt.Errorf("unexpected expression %T: %s", call.Fun, astgen.Format(call.Fun)))
	}
	switch x := sel.X.(type) {
	default:
		return nil, lDelim, rDelim, asterr.WrapWithFilename(workingDirectory, fileSet, sel.X.Pos(), fmt.Errorf("expected exactly one argument %s got %d", astgen.Format(sel.X), len(call.Args)))
	case *ast.Ident:
		if x.Name != templatePackageIdent {
			return nil, lDelim, rDelim, asterr.WrapWithFilename(workingDirectory, fileSet, sel.X.Pos(), fmt.Errorf("expected %s got %s", templatePackageIdent, astgen.Format(sel.X)))
		}
		switch sel.Sel.Name {
		case "Must":
			if len(call.Args) != 1 {
				return nil, lDelim, rDelim, asterr.WrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one argument %s got %d", astgen.Format(sel.X), len(call.Args)))
			}
			return evaluateTemplateSelector(ts, pkg, call.Args[0], workingDirectory, templatesVariable, templatePackageIdent, rDelim, lDelim, fileSet, files, embeddedPaths, funcTypeMaps, fm)
		case "New":
			if len(call.Args) != 1 {
				return nil, lDelim, rDelim, asterr.WrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one string literal argument"))
			}
			templateNames, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, lDelim, rDelim, err
			}
			return template.New(templateNames[0]), lDelim, rDelim, nil
		case "ParseFS":
			filePaths, err := evaluateCallParseFilesArgs(workingDirectory, fileSet, call, files, embeddedPaths)
			if err != nil {
				return nil, lDelim, rDelim, err
			}
			t, err := parseFiles(nil, fm, lDelim, rDelim, filePaths...)
			return t, lDelim, rDelim, err
		default:
			return nil, lDelim, rDelim, asterr.WrapWithFilename(workingDirectory, fileSet, call.Fun.Pos(), fmt.Errorf("unsupported function %s", sel.Sel.Name))
		}
	case *ast.CallExpr:
		up, upLDelim, upRDelim, err := evaluateTemplateSelector(ts, pkg, sel.X, workingDirectory, templatesVariable, templatePackageIdent, rDelim, lDelim, fileSet, files, embeddedPaths, funcTypeMaps, fm)
		if err != nil {
			return nil, lDelim, rDelim, err
		}
		switch sel.Sel.Name {
		case "Delims":
			if len(call.Args) != 2 {
				return nil, upLDelim, upRDelim, asterr.WrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly two string literal arguments"))
			}
			list, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			return up.Delims(list[0], list[1]), list[0], list[1], nil
		case "Parse":
			if len(call.Args) != 1 {
				return nil, upLDelim, upRDelim, asterr.WrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one string literal argument"))
			}
			sl, err := StringLiteralExpression(workingDirectory, fileSet, call.Args[0])
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			t, err := up.Parse(sl)
			return t, upLDelim, upRDelim, err
		case "New":
			if len(call.Args) != 1 {
				return nil, upLDelim, upRDelim, asterr.WrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one string literal argument"))
			}
			templateNames, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			return up.New(templateNames[0]), upLDelim, upRDelim, nil
		case "ParseFS":
			filePaths, err := evaluateCallParseFilesArgs(workingDirectory, fileSet, call, files, embeddedPaths)
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			t, err := parseFiles(up, fm, upLDelim, upRDelim, filePaths...)
			return t, upLDelim, upRDelim, err
		case "Option":
			list, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			return up.Option(list...), upLDelim, upRDelim, nil
		case "Funcs":
			if err := evaluateFuncMap(workingDirectory, templatePackageIdent, pkg, fileSet, call, fm, funcTypeMaps); err != nil {
				return nil, upLDelim, upRDelim, err
			}
			return up.Funcs(fm), upLDelim, upRDelim, nil
		default:
			return nil, upLDelim, upRDelim, asterr.WrapWithFilename(workingDirectory, fileSet, call.Fun.Pos(), fmt.Errorf("unsupported method %s", sel.Sel.Name))
		}
	}
}

func builtins() template.FuncMap {
	type nothing struct{}
	return template.FuncMap{
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

func parseFiles(t *template.Template, fm template.FuncMap, leftDelim, rightDelim string, filenames ...string) (*template.Template, error) {
	if len(filenames) == 0 {
		return nil, fmt.Errorf("html/template: no files named in call to ParseFiles")
	}
	for _, filename := range filenames {
		templateName := filepath.Base(filename)
		b, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}
		s := string(b)
		var tmpl *template.Template
		if t == nil {
			t = template.New(templateName)
		}
		if templateName == t.Name() {
			tmpl = t
		} else {
			tmpl = t.New(templateName)
		}
		trees, err := parse.Parse(templateName, s, leftDelim, rightDelim, fm, builtins())
		if err != nil {
			return nil, err
		}
		absoluteFilename, err := filepath.Abs(filename)
		if err != nil {
			return nil, err
		}
		for _, tree := range trees {
			tree.ParseName = absoluteFilename
			if _, err = tmpl.AddParseTree(tree.Name, tree); err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

func evaluateFuncMap(workingDirectory, templatePackageIdent string, pkg *types.Package, fileSet *token.FileSet, call *ast.CallExpr, fm template.FuncMap, funcTypesMap TemplateFunctions) error {
	const funcMapTypeIdent = "FuncMap"
	if len(call.Args) != 1 {
		return asterr.WrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly 1 template.FuncMap composite literal argument"))
	}
	arg := call.Args[0]
	lit, ok := arg.(*ast.CompositeLit)
	if !ok {
		return asterr.WrapWithFilename(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("expected a composite literal with type %s.%s got %s", templatePackageIdent, funcMapTypeIdent, astgen.Format(arg)))
	}
	typeSel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || typeSel.Sel.Name != funcMapTypeIdent {
		return asterr.WrapWithFilename(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("expected a composite literal with type %s.%s got %s", templatePackageIdent, funcMapTypeIdent, astgen.Format(arg)))
	}
	if tp, ok := typeSel.X.(*ast.Ident); !ok || tp.Name != templatePackageIdent {
		return asterr.WrapWithFilename(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("expected a composite literal with type %s.%s got %s", templatePackageIdent, funcMapTypeIdent, astgen.Format(arg)))
	}
	var buf bytes.Buffer
	for i, exp := range lit.Elts {
		el, ok := exp.(*ast.KeyValueExpr)
		if !ok {
			return asterr.WrapWithFilename(workingDirectory, fileSet, exp.Pos(), fmt.Errorf("expected element at index %d to be a key value pair got %s", i, astgen.Format(exp)))
		}
		funcName, err := StringLiteralExpression(workingDirectory, fileSet, el.Key)
		if err != nil {
			return err
		}

		// template.Parse does not evaluate the function signature parameters;
		// it ensures the function name is in scope and there is one or two results.
		// we could use something like func() string { return "" } for this signature
		// but this function from fmt works just fine.
		//
		// to explore the known requirements run:
		//   fm[funcName] = nil // will fail because nil does not have `reflect.Kind` Func
		// or
		//   fm[funcName] = func() {} // will fail because there are no results
		// or
		//   fm[funcName] = func() (int, int) {return 0, 0} // will fail because the second result is not an error
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
		funcTypesMap[funcName] = tv.Type.(*types.Signature)
	}
	return nil
}

func evaluateCallParseFilesArgs(workingDirectory string, fileSet *token.FileSet, call *ast.CallExpr, files []*ast.File, embeddedPaths []string) ([]string, error) {
	if len(call.Args) < 1 {
		return nil, asterr.WrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("missing required arguments"))
	}
	matches, err := embedFSFilePaths(workingDirectory, fileSet, files, call.Args[0], embeddedPaths)
	if err != nil {
		return nil, err
	}
	templateNames, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args[1:])
	if err != nil {
		return nil, err
	}
	filtered := matches[:0]
	for _, ef := range matches {
		for j, pattern := range templateNames {
			match, err := filepath.Match(pattern, ef)
			if err != nil {
				return nil, asterr.WrapWithFilename(workingDirectory, fileSet, call.Args[j+1].Pos(), fmt.Errorf("bad pattern %q: %w", pattern, err))
			}
			if !match {
				continue
			}
			filtered = append(filtered, ef)
			break
		}
	}
	return joinFilePaths(workingDirectory, filtered...), nil
}

func embedFSFilePaths(dir string, fileSet *token.FileSet, files []*ast.File, exp ast.Expr, embeddedFiles []string) ([]string, error) {
	varIdent, ok := exp.(*ast.Ident)
	if !ok {
		return nil, asterr.WrapWithFilename(dir, fileSet, exp.Pos(), fmt.Errorf("first argument to ParseFS must be an identifier"))
	}
	for _, decl := range astgen.IterateGenDecl(files, token.VAR) {
		for _, s := range decl.Specs {
			spec, ok := s.(*ast.ValueSpec)
			if !ok || !slices.ContainsFunc(spec.Names, func(e *ast.Ident) bool { return e.Name == varIdent.Name }) {
				continue
			}
			var comment strings.Builder
			commentNode := readComments(&comment, decl.Doc, spec.Doc)
			templateNames := parseTemplateNames(comment.String())
			absMat, err := embeddedFilesMatchingTemplateNameList(dir, fileSet, commentNode, templateNames, embeddedFiles)
			if err != nil {
				return nil, err
			}
			return absMat, nil
		}
	}
	return nil, asterr.WrapWithFilename(dir, fileSet, exp.Pos(), fmt.Errorf("variable %s not found", varIdent))
}

func embeddedFilesMatchingTemplateNameList(dir string, set *token.FileSet, comment ast.Node, templateNames, embeddedFiles []string) ([]string, error) {
	var matches []string
	for _, fp := range embeddedFiles {
		for _, pattern := range templateNames {
			pat := filepath.FromSlash(pattern)
			if !strings.ContainsAny(pat, "*[]") {
				prefix := filepath.FromSlash(pat + "/")
				if strings.HasPrefix(fp, prefix) {
					matches = append(matches, fp)
					continue
				}
			}
			if matched, err := filepath.Match(pat, fp); err != nil {
				return nil, asterr.WrapWithFilename(dir, set, comment.Pos(), fmt.Errorf("embed comment malformed: %w", err))
			} else if matched {
				matches = append(matches, fp)
			}
		}
	}
	return slices.Clip(matches), nil
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

func parseTemplateNames(input string) []string {
	// todo: refactor to use strconv.QuotedPrefix
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

	// Import any remaining pattern
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

func relativeFilePaths(wd string, abs ...string) ([]string, error) {
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
	fmtPkg, ok := findPackage(pkg, "fmt")
	if !ok || fmtPkg == nil {
		return funcTypeMap
	}
	funcTypeMap["printf"] = fmtPkg.Scope().Lookup("Sprintf").Type().(*types.Signature)
	funcTypeMap["print"] = fmtPkg.Scope().Lookup("Sprint").Type().(*types.Signature)
	funcTypeMap["println"] = fmtPkg.Scope().Lookup("Sprintln").Type().(*types.Signature)
	return funcTypeMap
}

func (functions TemplateFunctions) FindFunction(name string) (*types.Signature, bool) {
	m := (map[string]*types.Signature)(functions)
	fn, ok := m[name]
	if !ok {
		return nil, false
	}
	return fn, true
}

func ExecuteTemplateArguments(node ast.Node, info *types.Info, templatesVariableName string) (string, types.Type, bool) {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return "", nil, false
	}
	if len(call.Args) != 3 {
		return "", nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", nil, false
	}
	if sel.Sel.Name != templateExecuteFunc {
		return "", nil, false
	}
	templatesIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	if templatesIdent.Name != templatesVariableName {
		return "", nil, false
	}
	templateName, ok := basicLiteralString(call.Args[1])
	if !ok {
		return "", nil, false
	}
	dataVar := info.TypeOf(call.Args[2])
	return templateName, dataVar, true
}

func basicLiteralString(node ast.Node) (string, bool) {
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
