package check_test

import (
	"go/types"
	"testing"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/template"
)

func TestParseNodePosition(t *testing.T) {
	const src = `{{define "a"}}A{{end}}{{define "b"}}
{{.X}}{{end}}`
	tmpl := template.New[*types.Signature]("")
	tmpl, err := tmpl.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	tree, ok := tmpl.FindTree("b")
	if !ok {
		t.Fatal("template b not found")
	}
	node := template.Node(tree.Root)

	t.Run("offset comes from Node.Start", func(t *testing.T) {
		pos := check.ParseNodePosition(tree, node)
		if pos.Offset != int(node.Start()) {
			t.Errorf("Offset = %d, want %d", pos.Offset, int(node.Start()))
		}
	})

	t.Run("line and column come from Tree.Position", func(t *testing.T) {
		want := tree.Position(node)
		pos := check.ParseNodePosition(tree, node)
		if pos.Line != want.Line {
			t.Errorf("Line = %d, want %d", pos.Line, want.Line)
		}
		if pos.Column != want.Column {
			t.Errorf("Column = %d, want %d", pos.Column, want.Column)
		}
	})

	t.Run("filename prefers Tree.Filepath", func(t *testing.T) {
		tree.Filepath = "b.gohtml"
		pos := check.ParseNodePosition(tree, node)
		if pos.Filename != "b.gohtml" {
			t.Errorf("Filename = %q, want b.gohtml", pos.Filename)
		}
	})

	t.Run("filename falls back to ParseName", func(t *testing.T) {
		tree.Filepath = ""
		pos := check.ParseNodePosition(tree, node)
		if pos.Filename != tree.Position(node).ParseName {
			t.Errorf("Filename = %q, want ParseName %q", pos.Filename, tree.Position(node).ParseName)
		}
	})
}
