// Package diagnostic holds what one problem muxt reports to a person is: where
// it is, what is wrong and what to do about it. It imports nothing of muxt, so
// every package that reports a problem can build one.
package diagnostic

import (
	"fmt"
	"strings"
)

// Severity ranks a diagnostic. The zero value is Error.
type Severity uint8

const (
	// Error is a problem that stops the command.
	Error Severity = iota
	// Warning is a problem that does not stop the command. String writes it
	// as it writes an Error; the command's writer adds "warning: ".
	Warning
)

// Diagnostic is one problem.
type Diagnostic struct {
	Severity Severity
	// Code is a stable identifier of the kind of problem, such as htt's
	// "unescaped-ampersand". Empty until a kind is given one.
	Code  string
	Place Place
	// Message says what is wrong in one line, without the place or the fix.
	Message string
	// Fix says what to do about it. Empty when there is nothing to offer.
	Fix string
	// Related are the other places of the same problem, each with its role:
	// "first defined here", the second entry of a clash.
	Related []Related
}

// Place is where a problem is.
type Place struct {
	File               string // absolute path; empty when the problem is in no file
	Line, Column       int    // 1-based; zero when unknown
	EndLine, EndColumn int    // zero when the place is a point
	// Within is a place inside the file that is not a line: packages["KEY"][0],
	// a template name.
	Within string
}

// Related is another place of the same problem and its role in it.
type Related struct {
	Place   Place
	Message string
}

// String writes the diagnostic as one line,
// FILE:LINE:COLUMN: WITHIN: MESSAGE; FIX, each part and its separator left out
// when empty.
func (d Diagnostic) String() string {
	s := d.Message
	if place := d.Place.String(); place != "" {
		s = place + ": " + s
	}
	if d.Fix != "" {
		s += "; " + d.Fix
	}
	return s
}

// String writes the place as FILE:LINE:COLUMN, or FILE when the line is
// unknown, followed by ": WITHIN" when the place is inside the file. Without a
// file it starts at LINE:COLUMN, as token.Position does, since a template
// parsed from a string has a line but no file; with neither it is WITHIN
// alone, or empty.
func (p Place) String() string {
	var parts []string
	switch {
	case p.File != "" && p.Line != 0:
		parts = append(parts, fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column))
	case p.File != "":
		parts = append(parts, p.File)
	case p.Line != 0:
		parts = append(parts, fmt.Sprintf("%d:%d", p.Line, p.Column))
	}
	if p.Within != "" {
		parts = append(parts, p.Within)
	}
	return strings.Join(parts, ": ")
}
