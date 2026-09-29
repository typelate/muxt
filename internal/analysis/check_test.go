package analysis

import (
	"html/template"
	"log"
	"slices"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
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

func TestReportUnusedTemplates(t *testing.T) {
	const templates = `{{define "GET / Home()"}}<p>hi</p>{{end}}{{define "footer"}}<p>bye</p>{{end}}`
	for _, tt := range []struct {
		name         string
		executed     map[string][]TemplateExecution
		wantErrors   []string
		wantLogParts []string
		wantSilent   bool
	}{
		{name: "everything executed", executed: executed("GET / Home()", "footer"), wantSilent: true},
		{
			name:     "an unwired route and an unused partial",
			executed: executed(),
			wantErrors: []string{
				"1 route templates are not wired to generated handlers",
				"unused templates 1",
			},
			wantLogParts: []string{
				"Route templates with no generated handler; run muxt generate to wire them up:\n",
				`: "GET / Home()"`,
				"Unused templates:\n",
				`: "footer"`,
			},
		},
		{
			name:         "only a partial unused",
			executed:     executed("GET / Home()"),
			wantErrors:   []string{"unused templates 1"},
			wantLogParts: []string{"Unused templates:\n", `: "footer"`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs strings.Builder
			errs := reportUnusedTemplates(log.New(&logs, "", 0), parseTemplates(t, templates), tt.executed)
			var got []string
			for _, err := range errs {
				got = append(got, err.Error())
			}
			if !slices.Equal(got, tt.wantErrors) {
				t.Errorf("errors = %q, want %q", got, tt.wantErrors)
			}
			if tt.wantSilent && logs.Len() != 0 {
				t.Errorf("log = %q, want nothing", logs.String())
			}
			for _, part := range tt.wantLogParts {
				if !strings.Contains(logs.String(), part) {
					t.Errorf("log = %q, want containing %q", logs.String(), part)
				}
			}
		})
	}
}

func TestCollectTemplateReferencesFollowsEveryBranch(t *testing.T) {
	ts := parseTemplates(t, `{{define "root"}}{{range .}}{{template "in-range" .}}{{else}}{{template "in-range-else" .}}{{end}}{{with .}}{{template "in-with" .}}{{else}}{{template "in-with-else" .}}{{end}}{{template "loop"}}{{end}}{{define "loop"}}{{template "loop"}}{{template "leaf"}}{{end}}{{define "leaf"}}x{{end}}{{define "in-range"}}x{{end}}{{define "in-range-else"}}x{{end}}{{define "in-with"}}x{{end}}{{define "in-with-else"}}x{{end}}`)
	seen := make(map[string]bool)
	collectTemplateReferences(ts, ts.Lookup("root").Tree.Root, seen)
	for _, name := range []string{"in-range", "in-range-else", "in-with", "in-with-else", "loop", "leaf"} {
		if !seen[name] {
			t.Errorf("collectTemplateReferences did not reach %q; reached %v", name, seen)
		}
	}
	if seen["root"] {
		t.Errorf("collectTemplateReferences reached the root template itself")
	}
}

// declaredTemplates has a template the set knows by name but that was never
// parsed, so it has no tree, and one that is referenced but not defined.
func declaredTemplates(t *testing.T) *template.Template {
	t.Helper()
	ts := parseTemplates(t, `{{define "GET / Home()"}}{{template "declared"}}{{template "undefined"}}{{end}}`)
	ts.New("declared")
	ts.New("GET /bare Bare()")
	if ts.Lookup("declared") == nil || ts.Lookup("declared").Tree != nil {
		t.Fatal("the premise of this test is wrong: declared should exist without a tree")
	}
	return ts
}

func TestTemplatesWithoutATreeAreSkipped(t *testing.T) {
	ts := declaredTemplates(t)

	t.Run("collectTemplateReferences", func(t *testing.T) {
		seen := make(map[string]bool)
		collectTemplateReferences(ts, ts.Lookup("GET / Home()").Tree.Root, seen)
		if !seen["declared"] || !seen["undefined"] {
			t.Errorf("collectTemplateReferences reached %v, want declared and undefined", seen)
		}
	})
	t.Run("partitionUnusedTemplates", func(t *testing.T) {
		routes, partials := partitionUnusedTemplates(ts, []string{"GET / Home()"})
		if len(routes) != 1 || len(partials) != 0 {
			t.Errorf("partitionUnusedTemplates() = %q, %q, want the route only", routes, partials)
		}
	})
	t.Run("findUnusedTemplates", func(t *testing.T) {
		got := findUnusedTemplates(ts, executed())
		if !slices.Equal(got, []string{"GET / Home()"}) {
			t.Errorf("findUnusedTemplates() = %q, want only the route", got)
		}
	})
	t.Run("executeTemplateTree", func(t *testing.T) {
		// A nil global is never touched when there is no tree to walk.
		executeTemplateTree(nil, ts, "declared", nil)
		executeTemplateTree(nil, ts, "undefined", nil)
	})
}

func TestReportDefinitionErrorsIsSilentWithoutErrors(t *testing.T) {
	var logs strings.Builder
	if err := reportDefinitionErrors(log.New(&logs, "", 0), source.Variable{Set: parseTemplates(t, `{{define "footer"}}x{{end}}`)}); err != nil || logs.Len() != 0 {
		t.Errorf("reportDefinitionErrors() = %v with log %q, want neither", err, logs.String())
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
