package mutation

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template/parse"

	"github.com/typelate/check"
)

// identifyFixture writes text to a file and derives the definitions a
// template loader would report for it, so a test can state what changed
// in template source rather than in span arithmetic.
func identifyFixture(t *testing.T, text string) ([]check.Definition, *parse.Tree, map[string]*parse.Tree) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "templates.gohtml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	trees, err := parse.Parse("templates.gohtml", text, "", "")
	if err != nil {
		t.Fatal(err)
	}

	span := func(offset, length int) check.Span {
		return check.Span{
			Position: token.Position{Filename: path, Offset: offset, Line: 1, Column: offset + 1},
			Length:   length,
		}
	}

	// The root spans the whole text and has no define clause, which is
	// what tells it from the templates the clauses declare.
	defs := []check.Definition{{
		Name:   "templates.gohtml",
		Define: span(0, 0),
		End:    span(len(text), 0),
	}}

	for name := range trees {
		if name == "templates.gohtml" {
			continue
		}
		open := strings.Index(text, `{{define "`+name+`"}}`)
		if open < 0 {
			t.Fatalf("no define clause for %q", name)
		}
		openLen := len(`{{define "` + name + `"}}`)
		closeAt := strings.Index(text[open:], "{{end")
		if closeAt < 0 {
			t.Fatalf("no end clause for %q", name)
		}
		closeAt += open
		closeLen := strings.Index(text[closeAt:], "}}") + 2

		defs = append(defs, check.Definition{
			Name:         name,
			Define:       span(open, openLen),
			End:          span(closeAt, closeLen),
			TemplateName: span(open+len(`{{define `), len(name)+2),
		})
	}
	return defs, trees["templates.gohtml"], trees
}

func sumsOf(t *testing.T, text string, names ...string) map[string]string {
	t.Helper()
	defs, _, trees := identifyFixture(t, text)

	sums := make(map[string]string, len(names))
	for _, name := range names {
		tree, ok := trees[name]
		if !ok {
			t.Fatalf("no tree named %q", name)
		}
		id, err := Identify(defs, tree, nil, nil, nil)
		if err != nil {
			t.Fatalf("Identify(%q) = %v", name, err)
		}
		sums[name] = id.Sum
	}
	return sums
}

func TestIdentifySeparatesTemplates(t *testing.T) {
	const before = `outer {{.Title}}
{{define "a"}}A{{.Name}}{{end}}
{{define "b"}}B{{.Name}}{{end}}
`
	for _, tt := range []struct {
		name    string
		after   string
		changed []string
		same    []string
	}{
		{
			name: "editing one definition leaves the others alone",
			after: `outer {{.Title}}
{{define "a"}}CHANGED{{.Name}}{{end}}
{{define "b"}}B{{.Name}}{{end}}
`,
			changed: []string{"a"},
			same:    []string{"b", "templates.gohtml"},
		},
		{
			name: "editing the text around them changes only the template that carries it",
			after: `OUTER {{.Title}}
{{define "a"}}A{{.Name}}{{end}}
{{define "b"}}B{{.Name}}{{end}}
`,
			changed: []string{"templates.gohtml"},
			same:    []string{"a", "b"},
		},
		{
			// The marker is written inside the definition but trims the
			// newline after it, which belongs to the template around it.
			name: "a trim marker on an end clause changes the template around it too",
			after: `outer {{.Title}}
{{define "a"}}A{{.Name}}{{end -}}
{{define "b"}}B{{.Name}}{{end}}
`,
			changed: []string{"a", "templates.gohtml"},
			same:    []string{"b"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			names := []string{"a", "b", "templates.gohtml"}
			was := sumsOf(t, before, names...)
			now := sumsOf(t, tt.after, names...)

			for _, name := range tt.changed {
				if was[name] == now[name] {
					t.Errorf("identity of %q did not change, want a different sum", name)
				}
			}
			for _, name := range tt.same {
				if was[name] != now[name] {
					t.Errorf("identity of %q changed, want it to hold", name)
				}
			}
		})
	}
}

func TestIdentifyReachesEveryAction(t *testing.T) {
	const text = `{{define "a"}}{{.One}}{{if .Two}}{{.Three}}{{end}}{{end}}`
	defs, _, trees := identifyFixture(t, text)

	id, err := Identify(defs, trees["a"], nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, action := range id.Actions {
		got = append(got, action.Name)
	}
	want := []string{"{{.One}}", "{{if .Two}}", "{{.Three}}"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("actions = %v, want %v", got, want)
	}
}

func TestIdentifyTellsRepeatedActionsApart(t *testing.T) {
	const text = `{{define "a"}}<x>{{.Name}}</x><y>{{.Name}}</y>{{end}}`
	defs, _, trees := identifyFixture(t, text)

	id, err := Identify(defs, trees["a"], nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(id.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(id.Actions))
	}
	if id.Actions[0].Name != id.Actions[1].Name {
		t.Fatalf("fixture should write the same action twice, got %q and %q",
			id.Actions[0].Name, id.Actions[1].Name)
	}
	if id.Actions[0].Sum == id.Actions[1].Sum {
		t.Error("two identically written actions share an identifier, so the second would take the first's verdict")
	}
}
