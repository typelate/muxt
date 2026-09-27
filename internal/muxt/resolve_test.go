package muxt_test

import (
	"go/types"
	"html/template"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

func TestResolve(t *testing.T) {
	const server = `package server

type T struct{}

type In struct{}

func (T) Article(id int) any { return nil }
func (T) Form(In) any        { return nil }
`
	variable := func(name, templates string) source.Variable {
		return source.Variable{Name: name, Set: template.Must(template.New(name).Parse(templates))}
	}

	t.Run("every variable's routes are resolved", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": server})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("pages", `{{define "GET /article/{id} Article(id)"}}{{end}}`),
			variable("fragments", `{{define "GET /fragment/{id} Article(id)"}}{{end}}`),
		}}
		defs, err := muxt.Resolve(src, fake.Lookup(t, pkg, "T").(*types.Named), fake.NewChecker().Fake())
		require.NoError(t, err)
		require.Len(t, defs, 2)
		require.Equal(t, "pages", defs[0].TemplatesVariable())
		require.Equal(t, "fragments", defs[1].TemplatesVariable())
		for _, def := range defs {
			require.False(t, def.Signature().IsZero(), "%s is resolved", def.Name())
			segment, ok := def.PathParameter("id")
			require.True(t, ok)
			require.Equal(t, "int", segment.Type().Format(unqualified))
		}
	})

	t.Run("without a receiver methods are inferred", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": server})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("templates", `{{define "GET /{id} Missing(id)"}}{{end}}`),
		}}
		defs, err := muxt.Resolve(src, nil, fake.NewChecker().Fake())
		require.NoError(t, err)
		require.Len(t, defs, 1)
		require.Equal(t, []string{"Missing(id string) any"}, defs[0].SynthesizedMethods())
	})

	t.Run("a route without a call is left alone", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": server})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("templates", `{{define "GET /about"}}{{end}}`),
		}}
		defs, err := muxt.Resolve(src, nil, fake.NewChecker().Fake())
		require.NoError(t, err)
		require.Len(t, defs, 1)
		require.True(t, defs[0].Signature().IsZero())
	})

	t.Run("resolution errors are combined", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": server})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("templates", `{{define "GET /a/{id} Form(id)"}}{{end}}{{define "GET /b/{name} Form(name)"}}{{end}}`),
		}}
		_, err := muxt.Resolve(src, fake.Lookup(t, pkg, "T").(*types.Named), fake.NewChecker().Fake())
		require.ErrorContains(t, err, "unsupported type: In")
		require.ErrorContains(t, err, "(and 1 more error)")
	})
}
