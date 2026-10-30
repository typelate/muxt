package check

import (
	"go/token"
	"strings"
	"testing"
	"text/template"

	muxttemplate "github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// defineTree parses {{define "a"}}body{{end}} and returns the tree for
// "a", so a test can build definitions whose bodies differ.
func defineTree(t *testing.T, body string) *muxttemplate.Tree {
	t.Helper()
	root, err := muxttemplate.ParseTree("root", "root", `{{define "a"}}`+body+`{{end}}`, nil, lex.Options{ParseHTML: true})
	if err != nil {
		t.Fatalf("parsing %q: %v", body, err)
	}
	for name, tree := range muxttemplate.Extract(root) {
		if name == "a" {
			return tree
		}
	}
	t.Fatalf("parsing %q: no tree for %q", body, "a")
	return nil
}

// definitionOnLine builds a definition for "a" whose line records which
// of several definitions of the same name it is.
func definitionOnLine(line int, tree *muxttemplate.Tree) Definition {
	return Definition{
		Name:   "a",
		Tree:   tree,
		Define: Span{Position: token.Position{Filename: "t.gohtml", Line: line, Column: 1}, Length: 14},
	}
}

func TestDefinitionSet(t *testing.T) {
	t.Run("a later definition replaces an earlier one", func(t *testing.T) {
		set := newDefinitionSet([]Definition{
			definitionOnLine(1, defineTree(t, "first")),
			definitionOnLine(2, defineTree(t, "second")),
		})

		got, ok := set.FindDefinition("a")
		if !ok {
			t.Fatalf("FindDefinition(%q) not found", "a")
		}
		if got.Define.Line != 2 {
			t.Errorf("FindDefinition(%q).Define.Line = %d, want %d", "a", got.Define.Line, 2)
		}
	})

	t.Run("an empty definition does not replace a body", func(t *testing.T) {
		// text/template refuses to let an empty definition displace one
		// that already has a body. Confirm that is really what it does,
		// then confirm the set agrees.
		ts := template.Must(template.New("root").Parse(`{{define "a"}}first{{end}}`))
		template.Must(ts.Parse(`{{define "a"}}{{end}}`))
		var body strings.Builder
		if err := ts.ExecuteTemplate(&body, "a", nil); err != nil {
			t.Fatalf("executing a: %v", err)
		}
		if body.String() != "first" {
			t.Fatalf("text/template resolved %q to %q, want %q", "a", body.String(), "first")
		}

		set := newDefinitionSet([]Definition{
			definitionOnLine(1, defineTree(t, "first")),
			definitionOnLine(2, defineTree(t, "")),
		})

		got, _ := set.FindDefinition("a")
		if got.Define.Line != 1 {
			t.Errorf("FindDefinition(%q).Define.Line = %d, want %d", "a", got.Define.Line, 1)
		}
	})

	t.Run("a body replaces an empty definition", func(t *testing.T) {
		set := newDefinitionSet([]Definition{
			definitionOnLine(1, defineTree(t, "")),
			definitionOnLine(2, defineTree(t, "second")),
		})

		got, _ := set.FindDefinition("a")
		if got.Define.Line != 2 {
			t.Errorf("FindDefinition(%q).Define.Line = %d, want %d", "a", got.Define.Line, 2)
		}
	})

	t.Run("the surviving definition carries the tree the checker walks", func(t *testing.T) {
		// Definitions are built from a reparse, which is only good for
		// deciding which of them survives: those trees are not the ones
		// the checker walks and carry a different ParseName. Once the
		// winner is known it has to hold the executed tree, or a caller
		// following Definition.Tree reports a different file than
		// Definition.Define does.
		reparsed := defineTree(t, "first")
		executed := defineTree(t, "first")
		executed.ParseName = "/abs/path/a.gohtml"

		set := newDefinitionSet([]Definition{definitionOnLine(1, reparsed)})
		set.adoptTrees(FindTreeFunc(func(name string) (*muxttemplate.Tree, bool) {
			if name != "a" {
				return nil, false
			}
			return executed, true
		}))

		got, _ := set.FindDefinition("a")
		if got.Tree != executed {
			t.Errorf("FindDefinition(%q).Tree is the reparsed tree, want the executed one", "a")
		}
		if got.Tree.ParseName != "/abs/path/a.gohtml" {
			t.Errorf("FindDefinition(%q).Tree.ParseName = %q, want %q",
				"a", got.Tree.ParseName, "/abs/path/a.gohtml")
		}
	})

	t.Run("a definition the template set does not know keeps its own tree", func(t *testing.T) {
		reparsed := defineTree(t, "first")

		set := newDefinitionSet([]Definition{definitionOnLine(1, reparsed)})
		set.adoptTrees(FindTreeFunc(func(string) (*muxttemplate.Tree, bool) { return nil, false }))

		got, _ := set.FindDefinition("a")
		if got.Tree != reparsed {
			t.Errorf("FindDefinition(%q).Tree was replaced with nil, want the reparsed tree kept", "a")
		}
	})

	t.Run("an unknown name is not found", func(t *testing.T) {
		set := newDefinitionSet([]Definition{definitionOnLine(1, defineTree(t, "first"))})

		if got, ok := set.FindDefinition("missing"); ok {
			t.Errorf("FindDefinition(%q) = %v, want not found", "missing", got)
		}
	})
}
