package mutation

import (
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
			start := mutantContext{src: src}.valueStart(pipe, r)
			if got := tt.text[start:r.innerEnd]; got != tt.want {
				t.Errorf("value = %q, want %q", got, tt.want)
			}
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
			var out []Mutant
			with := action{region: src.regions[0], index: 0}
			mutantContext{src: src, template: "t"}.addConstructDrop(&out, with, OperatorWithEmpty)
			if len(out) != 1 {
				t.Fatalf("mutants = %d, want 1", len(out))
			}
			if got := src.mutatedText(out[0].edits); got != tt.want {
				t.Errorf("dropping the with leaves %q, want %q", got, tt.want)
			}
		})
	}
}
