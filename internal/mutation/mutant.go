package mutation

import (
	"cmp"
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
	e := &enumerator{
		src:       sc.src,
		template:  sc.template,
		functions: functions,
		values:    draw,
		maxCases:  maxCases,
	}
	walkActions(sc.src.text, sc.src.regions, sc.dataType, functions, sc.tree.Root, e.variations)

	slices.SortFunc(e.mutants, func(a, b Mutant) int {
		return cmp.Or(
			cmp.Compare(a.start(), b.start()),
			cmp.Compare(a.Operator, b.Operator),
			cmp.Compare(a.detail, b.detail),
		)
	})
	return e.mutants, e.notes
}

// enumerator collects the mutants of one template, holding what every
// variation needs regardless of which action it applies to.
//
// The regions are not held here: they belong to src, and a copy could
// come to describe a different text than the one the edits are written
// against.
type enumerator struct {
	src       *templateSource
	template  string
	functions check.Functions
	values    *values
	maxCases  int

	// mutants and notes are what the variations have found so far.
	mutants []Mutant
	notes   []budgetNote
}

// variations appends the mutants that apply to one action.
//
// The walk decided which actions there are and what dot each is rendered
// with; this decides only what to do with one.
func (e *enumerator) variations(a action) {
	switch a.node.(type) {
	case *parse.ActionNode:
		if zero, typed := zeroLiteral(a.dot, a.pipe, e.functions); typed {
			e.addPipeline(a, OperatorActionZero, zero)
		} else {
			e.addPipeline(a, OperatorActionEmpty, `""`)
		}
		// Emptying the whole action says only that something about it is
		// watched. Varying its operands says which ones.
		e.addOperandCombinations(a)
	case *parse.IfNode:
		e.addPipeline(a, OperatorIfTrue, "true")
		e.addPipeline(a, OperatorIfFalse, "false")
		// A decision written with and, or and not gets one mutant per
		// condition; anything else falls back to the general
		// combinations over its operands.
		if !e.addConditions(a) {
			e.addOperandCombinations(a)
		}
	case *parse.WithNode:
		e.addConstructDrop(a, OperatorWithEmpty)
	case *parse.RangeNode:
		e.addConstructDrop(a, OperatorRangeNever)
	case *parse.TemplateNode:
		e.addTemplateDrop(a.region)
	}
}

// addPipeline appends a mutant replacing the value an action's pipeline
// evaluates.
//
// A pipeline that declares variables keeps its declarations, so that
// references to them elsewhere in the template still resolve; only the
// value assigned changes.
func (e *enumerator) addPipeline(a action, operator Operator, replacement string) {
	r := a.region
	start := valueStart(a.pipe)
	if start >= r.innerEnd {
		return
	}
	e.appendMutant(r, operator, edit{start: start, end: r.innerEnd, text: replacement})
}

// addConstructDrop appends a mutant replacing a whole construct with its
// else branch, or with nothing when it has none.
func (e *enumerator) addConstructDrop(a action, operator Operator) {
	endIndex, elseIndex, ok := matchEnd(e.src.regions, a.index)
	if !ok {
		return
	}
	r := a.region
	text, end := e.src.text, e.src.regions[endIndex]
	replacement := ""
	if elseIndex >= 0 {
		els := e.src.regions[elseIndex]
		left := cmp.Or(e.src.leftDelim, "{{")
		content := text[trimLeft(text, els.start+len(left), els.innerEnd):els.innerEnd]
		if chained := strings.TrimLeft(strings.TrimPrefix(content, "else"), spaceChars); chained != "" {
			// {{else with .B}} is an else holding a second with, closed
			// by the same end. Dropping the first construct leaves the
			// second one standing, opened and closed.
			replacement = left + chained + text[els.innerEnd:end.end]
		} else {
			replacement = text[els.end:end.start]
		}
	}
	e.appendMutant(r, operator, edit{start: r.start, end: end.end, text: replacement})
}

// addTemplateDrop appends a mutant removing a {{template}} call.
func (e *enumerator) addTemplateDrop(r region) {
	if r.keyword != "template" {
		// A block defines its body in place, so removing its call
		// would leave the body and its {{end}} behind.
		return
	}
	e.appendMutant(r, OperatorTemplateDrop, edit{start: r.start, end: r.end})
}

// appendMutant records a mutant made of one substitution, described by
// what it substitutes.
func (e *enumerator) appendMutant(r region, operator Operator, change edit) {
	e.appendEdits(r, operator, []edit{change}, change.text)
}

// appendEdits records a mutant made of one or more substitutions.
func (e *enumerator) appendEdits(r region, operator Operator, edits []edit, detail string) {
	line, column := e.src.lines.at(e.src.fileOffset(r.start))
	e.mutants = append(e.mutants, Mutant{
		Operator: operator,
		Template: e.template,
		File:     e.src.file,
		Path:     e.src.path,
		src:      e.src,
		Line:     line,
		Column:   column,
		edits:    edits,
		detail:   detail,
		action:   e.src.text[r.start:r.end],
	})
}

// valueStart returns the offset of the value a pipeline assigns, which
// follows any variables it declares.
//
// The parser positions a command at its first token, so the first command
// starts exactly where the value is written. A pipeline always has one: an
// action with no command does not parse.
func valueStart(pipe *parse.PipeNode) int {
	return int(pipe.Cmds[0].Position())
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
		// Offsets are never negative and the first line starts at zero,
		// so the line before is always there.
		i--
	}
	return i + 1, offset - l[i] + 1
}
