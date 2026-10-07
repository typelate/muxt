package muxt

import (
	"go/ast"
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/source"
)

func TestCheckResponseWriterConflicts(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{name: "status code read from dot", body: `{{.StatusCode}}`, want: "StatusCode"},
		{name: "status code read from root", body: `{{$.StatusCode}}`, want: "StatusCode"},
		{name: "redirect", body: `{{.Redirect "/x" 302}}`, want: "Redirect"},
		{name: "a variable is not the root", body: `{{$x := .}}{{$x.StatusCode}}`},
		{name: "dot inside with is not template data", body: `{{with .Field}}{{.StatusCode}}{{end}}`},
		{name: "header is allowed", body: `{{.Header "X" "y"}}`},
		// Only the first name is TemplateData's; the rest select from
		// whatever it returned, here the method's result.
		{name: "a result field named StatusCode", body: `{{.Result.StatusCode}}`},
		{name: "a result field named StatusCode from root", body: `{{$.Result.StatusCode}}`},
		{name: "a result field named Redirect in a chain", body: `{{(.Result).Redirect}}`},
		{name: "a chain on parenthesised dot", body: `{{(.).StatusCode 201}}`, want: "StatusCode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := template.Must(template.New("root").Parse(`{{define "GET /a A(response)"}}` + tt.body + `{{end}}`))
			def := Definition{name: "GET /a A(response)", handler: "A(response)", hasResponseWriterArg: true}
			err := checkResponseWriterConflicts(ts, []Definition{def})
			if tt.want == "" {
				require.NoError(t, err, "checkResponseWriterConflicts()")
				return
			}
			conflict, ok := err.(*ResponseWriterTemplateStateError)
			require.True(t, ok, "checkResponseWriterConflicts() = %v, want a *ResponseWriterTemplateStateError", err)
			assert.Equal(t, tt.want, conflict.Method, "conflict method")
			assert.Equal(t, def.name, conflict.Template, "conflict template")
		})
	}
}

func TestCheckResponseWriterConflictsSkips(t *testing.T) {
	const body = `{{.StatusCode}}`
	ts := template.Must(template.New("root").Parse(`{{define "GET /a A(response)"}}` + body + `{{end}}`))
	ts.New("GET /b B(response)") // declared, never parsed: no tree
	for _, tt := range []struct {
		name string
		def  Definition
	}{
		{name: "the method did not take the response", def: Definition{name: "GET /a A(response)"}},
		{name: "not in the set", def: Definition{name: "GET /c C(response)", hasResponseWriterArg: true}},
		{name: "no tree", def: Definition{name: "GET /b B(response)", hasResponseWriterArg: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, checkResponseWriterConflicts(ts, []Definition{tt.def}), "checkResponseWriterConflicts()")
		})
	}
}

func TestCheckResponseWriterConflictsNamesTheFunction(t *testing.T) {
	ts := template.Must(template.New("root").Parse(`{{define "GET /a save(response)"}}{{.StatusCode}}{{end}}`))
	for _, tt := range []struct {
		name string
		fun  *ast.Ident
		want string
	}{
		{name: "the parsed function", fun: ast.NewIdent("save"), want: "save"},
		{name: "the handler text before it is parsed", want: "save(response)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			def := Definition{name: "GET /a save(response)", handler: "save(response)", fun: tt.fun, hasResponseWriterArg: true}
			conflict, ok := checkResponseWriterConflicts(ts, []Definition{def}).(*ResponseWriterTemplateStateError)
			require.True(t, ok, "checkResponseWriterConflicts() is not a *ResponseWriterTemplateStateError")
			assert.Equal(t, tt.want, conflict.Function, "Function")
		})
	}
}

func TestResponseWriterTemplateStateErrorMessage(t *testing.T) {
	const want = `template "GET /a A(response)" calls StatusCode but A takes the http.ResponseWriter, so muxt writes no status code or redirect for this route: either drop the response argument or call response.WriteHeader in the method`
	e := &ResponseWriterTemplateStateError{Template: "GET /a A(response)", Method: "StatusCode", Function: "A"}

	t.Run("without a location", func(t *testing.T) {
		assert.Equal(t, want, e.Error(), "Error()")
	})
	t.Run("with a location", func(t *testing.T) {
		located := *e
		located.Location = "a.gohtml:1:2"
		assert.Equal(t, "a.gohtml:1:2: "+want, located.Error(), "Error() with a location")
	})
	t.Run("for a redirect", func(t *testing.T) {
		redirect := *e
		redirect.Method = "Redirect"
		assert.Contains(t, redirect.Error(), "or call http.Redirect in the method", "Error() for a redirect")
	})
}

func TestDefinitionsAllowsReadingAResultStatusCode(t *testing.T) {
	ts := template.Must(template.New("root").Parse(`{{define "GET /a WithResponse(response)"}}{{.Result.StatusCode}}{{end}}`))
	defs, err := Definitions(source.Variable{Name: "templates", Set: ts})
	require.NoError(t, err, "Definitions() of a template reading its result's StatusCode")
	assert.Len(t, defs, 1, "Definitions() definitions")
}

func TestDefinitionsReportsResponseWriterConflicts(t *testing.T) {
	ts := template.Must(template.New("root").Parse(`{{define "GET /a A(response)"}}{{.StatusCode}}{{end}}`))
	defs, err := Definitions(source.Variable{Name: "templates", Set: ts})
	_, ok := err.(*ResponseWriterTemplateStateError)
	require.True(t, ok, "Definitions() error = %v, want a *ResponseWriterTemplateStateError", err)
	assert.Len(t, defs, 1, "Definitions() definitions returned with the error")
}
