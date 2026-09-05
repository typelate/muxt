package muxt

import (
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"strings"
)

// NameError locates an error in a route template name. Error is the
// short single-line form, prefixed with the position of the failing
// segment inside the file that defines the template; DetailedError
// writes the long form, the template name with a marker under the
// failing segment.
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

// DetailedError writes the template name with a marker under the
// failing segment.
func (e *NameError) DetailedError(w io.Writer) error {
	offset := min(max(e.Offset, 0), len(e.Name))
	length := max(e.Length, 1)
	if offset+length > len(e.Name) {
		length = max(len(e.Name)-offset, 1)
	}
	_, err := fmt.Fprintf(w, "  %s\n  %s%s\n", e.Name, strings.Repeat(" ", offset), strings.Repeat("^", length))
	return err
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
	return ne
}
