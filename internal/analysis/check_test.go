package analysis

import (
	"html/template"
	"slices"
	"testing"

	"github.com/typelate/muxt/internal/muxt"
)

// parseTemplates parses text as one template set named "set". Text outside
// the define clauses is left empty, so the set's own root template is
// empty and never reported as unused.
func parseTemplates(t *testing.T, text string) *template.Template {
	t.Helper()
	ts, err := template.New("set").Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

// executed names the templates an ExecuteTemplate call renders. Only the
// names are read, so the call sites behind them can be empty.
func executed(names ...string) map[string][]TemplateExecution {
	calls := make(map[string][]TemplateExecution, len(names))
	for _, name := range names {
		calls[name] = nil
	}
	return calls
}

// TestFindUnusedTemplates states which templates are reported as defined
// but never rendered: the ones no ExecuteTemplate call names, except those
// holding nothing to render.
func TestFindUnusedTemplates(t *testing.T) {
	for _, tt := range []struct {
		name      string
		templates string
		executed  map[string][]TemplateExecution
		want      []string
	}{
		{
			name:      "a template nothing executes",
			templates: `{{define "page"}}<p>{{.}}</p>{{end}}`,
			want:      []string{"page"},
		},
		{
			name:      "a template an ExecuteTemplate call names",
			templates: `{{define "page"}}<p>{{.}}</p>{{end}}`,
			executed:  executed("page"),
		},
		{
			name:      "reported in name order",
			templates: `{{define "row"}}<td>{{.}}</td>{{end}}{{define "page"}}<p>{{.}}</p>{{end}}`,
			want:      []string{"page", "row"},
		},
		{
			// Nothing renders from it, so naming it would send a reader to
			// a template with nothing in it to fix.
			name:      "a template holding only space",
			templates: `{{define "blank"}}   {{end}}{{define "page"}}<p>{{.}}</p>{{end}}`,
			want:      []string{"page"},
		},
		{
			name:      "a template holding only a comment",
			templates: `{{define "noted"}}{{/* later */}}{{end}}{{define "page"}}<p>{{.}}</p>{{end}}`,
			want:      []string{"page"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := findUnusedTemplates(parseTemplates(t, tt.templates), tt.executed)
			if !slices.Equal(got, tt.want) {
				t.Errorf("findUnusedTemplates = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPartitionUnusedTemplates states how the unused names are split for a
// reader: a route template's fix is running muxt generate, a plain
// template's is deleting it or rendering it, and a partial that a route
// template already renders is waiting on that route rather than unused.
func TestPartitionUnusedTemplates(t *testing.T) {
	const route = "GET / Home()"
	if !muxt.IsRouteDefinitionName(route) {
		t.Fatalf("the premise of this test is wrong: %q does not name a route", route)
	}

	for _, tt := range []struct {
		name         string
		templates    string
		unused       []string
		wantRoutes   []string
		wantPartials []string
	}{
		{
			name:         "a route template and a plain one",
			templates:    `{{define "GET / Home()"}}<p>hi</p>{{end}}{{define "footer"}}<p>bye</p>{{end}}`,
			unused:       []string{route, "footer"},
			wantRoutes:   []string{route},
			wantPartials: []string{"footer"},
		},
		{
			// "row" is not unused, it is waiting on the route that renders
			// it, and that route is already reported.
			name:         "a partial the route template renders",
			templates:    `{{define "GET / Home()"}}{{template "row" .}}{{end}}{{define "row"}}<td>{{.}}</td>{{end}}{{define "footer"}}<p>bye</p>{{end}}`,
			unused:       []string{route, "row", "footer"},
			wantRoutes:   []string{route},
			wantPartials: []string{"footer"},
		},
		{
			name:       "a partial reached through another partial",
			templates:  `{{define "GET / Home()"}}{{template "row" .}}{{end}}{{define "row"}}{{template "cell" .}}{{end}}{{define "cell"}}<td>{{.}}</td>{{end}}`,
			unused:     []string{route, "row", "cell"},
			wantRoutes: []string{route},
		},
		{
			name:         "a partial only inside a branch of a route template",
			templates:    `{{define "GET / Home()"}}{{if .}}{{template "row" .}}{{else}}{{template "empty" .}}{{end}}{{end}}{{define "row"}}<td>x</td>{{end}}{{define "empty"}}<td></td>{{end}}`,
			unused:       []string{route, "row", "empty"},
			wantRoutes:   []string{route},
			wantPartials: nil,
		},
		{
			name:         "no route templates at all",
			templates:    `{{define "footer"}}<p>bye</p>{{end}}`,
			unused:       []string{"footer"},
			wantPartials: []string{"footer"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			routes, partials := partitionUnusedTemplates(parseTemplates(t, tt.templates), tt.unused)
			if !slices.Equal(routes, tt.wantRoutes) {
				t.Errorf("routes = %q, want %q", routes, tt.wantRoutes)
			}
			if !slices.Equal(partials, tt.wantPartials) {
				t.Errorf("partials = %q, want %q", partials, tt.wantPartials)
			}
		})
	}
}

// TestIsEmptyTemplate states what counts as a template with nothing to
// render, which is what keeps such a template out of the unused report.
func TestIsEmptyTemplate(t *testing.T) {
	for _, tt := range []struct {
		name     string
		template string
		want     bool
	}{
		{name: "nothing at all", template: ``, want: true},
		{name: "only space", template: "  \n\t", want: true},
		{name: "only a comment", template: `{{/* later */}}`, want: true},
		{name: "text", template: `hello`},
		{name: "an action", template: `{{.}}`},
		{name: "a branch holding nothing", template: `{{if .}}{{end}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := parseTemplates(t, `{{define "t"}}`+tt.template+`{{end}}`)
			if got := isEmptyTemplate(ts.Lookup("t").Tree.Root); got != tt.want {
				t.Errorf("isEmptyTemplate(%q) = %t, want %t", tt.template, got, tt.want)
			}
		})
	}
}
