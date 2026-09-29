package mutation

import (
	"strings"
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/asteval"
)

// ifPipe parses `{{if pipeline}}x{{end}}` and returns the template text and
// the if's pipeline.
func ifPipe(t *testing.T, pipeline string) (string, *parse.PipeNode) {
	t.Helper()
	text := `{{if ` + pipeline + `}}x{{end}}`
	trees, err := asteval.ParseTrees("t", text, "", "", nil)
	require.NoError(t, err)
	node, ok := trees["t"].Root.Nodes[0].(*parse.IfNode)
	require.Truef(t, ok, "first node is %T, want an if", trees["t"].Root.Nodes[0])
	return text, node.Pipe
}

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
		{
			name:       "flattening a nested or",
			pipeline:   `or .A (or .B .C)`,
			canonical:  "or(.A,.B,.C)",
			conditions: []string{".A", ".B", ".C"},
		},
		{
			name:       "a nested duplicate is dropped once flattened",
			pipeline:   `and (and .A .B) .A`,
			canonical:  "and(.A,.B)",
			conditions: []string{".A", ".B"},
		},
		{
			name:       "not of true",
			pipeline:   `not true`,
			canonical:  "false",
			conditions: nil,
		},
		{
			name:       "not of false",
			pipeline:   `not false`,
			canonical:  "true",
			conditions: nil,
		},
		{
			name:       "a single negation stays",
			pipeline:   `and (not .A) .B`,
			canonical:  "and(.B,not(.A))",
			conditions: []string{".A", ".B"},
		},
		{
			name:       "a false is the identity of or",
			pipeline:   `or .A false`,
			canonical:  ".A",
			conditions: []string{".A"},
		},
		{
			name:       "only constants leave the identity",
			pipeline:   `and true true`,
			canonical:  "true",
			conditions: nil,
		},
		{
			name:       "absorption in an or drops the wider term",
			pipeline:   `or .A (and .A .B)`,
			canonical:  ".A",
			conditions: []string{".A"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			text, pipe := ifPipe(t, tt.pipeline)
			built, ok := decision(text, pipe)
			require.Truef(t, ok, "decision(%q) could not be modelled", tt.pipeline)

			simplified := built.simplify()
			assert.Equal(t, tt.canonical, simplified.canonical(), "simplify(%q).canonical()", tt.pipeline)
			assert.Equal(t, strings.Join(tt.conditions, ","), strings.Join(simplified.conditions(), ","), "simplify(%q).conditions()", tt.pipeline)
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
		`not .A .B`,
		`not`,
		`and`,
		`or`,
		`.A .B`,
		`and .A (eq .B 1)`,
	} {
		t.Run(pipeline, func(t *testing.T) {
			text, pipe := ifPipe(t, pipeline)
			_, ok := decision(text, pipe)
			assert.False(t, ok, "decision(%q) was modelled, want it declined so the general operand combinations apply instead", pipeline)
		})
	}
}

func TestDecisionCommandRefusesACommandWithoutArguments(t *testing.T) {
	for name, command := range map[string]*parse.CommandNode{
		"nil":   nil,
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			node, ok := decisionCommand("", command)
			assert.False(t, ok, "decisionCommand(%s command) = %v, want it declined", name, node)
		})
	}
}

func TestLogicalKind(t *testing.T) {
	for _, tt := range []struct {
		function string
		kind     boolKind
		ok       bool
	}{
		{function: "and", kind: boolAnd, ok: true},
		{function: "or", kind: boolOr, ok: true},
		{function: "not", kind: boolNot, ok: true},
		{function: "eq", ok: false},
		{function: "", ok: false},
	} {
		t.Run(tt.function, func(t *testing.T) {
			kind, ok := logicalKind(tt.function)
			assert.Equal(t, tt.ok, ok, "logicalKind(%q) ok", tt.function)
			if ok {
				assert.Equal(t, tt.kind, kind, "logicalKind(%q)", tt.function)
			}
		})
	}
}

func TestDecisionCallArity(t *testing.T) {
	one := []parse.Node{&parse.BoolNode{True: true}}
	two := []parse.Node{&parse.BoolNode{True: true}, &parse.BoolNode{}}
	for _, tt := range []struct {
		name string
		kind boolKind
		args []parse.Node
		ok   bool
	}{
		{name: "not takes one", kind: boolNot, args: one, ok: true},
		{name: "not refuses two", kind: boolNot, args: two, ok: false},
		{name: "not refuses none", kind: boolNot, args: nil, ok: false},
		{name: "and takes one", kind: boolAnd, args: one, ok: true},
		{name: "and takes two", kind: boolAnd, args: two, ok: true},
		{name: "and refuses none", kind: boolAnd, args: nil, ok: false},
		{name: "or refuses none", kind: boolOr, args: nil, ok: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			node, ok := decisionCall(tt.kind, "", tt.args)
			require.Equal(t, tt.ok, ok, "decisionCall(%v, %d args) ok", tt.kind, len(tt.args))
			if ok {
				assert.Len(t, node.kids, len(tt.args), "decisionCall(%v, %d args) operands", tt.kind, len(tt.args))
			}
		})
	}
}

func TestSimplifyHelpers(t *testing.T) {
	a := &boolNode{kind: boolCond, text: ".A"}
	b := &boolNode{kind: boolCond, text: ".B"}
	not := func(n *boolNode) *boolNode { return &boolNode{kind: boolNot, kids: []*boolNode{n}} }
	junction := func(kind boolKind, kids ...*boolNode) *boolNode { return &boolNode{kind: kind, kids: kids} }
	canonicals := func(kids []*boolNode) string {
		parts := make([]string, 0, len(kids))
		for _, kid := range kids {
			parts = append(parts, kid.canonical())
		}
		return strings.Join(parts, " ")
	}

	t.Run("negate", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			kid  *boolNode
			want string
		}{
			{name: "true", kid: constant(true), want: "false"},
			{name: "false", kid: constant(false), want: "true"},
			{name: "double", kid: not(a), want: ".A"},
			{name: "condition", kid: a, want: "not(.A)"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.want, negate(tt.kid).canonical(), "negate(%s)", tt.name)
			})
		}
	})

	t.Run("flatten", func(t *testing.T) {
		got := canonicals(flatten(boolAnd, []*boolNode{a, junction(boolAnd, b, a), junction(boolOr, a, b), not(not(b))}))
		assert.Equal(t, ".A .B .A or(.A,.B) .B", got, "flatten(and, ...)")
	})

	t.Run("foldConstants", func(t *testing.T) {
		for _, tt := range []struct {
			name        string
			kids        []*boolNode
			zero        bool
			want        string
			wantDecided bool
		}{
			{name: "and drops true", kids: []*boolNode{a, constant(true), b}, zero: false, want: ".A .B"},
			{name: "and decided by false", kids: []*boolNode{a, constant(false)}, zero: false, wantDecided: true},
			{name: "or drops false", kids: []*boolNode{constant(false), a}, zero: true, want: ".A"},
			{name: "or decided by true", kids: []*boolNode{a, constant(true)}, zero: true, wantDecided: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				kept, decided := foldConstants(tt.kids, tt.zero)
				assert.Equal(t, tt.wantDecided, decided, "foldConstants(%s) decided", tt.name)
				if !decided {
					assert.Equal(t, tt.want, canonicals(kept), "foldConstants(%s) kept", tt.name)
				}
			})
		}
	})

	t.Run("distinct", func(t *testing.T) {
		got := canonicals(distinct([]*boolNode{a, b, a, junction(boolAnd, b, a), junction(boolAnd, a, b)}))
		assert.Equal(t, ".A .B and(.A,.B)", got, "distinct")
	})

	t.Run("hasComplement", func(t *testing.T) {
		for _, tt := range []struct {
			name string
			kids []*boolNode
			want bool
		}{
			{name: "none", kids: []*boolNode{a, b}, want: false},
			{name: "not first", kids: []*boolNode{not(a), a}, want: true},
			{name: "not last", kids: []*boolNode{a, b, not(a)}, want: true},
			{name: "different negation", kids: []*boolNode{a, not(b)}, want: false},
		} {
			t.Run(tt.name, func(t *testing.T) {
				assert.Equal(t, tt.want, hasComplement(tt.kids), "hasComplement(%s)", tt.name)
			})
		}
	})
}
