package generate

import (
	"bytes"
	"cmp"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/crhntr/muxt"
)

const (
	responseIdentName     = "response"
	requestIdentName      = "request"
	receiverIdentName     = "receiver"
	contextIdentName      = "ctx"
	dataIdentName         = "data"
	serveMuxIdentName     = "mux"
	errorIdentName        = "err"
	errorHandlerIdentName = "handleError"
	executeIdentName      = "execute"

	receiverTypeIdentNameDefault = "Receiver"
	handlerFuncIdentNameDefault  = "TemplateRoutes"
)

func Command(args []string, wd string, logger *log.Logger, lookupEnv func(string) (string, bool)) error {
	var (
		receiverTypeIdentName string
		handlerFuncIdentName  string
	)
	flagSet := flag.NewFlagSet("generate", flag.ContinueOnError)
	flagSet.StringVar(&receiverTypeIdentName, "receiver", receiverTypeIdentNameDefault, "the name of an interface type used for template data function calls")
	flagSet.StringVar(&handlerFuncIdentName, "handler", handlerFuncIdentNameDefault, "the name of the generated function registering routes on an *http.ServeMux")
	if err := flagSet.Parse(args); err != nil {
		return err
	}
	const (
		goPackageEnvVar = "GOPACKAGE"
		goFileEnvVar    = "GOFILE"
		goLineEnvVar    = "GOLINE"
	)
	goPackage, ok := lookupEnv(goPackageEnvVar)
	if !ok {
		return fmt.Errorf("%s is not set", goPackageEnvVar)
	}
	goFile, ok := lookupEnv(goFileEnvVar)
	if !ok {
		return fmt.Errorf("%s is not set", goFileEnvVar)
	}
	goLine, ok := lookupEnv(goLineEnvVar)
	if !ok {
		return fmt.Errorf("%s is not set", goLineEnvVar)
	}
	_ = os.Remove(filepath.Join(wd, "template_routes.go"))

	pkg, err := loadPackage(wd, goPackage)
	if err != nil {
		return err
	}
	file, err := findFile(pkg, goFile)
	if err != nil {
		return err
	}
	spec, err := valueSpec(pkg.Fset, file, goLine)
	if err != nil || spec == nil {
		return err
	}

	stdLibImports := []string{
		"net/http",
	}

	handler := &ast.FuncDecl{
		Name: ast.NewIdent(handlerFuncIdentName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					{Names: []*ast.Ident{ast.NewIdent(serveMuxIdentName)}, Type: &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("http"), Sel: ast.NewIdent("ServeMux")}}},
					{Names: []*ast.Ident{ast.NewIdent(receiverIdentName)}, Type: ast.NewIdent(receiverTypeIdentName)},
				},
			},
		},
		Body: &ast.BlockStmt{},
	}

	receiverType := &ast.InterfaceType{
		Methods: &ast.FieldList{},
	}

	patSet := make(map[string]struct{})
	for i := 0; i < len(spec.Names) && i < len(spec.Values) && len(spec.Names) == len(spec.Values); i++ {
		n, v := spec.Names[i], spec.Values[i]

		ts, err := parseTemplates(wd, pkg, n, v)
		if err != nil {
			return err
		}

		var templateNames []muxt.TemplateName
		for _, t := range ts.Templates() {
			name, err, ok := muxt.NewTemplateName(t.Name())
			if !ok {
				continue
			}
			if err != nil {
				return err
			}
			if _, ok := patSet[name.Pattern]; ok {
				return fmt.Errorf("duplicate route pattern: %s", name.Pattern)
			}
			templateNames = append(templateNames, name)
		}
		slices.SortFunc(templateNames, func(a, b muxt.TemplateName) int {
			return cmp.Compare(strings.Join([]string{a.Path, a.Method, a.Handler}, " "), strings.Join([]string{b.Path, b.Method, b.Handler}, " "))
		})
		for _, pat := range templateNames {
			handleFunc, methodField, handlerImports, err := templateHandlers(ts.Lookup(pat.String()), pat, pkg, n)
			if err != nil {
				return err
			}
			if methodField != nil {
				receiverType.Methods.List = append(receiverType.Methods.List, methodField)
			}
			handler.Body.List = append(handler.Body.List, &ast.ExprStmt{X: handleFunc})
			logger.Println(handlerFuncIdentName, "has route for", pat.String())
			stdLibImports = append(stdLibImports, handlerImports...)
		}
	}

	executeFound := false
	handleErrorFound := false
	for _, f := range pkg.Syntax {
		executeFound = executeFound || f.Scope.Lookup(executeIdentName) != nil
		handleErrorFound = handleErrorFound || f.Scope.Lookup("handleError") != nil
	}
	if !executeFound {
		stdLibImports = append(stdLibImports, "html/template", "bytes")
	}
	if !handleErrorFound {
		stdLibImports = append(stdLibImports, "html/template")
	}

	imports := &ast.GenDecl{
		Tok: token.IMPORT,
	}
	slices.Sort(stdLibImports)
	stdLibImports = slices.Compact(stdLibImports)
	for _, im := range stdLibImports {
		imports.Specs = append(imports.Specs, &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(im)}})
	}

	fileAST := &ast.File{
		Name: ast.NewIdent(file.Name.Name),
		Decls: []ast.Decl{
			imports,
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{&ast.TypeSpec{
					Name: ast.NewIdent(receiverTypeIdentName),
					Type: receiverType,
				}},
			},
			handler,
		},
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), fileAST); err != nil {
		return err
	}
	if !executeFound {
		logger.Println("adding default implementation for func execute")
		buf.WriteString(defaultExecuteImplementation)
	}
	if !handleErrorFound {
		logger.Println("adding default implementation for func handleError")
		buf.WriteString(defaultHandleErrorImplementation)
	}
	out, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(wd, "template_routes.go"), out, 0666)
}

func templateHandlers(_ *template.Template, name muxt.TemplateName, _ *packages.Package, templatesVariable *ast.Ident) (*ast.CallExpr, *ast.Field, []string, error) {
	handler := &ast.FuncLit{
		Type: httpHandlerFuncType(),
		Body: &ast.BlockStmt{
			List: make([]ast.Stmt, 0, 2),
		},
	}
	var methodField *ast.Field
	data := ast.NewIdent(requestIdentName)
	var imports []string
	if name.Handler != "" {
		data = ast.NewIdent(dataIdentName)
		exp, err := parser.ParseExpr(name.Handler)
		if err != nil {
			return nil, nil, nil, err
		}
		call, ok := exp.(*ast.CallExpr)
		if !ok {
			return nil, nil, nil, fmt.Errorf("expected call expression")
		}
		if call.Ellipsis != token.NoPos {
			return nil, nil, nil, fmt.Errorf("ellipsis calls not permitted")
		}
		methodIdent, ok := call.Fun.(*ast.Ident)
		if !ok {
			return nil, nil, nil, fmt.Errorf("expected method name identifier")
		}
		pathParameters, err := name.PathParameters()
		if err != nil {
			return nil, nil, nil, err
		}
		methodFuncType := &ast.FuncType{
			Params: &ast.FieldList{},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: ast.NewIdent("any")},
					{Type: ast.NewIdent("error")},
				},
			},
		}
		args := make([]ast.Expr, 0, len(call.Args))
		for _, arg := range call.Args {
			ai, ok := arg.(*ast.Ident)
			if !ok {
				return nil, nil, nil, fmt.Errorf("arguments must be identifiers")
			}
			switch ai.Name {
			case responseIdentName:
				args = append(args, ast.NewIdent(responseIdentName))
				methodFuncType.Params.List = append(methodFuncType.Params.List, httpResponseField())
			case requestIdentName:
				args = append(args, ast.NewIdent(requestIdentName))
				methodFuncType.Params.List = append(methodFuncType.Params.List, httpRequestField())
			case contextIdentName:
				args = append(args, &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent(requestIdentName),
						Sel: ast.NewIdent("Context"),
					},
					Args: make([]ast.Expr, 0),
				})
				methodFuncType.Params.List = append(methodFuncType.Params.List, contextContextField())
				imports = append(imports, "context")
			default:
				if !slices.Contains(pathParameters, ai.Name) {
					return nil, nil, nil, fmt.Errorf("unknown variable %s", ai.Name)
				}
				handler.Body.List = append(handler.Body.List, &ast.AssignStmt{
					Tok: token.DEFINE,
					Lhs: []ast.Expr{ast.NewIdent(ai.Name)},
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent(requestIdentName),
							Sel: ast.NewIdent("PathValue"),
						},
						Args: []ast.Expr{
							&ast.BasicLit{
								Kind:  token.STRING,
								Value: strconv.Quote(ai.Name),
							},
						},
					}},
				})
				args = append(args, ast.NewIdent(ai.Name))
				methodFuncType.Params.List = append(methodFuncType.Params.List, &ast.Field{
					Names: []*ast.Ident{ast.NewIdent(ai.Name)},
					Type:  ast.NewIdent("string"),
				})
			}
		}
		methodField = &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(methodIdent.Name)},
			Type:  methodFuncType,
		}
		assignData := &ast.AssignStmt{
			Tok: token.DEFINE,
			Lhs: []ast.Expr{
				ast.NewIdent(dataIdentName),
				ast.NewIdent(errorIdentName),
			},
			Rhs: []ast.Expr{
				&ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent(receiverIdentName),
						Sel: methodIdent,
					},
					Args: args,
				},
			},
		}
		handler.Body.List = append(handler.Body.List, assignData, checkError(templatesVariable, "StatusInternalServerError"))
	}

	handler.Body.List = append(handler.Body.List, executeCall(name, templatesVariable, data))

	return handleFuncCall(name, handler), methodField, imports, nil
}

func generalDeclaration(p *packages.Package, ident *ast.Ident) (*ast.ValueSpec, *ast.GenDecl, error) {
	arg := p.TypesInfo.ObjectOf(ident)
	if arg == nil {
		return nil, nil, fmt.Errorf("declaration for argument %s not found", ident.Name)
	}
	if _, ok := arg.(*types.Var); !ok {
		return nil, nil, fmt.Errorf("declaration for argument %s is not a variable", ident.Name)
	}
	for _, f := range p.Syntax {
		for _, d := range f.Decls {
			decl, ok := d.(*ast.GenDecl)
			if !ok || decl.Tok != token.VAR {
				continue
			}
			for _, spec := range decl.Specs {
				v, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i := 0; i < len(v.Names); i++ {
					n := v.Names[i]
					if d := p.TypesInfo.ObjectOf(n); d == arg {
						return v, decl, nil
					}
				}
			}
		}
	}
	return nil, nil, fmt.Errorf("declartion for %s not found", ident.Name)
}

func parseTemplates(dir string, p *packages.Package, name *ast.Ident, exp ast.Expr) (*template.Template, error) {
	call, ok := exp.(*ast.CallExpr)
	if !ok {
		return nil, fmt.Errorf("failed to evaluate template expression at %s", p.Fset.Position(exp.Pos()))
	}

	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		switch sel.Sel.Name {
		case "Must":
			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("expected %s.%s", name.Name, sel.Sel.Name)
			}
			if x.Name != "template" {
				return nil, fmt.Errorf("expected %s.%s", name.Name, sel.Sel.Name)
			}
			if len(call.Args) != 1 {
				return nil, fmt.Errorf("expected %s.%s", name.Name, sel.Sel.Name)
			}
			return parseTemplates(dir, p, name, call.Args[0])
		case "ParseFS":
			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("expected %s.%s", name.Name, sel.Sel.Name)
			}
			if x.Name != "template" {
				return nil, fmt.Errorf("expected %s.%s", name.Name, sel.Sel.Name)
			}
			if len(call.Args) < 1 {
				return nil, fmt.Errorf("%s.%s is missing arguments", name.Name, sel.Sel.Name)
			}
			fsIdent, ok := call.Args[0].(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("%s.%s expected a variable with type embed.FS as the first argument", name.Name, sel.Sel.Name)
			}
			val, dec, err := generalDeclaration(p, fsIdent)
			if err != nil || dec == nil || val == nil {
				return nil, err
			}
			var comment strings.Builder
			readComments(&comment, val.Doc, dec.Doc)

			patterns, err := parsePatterns(comment.String())
			if err != nil {
				return nil, err
			}
			files, err := embeddedFilesMatchingPatternList(dir, p, patterns)
			if err != nil {
				return nil, err
			}
			return template.ParseFiles(files...)
		}
	}
	return nil, nil
}

func embeddedFilesMatchingPatternList(dir string, p *packages.Package, patterns []string) ([]string, error) {
	var matches []string
	for _, fp := range p.EmbedFiles {
		rel, err := filepath.Rel(dir, fp)
		if err != nil {
			return nil, err
		}
		for _, pattern := range patterns {
			pat := filepath.FromSlash(pattern)
			if matched, err := filepath.Match(pat, rel); err != nil {
				return nil, err
			} else if matched {
				matches = append(matches, fp)
			}
		}
	}
	return matches, nil
}

func readComments(s *strings.Builder, groups ...*ast.CommentGroup) {
	for _, c := range groups {
		if c == nil {
			continue
		}
		for _, line := range c.List {
			if !strings.HasPrefix(line.Text, "//go:embed") {
				continue
			}
			s.WriteString(strings.TrimSpace(strings.TrimPrefix(line.Text, "//go:embed")))
			s.WriteByte(' ')
		}
	}
}

func valueSpec(set *token.FileSet, file *ast.File, goLine string) (*ast.ValueSpec, error) {
	number, err := strconv.Atoi(goLine)
	if err != nil {
		return nil, err
	}
	for _, d := range file.Decls {
		decl, ok := d.(*ast.GenDecl)
		if !ok || decl.Tok != token.VAR {
			continue
		}
		if decl.Doc != nil && len(decl.Specs) == 1 {
			p := set.Position(decl.Doc.Pos())
			if p.Line != number {
				continue
			}
			spec, ok := decl.Specs[0].(*ast.ValueSpec)
			if !ok {
				continue
			}
			return spec, nil
		}
		for _, s := range decl.Specs {
			spec, ok := s.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if spec.Doc != nil {
				continue
			}
			p := set.Position(spec.Comment.Pos())
			if p.Line != number {
				continue
			}
			return spec, nil
		}
	}
	return nil, nil
}

func findFile(p *packages.Package, goFile string) (*ast.File, error) {
	i := slices.IndexFunc(p.Syntax, func(file *ast.File) bool {
		fp := p.Fset.Position(file.Pos())
		return filepath.Base(fp.Filename) == goFile
	})
	if i < 0 {
		return nil, fmt.Errorf("file %s not found", goFile)
	}
	return p.Syntax[i], nil
}

func loadPackage(wd, goPackage string) (*packages.Package, error) {
	list, err := packages.Load(&packages.Config{
		Mode:  packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps | packages.NeedEmbedFiles,
		Dir:   ".", // Current directory
		Tests: true,
	}, wd)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(list, func(p *packages.Package) bool { return p.Types.Name() == goPackage })
	if i < 0 {
		return nil, fmt.Errorf("package %s not found", goPackage)
	}
	return list[i], nil
}

func httpResponseField() *ast.Field {
	return &ast.Field{Names: []*ast.Ident{ast.NewIdent(responseIdentName)}, Type: &ast.SelectorExpr{X: ast.NewIdent("http"), Sel: ast.NewIdent("ResponseWriter")}}
}

func httpRequestField() *ast.Field {
	return &ast.Field{Names: []*ast.Ident{ast.NewIdent(requestIdentName)}, Type: &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("http"), Sel: ast.NewIdent("Request")}}}
}

func httpHandlerFuncType() *ast.FuncType {
	return &ast.FuncType{
		Params: &ast.FieldList{List: []*ast.Field{httpResponseField(), httpRequestField()}},
	}
}

func contextContextField() *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(contextIdentName)},
		Type: &ast.SelectorExpr{
			X:   ast.NewIdent("context"),
			Sel: ast.NewIdent("Context"),
		},
	}
}

func checkError(templatesVariable *ast.Ident, statusNameSelector string) *ast.IfStmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errorIdentName), Op: token.NEQ, Y: ast.NewIdent("nil")},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ExprStmt{X: &ast.CallExpr{
					Fun: ast.NewIdent(errorHandlerIdentName),
					Args: []ast.Expr{
						ast.NewIdent(responseIdentName),
						ast.NewIdent(requestIdentName),
						ast.NewIdent(templatesVariable.Name),
						&ast.SelectorExpr{
							X:   ast.NewIdent("http"),
							Sel: ast.NewIdent(statusNameSelector),
						},
						ast.NewIdent(errorIdentName),
					},
				}},
				&ast.ReturnStmt{Results: make([]ast.Expr, 0)},
			},
		},
	}
}

func executeCall(name muxt.TemplateName, templatesVariable, data *ast.Ident) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: ast.NewIdent(executeIdentName),
		Args: []ast.Expr{
			ast.NewIdent(responseIdentName),
			ast.NewIdent(requestIdentName),
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(templatesVariable.Name),
					Sel: ast.NewIdent("Lookup"),
				},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(name.String())}},
			},
			&ast.SelectorExpr{
				X:   ast.NewIdent("http"),
				Sel: ast.NewIdent("StatusOK"),
			},
			data,
		},
	}}
}

func handleFuncCall(name muxt.TemplateName, handler *ast.FuncLit) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(serveMuxIdentName),
			Sel: ast.NewIdent("HandleFunc"),
		},
		Args: []ast.Expr{
			&ast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote(name.Pattern),
			},
			handler,
		},
	}
}

const (
	defaultExecuteImplementation = `
// execute is a default implementation add a function with the same signature to the package and this function will not be generated
func execute(res http.ResponseWriter, _ *http.Request, t *template.Template, code int, data any) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	res.WriteHeader(code)
	_, _ = buf.WriteTo(res)
}
`

	defaultHandleErrorImplementation = `
// handleError is a default implementation add a function with the same signature to the package and this function will not be generated
func handleError(res http.ResponseWriter, _ *http.Request, _ *template.Template, code int, err error) {
	http.Error(res, err.Error(), code)
}
`
)
