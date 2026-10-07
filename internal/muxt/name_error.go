package muxt

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
	"unicode/utf8"
)

// MultiLineError is implemented by errors that carry a verbose
// multi-line rendering in addition to the single-line Error string.
// Printers show MultiLineError when the output has room for detail —
// source excerpts, markers, one location per line — and fall back to
// Error wherever a single line must do. The rendering has no trailing
// newline.
type MultiLineError interface {
	error
	MultiLineError() string
}

// NameError locates an error in a route template name. Error is the
// short single-line form, prefixed with the position of the failing
// segment inside the file that defines the template; MultiLineError
// renders the long form: the template name with a marker under the
// failing segment, the short form, and any related source locations.
type NameError struct {
	// Position locates the failing segment inside the file that defines
	// the template. It is the zero Position when the definition's
	// location is unknown.
	Position token.Position

	// SourceFile names the defining file when only the file is known.
	SourceFile string

	// Name is the full template name; Offset and Length span the
	// failing segment within it.
	Name   string
	Offset int
	Length int

	// Also lists further [start, end) byte ranges of Name marked along
	// with the failing segment, such as other uses of the same argument.
	Also [][2]int

	// Related lists source positions that give the error context, such
	// as where the handler method is defined. Each entry is a complete
	// "file:line:col: note" line.
	Related []string

	err error
}

func (e *NameError) Unwrap() error { return e.err }

func (e *NameError) Error() string {
	switch {
	case e.Position.IsValid():
		return fmt.Sprintf("%s: %v", e.Position, e.err)
	case e.SourceFile != "":
		return fmt.Sprintf("%s: %v", e.SourceFile, e.err)
	default:
		return e.err.Error()
	}
}

// MultiLineError renders the template name with markers under the
// failing segment and any Also ranges, then the short form, then the
// related locations. Offsets are byte ranges; markers are measured in
// runes so they line up under multi-byte characters.
func (e *NameError) MultiLineError() string {
	clamp := func(i int) int { return min(max(i, 0), len(e.Name)) }
	marker := []rune(strings.Repeat(" ", utf8.RuneCountInString(e.Name)+1))
	mark := func(start, end, minWidth int) {
		pad := utf8.RuneCountInString(e.Name[:start])
		width := max(utf8.RuneCountInString(e.Name[start:end]), minWidth)
		for i := pad; i < pad+width; i++ {
			marker[i] = '^'
		}
	}
	offset := clamp(e.Offset)
	mark(offset, min(offset+max(e.Length, 1), len(e.Name)), 1)
	for _, span := range e.Also {
		start := clamp(span[0])
		mark(start, max(start, clamp(span[1])), 0)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "  %s\n  %s\n", e.Name, strings.TrimRight(string(marker), " "))
	sb.WriteString(e.Error())
	for _, related := range e.Related {
		sb.WriteString("\n")
		sb.WriteString(related)
	}
	return sb.String()
}

// nameSpans records the byte offsets of the matched template name
// segments; a segment that did not match spans [-1, -1).
type nameSpans struct {
	method, host, path, status, call [2]int
}

func newNameSpans(idx []int) nameSpans {
	span := func(group string) [2]int {
		i := 2 * templateNameMux.SubexpIndex(group)
		if i+1 >= len(idx) {
			return [2]int{-1, -1}
		}
		return [2]int{idx[i], idx[i+1]}
	}
	return nameSpans{
		method: span("METHOD"),
		host:   span("HOST"),
		path:   span("PATH"),
		status: span("HTTP_STATUS"),
		call:   span("CALL"),
	}
}

// nameErrorf reports an error about the segment of the template name
// spanning length bytes from offset.
func (def *Definition) nameErrorf(offset, length int, format string, args ...any) error {
	return &NameError{Name: def.name, Offset: offset, Length: length, err: fmt.Errorf(format, args...)}
}

// spanErrorf reports an error about one of the matched name segments.
func (def *Definition) spanErrorf(span [2]int, format string, args ...any) error {
	if span[0] < 0 {
		return def.nameErrorf(0, len(def.name), format, args...)
	}
	return def.nameErrorf(span[0], span[1]-span[0], format, args...)
}

// positionedError carries a position inside the parsed handler
// expression until finishNameError can translate it into a name offset.
type positionedError struct {
	pos, end token.Pos
	also     []ast.Node
	err      error
}

func (e *positionedError) Error() string { return e.err.Error() }
func (e *positionedError) Unwrap() error { return e.err }

// errAt reports an error about the handler expression node.
func errAt(node ast.Node, format string, args ...any) error {
	return &positionedError{pos: node.Pos(), end: node.End(), err: fmt.Errorf(format, args...)}
}

// errAtNode positions err at node unless it already carries a position.
func errAtNode(node ast.Node, err error) error {
	if err == nil {
		return nil
	}
	switch err.(type) {
	case *positionedError, *NameError:
		return err
	}
	return &positionedError{pos: node.Pos(), end: node.End(), err: err}
}

// findIdent returns the first identifier named name among the call's
// arguments, searching nested calls, or nil.
func findIdent(call *ast.CallExpr, name string) ast.Node {
	if nodes := findIdents(call, name); len(nodes) > 0 {
		return nodes[0]
	}
	return nil
}

// findIdents returns every identifier named name among the call's
// arguments, depth first, searching nested calls.
func findIdents(call *ast.CallExpr, name string) []ast.Node {
	if call == nil {
		return nil
	}
	var nodes []ast.Node
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			if arg.Name == name {
				nodes = append(nodes, arg)
			}
		case *ast.CallExpr:
			nodes = append(nodes, findIdents(arg, name)...)
		}
	}
	return nodes
}

// argErrorf reports an error about the call argument named name,
// falling back to an unpositioned error when the argument cannot be
// found in the handler expression.
func (def *Definition) argErrorf(name, format string, args ...any) error {
	if node := findIdent(def.call, name); node != nil {
		return errAt(node, format, args...)
	}
	return fmt.Errorf(format, args...)
}

// argUsesErrorf reports an error about the call argument named name,
// marking every place the call passes it.
func (def *Definition) argUsesErrorf(name, format string, args ...any) error {
	nodes := findIdents(def.call, name)
	if len(nodes) == 0 {
		return fmt.Errorf(format, args...)
	}
	return &positionedError{pos: nodes[0].Pos(), end: nodes[0].End(), also: nodes[1:], err: fmt.Errorf(format, args...)}
}

// pathParamErrorf reports an error about the occurrence-th path
// parameter named n, pointing at the name inside its braces. The name must
// be whole -- {n}, {n...}, or an unclosed {n ending its segment -- so the
// error about {form} does not mark {formx}.
func (def *Definition) pathParamErrorf(n string, occurrence int, format string, args ...any) error {
	needle := "{" + n
	for i, start := 0, 0; ; {
		idx := strings.Index(def.path[start:], needle)
		if idx < 0 {
			return def.spanErrorf(def.spans.path, format, args...)
		}
		nameStart := start + idx + 1
		start = nameStart + len(n)
		if !endsPathParameterName(def.path[start:]) {
			continue
		}
		if i == occurrence {
			return def.nameErrorf(def.spans.path[0]+nameStart, len(n), format, args...)
		}
		i++
	}
}

// endsPathParameterName reports whether rest, the path after a parameter
// name, starts with what can follow a whole name.
func endsPathParameterName(rest string) bool {
	return rest == "" || strings.HasPrefix(rest, "}") || strings.HasPrefix(rest, "...}") || strings.HasPrefix(rest, "/")
}

// handlerSpan spans the trimmed handler expression within the name,
// falling back to the matched call segment when there is no handler.
func (def *Definition) handlerSpan() [2]int {
	if def.handler == "" {
		return def.spans.call
	}
	return [2]int{def.handlerOffset, def.handlerOffset + len(def.handler)}
}

// handlerNodeSpan translates a position range in the parsed handler
// expression into a [start, end) byte range of the template name.
func (def *Definition) handlerNodeSpan(pos, end token.Pos) [2]int {
	start := def.handlerOffset + def.fileSet.Position(pos).Column - 1
	return [2]int{start, start + def.fileSet.Position(end).Column - def.fileSet.Position(pos).Column}
}

// finishNameError gives err the definition's location: a positioned
// handler-expression error is translated to its offset within the name,
// any other error spans the segment given by fallback, and the
// definition's recorded name position and source file fill the prefix.
func (def *Definition) finishNameError(err error, fallback [2]int) error {
	if err == nil {
		return nil
	}
	ne, ok := err.(*NameError)
	if !ok {
		if pe, isPositioned := err.(*positionedError); isPositioned && def.fileSet != nil {
			span := def.handlerNodeSpan(pe.pos, pe.end)
			ne = &NameError{Name: def.name, Offset: span[0], Length: span[1] - span[0], err: pe.err}
			for _, node := range pe.also {
				ne.Also = append(ne.Also, def.handlerNodeSpan(node.Pos(), node.End()))
			}
		} else {
			ne = &NameError{Name: def.name, err: err}
			if fallback[0] >= 0 {
				ne.Offset, ne.Length = fallback[0], fallback[1]-fallback[0]
			} else {
				ne.Length = len(def.name)
			}
		}
	}
	if def.namePosition.IsValid() {
		pos := def.namePosition
		pos.Column += ne.Offset
		pos.Offset += ne.Offset
		ne.Position = pos
	}
	ne.SourceFile = def.sourceFile
	ne.Related = append(ne.Related, def.related...)
	return ne
}

// ErrorList joins several template errors so one run reports them all.
// Error stays a single line naming the first failure; MultiLineError
// renders each member's verbose form separated by blank lines.
type ErrorList []error

func (l ErrorList) Error() string {
	switch len(l) {
	case 1:
		return l[0].Error()
	case 2:
		return fmt.Sprintf("%s (and 1 more error)", l[0].Error())
	default:
		return fmt.Sprintf("%s (and %d more errors)", l[0].Error(), len(l)-1)
	}
}

func (l ErrorList) Unwrap() []error { return l }

// MultiLineError renders every member's verbose form.
func (l ErrorList) MultiLineError() string {
	var sb strings.Builder
	for i, err := range l {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		if multiLine, ok := err.(MultiLineError); ok {
			sb.WriteString(multiLine.MultiLineError())
		} else {
			sb.WriteString(err.Error())
		}
	}
	return sb.String()
}

// CombineErrors returns nil for no errors, the error itself for one,
// and an ErrorList for several, so single-error runs keep their
// original type for errors.As.
func CombineErrors(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		return ErrorList(errs)
	}
}
