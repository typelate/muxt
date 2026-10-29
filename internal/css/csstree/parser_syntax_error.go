// Ported from lib/parser/SyntaxError.js and lib/utils/create-custom-error.js.

package csstree

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	maxLineLength    = 100
	offsetCorrection = 60
	tabReplacement   = "    "
)

// SyntaxError is a CSS parse error (upstream parse.SyntaxError, built with
// createCustomError('SyntaxError', message)).
//
// Offset includes the offset passed in ParseOptions; Line and Column are
// 1-based, Column counts bytes.
type SyntaxError struct {
	Message string
	Source  string
	Offset  int
	Line    int
	Column  int

	baseLine   int
	baseColumn int
}

// Error returns Message, the same text as upstream's error.message.
func (e *SyntaxError) Error() string {
	return e.Message
}

// SourceFragment returns the source lines around the error with a caret
// line under Column, with extraLines lines of context before and after.
func (e *SyntaxError) SourceFragment(extraLines int) string {
	return sourceFragment(e.Source, e.Line, e.Column, e.baseLine, e.baseColumn, extraLines)
}

// FormattedMessage returns the message followed by a source fragment with
// two lines of context.
func (e *SyntaxError) FormattedMessage() string {
	return "Parse error: " + e.Message + "\n" +
		sourceFragment(e.Source, e.Line, e.Column, e.baseLine, e.baseColumn, 2)
}

func newSyntaxError(message, source string, offset, line, column, baseLine, baseColumn int) *SyntaxError {
	return &SyntaxError{
		Message:    message,
		Source:     source,
		Offset:     offset,
		Line:       line,
		Column:     column,
		baseLine:   baseLine,
		baseColumn: baseColumn,
	}
}

var (
	sourceFragmentLineBreak = regexp.MustCompile(`\r\n?|\n|\f`)
	sourceFragmentLeading   = regexp.MustCompile(`^(\s+\d+\s+\|\n)+`)
	sourceFragmentTrailing  = regexp.MustCompile(`\n(\s+\d+\s+\|)+$`)
)

func sourceFragment(source string, line, column, baseLine, baseColumn, extraLines int) string {
	prelines := strings.Repeat("\n", max(baseLine-1, 0))
	precolumns := strings.Repeat(" ", max(baseColumn-1, 0))
	lines := sourceFragmentLineBreak.Split(prelines+precolumns+source, -1)
	startLine := max(1, line-extraLines) - 1
	endLine := min(line+extraLines, len(lines)+1)
	maxNumLength := max(4, len(strconv.Itoa(endLine))) + 1
	cutLeft := 0

	processLines := func(start, end int) string {
		end = min(end, len(lines))
		if start >= end {
			return ""
		}
		var result []string
		for idx, line := range lines[start:end] {
			num := strconv.Itoa(start + idx + 1)
			result = append(result, strings.Repeat(" ", max(maxNumLength-len(num), 0))+num+" |"+line)
		}
		return strings.Join(result, "\n")
	}

	// column correction according to replaced tab before column
	column += (len(tabReplacement) - 1) * strings.Count(substr(lines[line-1], 0, column-1), "\t")

	if column > maxLineLength {
		cutLeft = column - offsetCorrection + 3
		column = offsetCorrection - 2
	}

	for i := startLine; i <= endLine; i++ {
		if i >= 0 && i < len(lines) {
			lines[i] = strings.ReplaceAll(lines[i], "\t", tabReplacement)
			prefix := ""
			if cutLeft > 0 && len(lines[i]) > cutLeft {
				prefix = "…"
			}
			suffix := ""
			if len(lines[i]) > cutLeft+maxLineLength-1 {
				suffix = "…"
			}
			lines[i] = prefix + substr(lines[i], cutLeft, maxLineLength-2) + suffix
		}
	}

	var parts []string
	for _, part := range []string{
		processLines(startLine, line),
		strings.Repeat("-", column+maxNumLength+2-1) + "^",
		processLines(line, endLine),
	} {
		if part != "" {
			parts = append(parts, part)
		}
	}

	result := strings.Join(parts, "\n")
	result = sourceFragmentLeading.ReplaceAllString(result, "")
	result = sourceFragmentTrailing.ReplaceAllString(result, "")
	return result
}

// substr mirrors String.prototype.substr(start, length) for non-negative
// arguments.
func substr(s string, start, length int) string {
	if start >= len(s) || length <= 0 {
		return ""
	}
	return s[start:min(start+length, len(s))]
}
