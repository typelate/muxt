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
	"strings"

	"github.com/crhntr/muxt/internal/source"
)

func TemplateNames(ts *template.Template) ([]TemplateName, error) {
	var templateNames []TemplateName
	routes := make(map[string]struct{})
	for _, t := range ts.Templates() {
		pat, err, ok := NewTemplateName(t.Name())
		if !ok {
			continue
		}
		if err != nil {
			return templateNames, err
		}
		if _, exists := routes[pat.method+pat.path]; exists {
			return templateNames, fmt.Errorf("duplicate route pattern: %s", pat.endpoint)
		}
		routes[pat.method+pat.path] = struct{}{}
		templateNames = append(templateNames, pat)
	}
	slices.SortFunc(templateNames, TemplateName.byPathThenMethod)
	return templateNames, nil
}

type TemplateName struct {
	name                         string
	method, host, path, endpoint string
	handler                      string

	fun  *ast.Ident
	call *ast.CallExpr

	fileSet *token.FileSet

	pathValueNames []string
}

func NewTemplateName(in string) (TemplateName, error, bool) { return newTemplate(in) }

func newTemplate(in string) (TemplateName, error, bool) {
	if !templateNameMux.MatchString(in) {
		return TemplateName{}, nil, false
	}
	matches := templateNameMux.FindStringSubmatch(in)
	p := TemplateName{
		name:     in,
		method:   matches[templateNameMux.SubexpIndex("method")],
		host:     matches[templateNameMux.SubexpIndex("host")],
		path:     matches[templateNameMux.SubexpIndex("path")],
		handler:  strings.TrimSpace(matches[templateNameMux.SubexpIndex("handler")]),
		endpoint: matches[templateNameMux.SubexpIndex("endpoint")],
		fileSet:  token.NewFileSet(),
	}

	switch p.method {
	default:
		return p, fmt.Errorf("%s method not allowed", p.method), true
	case "", http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	}

	names, err := p.parsePathValueNames()
	if err != nil {
		return TemplateName{}, err, true
	}
	p.pathValueNames = names
	if err := checkPathValueNames(p.pathValueNames); err != nil {
		return TemplateName{}, err, true
	}

	return p, parseHandler(p.fileSet, &p), true
}

var (
	pathSegmentPattern = regexp.MustCompile(`/\{([^}]*)}`)
	templateNameMux    = regexp.MustCompile(`^(?P<endpoint>(((?P<method>[A-Z]+)\s+)?)(?P<host>([^/])*)(?P<path>(/(\S)*)))(?P<handler>.*)$`)
)

func (def TemplateName) parsePathValueNames() ([]string, error) {
	var result []string
	for _, match := range pathSegmentPattern.FindAllStringSubmatch(def.path, strings.Count(def.path, "/")) {
		n := match[1]
		if n == "$" && strings.Count(def.path, "$") == 1 && strings.HasSuffix(def.path, "{$}") {
			continue
		}
		n = strings.TrimSuffix(n, "...")
		result = append(result, n)
	}
	return result, nil
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
			return fmt.Errorf("the name %s is not allowed as a path paramenter it is alredy in scope", n)
		}
	}
	return nil
}

func (def TemplateName) String() string  { return def.name }
func (def TemplateName) Pattern() string { return def.method + " " + def.path }

func (def TemplateName) sameRoute(p TemplateName) bool { return def.endpoint == p.endpoint }

func (def TemplateName) byPathThenMethod(d TemplateName) int {
	if n := cmp.Compare(def.path, d.path); n != 0 {
		return n
	}
	if m := cmp.Compare(def.method, d.method); m != 0 {
		return m
	}
	return cmp.Compare(def.handler, d.handler)
}

func parseHandler(fileSet *token.FileSet, def *TemplateName) error {
	if def.handler == "" {
		return nil
	}
	e, err := parser.ParseExprFrom(fileSet, "template_name.go", []byte(def.handler), 0)
	if err != nil {
		return fmt.Errorf("failed to parse handler expression: %v", err)
	}
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return fmt.Errorf("expected call, got: %s", source.Format(e))
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return fmt.Errorf("expected function identifier, got got: %s", source.Format(call.Fun))
	}
	if call.Ellipsis != token.NoPos {
		return fmt.Errorf("unexpected ellipsis")
	}
	args := make([]*ast.Ident, len(call.Args))
	scope := append(patternScope(), def.pathValueNames...)
	slices.Sort(scope)
	for i, a := range call.Args {
		arg, ok := a.(*ast.Ident)
		if !ok {
			return fmt.Errorf("expected only argument expressions as arguments, argument at index %d is: %s", i, source.Format(a))
		}
		if _, ok := slices.BinarySearch(scope, arg.Name); !ok {
			return fmt.Errorf("unknown argument %s at index %d", arg.Name, i)
		}
		args[i] = arg
	}
	def.fun = fun
	def.call = call
	return nil
}

const (
	TemplateNameScopeIdentifierHTTPRequest  = "request"
	TemplateNameScopeIdentifierHTTPResponse = "response"
	TemplateNameScopeIdentifierContext      = "ctx"
)

func patternScope() []string {
	return []string{
		TemplateNameScopeIdentifierHTTPRequest,
		TemplateNameScopeIdentifierHTTPResponse,
		TemplateNameScopeIdentifierContext,
	}
}
