package muxt

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"html/template"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/crhntr/muxt/internal/source"
)

func Templates(ts *template.Template) ([]Template, error) {
	var templateNames []Template
	patterns := make(map[string]struct{})
	for _, t := range ts.Templates() {
		mt, err, ok := NewTemplateName(t.Name())
		if !ok {
			continue
		}
		if err != nil {
			return templateNames, err
		}
		pattern := strings.Join([]string{mt.method, mt.host, mt.path}, " ")
		if _, exists := patterns[pattern]; exists {
			return templateNames, fmt.Errorf("duplicate route pattern: %s", mt.pattern)
		}
		mt.template = t
		patterns[pattern] = struct{}{}
		templateNames = append(templateNames, mt)
	}
	slices.SortFunc(templateNames, Template.byPathThenMethod)
	return templateNames, nil
}

type Template struct {
	// name has the full unaltered template name
	name string

	// method, host, path, and pattern are parsed sub-parts of the string passed to mux.Handle
	method, host, path, pattern string

	// handler is used to generate the method interface
	handler string

	// statusCode is the status code to use in the response header
	statusCode int

	fun  *ast.Ident
	call *ast.CallExpr

	fileSet *token.FileSet

	template *template.Template
}

func NewTemplateName(in string) (Template, error, bool) { return newTemplate(in) }

func newTemplate(in string) (Template, error, bool) {
	if !templateNameMux.MatchString(in) {
		return Template{}, nil, false
	}
	matches := templateNameMux.FindStringSubmatch(in)
	p := Template{
		name:       in,
		method:     matches[templateNameMux.SubexpIndex("METHOD")],
		host:       matches[templateNameMux.SubexpIndex("HOST")],
		path:       matches[templateNameMux.SubexpIndex("PATH")],
		handler:    strings.TrimSpace(matches[templateNameMux.SubexpIndex("CALL")]),
		pattern:    matches[templateNameMux.SubexpIndex("pattern")],
		fileSet:    token.NewFileSet(),
		statusCode: http.StatusOK,
	}
	httpStatusCode := matches[templateNameMux.SubexpIndex("HTTP_STATUS")]
	if httpStatusCode != "" {
		if strings.HasPrefix(httpStatusCode, "http.Status") {
			code, err := source.HTTPStatusName(httpStatusCode)
			if err != nil {
				return Template{}, fmt.Errorf("failed to parse status code: %w", err), true
			}
			p.statusCode = code
		} else {
			code, err := strconv.Atoi(strings.TrimSpace(httpStatusCode))
			if err != nil {
				return Template{}, fmt.Errorf("failed to parse status code: %w", err), true
			}
			p.statusCode = code
		}
	}

	switch p.method {
	default:
		return p, fmt.Errorf("%s method not allowed", p.method), true
	case "", http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	}

	pathParameterNames := p.parsePathValueNames()
	if err := checkPathValueNames(pathParameterNames); err != nil {
		return Template{}, err, true
	}

	err := parseHandler(p.fileSet, &p, pathParameterNames)
	if err != nil {
		return p, err, true
	}

	if httpStatusCode != "" && !p.callWriteHeader(nil) {
		return p, fmt.Errorf("you can not use %s as an argument and specify an HTTP status code", TemplateNameScopeIdentifierHTTPResponse), true
	}

	return p, nil, true
}

var (
	pathSegmentPattern = regexp.MustCompile(`/\{([^}]*)}`)
	templateNameMux    = regexp.MustCompile(`^(?P<pattern>(((?P<METHOD>[A-Z]+)\s+)?)(?P<HOST>([^/])*)(?P<PATH>(/(\S)*)))(\s+(?P<HTTP_STATUS>(\d|http\.Status)\S+))?(?P<CALL>.*)?$`)
)

func (t Template) parsePathValueNames() []string {
	var result []string
	for _, match := range pathSegmentPattern.FindAllStringSubmatch(t.path, strings.Count(t.path, "/")) {
		n := match[1]
		if n == "$" && strings.Count(t.path, "$") == 1 && strings.HasSuffix(t.path, "{$}") {
			continue
		}
		n = strings.TrimSuffix(n, "...")
		result = append(result, n)
	}
	return result
}

func checkPathValueNames(in []string) error {
	for i, n := range in {
		if !token.IsIdentifier(n) {
			return fmt.Errorf("path parameter name not permitted: %q is not a Go identifier", n)
		}
		if slices.Contains(in[:i], n) {
			return fmt.Errorf("forbidden repeated path parameter names: found at least 2 path parameters with name %q", n)
		}
		if slices.Contains(patternScope(), n) {
			return fmt.Errorf("the name %s is not allowed as a path parameter it is already in scope", n)
		}
	}
	return nil
}

func (t Template) String() string { return t.name }

func (t Template) Method() string {
	if t.fun == nil {
		return ""
	}
	return t.fun.Name
}

func (t Template) Template() *template.Template {
	return t.template
}

func (t Template) byPathThenMethod(d Template) int {
	if n := cmp.Compare(t.path, d.path); n != 0 {
		return n
	}
	if m := cmp.Compare(t.method, d.method); m != 0 {
		return m
	}
	return cmp.Compare(t.handler, d.handler)
}

func parseHandler(fileSet *token.FileSet, def *Template, pathParameterNames []string) error {
	if def.handler == "" {
		return nil
	}
	e, err := parser.ParseExprFrom(fileSet, "template_name.go", []byte(def.handler), 0)
	if err != nil {
		return fmt.Errorf("failed to parse handler expression: %v", err)
	}
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return fmt.Errorf("expected call expression, got: %s", source.Format(e))
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return fmt.Errorf("expected function identifier, got got: %s", source.Format(call.Fun))
	}
	if call.Ellipsis != token.NoPos {
		return fmt.Errorf("unexpected ellipsis")
	}

	scope := append(patternScope(), pathParameterNames...)
	slices.Sort(scope)
	if err := checkArguments(scope, call); err != nil {
		return err
	}

	def.fun = fun
	def.call = call
	return nil
}

func (t Template) callWriteHeader(receiverInterfaceType *ast.InterfaceType) bool {
	if t.call == nil {
		return true
	}
	return !hasIdentArgument(t.call.Args, TemplateNameScopeIdentifierHTTPResponse, receiverInterfaceType, 1, 1)
}

func hasIdentArgument(args []ast.Expr, ident string, receiverInterfaceType *ast.InterfaceType, depth, maxDepth int) bool {
	if depth > maxDepth {
		return false
	}
	for _, arg := range args {
		switch exp := arg.(type) {
		case *ast.Ident:
			if exp.Name == ident {
				return true
			}
		case *ast.CallExpr:
			methodIdent, ok := exp.Fun.(*ast.Ident)
			if ok && receiverInterfaceType != nil {
				field, ok := source.FindFieldWithName(receiverInterfaceType.Methods, methodIdent.Name)
				if ok {
					funcType, ok := field.Type.(*ast.FuncType)
					if ok {
						if funcType.Results.NumFields() == 1 {
							if hasIdentArgument(exp.Args, ident, receiverInterfaceType, depth+1, maxDepth+1) {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func checkArguments(identifiers []string, call *ast.CallExpr) error {
	for i, a := range call.Args {
		switch exp := a.(type) {
		case *ast.Ident:
			if _, ok := slices.BinarySearch(identifiers, exp.Name); !ok {
				return fmt.Errorf("unknown argument %s at index %d", exp.Name, i)
			}
		case *ast.CallExpr:
			if err := checkArguments(identifiers, exp); err != nil {
				return fmt.Errorf("call %s argument error: %w", source.Format(call.Fun), err)
			}
		default:
			return fmt.Errorf("expected only identifier or call expressions as arguments, argument at index %d is: %s", i, source.Format(a))
		}
	}
	return nil
}

const (
	TemplateNameScopeIdentifierHTTPRequest  = "request"
	TemplateNameScopeIdentifierHTTPResponse = "response"
	TemplateNameScopeIdentifierContext      = "ctx"
	TemplateNameScopeIdentifierForm         = "form"
)

func patternScope() []string {
	return []string{
		TemplateNameScopeIdentifierHTTPRequest,
		TemplateNameScopeIdentifierHTTPResponse,
		TemplateNameScopeIdentifierContext,
		TemplateNameScopeIdentifierForm,
	}
}

func (t Template) matchReceiver(funcDecl *ast.FuncDecl, receiverTypeIdent string) bool {
	if funcDecl == nil || funcDecl.Name == nil || funcDecl.Name.Name != t.fun.Name ||
		funcDecl.Recv == nil || len(funcDecl.Recv.List) < 1 {
		return false
	}
	exp := funcDecl.Recv.List[0].Type
	if star, ok := exp.(*ast.StarExpr); ok {
		exp = star.X
	}
	ident, ok := exp.(*ast.Ident)
	return ok && ident.Name == receiverTypeIdent
}

func (t Template) callHandleFunc(handlerFuncLit *ast.FuncLit) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(muxVarIdent),
			Sel: ast.NewIdent(httpHandleFuncIdent),
		},
		Args: []ast.Expr{source.String(t.pattern), handlerFuncLit},
	}}
}

func (t Template) callReceiverMethod(imports *source.Imports, dataVarIdent string, method *ast.FuncType, call *ast.CallExpr) ([]ast.Stmt, error) {
	const (
		okIdent = "ok"
	)
	if method.Results == nil || len(method.Results.List) == 0 {
		return nil, fmt.Errorf("method for pattern %q has no results it should have one or two", t)
	} else if len(method.Results.List) > 1 {
		_, lastResultType, ok := source.FieldIndex(method.Results.List, method.Results.NumFields()-1)
		if !ok {
			return nil, fmt.Errorf("failed to get the last method result")
		}
		switch rt := lastResultType.(type) {
		case *ast.Ident:
			switch rt.Name {
			case "error":
				return []ast.Stmt{
					&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
					&ast.IfStmt{
						Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: source.Nil()},
						Body: &ast.BlockStmt{
							List: []ast.Stmt{
								&ast.ExprStmt{X: imports.HTTPErrorCall(ast.NewIdent(httpResponseField(imports).Names[0].Name), source.CallError(errIdent), http.StatusInternalServerError)},
								&ast.ReturnStmt{},
							},
						},
					},
				}, nil
			case "bool":
				return []ast.Stmt{
					&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent), ast.NewIdent(okIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
					&ast.IfStmt{
						Cond: &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent(okIdent)},
						Body: &ast.BlockStmt{
							List: []ast.Stmt{
								&ast.ReturnStmt{},
							},
						},
					},
				}, nil
			default:
				return nil, fmt.Errorf("expected last result to be either an error or a bool")
			}
		default:
			return nil, fmt.Errorf("expected last result to be either an error or a bool")
		}
	} else {
		return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}}}, nil
	}
}
