package generate

import (
	"go/types"
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

func TestRoutePathWrapper(t *testing.T) {
	for _, tt := range []struct {
		name    string
		pattern string
		want    string
	}{
		{
			name: "a path without an error", pattern: "GET /b/{ok} ByBool(ok)",
			want: `func (routePaths TemplateRoutePaths) ByBool(okPathParam bool) string {
	return routePaths.Route.ByBool(okPathParam).String()
}`,
		},
		{
			name: "a marshaled value returns its error", pattern: "GET /m/{id} ByID(id)",
			want: `func (routePaths TemplateRoutePaths) ByID(idPathParam ID) (string, error) {
	route, err := routePaths.Route.ByID(idPathParam)
	if err != nil {
		return "", err
	}
	return route.String(), nil
}`,
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
			builder, _, err := routePathFunc(newFile(src), testConfig(), &defs[0])
			require.NoError(t, err)
			assert.Equal(t, tt.want, astgen.Format(routePathWrapper(testConfig(), builder)))
		})
	}
}
