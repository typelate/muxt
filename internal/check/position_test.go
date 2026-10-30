package check_test

import (
	"go/token"
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

// TestParseNodePositionOfATemplateCall states that a {{template}} call, and
// the call a {{block}} stands for, are located at the template's name, as
// text/template's parser placed them, wherever the name is written.
func TestParseNodePositionOfATemplateCall(t *testing.T) {
	for _, tt := range []struct {
		name, text   string
		line, column int
	}{
		{name: "template", text: `<p>{{template "x" .}}</p>`, line: 1, column: 15},
		{name: "trim marker", text: `<p>{{- template "x" .}}</p>`, line: 1, column: 17},
		{name: "spaces", text: `{{  template   "x"}}`, line: 1, column: 16},
		{name: "name on the next line", text: "{{template\n  \"x\"}}", line: 2, column: 3},
		{name: "raw string name", text: "{{template `x`}}", line: 1, column: 12},
		{name: "block", text: `<p>{{block "x" .}}y{{end}}</p>`, line: 1, column: 12},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := template.New[*types.Signature]("t").Parse(`{{define "x"}}{{end}}` + "\n" + tt.text)
			if err != nil {
				t.Fatal(err)
			}
			tree, _ := ts.FindTree("t")
			var call template.Node
			template.Walk(tree, func(_ *template.Tree, n template.Node) {
				switch n := n.(type) {
				case *template.TemplateNode:
					call = n
				case *template.BlockNode:
					call = n.Call()
				}
			})
			if call == nil {
				t.Fatalf("no call in %q", tt.text)
			}
			pos := check.ParseNodePosition(tree, call)
			if pos.Line != tt.line+1 || pos.Column != tt.column {
				t.Errorf("ParseNodePosition(%q) = %d:%d, want %d:%d", tt.text, pos.Line, pos.Column, tt.line+1, tt.column)
			}
		})
	}
}

// TestErrorLocatesATemplateCallAtItsName states that an error at a
// {{template}} call is located at the name, as text/template placed it.
func TestErrorLocatesATemplateCallAtItsName(t *testing.T) {
	ts, err := template.New[*types.Signature]("t").Parse(`{{define "p"}}<p>{{template "missing" .}}</p>{{end}}`)
	if err != nil {
		t.Fatal(err)
	}
	tree, _ := ts.FindTree("p")
	global := check.NewGlobal(types.NewPackage("example.com/p", "p"), token.NewFileSet(), ts, nil)
	err = check.Execute(global, tree, types.Typ[types.Int])
	const want = `t:1:29: executing "p" at <{{template "missing" .}}>: template "missing" not found`
	if err == nil || err.Error() != want {
		t.Errorf("Execute() = %v, want %s", err, want)
	}
}
