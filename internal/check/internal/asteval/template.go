package asteval

import (
	"bytes"
	"cmp"
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
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// TemplateMetadata accumulates metadata during template evaluation.
type TemplateMetadata struct {
	EmbedFilePaths []string
	ParseCalls     []*ast.BasicLit
}

func EvaluateTemplateSelector(ts *template.Template[*types.Signature], pkg *types.Package, typesInfo *types.Info, expression ast.Expr, workingDirectory, templatesVariable, lDelim, rDelim string, fileSet *token.FileSet, files []*ast.File, embeddedPaths []string, funcTypeMaps TemplateFunctions, fm map[string]any, meta *TemplateMetadata) (*template.Template[*types.Signature], string, string, error) {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return nil, lDelim, rDelim, wrapWithFilename(workingDirectory, fileSet, expression.Pos(), fmt.Errorf("expected call expression"))
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, lDelim, rDelim, wrapWithFilename(workingDirectory, fileSet, call.Fun.Pos(), fmt.Errorf("unexpected expression %T: %s", call.Fun, astgen.Format(call.Fun)))
	}
	switch x := sel.X.(type) {
	default:
		return nil, lDelim, rDelim, wrapWithFilename(workingDirectory, fileSet, sel.X.Pos(), fmt.Errorf("expected exactly one argument %s got %d", astgen.Format(sel.X), len(call.Args)))
	case *ast.Ident:
		if !IsTemplatePkgIdent(typesInfo, x) {
			if ts == nil {
				return nil, lDelim, rDelim, wrapWithFilename(workingDirectory, fileSet, sel.X.Pos(), fmt.Errorf("expected template package got %s", astgen.Format(sel.X)))
			}
			// Variable receiver — apply method to existing template.
			switch sel.Sel.Name {
			case "ParseFS":
				filePaths, err := evaluateCallParseFilesArgs(workingDirectory, fileSet, call, files, embeddedPaths)
				if err != nil {
					return nil, lDelim, rDelim, err
				}
				if meta != nil {
					meta.EmbedFilePaths = append(meta.EmbedFilePaths, filePaths...)
				}
				t, err := parseFiles(ts, fm, lDelim, rDelim, filePaths...)
				return t, lDelim, rDelim, err
			case "Parse":
				if len(call.Args) != 1 {
					return nil, lDelim, rDelim, wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one string literal argument"))
				}
				if meta != nil {
					if bl, ok := call.Args[0].(*ast.BasicLit); ok {
						meta.ParseCalls = append(meta.ParseCalls, bl)
					}
				}
				sl, err := StringLiteralExpression(workingDirectory, fileSet, call.Args[0])
				if err != nil {
					return nil, lDelim, rDelim, err
				}
				t, err := ts.Parse(sl)
				return t, lDelim, rDelim, err
			case "Funcs", "Functions":
				if err := evaluateFuncMap(workingDirectory, typesInfo, pkg, fileSet, call, fm, funcTypeMaps); err != nil {
					return nil, lDelim, rDelim, err
				}
				sigs := make(map[string]*types.Signature)
				for name, sig := range funcTypeMaps {
					sigs[name] = sig
				}
				return ts.Functions(sigs), lDelim, rDelim, nil
			case "Option":
				list, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args)
				if err != nil {
					return nil, lDelim, rDelim, err
				}
				t, err := ts.Option(list...)
				if err != nil {
					return nil, lDelim, rDelim, err
				}
				return t, lDelim, rDelim, nil
			case "Delimiters", "Delims":
				if len(call.Args) != 2 {
					return nil, lDelim, rDelim, wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly two string literal arguments"))
				}
				list, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args)
				if err != nil {
					return nil, lDelim, rDelim, err
				}
				return ts.Delimiters(list[0], list[1]), list[0], list[1], nil
			default:
				return nil, lDelim, rDelim, wrapWithFilename(workingDirectory, fileSet, call.Fun.Pos(), fmt.Errorf("unsupported method %s on variable receiver", sel.Sel.Name))
			}
		}
		switch sel.Sel.Name {
		case "Must":
			if len(call.Args) != 1 {
				return nil, lDelim, rDelim, wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one argument %s got %d", astgen.Format(sel.X), len(call.Args)))
			}
			return EvaluateTemplateSelector(ts, pkg, typesInfo, call.Args[0], workingDirectory, templatesVariable, lDelim, rDelim, fileSet, files, embeddedPaths, funcTypeMaps, fm, meta)
		case "New":
			if len(call.Args) != 1 {
				return nil, lDelim, rDelim, wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one string literal argument"))
			}
			templateNames, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, lDelim, rDelim, err
			}
			return template.New[*types.Signature](templateNames[0]), lDelim, rDelim, nil
		case "ParseFS":
			filePaths, err := evaluateCallParseFilesArgs(workingDirectory, fileSet, call, files, embeddedPaths)
			if err != nil {
				return nil, lDelim, rDelim, err
			}
			if meta != nil {
				meta.EmbedFilePaths = append(meta.EmbedFilePaths, filePaths...)
			}
			t, err := parseFiles(nil, fm, lDelim, rDelim, filePaths...)
			return t, lDelim, rDelim, err
		default:
			return nil, lDelim, rDelim, wrapWithFilename(workingDirectory, fileSet, call.Fun.Pos(), fmt.Errorf("unsupported function %s", sel.Sel.Name))
		}
	case *ast.CallExpr:
		up, upLDelim, upRDelim, err := EvaluateTemplateSelector(ts, pkg, typesInfo, sel.X, workingDirectory, templatesVariable, lDelim, rDelim, fileSet, files, embeddedPaths, funcTypeMaps, fm, meta)
		if err != nil {
			return nil, lDelim, rDelim, err
		}
		switch sel.Sel.Name {
		case "Delimiters", "Delims":
			if len(call.Args) != 2 {
				return nil, upLDelim, upRDelim, wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly two string literal arguments"))
			}
			list, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			return up.Delimiters(list[0], list[1]), list[0], list[1], nil
		case "Parse":
			if len(call.Args) != 1 {
				return nil, upLDelim, upRDelim, wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one string literal argument"))
			}
			if meta != nil {
				if bl, ok := call.Args[0].(*ast.BasicLit); ok {
					meta.ParseCalls = append(meta.ParseCalls, bl)
				}
			}
			sl, err := StringLiteralExpression(workingDirectory, fileSet, call.Args[0])
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			t, err := up.Parse(sl)
			return t, upLDelim, upRDelim, err
		case "New":
			if len(call.Args) != 1 {
				return nil, upLDelim, upRDelim, wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly one string literal argument"))
			}
			templateNames, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			// Create a new associated template
			// htt.Template doesn't have a New method on instances, so we use AddParseTree
			// to add a named tree to the existing template set.
			newTree, _ := template.ParseTree(templateNames[0], templateNames[0], "", nil, lex.Options{
				EmitComment: true,
				ParseHTML:   true,
				LeftDelim:   cmp.Or(upLDelim, "{{"),
				RightDelim:  cmp.Or(upRDelim, "}}"),
			})
			t, err := up.AddParseTree(templateNames[0], newTree)
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			return t, upLDelim, upRDelim, nil
		case "ParseFS":
			filePaths, err := evaluateCallParseFilesArgs(workingDirectory, fileSet, call, files, embeddedPaths)
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			if meta != nil {
				meta.EmbedFilePaths = append(meta.EmbedFilePaths, filePaths...)
			}
			t, err := parseFiles(up, fm, upLDelim, upRDelim, filePaths...)
			return t, upLDelim, upRDelim, err
		case "Option":
			list, err := StringLiteralExpressionList(workingDirectory, fileSet, call.Args)
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			t, err := up.Option(list...)
			if err != nil {
				return nil, upLDelim, upRDelim, err
			}
			return t, upLDelim, upRDelim, nil
		case "Funcs", "Functions":
			if err := evaluateFuncMap(workingDirectory, typesInfo, pkg, fileSet, call, fm, funcTypeMaps); err != nil {
				return nil, upLDelim, upRDelim, err
			}
			sigs := make(map[string]*types.Signature)
			for name, sig := range funcTypeMaps {
				sigs[name] = sig
			}
			return up.Functions(sigs), upLDelim, upRDelim, nil
		default:
			return nil, upLDelim, upRDelim, wrapWithFilename(workingDirectory, fileSet, call.Fun.Pos(), fmt.Errorf("unsupported method %s", sel.Sel.Name))
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

func parseFiles(t *template.Template[*types.Signature], fm map[string]any, leftDelim, rightDelim string, filenames ...string) (*template.Template[*types.Signature], error) {
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

		options := lex.Options{
			EmitComment: true,
			ParseHTML:   true,
			LeftDelim:   cmp.Or(leftDelim, "{{"),
			RightDelim:  cmp.Or(rightDelim, "}}"),
		}
		// Build a function name set for the parser.
		funcSet := make(map[string]bool)
		for name := range fm {
			funcSet[name] = true
		}
		for name := range builtins() {
			funcSet[name] = true
		}

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
			return nil, err
		}
		if _, err = t.AddParseTree(tree.Name, tree); err != nil {
			return nil, err
		}
		// Extract {{define}} blocks from the tree.
		for name, dt := range template.Extract(tree) {
			if _, err = t.AddParseTree(name, dt); err != nil {
				return nil, err
			}
		}
		// Register {{block}}-defined sub-templates collected above.
		for name, dt := range treeSet {
			if name == tree.Name {
				continue
			}
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

func evaluateFuncMap(workingDirectory string, typesInfo *types.Info, pkg *types.Package, fileSet *token.FileSet, call *ast.CallExpr, fm map[string]any, funcTypesMap TemplateFunctions) error {
	if len(call.Args) != 1 {
		return wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("expected exactly 1 template.FuncMap composite literal argument"))
	}
	arg := call.Args[0]
	lit, ok := arg.(*ast.CompositeLit)
	if !ok {
		return wrapWithFilename(workingDirectory, fileSet, arg.Pos(), fmt.Errorf("expected a template.FuncMap composite literal got %s", astgen.Format(arg)))
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

func evaluateCallParseFilesArgs(workingDirectory string, fileSet *token.FileSet, call *ast.CallExpr, files []*ast.File, embeddedPaths []string) ([]string, error) {
	if len(call.Args) < 1 {
		return nil, wrapWithFilename(workingDirectory, fileSet, call.Lparen, fmt.Errorf("missing required arguments"))
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
				return nil, wrapWithFilename(workingDirectory, fileSet, call.Args[j+1].Pos(), fmt.Errorf("bad pattern %q: %w", pattern, err))
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
		return nil, wrapWithFilename(dir, fileSet, exp.Pos(), fmt.Errorf("first argument to ParseFS must be an identifier"))
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
	return nil, wrapWithFilename(dir, fileSet, exp.Pos(), fmt.Errorf("variable %s not found", varIdent))
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

func parseTemplateNames(input string) []string {
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
