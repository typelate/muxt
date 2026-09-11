package mutation

import (
	"strings"
	"testing"
	"text/template/parse"

	"github.com/typelate/muxt/internal/asteval"
)

// TestValueStart states where a pipeline's value begins, which is where
// a substitution starts. A declaration is kept, so that references to the
// variable still resolve; only what it is assigned changes.
func TestValueStart(t *testing.T) {
	for _, tt := range []struct {
		text, want string
	}{
		{text: `{{.A}}`, want: `.A`},
		{text: `{{$x := .A}}`, want: `.A`},
		{text: `{{$x:=.A -}}`, want: `.A`},
		{text: `{{$x := 1}}{{$x = .A}}`, want: `.A`},
		{text: `{{range $i, $e := .Items}}{{end}}`, want: `.Items`},
		{text: `{{with $x := .A}}{{end}}`, want: `.A`},
	} {
		t.Run(tt.text, func(t *testing.T) {
			trees, err := asteval.ParseTrees("t", tt.text, "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			nodes := trees["t"].Root.Nodes
			var pipe *parse.PipeNode
			switch node := nodes[len(nodes)-1].(type) {
			case *parse.ActionNode:
				pipe = node.Pipe
			case *parse.RangeNode:
				pipe = node.Pipe
			case *parse.WithNode:
				pipe = node.Pipe
			}
			src := newFileSource("t.gohtml", "t.gohtml", tt.text, "", "")
			_, r, ok := regionAt(src.regions, int(pipe.Position()))
			if !ok {
				t.Fatalf("no region holds the pipeline at %d", pipe.Position())
			}
			start := valueStart(pipe)
			if got := tt.text[start:r.innerEnd]; got != tt.want {
				t.Errorf("value = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMutantsAreReportedWhereTheFileHoldsThem states that a mutant's line
// and column are in the file a reader opens, not in the template text.
// For a template written as a Go literal the two differ by everything in
// the file before the literal.
func TestMutantsAreReportedWhereTheFileHoldsThem(t *testing.T) {
	const goFile = "package p\n\nvar t = `x\n  {{.A}}`\n"
	start, end := strings.Index(goFile, "`"), strings.LastIndex(goFile, "`")+1
	literal, err := newLiteralSource("p.go", "p.go", "t", goFile, "", "", start, end)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name         string
		src          *templateSource
		line, column int
	}{
		{name: "a template file", src: newFileSource("t.gohtml", "t.gohtml", "x\n  {{.A}}", "", ""), line: 2, column: 3},
		{name: "a Go string literal", src: literal, line: 4, column: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := &enumerator{src: tt.src, template: "t"}
			r := tt.src.regions[0]
			e.appendEdits(r, OperatorActionEmpty, []edit{{start: r.start, end: r.end}}, "")
			if m := e.mutants[0]; m.Line != tt.line || m.Column != tt.column {
				t.Errorf("mutant at %d:%d, want %d:%d", m.Line, m.Column, tt.line, tt.column)
			}
		})
	}
}

// TestLineIndex states the line and column an offset is reported at,
// which is how a reader finds a mutant in the file.
func TestLineIndex(t *testing.T) {
	lines := newLineIndex("ab\ncd\n\nef")
	for _, tt := range []struct{ offset, line, column int }{
		{offset: 0, line: 1, column: 1},
		{offset: 1, line: 1, column: 2},
		{offset: 2, line: 1, column: 3}, // a newline ends its own line
		{offset: 3, line: 2, column: 1},
		{offset: 6, line: 3, column: 1},
		{offset: 7, line: 4, column: 1},
		{offset: 8, line: 4, column: 2},
	} {
		if line, column := lines.at(tt.offset); line != tt.line || column != tt.column {
			t.Errorf("at(%d) = %d:%d, want %d:%d", tt.offset, line, column, tt.line, tt.column)
		}
	}
}

// TestConstructDropKeepsTheElseBranch states what dropping a with or range
// leaves in its place: its else branch, or nothing.
//
// An {{else with}} is the interesting case. text/template reads it as an
// else holding a second with, so dropping the first has to leave that
// second one standing, opened and closed, or the mutant does not parse
// and is never run.
func TestConstructDropKeepsTheElseBranch(t *testing.T) {
	for _, tt := range []struct {
		name        string
		text        string
		left, right string
		want        string
	}{
		{name: "no else", text: `{{with .A}}a{{end}}`, want: ``},
		{name: "an else", text: `{{with .A}}a{{else}}c{{end}}`, want: `c`},
		{
			name: "an else with",
			text: `{{with .A}}a{{else with .B}}b{{else}}c{{end}}`,
			want: `{{with .B}}b{{else}}c{{end}}`,
		},
		{
			name: "an else with and trim markers",
			text: `{{with .A}}a{{- else with .B -}} b {{end}}`,
			want: `{{with .B -}} b {{end}}`,
		},
		{
			name: "an else with and other delimiters",
			text: `[[with .A]]a[[else with .B]]b[[end]]`,
			left: "[[", right: "]]",
			want: `[[with .B]]b[[end]]`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			src := newFileSource("t.gohtml", "t.gohtml", tt.text, tt.left, tt.right)
			e := &enumerator{src: src, template: "t"}
			e.addConstructDrop(action{region: src.regions[0], index: 0}, OperatorWithEmpty)
			out := e.mutants
			if len(out) != 1 {
				t.Fatalf("mutants = %d, want 1", len(out))
			}
			if got := src.mutatedText(out[0].edits); got != tt.want {
				t.Errorf("dropping the with leaves %q, want %q", got, tt.want)
			}
		})
	}
}
