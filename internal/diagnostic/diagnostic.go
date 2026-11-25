// Package diagnostic holds what one problem muxt reports to a person is: where
// it is, what is wrong and what to do about it. It imports nothing of muxt, so
// every package that reports a problem can build one.
package diagnostic

import "fmt"

// Severity ranks a diagnostic. The zero value is Error.
type Severity uint8

const (
	Error Severity = iota
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

// String writes the diagnostic as one line: FILE:LINE:COLUMN: MESSAGE; FIX.
func (d Diagnostic) String() string {
	s := fmt.Sprintf("%s: %s", d.Place, d.Message)
	if d.Fix != "" {
		s += "; " + d.Fix
	}
	return s
}

// String writes the place as FILE:LINE:COLUMN, or FILE when the line is unknown.
func (p Place) String() string {
	if p.Line == 0 {
		return p.File
	}
	return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
}
