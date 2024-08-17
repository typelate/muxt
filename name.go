package muxt

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

type TemplateName struct {
	name                        string
	Method, Host, Path, Pattern string
	Handler                     string
}

func NewTemplateName(in string) (TemplateName, error, bool) {
	if !templateNameMux.MatchString(in) {
		return TemplateName{}, nil, false
	}
	matches := templateNameMux.FindStringSubmatch(in)
	p := TemplateName{
		name:    in,
		Method:  matches[templateNameMux.SubexpIndex("Method")],
		Host:    matches[templateNameMux.SubexpIndex("Host")],
		Path:    matches[templateNameMux.SubexpIndex("Path")],
		Handler: matches[templateNameMux.SubexpIndex("Handler")],
		Pattern: matches[templateNameMux.SubexpIndex("Pattern")],
	}

	switch p.Method {
	default:
		return p, fmt.Errorf("%s method not allowed", p.Method), true
	case "", http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	}

	return p, nil, true
}

var (
	pathSegmentPattern = regexp.MustCompile(`/\{([^}]*)}`)
	templateNameMux    = regexp.MustCompile(`^(?P<Pattern>(((?P<Method>[A-Z]+)\s+)?)(?P<Host>([^/])*)(?P<Path>(/(\S)*)))(?P<Handler>.*)$`)
)

func (def TemplateName) PathParameters() ([]string, error) {
	var result []string
	for _, matches := range pathSegmentPattern.FindAllStringSubmatch(def.Path, strings.Count(def.Path, "/")) {
		n := matches[1]
		if n == "$" {
			continue
		}
		n = strings.TrimSuffix(n, "...")
		if !token.IsIdentifier(n) {
			return nil, fmt.Errorf("path parameter name not permitted: %q is not a Go identifier", n)
		}
		result = append(result, n)
	}
	for i, n := range result {
		if slices.Contains(result[:i], n) {
			return nil, fmt.Errorf("path parameter %s defined at least twice", n)
		}
	}
	return result, nil
}

func (def TemplateName) String() string {
	return def.name
}

func (def TemplateName) ByPathThenMethod(d TemplateName) int {
	if n := cmp.Compare(def.Path, d.Path); n != 0 {
		return n
	}
	if m := cmp.Compare(def.Method, d.Method); m != 0 {
		return m
	}
	return cmp.Compare(def.Handler, d.Handler)
}

func (def TemplateName) CallExpr() (*ast.CallExpr, *ast.Ident, error) {
	e, err := parser.ParseExpr(def.Handler)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse handler expression: %v", err)
	}
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return nil, nil, fmt.Errorf("expected call, got: %s", formatNode(e))
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, nil, fmt.Errorf("expected function identifier, got got: %s", formatNode(call.Fun))
	}
	return call, fun, nil
}

func formatNode(node ast.Node) string {
	var buf strings.Builder
	if err := printer.Fprint(&buf, token.NewFileSet(), node); err != nil {
		return fmt.Sprintf("formatting error: %v", err)
	}
	return buf.String()
}
