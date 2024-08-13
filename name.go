package muxt

import (
	"fmt"
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

var pathSegmentPattern = regexp.MustCompile(`/\{([^}]*)}`)

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

var templateNameMux = regexp.MustCompile(`^(?P<Pattern>(((?P<Method>[A-Z]+)\s+)?)(?P<Host>([^/])*)(?P<Path>(/(\S)*)))(?P<Handler>.*)$`)

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
	case "", http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	}

	return p, nil, true
}
