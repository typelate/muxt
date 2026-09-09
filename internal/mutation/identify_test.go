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

// matchingEnd returns the span of the {{end}} closing the clause that
// opens at start.
//
// It counts depth over the actions it passes rather than taking the first
// {{end}} it finds. Taking the first one is wrong the moment a definition
// holds an if, a range or a with, and it is wrong quietly: the definition
// hashes a truncated span and the template around it hashes the leftover,
// so an edit inside the definition is attributed to its neighbour.
//
// This is written out here rather than borrowed from the package so that
// a fault in the package's own matching cannot make a fixture agree with
// it.
func matchingEnd(t *testing.T, text string, start int) (int, int) {
	t.Helper()

	depth := 0
	for i := start; i < len(text); {
		open := strings.Index(text[i:], "{{")
		if open < 0 {
			break
		}
		open += i
		closeAt := strings.Index(text[open:], "}}")
		if closeAt < 0 {
			break
		}
		closeAt += open + 2

		word := strings.TrimLeft(text[open+2:closeAt-2], "- ")
		switch {
		case strings.HasPrefix(word, "if"), strings.HasPrefix(word, "range"),
			strings.HasPrefix(word, "with"), strings.HasPrefix(word, "define"),
			strings.HasPrefix(word, "block"):
			depth++
		case strings.HasPrefix(word, "end"):
			depth--
			if depth == 0 {
				return open, closeAt - open
			}
		}
		i = closeAt
	}
	t.Fatalf("no matching end for the clause at offset %d", start)
	return 0, 0
}

// identifyFixture writes text to a file and derives the definitions a
// template loader would report for it, so a test can state what changed
// in template source rather than in span arithmetic.
func identifyFixture(t *testing.T, text string) ([]check.Definition, map[string]*parse.Tree) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "templates.gohtml")
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
		Tree:   trees["templates.gohtml"],
	}}

	for name, tree := range trees {
		if name == "templates.gohtml" {
			continue
		}
		clause := `{{define "` + name + `"}}`
		open := strings.Index(text, clause)
		if open < 0 {
			t.Fatalf("no define clause for %q", name)
		}
		endAt, endLen := matchingEnd(t, text, open)

		defs = append(defs, check.Definition{
			Name:         name,
			Define:       span(open, len(clause)),
			End:          span(endAt, endLen),
			TemplateName: span(open+len(`{{define `), len(name)+2),
			Tree:         tree,
		})
	}
	return defs, trees
}

func sumsOf(t *testing.T, text string, names ...string) map[string]string {
	t.Helper()
	defs, trees := identifyFixture(t, text)

	ids := NewIdentifiers(defs, nil, 1, "test")
	sums := make(map[string]string, len(names))
	for _, name := range names {
		tree, ok := trees[name]
		if !ok {
			t.Fatalf("no tree named %q", name)
		}
		id, err := ids.Identify(tree, nil)
		if err != nil {
			t.Fatalf("Identify(%q) = %v", name, err)
		}
		sums[name] = id.Sum
	}
	return sums
}

func TestIdentifySeparatesTemplates(t *testing.T) {
	// "a" holds a nested if, so a fixture that matched the first {{end}}
	// rather than the matching one would mis-attribute every edit below.
	const before = `outer {{.Title}}
{{define "a"}}A{{if .On}}{{.Name}}{{end}}!{{end}}
{{define "b"}}B{{.Name}}{{end}}
`
	names := []string{"a", "b", "templates.gohtml"}

	for _, tt := range []struct {
		name    string
		after   string
		changed []string
		same    []string
	}{
		{
			name: "an edit inside a definition changes only that definition",
			after: `outer {{.Title}}
{{define "a"}}A{{if .On}}{{.Name}}{{end}}CHANGED{{end}}
{{define "b"}}B{{.Name}}{{end}}
`,
			changed: []string{"a"},
			same:    []string{"b", "templates.gohtml"},
		},
		{
			name: "an edit to the text around them changes only the template that carries it",
			after: `OUTER {{.Title}}
{{define "a"}}A{{if .On}}{{.Name}}{{end}}!{{end}}
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
{{define "a"}}A{{if .On}}{{.Name}}{{end}}!{{end -}}
{{define "b"}}B{{.Name}}{{end}}
`,
			changed: []string{"a", "templates.gohtml"},
			same:    []string{"b"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
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

func TestIdentifyRestsOnSeedAndEngine(t *testing.T) {
	const text = `{{define "a"}}{{.Name}}{{end}}`
	defs, trees := identifyFixture(t, text)

	sum := func(seed uint64, engine string) string {
		t.Helper()
		id, err := NewIdentifiers(defs, nil, seed, engine).Identify(trees["a"], nil)
		if err != nil {
			t.Fatal(err)
		}
		return id.Actions[0].Sum
	}

	base := sum(1, "v1")
	if sum(2, "v1") == base {
		t.Error("a different seed reached the same identifier, so verdicts would carry across a change of substituted values")
	}
	if sum(1, "v2") == base {
		t.Error("a different muxt version reached the same identifier, so verdicts would carry across a change to the engine")
	}
}

func TestIdentifyReachesPartials(t *testing.T) {
	const text = `{{define "page"}}{{template "part" .}}{{template "part" .}}{{end}}
{{define "part"}}{{.Name}}{{end}}
`
	defs, trees := identifyFixture(t, text)

	id, err := NewIdentifiers(defs, nil, 1, "test").Identify(trees["page"], nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(id.Templates) != 1 {
		t.Fatalf("reached templates = %d, want 1: a partial invoked twice with the same dot is one execution", len(id.Templates))
	}
	if id.Templates[0].Name != "part" {
		t.Errorf("reached %q, want %q", id.Templates[0].Name, "part")
	}
	if len(id.Templates[0].Actions) != 1 {
		t.Errorf("actions of the reached partial = %d, want 1", len(id.Templates[0].Actions))
	}
}

func TestIdentifyTerminatesOnSelfReference(t *testing.T) {
	const text = `{{define "loop"}}{{template "loop" .}}{{end}}`
	defs, trees := identifyFixture(t, text)

	done := make(chan struct{})
	go func() {
		defer close(done)
		id, err := NewIdentifiers(defs, nil, 1, "test").Identify(trees["loop"], nil)
		if err != nil {
			t.Errorf("Identify = %v", err)
			return
		}
		if len(id.Templates) != 1 || id.Templates[0].Name != "loop" {
			t.Errorf("a self-reference should be named once, got %d entries", len(id.Templates))
		}
		if id.Templates[0].Sum != "" {
			t.Error("the stub for a template still being identified should carry no sum")
		}
	}()
	<-done
}

func TestIdentifyRefusesGoStringLiterals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "template.go")
	if err := os.WriteFile(path, []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	trees, err := parse.Parse("t", `{{define "a"}}{{.Name}}{{end}}`, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defs := []check.Definition{{
		Name:         "a",
		Define:       check.Span{Position: token.Position{Filename: path, Offset: 0, Line: 1, Column: 1}, Length: 14},
		End:          check.Span{Position: token.Position{Filename: path, Offset: 22, Line: 1, Column: 23}, Length: 7},
		TemplateName: check.Span{Position: token.Position{Filename: path, Offset: 9, Line: 1, Column: 10}, Length: 3},
	}}

	// A tree positioned against a decoded literal cannot be matched
	// against Go source, so the answer must be an error rather than an
	// identifier with no actions in it.
	if _, err := NewIdentifiers(defs, nil, 1, "test").Identify(trees["a"], nil); err == nil {
		t.Error("Identify accepted a template written in a .go file, want an error")
	}
}

func TestIdentifyTellsRepeatedActionsApart(t *testing.T) {
	const text = `{{define "a"}}<x>{{.Name}}</x><y>{{.Name}}</y>{{end}}`
	defs, trees := identifyFixture(t, text)

	id, err := NewIdentifiers(defs, nil, 1, "test").Identify(trees["a"], nil)
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
