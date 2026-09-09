package mutation

import (
	"cmp"
	"slices"
	"strings"
	"text/template/parse"
)

// Operator names the variation applied to one action.
//
// Every operator rewrites a pipeline in place, leaving the surrounding
// template text and every template's name untouched, so a mutant never
// moves a route or changes which templates exist.
const (
	// OperatorActionEmpty makes an action print nothing, standing in for
	// the value it prints being absent. A test that never looks at the
	// printed value survives it.
	OperatorActionEmpty = "action-empty"

	// OperatorIfTrue takes the then branch unconditionally.
	OperatorIfTrue = "if-true"

	// OperatorIfFalse takes the else branch, or no branch, unconditionally.
	OperatorIfFalse = "if-false"
)

// Status is the verdict on one mutant.
type Status string

const (
	// StatusKilled means the tests failed while the mutation was in
	// place, which is the outcome to want: something asserts on the
	// behaviour the mutated action controls.
	StatusKilled Status = "KILL"

	// StatusMissed means the tests still passed, so nothing observes
	// what the action does.
	StatusMissed Status = "MISS"
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

	// Line and Column locate the pipeline in File, both one based, with
	// Column counting bytes.
	Line, Column int

	// start and end bound the pipeline within the file's text.
	start, end int

	// replacement is the pipeline text the mutation substitutes.
	replacement string
}

// Apply returns the file's text with the mutation in place.
func (m Mutant) Apply(text string) string {
	return text[:m.start] + m.replacement + text[m.end:]
}

// Pipeline returns the pipeline text the mutation replaces, which is what
// a report shows as the mutated source.
func (m Mutant) Pipeline(text string) string {
	if m.start < 0 || m.end > len(text) || m.start > m.end {
		return ""
	}
	return text[m.start:m.end]
}

// Replacement returns the pipeline text the mutation substitutes.
func (m Mutant) Replacement() string { return m.replacement }

// mutantsInFile enumerates every mutation available in the templates that
// text defines.
//
// rootName is the name text's own template carries, which for a template
// file is the file's base name.
func mutantsInFile(file, path, rootName, text string, funcs map[string]any, include func(string) bool) ([]Mutant, error) {
	trees, err := parse.Parse(rootName, text, "", "", funcs)
	if err != nil {
		return nil, err
	}

	found := regions(text, "", "")
	lines := newLineIndex(text)

	var all []Mutant
	for name, tree := range trees {
		if tree == nil || tree.Root == nil {
			continue
		}
		if include != nil && !include(name) {
			continue
		}
		collect(&all, tree.Root, mutantContext{
			file:     file,
			path:     path,
			template: name,
			regions:  found,
			lines:    lines,
		})
	}

	slices.SortFunc(all, func(a, b Mutant) int {
		return cmp.Or(
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.start, b.start),
			cmp.Compare(a.Operator, b.Operator),
			cmp.Compare(a.Template, b.Template),
		)
	})
	return all, nil
}

type mutantContext struct {
	file     string
	path     string
	template string
	regions  []region
	lines    lineIndex
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
		ctx.add(out, n.Pipe, OperatorActionEmpty, `""`)
	case *parse.IfNode:
		ctx.add(out, n.Pipe, OperatorIfTrue, "true")
		ctx.add(out, n.Pipe, OperatorIfFalse, "false")
		collect(out, n.List, ctx)
		collect(out, n.ElseList, ctx)
	case *parse.RangeNode:
		collect(out, n.List, ctx)
		collect(out, n.ElseList, ctx)
	case *parse.WithNode:
		collect(out, n.List, ctx)
		collect(out, n.ElseList, ctx)
	}
}

// add appends one mutant for pipe, unless the pipeline cannot be varied
// without breaking the template around it.
func (ctx mutantContext) add(out *[]Mutant, pipe *parse.PipeNode, operator, replacement string) {
	if pipe == nil {
		return
	}
	if len(pipe.Decl) > 0 {
		// The pipeline declares variables the rest of the template
		// refers to. Replacing it would leave those references
		// undefined, which is a broken template rather than a
		// behaviour change worth testing for.
		return
	}
	start := int(pipe.Position())
	end, ok := pipelineEnd(ctx.regions, start)
	if !ok || end <= start {
		return
	}
	line, column := ctx.lines.at(start)
	*out = append(*out, Mutant{
		Operator:    operator,
		Template:    ctx.template,
		File:        ctx.file,
		Path:        ctx.path,
		Line:        line,
		Column:      column,
		start:       start,
		end:         end,
		replacement: replacement,
	})
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

// functionNames adapts the names a template set may call into the shape
// text/template/parse wants, which checks only that a name is known.
func functionNames(names []string) map[string]any {
	funcs := make(map[string]any, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		funcs[name] = func() string { return "" }
	}
	return funcs
}
