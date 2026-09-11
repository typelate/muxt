package mutation

import "testing"

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
			mutantContext{src: src, template: "t"}.addConstructDrop(&out, src.regions[0], OperatorWithEmpty)
			if len(out) != 1 {
				t.Fatalf("mutants = %d, want 1", len(out))
			}
			if got := src.mutatedText(out[0].edits); got != tt.want {
				t.Errorf("dropping the with leaves %q, want %q", got, tt.want)
			}
		})
	}
}
