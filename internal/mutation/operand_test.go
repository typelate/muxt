package mutation

import (
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/asteval"
)

// TestOperandsAreFoundWhereTheyAreWritten states that every operand's span
// holds exactly the operand, which is what a substitution is spliced over.
//
// A dotted path is the case that matters: the parser reports it at its
// last segment, so its start has to be found in the text.
func TestOperandsAreFoundWhereTheyAreWritten(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		want       []string
	}{
		{name: "one field", text: `{{.Name}}`, want: []string{".Name"}},
		{name: "dot", text: `{{printf "%v" .}}`, want: []string{"."}},
		{name: "a dotted path at the end of the text", text: `{{.Alpha.Beta}}`, want: []string{".Alpha.Beta"}},
		{name: "a three segment path", text: `x {{.A.B.C}} y`, want: []string{".A.B.C"}},
		{name: "a nested pipeline", text: `{{and .A (or .B .C)}}`, want: []string{".A", ".B", ".C"}},
		{name: "a repeated operand", text: `{{and .A .A}}`, want: []string{".A", ".A"}},
		{name: "functions and literals are not operands", text: `{{printf "%s" .Name}}`, want: []string{".Name"}},
		{name: "a variable", text: `{{$x := .A}}{{$x}}`, want: []string{".A", "$x"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			trees, err := asteval.ParseTrees("t", tt.text, "", "", nil)
			require.NoError(t, err)
			var got []string
			for _, node := range trees["t"].Root.Nodes {
				a, ok := node.(*parse.ActionNode)
				if !ok {
					continue
				}
				for _, op := range operands(tt.text, nil, a.Pipe) {
					assert.Equal(t, op.text, tt.text[op.start:op.end], "operand %q span", op.text)
					got = append(got, op.text)
				}
			}
			assert.Equal(t, tt.want, got, "operands")
		})
	}
}

// TestCombinations states that every non-empty subset of the operands is
// offered, singles first, each with the edits and description to match.
func TestCombinations(t *testing.T) {
	ops := []operand{
		{start: 2, end: 4, text: ".A"},
		{start: 5, end: 7, text: ".B"},
	}
	editSets, details := combinations(ops, []string{`"x"`, `1`})

	assert.Equal(t, []string{`.A="x"`, `.B=1`, `.A="x" .B=1`}, details, "details")
	wantEdits := [][]edit{
		{{start: 2, end: 4, text: `"x"`}},
		{{start: 5, end: 7, text: `1`}},
		{{start: 2, end: 4, text: `"x"`}, {start: 5, end: 7, text: `1`}},
	}
	assert.Equal(t, wantEdits, editSets, "edits")
}
