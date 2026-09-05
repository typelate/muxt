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

// MultiLineError renders the template name with a marker under the
// failing segment, then the short form, then the related locations.
// Offset and Length are byte ranges; the marker is measured in runes
// so it lines up under multi-byte characters.
func (e *NameError) MultiLineError() string {
	offset := min(max(e.Offset, 0), len(e.Name))
	end := min(offset+max(e.Length, 1), len(e.Name))
	pad := utf8.RuneCountInString(e.Name[:offset])
	width := max(utf8.RuneCountInString(e.Name[offset:end]), 1)
	var sb strings.Builder
	fmt.Fprintf(&sb, "  %s\n  %s%s\n", e.Name, strings.Repeat(" ", pad), strings.Repeat("^", width))
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
		if i < 0 || i+1 >= len(idx) {
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

// findIdent returns the identifier named name among the call's
// arguments, searching nested calls, or nil.
func findIdent(call *ast.CallExpr, name string) ast.Node {
	if call == nil {
		return nil
	}
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			if arg.Name == name {
				return arg
			}
		case *ast.CallExpr:
			if node := findIdent(arg, name); node != nil {
				return node
			}
		}
	}
	return nil
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

// pathParamErrorf reports an error about the occurrence-th path
// parameter named n, pointing at the name inside its braces.
func (def *Definition) pathParamErrorf(n string, occurrence int, format string, args ...any) error {
	needle := "{" + n
	offset := def.spans.path[0]
	rest := def.path
	for i := 0; ; i++ {
		idx := strings.Index(rest, needle)
		if idx < 0 {
			return def.spanErrorf(def.spans.path, format, args...)
		}
		if i == occurrence {
			return def.nameErrorf(offset+idx+1, len(n), format, args...)
		}
		offset += idx + len(needle)
		rest = rest[idx+len(needle):]
	}
}

// handlerSpan spans the trimmed handler expression within the name,
// falling back to the matched call segment when there is no handler.
func (def *Definition) handlerSpan() [2]int {
	if def.handler == "" {
		return def.spans.call
	}
	return [2]int{def.handlerOffset, def.handlerOffset + len(def.handler)}
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
			start := def.handlerOffset + def.fileSet.Position(pe.pos).Column - 1
			length := def.fileSet.Position(pe.end).Column - def.fileSet.Position(pe.pos).Column
			ne = &NameError{Name: def.name, Offset: start, Length: length, err: pe.err}
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
