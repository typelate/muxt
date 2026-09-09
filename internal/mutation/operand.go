package mutation

import (
	"fmt"
	"go/types"
	"math/rand/v2"
	"strconv"
	"strings"
	"text/template/parse"
)

// OperatorOperands varies the leaf operands of one action together,
// across every combination of keeping and replacing them.
//
// A single-operand action is already covered by action-zero. The point of
// the combinations is an action reading more than one thing: a test may
// assert on the rendered result without depending on all of it, and only
// varying the operands separately shows which ones nothing is watching.
const OperatorOperands = "operands"

// operand is one leaf input of an action: a field access, a variable or
// dot itself, with where it is written and what it evaluates to.
//
// A function name is not an operand, and neither is a literal: neither
// carries data into the template, so neither is an input a test could be
// coupled to.
type operand struct {
	start, end int
	text       string
	dataType   types.Type

	// resolution describes how the type was reached, segment by segment,
	// so a method signature changing invalidates what depends on it.
	resolution string
}

// operands returns the leaf inputs a pipeline reads, in source order.
//
// A nested pipeline is walked into, so {{if and .A (or .B .C)}} reports
// three operands rather than two.
func operands(text string, dot types.Type, pipe *parse.PipeNode) []operand {
	var found []operand
	collectOperands(&found, text, dot, pipe)
	return found
}

func collectOperands(out *[]operand, text string, dot types.Type, pipe *parse.PipeNode) {
	if pipe == nil {
		return
	}
	for _, command := range pipe.Cmds {
		for _, arg := range command.Args {
			switch node := arg.(type) {
			case *parse.PipeNode:
				collectOperands(out, text, dot, node)
			case *parse.FieldNode:
				resolved, trace := fieldType(dot, node.Ident)
				appendOperand(out, text, node, resolved, trace)
			case *parse.VariableNode:
				// A variable's type would have to be tracked through the
				// declaration that bound it, which the checker does and
				// this does not. It is still an input worth varying.
				appendOperand(out, text, node, nil, "")
			case *parse.DotNode:
				appendOperand(out, text, node, dot, "dot")
			}
		}
	}
}

// appendOperand records a leaf, but only where the node's own rendering
// matches the source.
//
// A node's String is a reconstruction, not a quotation, so requiring the
// two to agree is what keeps a mutation from splicing over the wrong
// bytes.
func appendOperand(out *[]operand, text string, node parse.Node, dataType types.Type, resolution string) {
	written := node.String()
	start, ok := operandStart(text, int(node.Position()), written)
	if !ok {
		return
	}
	*out = append(*out, operand{start: start, end: start + len(written), text: written, dataType: dataType, resolution: resolution})
}

// operandStart locates where a node is written in text.
//
// A field path of more than one segment does not begin where the parser
// says it does: the lexer emits one item per segment, and the node keeps
// the position of the segment that extended it, so .A.B.C is reported at
// .B and .Path.Link at .Link. Taking that position literally makes the
// rendering disagree with the text, and the operand is dropped -- which
// silently cost every dotted path its mutants.
//
// The rendering is exact, so the start is the nearest offset at or before
// the reported one where the text equals it. The search is bounded by the
// rendering's own length, which is longer than any prefix the position
// can have skipped.
func operandStart(text string, pos int, written string) (int, bool) {
	if written == "" || pos < 0 || len(written) > len(text) {
		return 0, false
	}
	lowest := max(pos-len(written), 0)
	for start := min(pos, len(text)-len(written)); start >= lowest; start-- {
		if text[start:start+len(written)] == written {
			return start, true
		}
	}
	return 0, false
}

func fieldType(dot types.Type, idents []string) (types.Type, string) {
	resolved, trace, ok := fieldPathTrace(dot, idents)
	if !ok {
		return nil, ""
	}
	return resolved, trace
}

// values draws replacement literals.
//
// The draws are seeded so that a report can be reproduced: the same seed
// over unchanged templates produces the same mutants, and a different
// seed shakes out a coupling that one set of values happened to miss.
type values struct {
	rand *rand.Rand
}

func newValues(seed uint64) *values {
	return &values{rand: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

// draw returns a literal of the operand's type, different from anything
// the template would ordinarily render there.
func (v *values) draw(t types.Type) string {
	basic, ok := underlyingBasic(t)
	if !ok {
		// Without a type the only literal that is valid wherever a value
		// is read is a string.
		return strconv.Quote(v.word())
	}
	switch info := basic.Info(); {
	case info&types.IsString != 0:
		return strconv.Quote(v.word())
	case info&types.IsBoolean != 0:
		return strconv.FormatBool(v.rand.IntN(2) == 1)
	case info&types.IsInteger != 0:
		return strconv.Itoa(v.rand.IntN(1000))
	case info&types.IsFloat != 0:
		return strconv.FormatFloat(float64(v.rand.IntN(10000))/100, 'g', -1, 64)
	default:
		return strconv.Quote(v.word())
	}
}

const wordAlphabet = "abcdefghijklmnopqrstuvwxyz"

func (v *values) word() string {
	var b strings.Builder
	for range 4 {
		b.WriteByte(wordAlphabet[v.rand.IntN(len(wordAlphabet))])
	}
	return b.String()
}

func underlyingBasic(t types.Type) (*types.Basic, bool) {
	if t == nil {
		return nil, false
	}
	if isSafeString(t) {
		// A safe string type reads as an ordinary string here, and a
		// drawn literal is an ordinary string, so the substitution is
		// honest even though the trust marking is dropped.
		return types.Typ[types.String], true
	}
	basic, ok := t.Underlying().(*types.Basic)
	return basic, ok
}

// combinations returns the ways of replacing some non-empty subset of the
// operands, as edit lists paired with a description.
//
// Every subset is offered, rather than one operand at a time, because
// operands can compensate for each other: a test may notice either of two
// values changing while noticing neither of them alone.
func combinations(ops []operand, drawn []string) ([][]edit, []string) {
	total := 1<<len(ops) - 1
	editSets := make([][]edit, 0, total)
	details := make([]string, 0, total)

	for mask := 1; mask <= total; mask++ {
		var (
			edits  []edit
			detail strings.Builder
		)
		for i, op := range ops {
			if mask&(1<<i) == 0 {
				continue
			}
			edits = append(edits, edit{start: op.start, end: op.end, text: drawn[i]})
			if detail.Len() > 0 {
				detail.WriteByte(' ')
			}
			fmt.Fprintf(&detail, "%s=%s", op.text, drawn[i])
		}
		editSets = append(editSets, edits)
		details = append(details, detail.String())
	}
	return editSets, details
}

// addOperandCombinations appends a mutant per combination of the action's
// operands, or nothing when there is only one operand, which the
// single-value operators already cover.
func (ctx mutantContext) addOperandCombinations(out *[]Mutant, pipe *parse.PipeNode, maxCases int) {
	if pipe == nil {
		return
	}
	_, r, ok := regionAt(ctx.src.regions, int(pipe.Position()))
	if !ok {
		return
	}
	ops := operands(ctx.src.text, ctx.dot, pipe)
	if len(ops) < 2 {
		return
	}
	if cases := 1<<len(ops) - 1; cases > maxCases {
		line, column := ctx.src.lines.at(ctx.src.fileOffset(r.start))
		*ctx.notes = append(*ctx.notes, budgetNote{
			template: ctx.template,
			line:     line,
			column:   column,
			inputs:   len(ops),
			cases:    cases,
			maxCases: maxCases,
		})
		return
	}

	drawn := make([]string, len(ops))
	for i, op := range ops {
		drawn[i] = ctx.values.draw(op.dataType)
	}

	editSets, details := combinations(ops, drawn)
	for i, edits := range editSets {
		ctx.appendEdits(out, r, OperatorOperands, edits, details[i])
	}
}

// budgetNote records an action whose combinations were not enumerated,
// so a run that skipped work says so rather than looking thorough.
type budgetNote struct {
	template string
	line     int
	column   int
	inputs   int
	cases    int
	maxCases int
}

func (n budgetNote) reason() string {
	return fmt.Sprintf("%d operands need %d cases, over --max-cases=%d", n.inputs, n.cases, n.maxCases)
}
