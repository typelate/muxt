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
	return routePaths.routes.ByBool(okPathParam).Path()
}`,
		},
		{
			name: "a marshaled value returns its error", pattern: "GET /m/{id} ByID(id)",
			want: `func (routePaths TemplateRoutePaths) ByID(idPathParam ID) (string, error) {
	route, err := routePaths.routes.ByID(idPathParam)
	if err != nil {
		return "", err
	}
	return route.Path(), nil
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
			wrapper := routePathWrapper(testConfig(), builder)
			assert.Equal(t, tt.want, astgen.Format(wrapper))

			// The two declarations are separate nodes in the file:
			// changing the wrapper's parameters leaves the builder's alone.
			before := astgen.Format(builder)
			wrapper.Type.Params.List[0].Names[0].Name = "changed"
			wrapper.Type.Params.List = nil
			assert.Equal(t, before, astgen.Format(builder), "the builder after changing the wrapper's parameters")
		})
	}
}
