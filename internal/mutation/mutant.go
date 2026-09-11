package mutation

import (
	"cmp"
	"go/types"
	"slices"
	"strings"
	"text/template/parse"

	"github.com/typelate/check"
)

// Operator names the variation applied to one action.
//
// No operator renames a template or changes which templates exist, so a
// mutant never moves a route: only what a template does with its data
// changes.
type Operator string

const (
	// OperatorActionEmpty makes an action print nothing, standing in for
	// the value it prints being absent. A test that never looks at the
	// printed value survives it.
	//
	// It is used where the type of the value could not be resolved from
	// dot, so an empty string is the only substitution available.
	OperatorActionEmpty Operator = "action-empty"

	// OperatorActionZero replaces the value an action prints with the
	// zero value of its own type: an empty string for a string, 0 for a
	// count, false for a flag.
	//
	// It stays closer to a real defect than substituting an empty string
	// regardless of type, and it says in the report that the type was
	// known.
	OperatorActionZero Operator = "action-zero"

	// OperatorIfTrue takes the then branch unconditionally.
	OperatorIfTrue Operator = "if-true"

	// OperatorIfFalse takes the else branch, or no branch at all,
	// unconditionally.
	OperatorIfFalse Operator = "if-false"

	// OperatorWithEmpty makes a with behave as though its value were
	// absent, so the body never runs and the else branch does. Like
	// range-never it replaces the whole construct.
	//
	// Replacing the pipeline with false would do it at run time, but it
	// also rebinds dot to a boolean, so every field access in the body
	// stops type checking and the mutant would be skipped as broken
	// rather than run as a behaviour change.
	OperatorWithEmpty Operator = "with-empty"

	// OperatorRangeNever makes a range iterate zero times, replacing the
	// whole construct with its else branch.
	//
	// It cannot be done by replacing the pipeline: there is no literal
	// for an empty sequence, and while ranging over the literal 0
	// iterates zero times, text/template refuses that for a range
	// declaring more than one variable.
	OperatorRangeNever Operator = "range-never"

	// OperatorTemplateDrop removes a {{template}} call, standing in for
	// the partial rendering nothing. It does not apply to {{block}},
	// whose call cannot be removed without orphaning its {{end}}.
	OperatorTemplateDrop Operator = "template-drop"
)

// Mutant is one variation of one action in one template.
type Mutant struct {
	// Operator is the variation applied.
	Operator Operator

	// Template is the name of the template holding the action.
	Template string

	// File is the absolute path of the file whose bytes the mutation
	// replaces.
	File string

	// Path is File relative to the directory the command ran in, which
	// is what reports show.
	Path string

	// Line and Column locate the action's left delimiter in File, both
	// one based, with Column counting bytes.
	Line, Column int

	// edits are the substitutions the mutation makes, sorted by start
	// and non-overlapping.
	//
	// Most operators make one: a pipeline, or a whole construct. An
	// operator that varies several operands of one action at once makes
	// one per operand.
	edits []edit

	// action is the opening action as it is written, which is what a
	// report shows so that a whole range body does not land in it.
	action string

	// detail is what the mutation substituted, written for a reader:
	// the replacement text for a single edit, or operand=value pairs
	// for several.
	detail string

	// src is the file the template text was read from, which knows how
	// to put mutated text back into it.
	src *templateSource
}

// edit is one substitution within a template's text.
type edit struct {
	start, end int
	text       string
}

// start reports where in the template text the mutation begins, which is
// what orders mutants within a template.
func (m Mutant) start() int {
	if len(m.edits) == 0 {
		return 0
	}
	return m.edits[0].start
}

// Apply returns the whole file with the mutation in place.
func (m Mutant) Apply() string {
	return m.src.apply(m.edits)
}

// Action returns the mutated action as it is written in the template.
func (m Mutant) Action() string { return m.action }

// Replacement returns what the mutation substituted.
func (m Mutant) Replacement() string { return m.detail }

// mutantsInScope enumerates every mutation available in one template,
// rendered with the type of dot its scope carries.
func mutantsInScope(sc scope, functions check.Functions, draw *values, maxCases int) ([]Mutant, []budgetNote) {
	var notes []budgetNote
	ctx := mutantContext{
		src:       sc.src,
		template:  sc.template,
		functions: functions,
		values:    draw,
		maxCases:  maxCases,
		notes:     &notes,
	}

	var all []Mutant
	walkActions(sc.src.text, sc.src.regions, sc.dataType, functions, sc.tree.Root, func(a action) {
		ctx.variations(&all, a)
	})

	slices.SortFunc(all, func(a, b Mutant) int {
		return cmp.Or(
			cmp.Compare(a.start(), b.start()),
			cmp.Compare(a.Operator, b.Operator),
			cmp.Compare(a.detail, b.detail),
		)
	})
	return all, notes
}

// mutantContext is what every variation needs regardless of which action
// it applies to, plus which action that is.
//
// The regions are not held here: they belong to src, and a copy could
// come to describe a different text than the one the edits are written
// against.
type mutantContext struct {
	src       *templateSource
	template  string
	dot       types.Type
	functions check.Functions
	values    *values
	maxCases  int
	notes     *[]budgetNote
}

// variations appends the mutants that apply to one action.
//
// The walk decided which actions there are and what dot each is rendered
// with; this decides only what to do with one.
func (ctx mutantContext) variations(out *[]Mutant, a action) {
	ctx.dot = a.dot

	switch a.node.(type) {
	case *parse.ActionNode:
		if zero, typed := zeroLiteral(a.dot, a.pipe, ctx.functions); typed {
			ctx.addPipeline(out, a.pipe, OperatorActionZero, zero)
		} else {
			ctx.addPipeline(out, a.pipe, OperatorActionEmpty, `""`)
		}
		// Emptying the whole action says only that something about it is
		// watched. Varying its operands says which ones.
		ctx.addOperandCombinations(out, a.pipe, ctx.maxCases)
	case *parse.IfNode:
		ctx.addPipeline(out, a.pipe, OperatorIfTrue, "true")
		ctx.addPipeline(out, a.pipe, OperatorIfFalse, "false")
		// A decision written with and, or and not gets one mutant per
		// condition; anything else falls back to the general
		// combinations over its operands.
		if !ctx.addConditions(out, a.pipe) {
			ctx.addOperandCombinations(out, a.pipe, ctx.maxCases)
		}
	case *parse.WithNode:
		ctx.addConstructDrop(out, a.region, OperatorWithEmpty)
	case *parse.RangeNode:
		ctx.addConstructDrop(out, a.region, OperatorRangeNever)
	case *parse.TemplateNode:
		ctx.addTemplateDrop(out, a.region)
	}
}

// addPipeline appends a mutant replacing the value a pipeline evaluates.
//
// A pipeline that declares variables keeps its declarations, so that
// references to them elsewhere in the template still resolve; only the
// value assigned changes.
func (ctx mutantContext) addPipeline(out *[]Mutant, pipe *parse.PipeNode, operator Operator, replacement string) {
	if pipe == nil {
		return
	}
	_, r, ok := regionAt(ctx.src.regions, int(pipe.Position()))
	if !ok {
		return
	}
	start := ctx.valueStart(pipe, r)
	if start >= r.innerEnd {
		return
	}
	ctx.appendMutant(out, r, operator, edit{start: start, end: r.innerEnd, text: replacement})
}

// addConstructDrop appends a mutant replacing a whole construct with its
// else branch, or with nothing when it has none.
func (ctx mutantContext) addConstructDrop(out *[]Mutant, r region, operator Operator) {
	index, _, ok := regionAt(ctx.src.regions, r.start)
	if !ok {
		return
	}
	endIndex, elseIndex, ok := matchEnd(ctx.src.regions, index)
	if !ok {
		return
	}
	replacement := ""
	if elseIndex >= 0 {
		replacement = ctx.src.text[ctx.src.regions[elseIndex].end:ctx.src.regions[endIndex].start]
	}
	ctx.appendMutant(out, r, operator, edit{start: r.start, end: ctx.src.regions[endIndex].end, text: replacement})
}

// addTemplateDrop appends a mutant removing a {{template}} call.
func (ctx mutantContext) addTemplateDrop(out *[]Mutant, r region) {
	if r.keyword != "template" {
		// A block defines its body in place, so removing its call
		// would leave the body and its {{end}} behind.
		return
	}
	ctx.appendMutant(out, r, OperatorTemplateDrop, edit{start: r.start, end: r.end})
}

// appendMutant records a mutant made of one substitution.
//
// The span arrives as an edit rather than as two ints so that a caller
// naming its bounds cannot swap them: a reversed span is refused here,
// which would turn a typo into a mutant that is silently never run.
func (ctx mutantContext) appendMutant(out *[]Mutant, r region, operator Operator, e edit) {
	if e.start < 0 || e.end > len(ctx.src.text) || e.start > e.end {
		return
	}
	ctx.appendEdits(out, r, operator, []edit{e}, e.text)
}

// appendEdits records a mutant made of one or more substitutions.
func (ctx mutantContext) appendEdits(out *[]Mutant, r region, operator Operator, edits []edit, detail string) {
	if len(edits) == 0 {
		return
	}
	line, column := ctx.src.lines.at(ctx.src.fileOffset(r.start))
	*out = append(*out, Mutant{
		Operator: operator,
		Template: ctx.template,
		File:     ctx.src.file,
		Path:     ctx.src.path,
		src:      ctx.src,
		Line:     line,
		Column:   column,
		edits:    edits,
		detail:   detail,
		action:   ctx.src.text[r.start:r.end],
	})
}

// valueStart returns the offset of the value a pipeline assigns, which is
// the pipeline itself unless it declares variables first.
func (ctx mutantContext) valueStart(pipe *parse.PipeNode, r region) int {
	start := int(pipe.Position())
	if len(pipe.Decl) == 0 {
		return start
	}
	last := pipe.Decl[len(pipe.Decl)-1]
	i := int(last.Position()) + len(last.String())
	for i < r.innerEnd && isSpace(ctx.src.text[i]) {
		i++
	}
	switch {
	case strings.HasPrefix(ctx.src.text[i:], ":="):
		i += 2
	case i < r.innerEnd && ctx.src.text[i] == '=':
		i++
	default:
		return start
	}
	for i < r.innerEnd && isSpace(ctx.src.text[i]) {
		i++
	}
	return i
}

// lineIndex turns a byte offset into a one based line and column.
type lineIndex []int

func newLineIndex(text string) lineIndex {
	starts := []int{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func (l lineIndex) at(offset int) (line, column int) {
	i, found := slices.BinarySearch(l, offset)
	if !found {
		i--
	}
	if i < 0 {
		i = 0
	}
	return i + 1, offset - l[i] + 1
}
