// Ported from lib/lexer/error.js.

package csstree

import (
	"regexp"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

func defaultLoc() tokenizer.Position {
	return tokenizer.Position{Offset: 0, Line: 1, Column: 1}
}

// mismatchLocation is the result of locateMismatch.
type mismatchLocation struct {
	css            string
	mismatchOffset int
	mismatchLength int
	start          tokenizer.Position
	end            tokenizer.Position
}

func locateMismatch(matchResult *matchResult, node Node) mismatchLocation {
	tokens := matchResult.tokens
	longestMatch := matchResult.longestMatch
	var mismatchNode Node
	if longestMatch < len(tokens) {
		mismatchNode = tokens[longestMatch].node
	}
	var badNode Node
	if mismatchNode != node {
		badNode = mismatchNode
	}
	mismatchOffset := 0
	mismatchLength := 0
	entries := 0
	var css strings.Builder
	var start tokenizer.Position
	var end tokenizer.Position

	for i := 0; i < len(tokens); i++ {
		token := tokens[i].value

		if i == longestMatch {
			mismatchLength = len(token)
			mismatchOffset = css.Len()
		}

		if badNode != nil && tokens[i].node == badNode {
			if i <= longestMatch {
				entries++
			} else {
				entries = 0
			}
		}

		css.WriteString(token)
	}

	cssText := css.String()

	if longestMatch == len(tokens) || entries > 1 { // last
		lastNode := badNode
		if lastNode == nil {
			lastNode = node
		}
		if loc, ok := fromLoc(lastNode, "end"); ok {
			start = loc
		} else {
			start = buildLoc(defaultLoc(), cssText)
		}
		end = buildLoc(start, "")
	} else {
		if loc, ok := fromLoc(badNode, "start"); ok {
			start = loc
		} else {
			nodeStart, ok := fromLoc(node, "start")
			if !ok {
				nodeStart = defaultLoc()
			}
			start = buildLoc(nodeStart, cssText[:mismatchOffset])
		}
		if loc, ok := fromLoc(badNode, "end"); ok {
			end = loc
		} else {
			end = buildLoc(start, substr(cssText, mismatchOffset, mismatchLength))
		}
	}

	return mismatchLocation{
		css:            cssText,
		mismatchOffset: mismatchOffset,
		mismatchLength: mismatchLength,
		start:          start,
		end:            end,
	}
}

// fromLoc returns the start or end (point) of node's location; ok is false
// when node has no location (upstream null).
func fromLoc(node Node, point string) (tokenizer.Position, bool) {
	if node == nil {
		return tokenizer.Position{}, false
	}

	loc := node.Location()
	if loc == nil {
		return tokenizer.Position{}, false
	}

	if point == "start" {
		return buildLoc(loc.Start, ""), true
	}

	return buildLoc(loc.End, ""), true
}

var buildLocLineBreak = regexp.MustCompile(`\n|\r\n?|\f`)

func buildLoc(position tokenizer.Position, extra string) tokenizer.Position {
	loc := tokenizer.Position{
		Offset: position.Offset,
		Line:   position.Line,
		Column: position.Column,
	}

	if extra != "" {
		lines := buildLocLineBreak.Split(extra, -1)

		loc.Offset += len(extra)
		loc.Line += len(lines) - 1
		if len(lines) == 1 {
			loc.Column = loc.Column + len(extra)
		} else {
			loc.Column = len(lines[len(lines)-1]) + 1
		}
	}

	return loc
}

// SyntaxReferenceError reports a reference to an unknown at-rule,
// descriptor, property or type, or a bad syntax (upstream
// createCustomError('SyntaxReferenceError', ...)).
type SyntaxReferenceError struct {
	// Message is the error type followed by the reference in backquotes.
	Message string
	// Reference is the name that is unknown ("" when there is none).
	Reference string
}

func newSyntaxReferenceError(typ, referenceName string) *SyntaxReferenceError {
	message := typ
	if referenceName != "" {
		message += " `" + referenceName + "`"
	}

	return &SyntaxReferenceError{
		Message:   message,
		Reference: referenceName,
	}
}

// Error returns Message, the same text as upstream's error.message.
func (e *SyntaxReferenceError) Error() string {
	return e.Message
}

// SyntaxMatchError reports a value that does not match a syntax (upstream
// createCustomError('SyntaxMatchError', ...)).
//
// Offsets and columns count bytes.
type SyntaxMatchError struct {
	// Message is RawMessage followed by the syntax, the value and a caret
	// line pointing at the mismatch.
	Message string
	// RawMessage is the reason of the mismatch, e.g. "Mismatch".
	RawMessage string
	// Syntax is the definition syntax that was matched ("<generic>" for a
	// generic type).
	Syntax string
	// CSS is the text of the value's tokens.
	CSS string
	// MismatchOffset and MismatchLength locate the first unmatched token
	// in CSS.
	MismatchOffset int
	MismatchLength int
	// Offset, Line and Column are Loc.Start (upstream assigns the start
	// position's fields to the error).
	Offset int
	Line   int
	Column int
	// Loc is the mismatch's range in the source of the value; Source is
	// "<unknown>" when the value has no location.
	Loc Location
}

func newSyntaxMatchError(message string, syntax definitionsyntax.Node, node Node, matchResult *matchResult) *SyntaxMatchError {
	mismatch := locateMismatch(matchResult, node)

	err := &SyntaxMatchError{
		RawMessage:     message,
		Syntax:         "<generic>",
		CSS:            mismatch.css,
		MismatchOffset: mismatch.mismatchOffset,
		MismatchLength: mismatch.mismatchLength,
	}
	if syntax != nil {
		err.Syntax = definitionsyntax.Generate(syntax, definitionsyntax.GenerateOptions{})
	}
	value := mismatch.css
	if value == "" {
		value = "<empty string>"
	}
	err.Message = message + "\n" +
		"  syntax: " + err.Syntax + "\n" +
		"   value: " + value + "\n" +
		"  --------" + strings.Repeat("-", err.MismatchOffset) + "^"

	err.Offset = mismatch.start.Offset
	err.Line = mismatch.start.Line
	err.Column = mismatch.start.Column
	source := "<unknown>"
	if node != nil {
		if loc := node.Location(); loc != nil && loc.Source != "" {
			source = loc.Source
		}
	}
	err.Loc = Location{
		Source: source,
		Start:  mismatch.start,
		End:    mismatch.end,
	}

	return err
}

// Error returns Message, the same text as upstream's error.message.
func (e *SyntaxMatchError) Error() string {
	return e.Message
}
