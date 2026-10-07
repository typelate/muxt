package muxt_test

import (
	"errors"
	"go/types"
	"html/template"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

func TestResolveDefinitions(t *testing.T) {
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
		defs, err := muxt.ResolveDefinitions(src, fake.Lookup(t, pkg, "T").(*types.Named), fake.NewChecker().Fake())
		require.NoError(t, err)
		require.Len(t, defs, 2)
		require.Equal(t, "pages", defs[0].TemplatesVariable())
		require.Equal(t, "fragments", defs[1].TemplatesVariable())
		for _, def := range defs {
			require.False(t, def.Signature().IsZero(), "%s is resolved", def.Name())
			segment, ok := segmentByName(def.Segments, "id")
			require.True(t, ok)
			require.Equal(t, "int", pathParameterType(segment))
		}
	})

	t.Run("without a receiver methods are inferred", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": server})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("templates", `{{define "GET /{id} Missing(id)"}}{{end}}`),
		}}
		defs, err := muxt.ResolveDefinitions(src, nil, fake.NewChecker().Fake())
		require.NoError(t, err)
		require.Len(t, defs, 1)
		require.Equal(t, []string{"Missing(id string) any"}, defs[0].SynthesizedMethods())
	})

	t.Run("every route calling an inferred method records it", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": `package server

type T struct{}

func (T) Wrap(any) any      { return nil }
func (T) Pair(any, any) any { return nil }
`})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("pages", `{{define "GET /a/{id} Missing(id)"}}{{end}}{{define "GET /b/{id} Missing(id)"}}{{end}}`),
			variable("fragments", `{{define "GET /c/{id} Wrap(Missing(id))"}}{{end}}{{define "GET /d/{id} Pair(Missing(id), Missing(id))"}}{{end}}`),
			variable("partials", `{{define "GET /e Wrap(ctx)"}}{{end}}`),
		}}
		defs, err := muxt.ResolveDefinitions(src, fake.Lookup(t, pkg, "T").(*types.Named), fake.NewChecker().Binds(muxt.TemplateNameScopeIdentifierContext, types.Universe.Lookup("any").Type()).Fake())
		require.NoError(t, err)
		require.Len(t, defs, 5)
		for _, def := range defs[:4] {
			require.Equal(t, []string{"Missing(id string) any"}, def.SynthesizedMethods(), "SynthesizedMethods() of %s", def.Name())
		}
		require.Empty(t, defs[4].SynthesizedMethods(), "SynthesizedMethods() of %s, which calls a defined method", defs[4].Name())
	})

	t.Run("a route without a call is left alone", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": server})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("templates", `{{define "GET /about"}}{{end}}`),
		}}
		defs, err := muxt.ResolveDefinitions(src, nil, fake.NewChecker().Fake())
		require.NoError(t, err)
		require.Len(t, defs, 1)
		require.True(t, defs[0].Signature().IsZero())
	})

	t.Run("a receiver field named like the call is not a method", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": `package server

type Store struct{}

type T struct{ Users Store }

type U struct{ Nested }

type Nested struct{ Users func() any }
`})
		for _, receiver := range []string{"T", "U"} {
			src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
				variable("templates", `{{define "GET /users Users()"}}{{end}}`),
			}}
			_, err := muxt.ResolveDefinitions(src, fake.Lookup(t, pkg, receiver).(*types.Named), fake.NewChecker().Fake())
			require.ErrorContains(t, err, "Users is a field of "+receiver+", not a method", "ResolveDefinitions() with receiver %s", receiver)
		}
	})

	t.Run("an alias parameter type parses like the type it names", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": `package server

type ID = int

type Tags = []ID

type Values map[string][]string

type Form struct{ Tags Tags }

type T struct{}

func (T) Show(id ID) any     { return nil }
func (T) Save(form Form) any { return nil }
`})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("templates", `{{define "GET /{id} Show(id)"}}{{end}}{{define "POST / Save(form)"}}{{end}}`),
		}}
		defs, err := muxt.ResolveDefinitions(src, fake.Lookup(t, pkg, "T").(*types.Named), fake.StandInChecker(t, pkg).Fake())
		require.NoError(t, err)
		require.Len(t, defs, 2)
		require.Equal(t, "POST /", defs[0].Pattern())
		fields := defs[0].Arguments[0].FormFields()
		require.Len(t, fields, 1)
		require.True(t, fields[0].Slice, "a Tags = []ID field binds every value")
		require.Equal(t, muxt.UnmarshalInt, fields[0].Method, "Method of a Tags = []ID field")
		require.Equal(t, "GET /{id}", defs[1].Pattern())
		require.Equal(t, muxt.UnmarshalInt, defs[1].Arguments[0].UnmarshalMethod(), "UnmarshalMethod() of an ID = int path value")
	})

	t.Run("a synthesized parameter is not named like another argument", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": `package server

type Context interface{ Done() }
`})
		for _, tt := range []struct {
			name string
			want string
		}{
			{name: "GET /{ctx2} Missing(ctx, ctx, ctx2)", want: "Missing(ctx Context, ctx3 Context, ctx2 string) any"},
			{name: "GET /{ctx2} Missing(ctx2, ctx, ctx)", want: "Missing(ctx2 string, ctx Context, ctx3 Context) any"},
			{name: "GET /{ctx2}/{ctx3} Missing(ctx, ctx3, ctx, ctx2)", want: "Missing(ctx Context, ctx3 string, ctx4 Context, ctx2 string) any"},
		} {
			src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
				variable("templates", `{{define "`+tt.name+`"}}{{end}}`),
			}}
			defs, err := muxt.ResolveDefinitions(src, nil, fake.StandInChecker(t, pkg).Fake())
			require.NoError(t, err, "ResolveDefinitions(%s)", tt.name)
			require.Len(t, defs, 1)
			require.Equal(t, []string{tt.want}, defs[0].SynthesizedMethods(), "SynthesizedMethods() of %s", tt.name)
		}
	})

	t.Run("an error about a rewritten body wrapper points at the wrapper", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": `package server

type Values map[string][]string

type T struct{}

func (T) Save(form int) any { return nil }
`})
		const name = "POST / Save(unmarshalForm(body))"
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("templates", `{{define "`+name+`"}}{{end}}`),
		}}
		_, err := muxt.ResolveDefinitions(src, fake.Lookup(t, pkg, "T").(*types.Named), fake.StandInChecker(t, pkg).Fake())
		nameErr, ok := errors.AsType[*muxt.NameError](err)
		require.True(t, ok, "ResolveDefinitions() = %v, want a *NameError", err)
		require.ErrorContains(t, err, "expected form parameter type to be a struct")
		require.Equal(t, strings.Index(name, "unmarshalForm"), nameErr.Offset, "NameError.Offset")
	})

	t.Run("a form field's validations come from its input or textarea", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": `package server

type Values map[string][]string

type Form struct {
	Color string ` + "`" + `template:"color"` + "`" + `
	Note  string ` + "`" + `template:"note"` + "`" + `
	Go    string ` + "`" + `template:"go"` + "`" + `
	Code  string ` + "`" + `template:"code"` + "`" + `
}

type T struct{}

func (T) Save(form Form) any { return nil }
`})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("templates", `{{define "POST / Save(form)"}}{{end}}`+
				`{{define "color"}}<select name="Color"><option>red</option></select>{{end}}`+
				`{{define "note"}}<textarea name="Note" minlength="2" maxlength="9"></textarea>{{end}}`+
				`{{define "go"}}<button name="Go" value="x">Go</button>{{end}}`+
				`{{define "code"}}<label for="Code">Code</label><select name="Code"></select><input name="Code" minlength="3">{{end}}`),
		}}
		defs, err := muxt.ResolveDefinitions(src, fake.Lookup(t, pkg, "T").(*types.Named), fake.StandInChecker(t, pkg).Fake())
		require.NoError(t, err)
		require.Len(t, defs, 1)
		fields := defs[0].Arguments[0].FormFields()
		require.Len(t, fields, 4)
		assert.Empty(t, fields[0].Validations, "validations of a select")
		assert.Equal(t, []muxt.InputValidation{
			muxt.MinLengthValidation{Name: "Note", MinLength: 2},
			muxt.MaxLengthValidation{Name: "Note", MaxLength: 9},
		}, fields[1].Validations, "validations of a textarea")
		assert.Empty(t, fields[2].Validations, "validations of a button")
		assert.Equal(t, []muxt.InputValidation{
			muxt.MinLengthValidation{Name: "Code", MinLength: 3},
		}, fields[3].Validations, "validations of the input named like a select")
	})

	t.Run("every variable's errors are reported", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": server})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("pages", `{{define "OPTIONS /a F()"}}{{end}}{{define "HEAD /b F()"}}{{end}}`),
			variable("fragments", `{{define "GET /c/{id} Form(id)"}}{{end}}`),
			variable("partials", `{{define "TRACE /d F()"}}{{end}}`),
		}}
		_, err := muxt.ResolveDefinitions(src, fake.Lookup(t, pkg, "T").(*types.Named), fake.NewChecker().Fake())
		list, ok := errors.AsType[muxt.ErrorList](err)
		require.True(t, ok, "ResolveDefinitions() = %v, want a muxt.ErrorList", err)
		var got []string
		for _, err := range list {
			got = append(got, err.Error())
		}
		require.Equal(t, []string{
			"pages: HEAD method not allowed; allowed methods: GET, POST, PUT, PATCH, and DELETE",
			"pages: OPTIONS method not allowed; allowed methods: GET, POST, PUT, PATCH, and DELETE",
			"fragments: unsupported type: In (supported: string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, or a type whose pointer implements encoding.TextUnmarshaler)",
			"partials: TRACE method not allowed; allowed methods: GET, POST, PUT, PATCH, and DELETE",
		}, got, "ResolveDefinitions() errors in variable order")
	})

	t.Run("resolution errors are combined", func(t *testing.T) {
		pkg := fake.Check(t, "example.com/server", map[string]string{"server.go": server})
		src := source.Package{Fset: fake.FileSet, Types: pkg, Variables: []source.Variable{
			variable("templates", `{{define "GET /a/{id} Form(id)"}}{{end}}{{define "GET /b/{name} Form(name)"}}{{end}}`),
		}}
		_, err := muxt.ResolveDefinitions(src, fake.Lookup(t, pkg, "T").(*types.Named), fake.NewChecker().Fake())
		require.ErrorContains(t, err, "unsupported type: In")
		require.ErrorContains(t, err, "(and 1 more error)")
	})
}
