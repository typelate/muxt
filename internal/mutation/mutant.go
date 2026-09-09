package mutation

import (
	"cmp"
	"go/types"
	"slices"
	"strings"
	"text/template/parse"
)

// Operator names the variation applied to one action.
//
// No operator renames a template or changes which templates exist, so a
// mutant never moves a route: only what a template does with its data
// changes.
const (
	// OperatorActionEmpty makes an action print nothing, standing in for
	// the value it prints being absent. A test that never looks at the
	// printed value survives it.
	//
	// It is used where the type of the value could not be resolved from
	// dot, so an empty string is the only substitution available.
	OperatorActionEmpty = "action-empty"

	// OperatorActionZero replaces the value an action prints with the
	// zero value of its own type: an empty string for a string, 0 for a
	// count, false for a flag.
	//
	// It stays closer to a real defect than substituting an empty string
	// regardless of type, and it says in the report that the type was
	// known.
	OperatorActionZero = "action-zero"

	// OperatorIfTrue takes the then branch unconditionally.
	OperatorIfTrue = "if-true"

	// OperatorIfFalse takes the else branch, or no branch at all,
	// unconditionally.
	OperatorIfFalse = "if-false"

	// OperatorWithEmpty makes a with behave as though its value were
	// absent, so the body never runs and the else branch does. Like
	// range-never it replaces the whole construct.
	//
	// Replacing the pipeline with false would do it at run time, but it
	// also rebinds dot to a boolean, so every field access in the body
	// stops type checking and the mutant would be skipped as broken
	// rather than run as a behaviour change.
	OperatorWithEmpty = "with-empty"

	// OperatorRangeNever makes a range iterate zero times, replacing the
	// whole construct with its else branch.
	//
	// It cannot be done by replacing the pipeline: there is no literal
	// for an empty sequence, and while ranging over the literal 0
	// iterates zero times, text/template refuses that for a range
	// declaring more than one variable.
	OperatorRangeNever = "range-never"

	// OperatorTemplateDrop removes a {{template}} call, standing in for
	// the partial rendering nothing. It does not apply to {{block}},
	// whose call cannot be removed without orphaning its {{end}}.
	OperatorTemplateDrop = "template-drop"
)

// Mutant is one variation of one action in one template.
type Mutant struct {
	// Operator is the variation applied.
	Operator string

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

	// start and end bound the template text the mutation replaces. For a
	// pipeline variation that is the pipeline; for a structural one it
	// is the whole construct, opening action through matching end.
	start, end int

	// replacement is the text substituted for that range.
	replacement string

	// action is the opening action as it is written, which is what a
	// report shows so that a whole range body does not land in it.
	action string

	// src is the file the template text was read from, which knows how
	// to put mutated text back into it.
	src *templateSource
}

// Apply returns the whole file with the mutation in place.
func (m Mutant) Apply() string {
	return m.src.apply(m.start, m.end, m.replacement)
}

// Action returns the mutated action as it is written in the template.
func (m Mutant) Action() string { return m.action }

// Replacement returns the text the mutation substitutes.
func (m Mutant) Replacement() string { return m.replacement }

// mutantsInScope enumerates every mutation available in one template,
// rendered with the type of dot its scope carries.
func mutantsInScope(sc scope) []Mutant {
	ctx := mutantContext{
		src:      sc.src,
		template: sc.template,
		regions:  sc.src.regions,
		dot:      sc.dataType,
	}

	var all []Mutant
	collect(&all, sc.tree.Root, ctx)

	slices.SortFunc(all, func(a, b Mutant) int {
		return cmp.Or(
			cmp.Compare(a.start, b.start),
			cmp.Compare(a.Operator, b.Operator),
		)
	})
	return all
}

type mutantContext struct {
	src      *templateSource
	template string
	regions  []region
	dot      types.Type
}

// narrowed returns the context for a body where dot has changed, as it
// does inside a range or a with.
func (ctx mutantContext) narrowed(dot types.Type) mutantContext {
	ctx.dot = dot
	return ctx
}

// collect walks a parse tree and appends a mutant for every action a
// variation applies to.
func collect(out *[]Mutant, node parse.Node, ctx mutantContext) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			collect(out, child, ctx)
		}
	case *parse.ActionNode:
		if zero, typed := zeroLiteral(ctx.dot, n.Pipe); typed {
			ctx.addPipeline(out, n.Pipe, OperatorActionZero, zero)
		} else {
			ctx.addPipeline(out, n.Pipe, OperatorActionEmpty, `""`)
		}
	case *parse.IfNode:
		ctx.addPipeline(out, n.Pipe, OperatorIfTrue, "true")
		ctx.addPipeline(out, n.Pipe, OperatorIfFalse, "false")
		collect(out, n.List, ctx)
		collect(out, n.ElseList, ctx)
	case *parse.WithNode:
		ctx.addConstructDrop(out, int(n.Position()), OperatorWithEmpty)
		// Inside the body, dot is what the with selected.
		collect(out, n.List, ctx.narrowed(withDot(ctx.dot, n.Pipe)))
		collect(out, n.ElseList, ctx)
	case *parse.RangeNode:
		ctx.addConstructDrop(out, int(n.Position()), OperatorRangeNever)
		// Inside the body, dot is one element of what was ranged over.
		collect(out, n.List, ctx.narrowed(rangeDot(ctx.dot, n.Pipe)))
		collect(out, n.ElseList, ctx)
	case *parse.TemplateNode:
		ctx.addTemplateDrop(out, int(n.Position()))
	}
}

// addPipeline appends a mutant replacing the value a pipeline evaluates.
//
// A pipeline that declares variables keeps its declarations, so that
// references to them elsewhere in the template still resolve; only the
// value assigned changes.
func (ctx mutantContext) addPipeline(out *[]Mutant, pipe *parse.PipeNode, operator, replacement string) {
	if pipe == nil {
		return
	}
	_, r, ok := regionAt(ctx.regions, int(pipe.Position()))
	if !ok {
		return
	}
	start := ctx.valueStart(pipe, r)
	if start >= r.innerEnd {
		return
	}
	ctx.appendMutant(out, r, operator, start, r.innerEnd, replacement)
}

// addConstructDrop appends a mutant replacing a whole construct with its
// else branch, or with nothing when it has none.
func (ctx mutantContext) addConstructDrop(out *[]Mutant, pos int, operator string) {
	index, r, ok := regionAt(ctx.regions, pos)
	if !ok {
		return
	}
	endIndex, elseIndex, ok := matchEnd(ctx.regions, index)
	if !ok {
		return
	}
	replacement := ""
	if elseIndex >= 0 {
		replacement = ctx.src.text[ctx.regions[elseIndex].end:ctx.regions[endIndex].start]
	}
	ctx.appendMutant(out, r, operator, r.start, ctx.regions[endIndex].end, replacement)
}

// addTemplateDrop appends a mutant removing a {{template}} call.
func (ctx mutantContext) addTemplateDrop(out *[]Mutant, pos int) {
	_, r, ok := regionAt(ctx.regions, pos)
	if !ok || r.keyword != "template" {
		// A block defines its body in place, so removing its call
		// would leave the body and its {{end}} behind.
		return
	}
	ctx.appendMutant(out, r, OperatorTemplateDrop, r.start, r.end, "")
}

func (ctx mutantContext) appendMutant(out *[]Mutant, r region, operator string, start, end int, replacement string) {
	if start < 0 || end > len(ctx.src.text) || start > end {
		return
	}
	line, column := ctx.src.lines.at(ctx.src.fileOffset(r.start))
	*out = append(*out, Mutant{
		Operator:    operator,
		Template:    ctx.template,
		File:        ctx.src.file,
		Path:        ctx.src.path,
		src:         ctx.src,
		Line:        line,
		Column:      column,
		start:       start,
		end:         end,
		replacement: replacement,
		action:      ctx.src.text[r.start:r.end],
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
