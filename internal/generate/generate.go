package generate

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
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
	serveMuxIdentName = "mux"

	contextIdentName  = muxt.PatternScopeIdentifierContext
	requestIdentName  = muxt.PatternScopeIdentifierHTTPRequest
	responseIdentName = muxt.PatternScopeIdentifierHTTPResponse

	receiverIdentName = "receiver"
	dataIdentName     = "data"
	errorIdentName    = "err"

	errorHandlerIdentName = "handleError"
	executeIdentName      = "execute"

	outputFilename = "template_routes.go"

	receiverTypeIdentNameDefault = "Receiver"
	handlerFuncIdentNameDefault  = "TemplateRoutes"

	goEmbedCommentPrefix = "//go:embed"

	goPackageEnvVar = "GOPACKAGE"
	goFileEnvVar    = "GOFILE"
	goLineEnvVar    = "GOLINE"

	// httpStatusInternalServerError is the identifier for http.StatusInternalServerError
	httpStatusInternalServerError = "StatusInternalServerError"
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
	goPackage, goFile, goLine, err := goGenerateEnv(lookupEnv)
	if err != nil {
		return err
	}
	pkg, err := loadPackage(wd, goPackage)
	if err != nil {
		return err
	}
	file, err := findFile(pkg, goFile)
	if err != nil {
		return err
	}
	spec, err := valueSpecForComment(pkg.Fset, file, goLine)
	if err != nil || spec == nil {
		return err
	}
	_ = os.Remove(filepath.Join(wd, outputFilename))
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

	for i := 0; i < len(spec.Names) && i < len(spec.Values) && len(spec.Names) == len(spec.Values); i++ {
		n, v := spec.Names[i], spec.Values[i]

		ts, err := parseTemplates(wd, pkg, n, v)
		if err != nil {
			return err
		}

		patterns, err := muxt.TemplatePatterns(ts)
		if err != nil {
			return err
		}

		for _, pat := range patterns {
			handleFunc, methodField, handlerImports, err := templateHandlers(pat, n)
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

func templateHandlers(pat muxt.Pattern, templatesVariable *ast.Ident) (*ast.CallExpr, *ast.Field, []string, error) {
	handler := &ast.FuncLit{
		Type: httpHandlerFuncType(),
		Body: &ast.BlockStmt{
			List: make([]ast.Stmt, 0, 2),
		},
	}
	var methodField *ast.Field
	data := ast.NewIdent(requestIdentName)
	var imports []string
	if pat.Handler != "" {
		data = ast.NewIdent(dataIdentName)
		pathParameters, err := pat.PathParameters()
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
		args := make([]ast.Expr, 0, len(pat.ArgIdents()))
		for _, arg := range pat.ArgIdents() {
			switch arg.Name {
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
				if !slices.Contains(pathParameters, arg.Name) {
					return nil, nil, nil, fmt.Errorf("unknown variable %s", arg.Name)
				}
				handler.Body.List = append(handler.Body.List, &ast.AssignStmt{
					Tok: token.DEFINE,
					Lhs: []ast.Expr{ast.NewIdent(arg.Name)},
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent(requestIdentName),
							Sel: ast.NewIdent("PathValue"),
						},
						Args: []ast.Expr{
							&ast.BasicLit{
								Kind:  token.STRING,
								Value: strconv.Quote(arg.Name),
							},
						},
					}},
				})
				args = append(args, ast.NewIdent(arg.Name))
				methodFuncType.Params.List = append(methodFuncType.Params.List, &ast.Field{
					Names: []*ast.Ident{ast.NewIdent(arg.Name)},
					Type:  ast.NewIdent("string"),
				})
			}
		}
		methodField = &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(pat.FunIdent().Name)},
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
						Sel: ast.NewIdent(pat.FunIdent().Name),
					},
					Args: args,
				},
			},
		}
		handler.Body.List = append(handler.Body.List, assignData, checkError(templatesVariable, httpStatusInternalServerError))
	}

	handler.Body.List = append(handler.Body.List, executeCall(pat, templatesVariable, data))

	return handleFuncCall(pat, handler), methodField, imports, nil
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
			var parseFiles func(files ...string) (*template.Template, error)
			switch x := sel.X.(type) {
			case *ast.Ident:
				x, ok := sel.X.(*ast.Ident)
				if !ok {
					return nil, fmt.Errorf("expected %s.%s", name.Name, sel.Sel.Name)
				}
				if x.Name != "template" {
					return nil, fmt.Errorf("expected %s.%s", name.Name, sel.Sel.Name)
				}
				parseFiles = template.ParseFiles
			case *ast.CallExpr:
				ts, err := parseTemplates(dir, p, name, x)
				if err != nil {
					return nil, err
				}
				parseFiles = ts.ParseFiles
			default:
				return nil, fmt.Errorf("expected %s.%s", name.Name, sel.Sel.Name)
			}
			if len(call.Args) < 1 {
				return nil, fmt.Errorf("%s.%s is missing required fs.FS argument", name.Name, sel.Sel.Name)
			}
			fsIdent, ok := call.Args[0].(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("%s.%s expected a variable with type embed.FS as the first argument", name.Name, sel.Sel.Name)
			}
			files, err := embedFSFilepaths(dir, p, fsIdent)
			if err != nil {
				return nil, err
			}
			globs := make([]string, 0, len(call.Args[1:]))
			for _, a := range call.Args[1:] {
				switch arg := a.(type) {
				case *ast.BasicLit:
					if arg.Kind != token.STRING {
						return nil, fmt.Errorf("expected string literal")
					}
					value, err := strconv.Unquote(arg.Value)
					if err != nil {
						return nil, err
					}
					globs = append(globs, value)
				}
			}
			filtered := files[:0]
			for _, file := range files {
				rel, err := filepath.Rel(dir, file)
				if err != nil {
					return nil, err
				}
				for _, pattern := range globs {
					match, err := filepath.Match(pattern, rel)
					if err != nil || !match {
						continue
					}
					filtered = append(filtered, file)
					break
				}
			}
			files = slices.Clip(filtered)
			return parseFiles(files...)
		}
	}
	return nil, nil
}

func embedFSFilepaths(dir string, p *packages.Package, fsIdent *ast.Ident) ([]string, error) {
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
	return files, nil
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

			fullPat := filepath.Join(dir, filepath.FromSlash(pat)) + "/"
			if i := slices.IndexFunc(p.EmbedFiles, func(file string) bool {
				return strings.HasPrefix(file, fullPat)
			}); i >= 0 {
				matches = append(matches, p.EmbedFiles[i])
				continue
			}

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
			if !strings.HasPrefix(line.Text, goEmbedCommentPrefix) {
				continue
			}
			s.WriteString(strings.TrimSpace(strings.TrimPrefix(line.Text, goEmbedCommentPrefix)))
			s.WriteByte(' ')
		}
	}
}

func valueSpecForComment(set *token.FileSet, file *ast.File, commentLine int) (*ast.ValueSpec, error) {
	for _, d := range file.Decls {
		decl, ok := d.(*ast.GenDecl)
		if !ok || decl.Tok != token.VAR {
			continue
		}
		if decl.Doc != nil && len(decl.Specs) == 1 {
			spec, ok := decl.Specs[0].(*ast.ValueSpec)
			if !ok {
				continue
			}
			if p := set.Position(decl.Doc.Pos()); p.Line != commentLine {
				continue
			}
			return spec, nil
		}
		for _, s := range decl.Specs {
			spec, ok := s.(*ast.ValueSpec)
			if !ok || spec.Doc == nil {
				continue
			}
			if p := set.Position(spec.Doc.Pos()); p.Line != commentLine {
				continue
			}
			return spec, nil
		}
	}
	return nil, fmt.Errorf("comment on line %d must be followed by a variable declaration", commentLine)
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
		Mode:  packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedEmbedPatterns | packages.NeedDeps | packages.NeedEmbedFiles,
		Dir:   wd,
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

func goGenerateEnv(lookupEnv func(string) (string, bool)) (string, string, int, error) {
	goPackage, ok := lookupEnv(goPackageEnvVar)
	if !ok {
		return "", "", 0, fmt.Errorf("%s is not set", goPackageEnvVar)
	}
	goFile, ok := lookupEnv(goFileEnvVar)
	if !ok {
		return "", "", 0, fmt.Errorf("%s is not set", goFileEnvVar)
	}
	goLine, ok := lookupEnv(goLineEnvVar)
	if !ok {
		return "", "", 0, fmt.Errorf("%s is not set", goLineEnvVar)
	}
	number, err := strconv.Atoi(goLine)
	if err != nil {
		return "", "", 0, err
	}
	return goPackage, goFile, number, nil
}

func httpResponseField() *ast.Field {
	return &ast.Field{Names: []*ast.Ident{ast.NewIdent(responseIdentName)}, Type: &ast.SelectorExpr{X: ast.NewIdent("http"), Sel: ast.NewIdent("ResponseWriter")}}
}

func httpRequestField() *ast.Field {
	return &ast.Field{Names: []*ast.Ident{ast.NewIdent(requestIdentName)}, Type: &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("http"), Sel: ast.NewIdent("Request")}}}
}

func httpHandlerFuncType() *ast.FuncType {
	return &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{httpResponseField(), httpRequestField()}}}
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

func executeCall(name muxt.Pattern, templatesVariable, data *ast.Ident) *ast.ExprStmt {
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

func handleFuncCall(name muxt.Pattern, handler *ast.FuncLit) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(serveMuxIdentName),
			Sel: ast.NewIdent("HandleFunc"),
		},
		Args: []ast.Expr{
			&ast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote(name.Route),
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
