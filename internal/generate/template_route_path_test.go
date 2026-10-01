package generate

import (
	"go/types"
	"html/template"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

const routePathTestSource = `package server

type T struct{}

type ID int

func (ID) MarshalText() ([]byte, error) { return nil, nil }

func (T) ByID(id ID) string { return "" }

func (T) ByNumber(n int) string { return "" }

func (T) ByName(name string) string { return "" }

func (T) ByPair(a, b string) string { return "" }

func (T) ByBool(ok bool) string { return "" }

func (T) ByTwoIDs(a, b ID) string { return "" }

func (T) ByRest(rest string) string { return "" }
`

func TestRoutePathFunc(t *testing.T) {
	for _, tt := range []struct {
		name       string
		pattern    string
		pathPrefix bool
		want       string
		escapers   escaperUse
	}{
		{
			name: "index", pattern: "GET /{$}",
			want: `func (routePaths TemplateRoutePaths) ReadExact() TemplateRoute {
	return TemplateRoute{method: "GET", path: "/"}
}`,
		},
		{
			name: "index with prefix", pattern: "GET /{$}", pathPrefix: true,
			want: `func (routePaths TemplateRoutePaths) ReadExact() TemplateRoute {
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"))}
}`,
		},
		{
			name: "literals fold into one segment", pattern: "GET /a/b/c",
			want: `func (routePaths TemplateRoutePaths) ReadABC() TemplateRoute {
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "a/b/c")}
}`,
		},
		{
			name: "int", pattern: "GET /n/{n} ByNumber(n)",
			want: `func (routePaths TemplateRoutePaths) ByNumber(nPathParam int) TemplateRoute {
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "n", strconv.Itoa(nPathParam))}
}`,
		},
		{
			name: "bool", pattern: "GET /b/{ok} ByBool(ok)",
			want: `func (routePaths TemplateRoutePaths) ByBool(okPathParam bool) TemplateRoute {
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "b", strconv.FormatBool(bool(okPathParam)))}
}`,
		},
		{
			name: "string is escaped", pattern: "GET /s/{name} ByName(name)",
			want: `func (routePaths TemplateRoutePaths) ByName(namePathParam string) TemplateRoute {
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "s", routePaths.escapePathSegment(namePathParam))}
}`,
			escapers: escaperUse{segment: true},
		},
		{
			name: "same typed parameters share a field", pattern: "GET /p/{a}/{b} ByPair(a, b)",
			want: `func (routePaths TemplateRoutePaths) ByPair(aPathParam, bPathParam string) TemplateRoute {
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "p", routePaths.escapePathSegment(aPathParam), routePaths.escapePathSegment(bPathParam))}
}`,
			escapers: escaperUse{segment: true},
		},
		{
			name: "unlinked segment is a string", pattern: "GET /u/{name}",
			want: `func (routePaths TemplateRoutePaths) ReadUByName(namePathParam string) TemplateRoute {
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "u", routePaths.escapePathSegment(namePathParam))}
}`,
			escapers: escaperUse{segment: true},
		},
		{
			name: "text marshaler returns an error", pattern: "GET /m/{id} ByID(id)",
			want: `func (routePaths TemplateRoutePaths) ByID(idPathParam ID) (TemplateRoute, error) {
	segment2_4d0556ab, err := idPathParam.MarshalText()
	if err != nil {
		return TemplateRoute{}, fmt.Errorf("failed to marshal path value {id} (segment 2) in /m/{id}: %w", err)
	}
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "m", routePaths.escapePathSegment(string(segment2_4d0556ab)))}, nil
}`,
			escapers: escaperUse{segment: true},
		},
		{
			name: "two text marshalers", pattern: "GET /mm/{a}/{b} ByTwoIDs(a, b)",
			want: `func (routePaths TemplateRoutePaths) ByTwoIDs(aPathParam, bPathParam ID) (TemplateRoute, error) {
	segment2_b8dad819, err := aPathParam.MarshalText()
	if err != nil {
		return TemplateRoute{}, fmt.Errorf("failed to marshal path value {a} (segment 2) in /mm/{a}/{b}: %w", err)
	}
	segment3_b8dad819, err := bPathParam.MarshalText()
	if err != nil {
		return TemplateRoute{}, fmt.Errorf("failed to marshal path value {b} (segment 3) in /mm/{a}/{b}: %w", err)
	}
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "mm", routePaths.escapePathSegment(string(segment2_b8dad819)), routePaths.escapePathSegment(string(segment3_b8dad819)))}, nil
}`,
			escapers: escaperUse{segment: true},
		},
		{
			name: "remainder wildcard", pattern: "GET /r/{rest...} ByRest(rest)",
			want: `func (routePaths TemplateRoutePaths) ByRest(restPathParam string) TemplateRoute {
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "r", routePaths.escapePathSegments(restPathParam))}
}`,
			escapers: escaperUse{segments: true},
		},
		{
			name: "path end wildcard", pattern: "GET /w/{$}",
			want: `func (routePaths TemplateRoutePaths) ReadWExact() TemplateRoute {
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "w") + "/"}
}`,
		},
		{
			name: "path end wildcard after a parameter", pattern: "GET /w/{name}/{$} ByName(name)",
			want: `func (routePaths TemplateRoutePaths) ByName(namePathParam string) TemplateRoute {
	return TemplateRoute{method: "GET", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "w", routePaths.escapePathSegment(namePathParam)) + "/"}
}`,
			escapers: escaperUse{segment: true},
		},
		{
			name: "the route carries its HTTP method", pattern: "POST /w/{name} ByName(name)",
			want: `func (routePaths TemplateRoutePaths) ByName(namePathParam string) TemplateRoute {
	return TemplateRoute{method: "POST", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "w", routePaths.escapePathSegment(namePathParam))}
}`,
			escapers: escaperUse{segment: true},
		},
		{
			name: "a pattern without a method has an empty one", pattern: "/any/{name} ByName(name)",
			want: `func (routePaths TemplateRoutePaths) ByName(namePathParam string) TemplateRoute {
	return TemplateRoute{method: "", path: path.Join(cmp.Or(routePaths.pathsPrefix, "/"), "any", routePaths.escapePathSegment(namePathParam))}
}`,
			escapers: escaperUse{segment: true},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": routePathTestSource})
			src := source.Package{
				Fset:  fake.FileSet,
				Types: pkg,
				Variables: []source.Variable{{
					Name: "templates",
					Set:  template.Must(template.New("templates").Parse(`{{define "` + tt.pattern + `"}}{{end}}`)),
				}},
			}
			id := fake.Lookup(t, pkg, "ID")
			checker := fake.StandInChecker(t, pkg).ParsesFromText(id).FormatsAsText(id).Fake()
			defs, err := muxt.ResolveDefinitions(src, fake.Lookup(t, pkg, "T").(*types.Named), checker)
			require.NoError(t, err)
			config := testConfig()
			config.PathPrefix = tt.pathPrefix
			decl, escapers, err := routePathFunc(newFile(src), config, &defs[0])
			require.NoError(t, err)
			assert.Equal(t, tt.want, astgen.Format(decl), "routePathFunc(%q)", tt.pattern)
			assert.Equal(t, tt.escapers, escapers, "routePathFunc(%q) escapers", tt.pattern)
		})
	}
}

func TestRouteTypeDecls(t *testing.T) {
	for _, tt := range []struct {
		name     string
		typeName string
		want     string
	}{
		{
			name: "the zero value names the default",
			want: `type TemplateRoute struct {
	method string
	path   string
}

func (route TemplateRoute) String() string {
	return route.path
}

func (route TemplateRoute) Method() string {
	return route.method
}`,
		},
		{
			name: "exported", typeName: "TemplateRoute",
			want: `type TemplateRoute struct {
	method string
	path   string
}

func (route TemplateRoute) String() string {
	return route.path
}

func (route TemplateRoute) Method() string {
	return route.method
}`,
		},
		{
			name: "named", typeName: "Link",
			want: `type Link struct {
	method string
	path   string
}

func (route Link) String() string {
	return route.path
}

func (route Link) Method() string {
	return route.method
}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig()
			config.TemplateRouteTypeName = tt.typeName
			var got []string
			for _, decl := range routeTypeDecls(config) {
				got = append(got, astgen.Format(decl))
			}
			assert.Equal(t, tt.want, strings.Join(got, "\n\n"), "routeTypeDecls()")
		})
	}
}
