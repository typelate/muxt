package muxt

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"html/template"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/source"
)

func Templates(ts *template.Template) ([]Template, error) {
	var templates []Template
	patterns := make(map[string]struct{})
	for _, t := range ts.Templates() {
		mt, err, ok := newTemplate(t.Name())
		if !ok {
			continue
		}
		if err != nil {
			return templates, err
		}
		pattern := strings.Join([]string{mt.method, mt.host, mt.path}, " ")
		if _, exists := patterns[pattern]; exists {
			return templates, fmt.Errorf("duplicate route pattern: %s", mt.pattern)
		}
		mt.template = t

		// Extract source file from ParseName if available
		if t.Tree != nil && t.Tree.ParseName != "" {
			// ParseName contains the filename used when parsing
			mt.sourceFile = t.Tree.ParseName
		}
		// else sourceFile remains empty string for Parse() defined templates

		patterns[pattern] = struct{}{}
		templates = append(templates, mt)
	}
	slices.SortFunc(templates, Template.byPathThenMethod)
	calculateIdentifiers(templates)
	return templates, nil
}

type Template struct {
	// name has the full unaltered template name
	name string

	// method, host, path, and pattern are parsed sub-parts of the string passed to mux.Handle
	method, host, path, pattern string

	// handler is used to generate the method interface
	handler string

	// defaultStatusCode is the status code to use in the response header for this template endpoint
	defaultStatusCode int

	fun  *ast.Ident
	call *ast.CallExpr

	fileSet *token.FileSet

	template *template.Template

	pathValueTypes map[string]types.Type
	pathValueNames []string

	identifier string

	hasResponseWriterArg bool

	// sourceFile is the base filename (e.g., "index.gohtml") from which this template was parsed.
	// Empty string means the template was defined via Parse() calls rather than from a file.
	sourceFile string
}

func newTemplate(in string) (Template, error, bool) {
	if !templateNameMux.MatchString(in) {
		return Template{}, nil, false
	}
	matches := templateNameMux.FindStringSubmatch(in)
	p := Template{
		name:              in,
		method:            matches[templateNameMux.SubexpIndex("METHOD")],
		host:              matches[templateNameMux.SubexpIndex("HOST")],
		path:              matches[templateNameMux.SubexpIndex("PATH")],
		handler:           strings.TrimSpace(matches[templateNameMux.SubexpIndex("CALL")]),
		pattern:           matches[templateNameMux.SubexpIndex("pattern")],
		fileSet:           token.NewFileSet(),
		defaultStatusCode: http.StatusOK,
		pathValueTypes:    make(map[string]types.Type),
	}
	httpStatusCode := matches[templateNameMux.SubexpIndex("HTTP_STATUS")]
	if httpStatusCode != "" {
		if strings.HasPrefix(httpStatusCode, "http.Status") {
			code, err := source.HTTPStatusName(httpStatusCode)
			if err != nil {
				return Template{}, fmt.Errorf("failed to parse status code: %w", err), true
			}
			p.defaultStatusCode = code
		} else {
			code, err := strconv.Atoi(strings.TrimSpace(httpStatusCode))
			if err != nil {
				return Template{}, fmt.Errorf("failed to parse status code: %w", err), true
			}
			p.defaultStatusCode = code
		}
	}

	if len(p.path) > 1 {
		segments := strings.Split(p.path[1:], "/")
		for _, segment := range segments {
			if segment == "" {
				return Template{}, fmt.Errorf("template has an empty path segment: %s", p.name), true
			}
		}
	}

	switch p.method {
	default:
		return p, fmt.Errorf("%s method not allowed", p.method), true
	case "", http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	}

	pathValueNames := p.parsePathValueNames()
	if err := checkPathValueNames(pathValueNames); err != nil {
		return Template{}, err, true
	}
	p.pathValueNames = pathValueNames

	err := parseHandler(p.fileSet, &p, p.pathValueNames)
	if err != nil {
		return p, err, true
	}

	if p.fun == nil {
		for _, name := range p.pathValueNames {
			p.pathValueTypes[name] = types.Universe.Lookup("string").Type()
		}
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

func hasHTTPResponseWriterArgument(call *ast.CallExpr) bool {
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			if arg.Name == TemplateNameScopeIdentifierHTTPResponse {
				return true
			}
		case *ast.CallExpr:
			if hasHTTPResponseWriterArgument(arg) {
				return true
			}
		}
	}
	return false
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

	def.hasResponseWriterArg = hasHTTPResponseWriterArgument(call)

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

	TemplateDataFieldIdentifierResult        = "result"
	TemplateDataFieldIdentifierOkay          = "okay"
	TemplateDataFieldIdentifierRedirectURL   = "redirectURL"
	TemplateDataFieldIdentifierError         = "errList"
	TemplateDataFieldIdentifierReceiver      = "receiver"
	TemplateDataFieldIdentifierStatusCode    = "statusCode"
	TemplateDataFieldIdentifierErrStatusCode = "errStatusCode"
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

func (t Template) callHandleFunc(file *source.File, handlerFuncLit *ast.FuncLit, config RoutesFileConfiguration) *ast.ExprStmt {
	pattern := ast.Expr(source.String(t.pattern))
	if config.PathPrefix {
		i := strings.Index(t.pattern, "/")
		pattern = &ast.BinaryExpr{
			X:  source.String(t.pattern[:i]),
			Op: token.ADD,
			Y:  file.Call("path", "path", "Join", []ast.Expr{ast.NewIdent(pathPrefixPathsStructFieldName), source.String(t.pattern[i:])}),
		}
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(muxVarIdent),
			Sel: ast.NewIdent(httpHandleFuncIdent),
		},
		Args: []ast.Expr{pattern, handlerFuncLit},
	}}
}
