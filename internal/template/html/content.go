package html

import (
	"html/template"
	"strconv"
)

// CSS encapsulates known safe content that matches any of:
//  1. The CSS3 stylesheet production, such as `p { color: purple }`.
//  2. The CSS3 rule production, such as `a[href=~"https:"].foo#bar`.
//  3. CSS3 declaration productions, such as `color: red; margin: 2px`.
//  4. The CSS3 value production, such as `rgba(0, 0, 255, 127)`.
type CSS = template.CSS

// HTML encapsulates a known safe HTML document fragment.
// It should not be used for HTML from a third-party, or HTML with
// unclosed tags or comments.
type HTML = template.HTML

// HTMLAttr encapsulates an HTML attribute from a trusted source.
type HTMLAttr = template.HTMLAttr

// JS encapsulates a known safe EcmaScript5 Expression.
type JS = template.JS

// JSStr encapsulates a sequence of characters meant to be embedded
// between quotes in a JavaScript expression.
type JSStr = template.JSStr

// Srcset encapsulates a known safe srcset attribute.
type Srcset = template.Srcset

// URL encapsulates a known safe URL or URL substring.
type URL = template.URL

// ErrorCode is an error classification for template errors.
type ErrorCode = template.ErrorCode

const (
	// OK indicates the lack of an error.
	OK ErrorCode = iota

	// ErrAmbigContext: "... appears in an ambiguous context within a URL"
	ErrAmbigContext

	// ErrBadHTML: "expected space, attr name, or end of tag, but got ..."
	ErrBadHTML

	// ErrBranchEnd: "{{if}} branches end in different contexts"
	ErrBranchEnd

	// ErrEndContext: "... ends in a non-text context: ..."
	ErrEndContext

	// ErrNoSuchTemplate: "no such template ..."
	ErrNoSuchTemplate

	// ErrOutputContext: "cannot compute output context for template ..."
	ErrOutputContext

	// ErrPartialCharset: "unfinished JS regexp charset in ..."
	ErrPartialCharset

	// ErrPartialEscape: "unfinished escape sequence in ..."
	ErrPartialEscape

	// ErrRangeLoopReentry: "on range loop re-entry: ..."
	ErrRangeLoopReentry

	// ErrSlashAmbig: "/ could start a division or regexp: ..."
	ErrSlashAmbig

	// ErrPredefinedEscaper: "predefined escaper ... disallowed in template"
	ErrPredefinedEscaper

	// ErrJSTemplate: "... appears in a JS template literal"
	ErrJSTemplate
)

// Error describes a problem encountered during template escaping.
type Error struct {
	// ErrorCode describes the kind of error.
	ErrorCode ErrorCode
	// Node is the node that caused the problem, if known.
	// If not nil, it overrides Name and Line.
	Node interface{ String() string }
	// Name is the name of the template in which the error was encountered.
	Name string
	// Line is the line number of the error in the template source or 0.
	Line int
	// Description is a human-readable description of the problem.
	Description string
}

func (e *Error) Error() string {
	switch {
	case e.Node != nil:
		return "html/template:" + e.Name + ": " + e.Description
	case e.Line != 0:
		return "html/template:" + e.Name + ":" + strconv.Itoa(e.Line) + ": " + e.Description
	case e.Name != "":
		return "html/template:" + e.Name + ": " + e.Description
	default:
		return "html/template: " + e.Description
	}
}
