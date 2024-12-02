package source

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"html/template"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

type TemplateFuncMap = map[string]*types.Signature

func Templates(workingDirectory, templatesVariable string, pkg *packages.Package) (*template.Template, TemplateFuncMap, error) {
	funcTypeMap := registerDefaultFunctions(pkg.Types)
	for _, tv := range IterateValueSpecs(pkg.Syntax) {
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
		const templatePackageIdent = "template"
		ts, err := evaluateTemplateSelector(nil, pkg.Types, tv.Values[i], workingDirectory, templatesVariable, templatePackageIdent, "", "", pkg.Fset, pkg.Syntax, embeddedPaths, funcTypeMap, make(template.FuncMap))
		if err != nil {
			return nil, nil, fmt.Errorf("run template %s failed at %w", templatesVariable, err)
		}
		return ts, funcTypeMap, nil
	}
	return nil, nil, fmt.Errorf("variable %s not found", templatesVariable)
}

func findPackage(pkg *types.Package, path string) (*types.Package, bool) {
	if pkg.Path() == path {
		return pkg, true
	}
	for _, im := range pkg.Imports() {
		if p, ok := findPackage(im, path); ok {
			return p, true
		}
	}
	return nil, false
}

func registerDefaultFunctions(pkg *types.Package) TemplateFuncMap {
	funcTypeMap := make(TemplateFuncMap)
	fmtPkg, ok := findPackage(pkg, "fmt")
	if !ok || fmtPkg == nil {
		return funcTypeMap
	}
	funcTypeMap["printf"] = fmtPkg.Scope().Lookup("Sprintf").Type().(*types.Signature)
	funcTypeMap["print"] = fmtPkg.Scope().Lookup("Sprint").Type().(*types.Signature)
	funcTypeMap["println"] = fmtPkg.Scope().Lookup("Sprintln").Type().(*types.Signature)
	return funcTypeMap
}

func evaluateTemplateSelector(ts *template.Template, pkg *types.Package, expression ast.Expr, workingDirectory, templatesVariable, templatePackageIdent, rDelim, lDelim string, fileSet *token.FileSet, files []*ast.File, embeddedPaths []string, funcTypeMaps TemplateFuncMap, fm template.FuncMap) (*template.Template, error) {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return nil, contextError(workingDirectory, fileSet, expression.Pos(), fmt.Errorf("expected call expression"))
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, contextError(workingDirectory, fileSet, call.Fun.Pos(), fmt.Errorf("unexpected expression %T: %s", call.Fun, Format(call.Fun)))
	}
	switch x := sel.X.(type) {
	default:
		return nil, contextError(workingDirectory, fileSet, sel.X.Pos(), fmt.Errorf("expected exactly one argument %s got %d", Format(sel.X), len(call.Args)))
	case *ast.Ident:
		if x.Name != templatePackageIdent {
			return nil, contextError(workingDirectory, fileSet, sel.X.Pos(), fmt.Errorf("expected %s got %s", templatePackageIdent, Format(sel.X)))
		}
		switch sel.Sel.Name {
		case "Must":
			if len(call.Args) != 1 {
				return nil, contextError(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one argument %s got %d", Format(sel.X), len(call.Args)))
			}
			return evaluateTemplateSelector(ts, pkg, call.Args[0], workingDirectory, templatesVariable, templatePackageIdent, rDelim, lDelim, fileSet, files, embeddedPaths, funcTypeMaps, fm)
		case "New":
			if len(call.Args) != 1 {
				return nil, contextError(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one string literal argument"))
			}
			templateNames, err := evaluateStringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, err
			}
			return template.New(templateNames[0]), nil
		case "ParseFS":
			filePaths, err := evaluateCallParseFilesArgs(workingDirectory, fileSet, call, files, embeddedPaths)
			if err != nil {
				return nil, err
			}
			return template.ParseFiles(filePaths...)
		default:
			return nil, contextError(workingDirectory, fileSet, call.Fun.Pos(), fmt.Errorf("unsupported function %s", sel.Sel.Name))
		}
	case *ast.CallExpr:
		up, err := evaluateTemplateSelector(ts, pkg, sel.X, workingDirectory, templatesVariable, templatePackageIdent, rDelim, lDelim, fileSet, files, embeddedPaths, funcTypeMaps, fm)
		if err != nil {
			return nil, err
		}
		switch sel.Sel.Name {
		case "Delims":
			if len(call.Args) != 2 {
				return nil, contextError(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly two string literal arguments"))
			}
			list, err := evaluateStringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, err
			}
			return up.Delims(list[0], list[1]), nil
		case "Parse":
			if len(call.Args) != 1 {
				return nil, contextError(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one string literal argument"))
			}
			sl, err := evaluateStringLiteralExpression(workingDirectory, fileSet, call.Args[0])
			if err != nil {
				return nil, err
			}
			return up.Parse(sl)
		case "New":
			if len(call.Args) != 1 {
				return nil, contextError(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one string literal argument"))
			}
			templateNames, err := evaluateStringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, err
			}
			return up.New(templateNames[0]), nil
		case "ParseFS":
			filePaths, err := evaluateCallParseFilesArgs(workingDirectory, fileSet, call, files, embeddedPaths)
			if err != nil {
				return nil, err
			}
			return up.ParseFiles(filePaths...)
		case "Option":
			list, err := evaluateStringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, err
			}
			return up.Option(list...), nil
		case "Funcs":
			if err := evaluateFuncMap(workingDirectory, templatePackageIdent, pkg, fileSet, call, fm, funcTypeMaps); err != nil {
				return nil, err
			}
			return up.Funcs(fm), nil
		default:
			return nil, contextError(workingDirectory, fileSet, call.Fun.Pos(), fmt.Errorf("unsupported method %s", sel.Sel.Name))
		}
	}
}

func evaluateFuncMap(workingDirectory, templatePackageIdent string, pkg *types.Package, fileSet *token.FileSet, call *ast.CallExpr, fm template.FuncMap, funcTypesMap TemplateFuncMap) error {
	const funcMapTypeIdent = "FuncMap"
	if len(call.Args) != 1 {
		return contextError(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly 1 template.FuncMap composite literal argument"))
	}
	arg := call.Args[0]
	lit, ok := arg.(*ast.CompositeLit)
	if !ok {
		return contextError(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("expected a composite literal with type %s.%s got %s", templatePackageIdent, funcMapTypeIdent, Format(arg)))
	}
	typeSel, ok := lit.Type.(*ast.SelectorExpr)
	if !ok || typeSel.Sel.Name != funcMapTypeIdent {
		return contextError(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("expected a composite literal with type %s.%s got %s", templatePackageIdent, funcMapTypeIdent, Format(arg)))
	}
	if tp, ok := typeSel.X.(*ast.Ident); !ok || tp.Name != templatePackageIdent {
		return contextError(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("expected a composite literal with type %s.%s got %s", templatePackageIdent, funcMapTypeIdent, Format(arg)))
	}
	var buf bytes.Buffer
	for i, exp := range lit.Elts {
		el, ok := exp.(*ast.KeyValueExpr)
		if !ok {
			return contextError(workingDirectory, fileSet, exp.Pos(), fmt.Errorf("expected element at index %d to be a key value pair got %s", i, Format(exp)))
		}
		funcName, err := evaluateStringLiteralExpression(workingDirectory, fileSet, el.Key)
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
		return nil, contextError(workingDirectory, fileSet, call.Lparen, fmt.Errorf("missing required arguments"))
	}
	matches, err := embedFSFilePaths(workingDirectory, fileSet, files, call.Args[0], embeddedPaths)
	if err != nil {
		return nil, err
	}
	templateNames, err := evaluateStringLiteralExpressionList(workingDirectory, fileSet, call.Args[1:])
	if err != nil {
		return nil, err
	}
	filtered := matches[:0]
	for _, ef := range matches {
		for j, pattern := range templateNames {
			match, err := filepath.Match(pattern, ef)
			if err != nil {
				return nil, contextError(workingDirectory, fileSet, call.Args[j+1].Pos(), fmt.Errorf("bad pattern %q: %w", pattern, err))
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
		return nil, contextError(dir, fileSet, exp.Pos(), fmt.Errorf("first argument to ParseFS must be an identifier"))
	}
	for _, decl := range IterateGenDecl(files, token.VAR) {
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
	return nil, contextError(dir, fileSet, exp.Pos(), fmt.Errorf("variable %s not found", varIdent))
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
				return nil, contextError(dir, set, comment.Pos(), fmt.Errorf("embed comment malformed: %w", err))
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

	// Add any remaining pattern
	if currentTemplateName.Len() > 0 {
		templateNames = append(templateNames, currentTemplateName.String())
	}

	return templateNames
}

func contextError(workingDirectory string, set *token.FileSet, pos token.Pos, err error) error {
	p := set.Position(pos)
	p.Filename, _ = filepath.Rel(workingDirectory, p.Filename)
	return fmt.Errorf("%s: %w", p, err)
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
