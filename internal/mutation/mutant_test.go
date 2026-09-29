package mutation

import (
	"strconv"
	"strings"
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			require.NoError(t, err)
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
			require.True(t, ok, "no region holds the pipeline at %d", pipe.Position())
			start := valueStart(pipe)
			assert.Equal(t, tt.want, tt.text[start:r.innerEnd], "value")
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
	require.NoError(t, err)
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
			m := e.mutants[0]
			assert.Equal(t, tt.line, m.Line, "mutant line")
			assert.Equal(t, tt.column, m.Column, "mutant column")
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
		t.Run(strconv.Itoa(tt.offset), func(t *testing.T) {
			line, column := lines.at(tt.offset)
			assert.Equal(t, tt.line, line, "at(%d) line", tt.offset)
			assert.Equal(t, tt.column, column, "at(%d) column", tt.offset)
		})
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
			require.Len(t, e.mutants, 1)
			assert.Equal(t, tt.want, src.mutatedText(e.mutants[0].edits), "text left by dropping the with")
		})
	}
}
