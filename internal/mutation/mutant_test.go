package mutation

import (
	"strconv"
	"strings"
	"testing"
	texttemplate "text/template"
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
		{
			name: "no else and trim markers on both sides",
			text: `x {{- with .A -}} a {{- end -}} y`,
			want: `x {{- /* */ -}} y`,
		},
		{
			name: "no else and a trim marker before",
			text: `x {{- with .A}} a {{end}} y`,
			want: `x {{- /* */}} y`,
		},
		{
			name: "no else and trim markers inside only",
			text: `x {{with .A -}} a {{- end}} y`,
			want: `x  y`,
		},
		{
			name: "an else and trim markers on both sides",
			text: `x {{- with .A -}} a {{- else -}} c {{- end -}} y`,
			want: `x {{- if true -}} c {{- end -}} y`,
		},
		{
			name: "an else and a trim marker after the else",
			text: `x {{with .A}} a {{else -}} c {{end}} y`,
			want: `x {{if true -}} c {{end}} y`,
		},
		{
			name: "an else with and trim markers on both sides",
			text: `x {{- with .A -}} a {{- else with .B -}} b {{- end -}} y`,
			want: `x {{- with .B -}} b {{- end -}} y`,
		},
		{
			name: "no else, trim markers and other delimiters",
			text: `x [[- with .A -]] a [[- end -]] y`,
			left: "[[", right: "]]",
			want: `x [[- /* */ -]] y`,
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

// TestDropsRenderWhatTheSkippedConstructRendered states that dropping a
// construct or a {{template}} call changes only what the dropped part
// would have rendered, never the whitespace around it.
//
// Each case renders with data under which the dropped part renders
// nothing, so the mutant is equivalent: a test comparing the output
// exactly must not catch it. Losing a trim marker along with the
// construct changes the whitespace, and such a test would report a kill
// that no assertion on the behaviour earned.
func TestDropsRenderWhatTheSkippedConstructRendered(t *testing.T) {
	for _, tt := range []struct {
		name     string
		text     string
		operator Operator
		data     map[string]any
	}{
		{
			name:     "a range with trim markers",
			text:     "<ul>\n{{- range .Items}}\n<li>{{.}}</li>\n{{- end}}\n</ul>",
			operator: OperatorRangeNever,
			data:     map[string]any{"Items": []string{}},
		},
		{
			name:     "a range with an else and trim markers",
			text:     "<ul>\n{{- range .Items -}}\n<li>{{.}}</li>\n{{- else -}}\n<li>none</li>\n{{- end -}}\n</ul>",
			operator: OperatorRangeNever,
			data:     map[string]any{"Items": []string{}},
		},
		{
			name:     "a with with trim markers",
			text:     "a\n{{- with .A -}}\n{{.}}\n{{- end -}}\nb",
			operator: OperatorWithEmpty,
			data:     map[string]any{"A": ""},
		},
		{
			name:     "a template call with trim markers",
			text:     "<p>a</p>\n{{- template \"footer\" . -}}\n<p>end</p>{{define \"footer\"}}{{end}}",
			operator: OperatorTemplateDrop,
			data:     map[string]any{},
		},
		{
			name:     "a template call with a trim marker after",
			text:     "<p>a</p>\n{{template \"footer\" . -}}\n<p>end</p>{{define \"footer\"}}{{end}}",
			operator: OperatorTemplateDrop,
			data:     map[string]any{},
		},
		{
			name:     "a chained with",
			text:     "{{with .A}}a{{.}}{{else with .B}}b{{.}}{{else}}c{{end}}",
			operator: OperatorWithEmpty,
			data:     map[string]any{"A": "", "B": ""},
		},
		{
			name:     "a chained with and trim markers",
			text:     "x\n{{- with .A -}}\na\n{{- else with .B -}}\nb\n{{- else -}}\nc\n{{- end -}}\ny",
			operator: OperatorWithEmpty,
			data:     map[string]any{"A": "", "B": ""},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := render(t, tt.text, tt.data)
			mutants := dropMutants(t, tt.text)
			var tried int
			for _, m := range mutants {
				if m.Operator != tt.operator {
					continue
				}
				tried++
				mutated := m.src.mutatedText(m.edits)
				assert.Equal(t, want, render(t, mutated, tt.data), "%s at %d:%d renders differently:\n%s", m.Operator, m.Line, m.Column, mutated)
			}
			assert.NotZero(t, tried, "no %s mutant among %d", tt.operator, len(mutants))
		})
	}
}

// TestConstructDropReachesAChainedWith states that each with of an
// {{else with}} chain gets a mutant of its own, and that dropping a later
// one leaves the chain around it standing.
func TestConstructDropReachesAChainedWith(t *testing.T) {
	for _, tt := range []struct {
		name string
		text string
		want []string
	}{
		{
			name: "a chain with an else",
			text: `{{with .A}}a{{.}}{{else with .B}}b{{.}}{{else}}c{{end}}`,
			want: []string{
				`{{with .B}}b{{.}}{{else}}c{{end}}`,
				`{{with .A}}a{{.}}{{else}}c{{end}}`,
			},
		},
		{
			name: "a chain without an else",
			text: `{{with .A}}a{{else with .B}}b{{end}}`,
			want: []string{
				`{{with .B}}b{{end}}`,
				`{{with .A}}a{{else}}{{end}}`,
			},
		},
		{
			name: "a chain of three",
			text: `{{with .A}}a{{else with .B}}b{{else with .C}}c{{end}}`,
			want: []string{
				`{{with .B}}b{{else with .C}}c{{end}}`,
				`{{with .A}}a{{else with .C}}c{{end}}`,
				`{{with .A}}a{{else with .B}}b{{else}}{{end}}`,
			},
		},
		{
			name: "a chain with trim markers",
			text: `{{with .A}} a {{- else with .B -}} b {{- else -}} c {{end}}`,
			want: []string{
				`{{with .B -}} b {{- else -}} c {{end}}`,
				`{{with .A}} a {{- else -}} c {{end}}`,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, m := range dropMutants(t, tt.text) {
				got = append(got, m.src.mutatedText(m.edits))
			}
			assert.Equal(t, tt.want, got, "mutated texts")
		})
	}
}

// dropMutants enumerates the construct and template drops of a template
// named t, in the order they are written.
func dropMutants(t *testing.T, text string) []mutant {
	t.Helper()
	trees, err := asteval.ParseTrees("t", text, "", "", nil)
	require.NoError(t, err)
	src := newFileSource("t.gohtml", "t.gohtml", text, "", "")
	e := &enumerator{src: src, template: "t"}
	walkActions(src.regions, nil, nil, trees["t"].Root, func(a action) {
		switch a.node.(type) {
		case *parse.WithNode, *parse.RangeNode, *parse.TemplateNode:
			e.variations(a)
		}
	})
	return e.mutants
}

func render(t *testing.T, text string, data any) string {
	t.Helper()
	tmpl, err := texttemplate.New("t").Parse(text)
	require.NoError(t, err, "parse:\n%s", text)
	var out strings.Builder
	require.NoError(t, tmpl.Execute(&out, data), "execute:\n%s", text)
	return out.String()
}
