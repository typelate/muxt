package mutation

import (
	"strings"
	"testing"
	"text/template/parse"

	"github.com/typelate/muxt/internal/asteval"
)

// TestSimplifyReducesConditions states the laws the decision simplifier
// applies.
//
// These rules decide whether a condition gets mutants or is reported
// condition-dead, which is the tool saying "no test could ever be coupled
// to this". That is the most damaging thing it can say wrongly: it claims
// a gap does not exist. Each law gets a row, because a golden that
// happens to exercise one of them leaves the rest free.
func TestSimplifyReducesConditions(t *testing.T) {
	for _, tt := range []struct {
		name       string
		pipeline   string
		canonical  string
		conditions []string
	}{
		{
			name:       "idempotence",
			pipeline:   `and .A .A`,
			canonical:  ".A",
			conditions: []string{".A"},
		},
		{
			name:       "complement makes an and impossible",
			pipeline:   `and .A (not .A)`,
			canonical:  "false",
			conditions: nil,
		},
		{
			name:       "complement makes an or certain",
			pipeline:   `or .A (not .A)`,
			canonical:  "true",
			conditions: nil,
		},
		{
			name:       "double negation",
			pipeline:   `and (not (not .A)) .B`,
			canonical:  "and(.A,.B)",
			conditions: []string{".A", ".B"},
		},
		{
			name:       "a true is the identity of and",
			pipeline:   `and .A true`,
			canonical:  ".A",
			conditions: []string{".A"},
		},
		{
			name:       "a true short circuits an or",
			pipeline:   `or .A true`,
			canonical:  "true",
			conditions: nil,
		},
		{
			name:       "a false short circuits an and",
			pipeline:   `and .A false`,
			canonical:  "false",
			conditions: nil,
		},
		{
			name:       "absorption drops the wider term",
			pipeline:   `and .A (or .A .B)`,
			canonical:  ".A",
			conditions: []string{".A"},
		},
		{
			name:       "flattening a nested and",
			pipeline:   `and .A (and .B .C)`,
			canonical:  "and(.A,.B,.C)",
			conditions: []string{".A", ".B", ".C"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			text := `{{if ` + tt.pipeline + `}}x{{end}}`
			trees, err := asteval.ParseTrees("t", text, "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			node, ok := trees["t"].Root.Nodes[0].(*parse.IfNode)
			if !ok {
				t.Fatalf("first node is %T, want an if", trees["t"].Root.Nodes[0])
			}
			built, ok := decision(text, node.Pipe)
			if !ok {
				t.Fatalf("decision(%q) could not be modelled", tt.pipeline)
			}

			simplified := built.simplify()
			if got := simplified.canonical(); got != tt.canonical {
				t.Errorf("simplify(%q).canonical() = %q, want %q", tt.pipeline, got, tt.canonical)
			}
			got := simplified.conditions()
			if strings.Join(got, ",") != strings.Join(tt.conditions, ",") {
				t.Errorf("simplify(%q).conditions() = %v, want %v", tt.pipeline, got, tt.conditions)
			}
		})
	}
}

// TestDecisionRefusesWhatItCannotModel states that a decision the
// simplifier does not understand exactly is declined, rather than
// half-understood.
//
// A half-understood decision would produce condition mutants claiming to
// force a condition they do not control.
func TestDecisionRefusesWhatItCannotModel(t *testing.T) {
	for _, pipeline := range []string{
		`eq .A .B`,
		`and (eq .A 1) .B`,
		`.A | not`,
	} {
		t.Run(pipeline, func(t *testing.T) {
			text := `{{if ` + pipeline + `}}x{{end}}`
			trees, err := asteval.ParseTrees("t", text, "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			node := trees["t"].Root.Nodes[0].(*parse.IfNode)
			if _, ok := decision(text, node.Pipe); ok {
				t.Errorf("decision(%q) was modelled, want it declined so the general operand combinations apply instead", pipeline)
			}
		})
	}
}
