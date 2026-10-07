package analysis

import (
	"go/token"
	"go/types"
	"html/template"
	"log"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

// parseTemplates parses text as one template set named "set". Text outside
// the define clauses is left empty, so the set's own root template is
// empty and never reported as unused.
func parseTemplates(t *testing.T, text string) *template.Template {
	t.Helper()
	ts, err := template.New("set").Parse(text)
	require.NoError(t, err)
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
			assert.Equal(t, tt.want, got, "findUnusedTemplates")
		})
	}
}

// TestPartitionUnusedTemplates states how the unused names are split for a
// reader: a route template's fix is running muxt generate, a plain
// template's is deleting it or rendering it, and a partial that a route
// template already renders is waiting on that route rather than unused.
func TestPartitionUnusedTemplates(t *testing.T) {
	const route = "GET / Home()"
	require.True(t, muxt.IsRouteDefinitionName(route), "the premise of this test is wrong: %q does not name a route", route)

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
			assert.Equal(t, tt.wantRoutes, routes, "routes")
			assert.Equal(t, tt.wantPartials, partials, "partials")
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
			assert.Equal(t, tt.wantErrors, got, "errors")
			if tt.wantSilent {
				assert.Empty(t, logs.String(), "log, want nothing")
			}
			for _, part := range tt.wantLogParts {
				assert.Contains(t, logs.String(), part, "log, want containing %q", part)
			}
		})
	}
}

func TestCollectTemplateReferencesFollowsEveryBranch(t *testing.T) {
	ts := parseTemplates(t, `{{define "root"}}{{range .}}{{template "in-range" .}}{{else}}{{template "in-range-else" .}}{{end}}{{with .}}{{template "in-with" .}}{{else}}{{template "in-with-else" .}}{{end}}{{template "loop"}}{{end}}{{define "loop"}}{{template "loop"}}{{template "leaf"}}{{end}}{{define "leaf"}}x{{end}}{{define "in-range"}}x{{end}}{{define "in-range-else"}}x{{end}}{{define "in-with"}}x{{end}}{{define "in-with-else"}}x{{end}}`)
	seen := make(map[string]bool)
	collectTemplateReferences(ts, ts.Lookup("root").Tree.Root, seen)
	for _, name := range []string{"in-range", "in-range-else", "in-with", "in-with-else", "loop", "leaf"} {
		assert.True(t, seen[name], "collectTemplateReferences did not reach %q; reached %v", name, seen)
	}
	assert.False(t, seen["root"], "collectTemplateReferences reached the root template itself")
}

// declaredTemplates has a template the set knows by name but that was never
// parsed, so it has no tree, and one that is referenced but not defined.
func declaredTemplates(t *testing.T) *template.Template {
	t.Helper()
	ts := parseTemplates(t, `{{define "GET / Home()"}}{{template "declared"}}{{template "undefined"}}{{end}}`)
	ts.New("declared")
	ts.New("GET /bare Bare()")
	declared := ts.Lookup("declared")
	require.NotNil(t, declared, "the premise of this test is wrong: declared should exist without a tree")
	require.Nil(t, declared.Tree, "the premise of this test is wrong: declared should exist without a tree")
	return ts
}

func TestTemplatesWithoutATreeAreSkipped(t *testing.T) {
	ts := declaredTemplates(t)

	t.Run("collectTemplateReferences", func(t *testing.T) {
		seen := make(map[string]bool)
		collectTemplateReferences(ts, ts.Lookup("GET / Home()").Tree.Root, seen)
		assert.True(t, seen["declared"], "collectTemplateReferences reached %v, want declared", seen)
		assert.True(t, seen["undefined"], "collectTemplateReferences reached %v, want undefined", seen)
	})
	t.Run("partitionUnusedTemplates", func(t *testing.T) {
		routes, partials := partitionUnusedTemplates(ts, []string{"GET / Home()"})
		assert.Len(t, routes, 1, "partitionUnusedTemplates() routes, want the route only")
		assert.Empty(t, partials, "partitionUnusedTemplates() partials, want the route only")
	})
	t.Run("findUnusedTemplates", func(t *testing.T) {
		got := findUnusedTemplates(ts, executed())
		assert.Equal(t, []string{"GET / Home()"}, got, "findUnusedTemplates() want only the route")
	})
	t.Run("executeTemplateTree", func(t *testing.T) {
		// A nil global is never touched when there is no tree to walk.
		executeTemplateTree(nil, ts, "declared", nil)
		executeTemplateTree(nil, ts, "undefined", nil)
	})
}

func TestReportDefinitionErrorsIsSilentWithoutErrors(t *testing.T) {
	var logs strings.Builder
	err := reportDefinitionErrors(log.New(&logs, "", 0), source.Variable{Set: parseTemplates(t, `{{define "footer"}}x{{end}}`)})
	assert.NoError(t, err, "reportDefinitionErrors()")
	assert.Empty(t, logs.String(), "reportDefinitionErrors() log")
}

// untypedCallPackage holds one ExecuteTemplate call with no data type, as
// load reports a call in code the type checker skipped: a package that
// does not compile, such as one beside a stale generated file.
func untypedCallPackage(t *testing.T) source.Package {
	t.Helper()
	return source.Package{
		Fset:  token.NewFileSet(),
		Types: types.NewPackage("example.com/server", "server"),
		Variables: []source.Variable{{
			Name:  "templates",
			Set:   parseTemplates(t, `{{define "page"}}<p>{{.Title}}</p>{{template "row" .}}{{end}}{{define "row"}}{{.}}{{end}}`),
			Calls: []source.Call{{Position: token.Position{Filename: "routes.go", Line: 3, Column: 9}, Template: "page"}},
		}},
	}
}

func TestCheckReportsACallWithNoDataType(t *testing.T) {
	var logs strings.Builder
	n, err := Check(CheckConfiguration{}, log.New(&logs, "", 0), untypedCallPackage(t))
	require.EqualError(t, err, "1 error", "Check()")
	assert.Equal(t, 1, n, "Check() checked")
	assert.Equal(t, "routes.go:3:9 ExecuteTemplate \"page\"\n -  the data argument has no type because the package does not type check; run go build (a stale generated file can cause this)\n", logs.String(), "Check() logged")
}

func TestListingsSkipACallWithNoDataType(t *testing.T) {
	pkg := untypedCallPackage(t)
	callers, err := NewTemplateCallers(TemplateCallersConfiguration{}, pkg)
	require.NoError(t, err, "NewTemplateCallers()")
	require.NotNil(t, callers)
	_, err = NewTemplateCalls(TemplateCallsConfiguration{}, pkg)
	require.NoError(t, err, "NewTemplateCalls()")
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
			assert.Equal(t, tt.want, isEmptyTemplate(ts.Lookup("t").Tree.Root), "isEmptyTemplate(%q)", tt.template)
		})
	}
}
