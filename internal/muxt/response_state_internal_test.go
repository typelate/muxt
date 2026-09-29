package muxt

import (
	"go/ast"
	"html/template"
	"strings"
	"testing"

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
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := template.Must(template.New("root").Parse(`{{define "GET /a A(response)"}}` + tt.body + `{{end}}`))
			def := Definition{name: "GET /a A(response)", handler: "A(response)", hasResponseWriterArg: true}
			err := checkResponseWriterConflicts(ts, []Definition{def})
			if tt.want == "" {
				if err != nil {
					t.Fatalf("checkResponseWriterConflicts() = %v, want nil", err)
				}
				return
			}
			conflict, ok := err.(*ResponseWriterTemplateStateError)
			if !ok {
				t.Fatalf("checkResponseWriterConflicts() = %v, want a *ResponseWriterTemplateStateError", err)
			}
			if conflict.Method != tt.want || conflict.Template != def.name {
				t.Errorf("checkResponseWriterConflicts() = %+v, want method %s of template %s", conflict, tt.want, def.name)
			}
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
			if err := checkResponseWriterConflicts(ts, []Definition{tt.def}); err != nil {
				t.Errorf("checkResponseWriterConflicts() = %v, want nil", err)
			}
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
			if !ok {
				t.Fatal("checkResponseWriterConflicts() is not a *ResponseWriterTemplateStateError")
			}
			if conflict.Function != tt.want {
				t.Errorf("Function = %q, want %q", conflict.Function, tt.want)
			}
		})
	}
}

func TestResponseWriterTemplateStateErrorMessage(t *testing.T) {
	e := &ResponseWriterTemplateStateError{Template: "GET /a A(response)", Method: "StatusCode", Function: "A"}
	const want = `template "GET /a A(response)" calls StatusCode but A takes the http.ResponseWriter, so muxt writes no status code or redirect for this route: either drop the response argument or call response.WriteHeader in the method`
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	e.Location = "a.gohtml:1:2"
	if got := e.Error(); got != "a.gohtml:1:2: "+want {
		t.Errorf("Error() with a location = %q, want it prefixed with the location", got)
	}
	e.Method = "Redirect"
	if got := e.Error(); !strings.Contains(got, "or call http.Redirect in the method") {
		t.Errorf("Error() for a redirect = %q, want the http.Redirect remedy", got)
	}
}

func TestDefinitionsReportsResponseWriterConflicts(t *testing.T) {
	ts := template.Must(template.New("root").Parse(`{{define "GET /a A(response)"}}{{.StatusCode}}{{end}}`))
	defs, err := Definitions(source.Variable{Name: "templates", Set: ts})
	if _, ok := err.(*ResponseWriterTemplateStateError); !ok {
		t.Fatalf("Definitions() error = %v, want a *ResponseWriterTemplateStateError", err)
	}
	if len(defs) != 1 {
		t.Errorf("Definitions() returned %d definitions with the error, want 1", len(defs))
	}
}
