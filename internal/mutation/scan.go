package mutation

import (
	"strings"
	"unicode"
)

// region is one {{...}} action as it is written in a template's text.
//
// text/template/parse does not report where an action's delimiters are:
// a node's Pos is the position of its pipeline, so {{.Name}} reports the
// offset of .Name and {{if .Loud}} the offset of .Loud. Replacing a
// pipeline therefore needs the action's right delimiter, which only the
// text itself can supply.
type region struct {
	// start is the offset of the left delimiter.
	start int

	// innerEnd is the offset one past the last byte of the action's
	// content, with a trailing trim marker and the whitespace before it
	// excluded. A pipeline always runs to here, because the pipeline is
	// the last thing an action holds.
	innerEnd int

	// end is the offset one past the right delimiter.
	end int

	// keyword is the word the action opens with, one of if, range,
	// with, block, define, else, end or template, and empty for an
	// action that just evaluates a pipeline.
	keyword string
}

// opensBlock reports whether an action of this kind is closed by a
// matching {{end}}.
//
// An else does not open anything, including the "else if" form, which
// text/template folds into the enclosing if rather than nesting a second
// construct with an end of its own.
func (r region) opensBlock() bool {
	switch r.keyword {
	case "if", "range", "with", "block", "define":
		return true
	default:
		return false
	}
}

// regions splits text into the actions it holds, in source order.
//
// A right delimiter inside a quoted string does not end an action, so
// "{{printf \"}}\"}}" is one region and not two. Comments are scanned as
// a unit for the same reason.
func regions(text, leftDelim, rightDelim string) []region {
	if leftDelim == "" {
		leftDelim = "{{"
	}
	if rightDelim == "" {
		rightDelim = "}}"
	}

	var found []region
	i := 0
	for {
		rel := strings.Index(text[i:], leftDelim)
		if rel < 0 {
			return found
		}
		start := i + rel
		content := start + len(leftDelim)

		closing, ok := closeOffset(text, content, rightDelim)
		if !ok {
			// Whatever this is, it is not an action this scanner
			// understands. Carry on past the left delimiter rather
			// than abandoning the rest of the template: one region
			// that cannot be read must not cost every action after
			// it its mutants.
			i = content
			continue
		}

		end := closing + len(rightDelim)
		found = append(found, region{
			start:    start,
			innerEnd: trimRight(text, content, closing),
			end:      end,
			keyword:  leadingWord(text[trimLeft(text, content, closing):closing]),
		})
		i = end
	}
}

// closeOffset returns the offset of the right delimiter that closes the
// action whose content begins at content.
func closeOffset(text string, content int, rightDelim string) (int, bool) {
	if body, ok := commentBody(text, content); ok {
		// A comment runs to */ and then to the right delimiter, and may
		// hold anything in between: an apostrophe, an unbalanced quote,
		// even the right delimiter itself. Reading it as a unit is what
		// keeps prose from being lexed as template source.
		rel := strings.Index(text[body:], "*/")
		if rel < 0 {
			return 0, false
		}
		rest := body + rel + 2
		next := strings.Index(text[rest:], rightDelim)
		if next < 0 {
			return 0, false
		}
		return rest + next, true
	}

	for i := content; i < len(text); {
		switch c := text[i]; c {
		case '"', '\'':
			end, ok := skipQuoted(text, i, c, true)
			if !ok {
				return 0, false
			}
			i = end
		case '`':
			end, ok := skipQuoted(text, i, c, false)
			if !ok {
				return 0, false
			}
			i = end
		default:
			if strings.HasPrefix(text[i:], rightDelim) {
				return i, true
			}
			i++
		}
	}
	return 0, false
}

// commentBody reports whether the action whose content begins at content
// is a comment, and returns the offset just past the opening /*.
//
// A trim marker may sit between the left delimiter and the /*, so the
// marker and the whitespace after it are skipped before looking.
func commentBody(text string, content int) (int, bool) {
	i := trimLeft(text, content, len(text))
	if !strings.HasPrefix(text[i:], "/*") {
		return 0, false
	}
	return i + len("/*"), true
}

// skipQuoted returns the offset one past the literal that opens at start
// with quote character quote. Backslash escapes are honoured only when
// escapes is true, which is what separates Go's interpreted strings and
// character constants from its raw strings.
func skipQuoted(text string, start int, quote byte, escapes bool) (int, bool) {
	for i := start + 1; i < len(text); i++ {
		switch text[i] {
		case '\\':
			if escapes {
				i++
			}
		case quote:
			return i + 1, true
		case '\n':
			if escapes {
				// An interpreted string or character constant cannot
				// span a line; treat the literal as unterminated
				// rather than swallowing the rest of the template.
				return 0, false
			}
		}
	}
	return 0, false
}

// trimRight returns the offset one past the action's content, excluding a
// trailing trim marker and the whitespace separating it from the pipeline.
//
// text/template only reads "-" as a trim marker when whitespace separates
// it from what precedes it, so "{{if 1 -}}" trims and "{{$x-}}" does not.
func trimRight(text string, content, closing int) int {
	inner := text[content:closing]
	if n := len(inner); n >= 2 && inner[n-1] == '-' && isSpace(inner[n-2]) {
		inner = inner[:n-1]
	}
	return content + len(strings.TrimRight(inner, spaceChars))
}

// trimLeft returns the offset of the action's first content byte, with a
// leading trim marker and the whitespace after it skipped.
func trimLeft(text string, content, closing int) int {
	inner := text[content:closing]
	if len(inner) >= 2 && inner[0] == '-' && isSpace(inner[1]) {
		inner = inner[1:]
	}
	return closing - len(strings.TrimLeft(inner, spaceChars))
}

// leadingWord returns the keyword the action opens with, or the empty
// string when it opens with anything else.
//
// The word runs as far as text/template's lexer would read one identifier,
// so an action calling a function named "endorse" or "withDefault" is not
// mistaken for an end or a with.
func leadingWord(content string) string {
	word := content
	if n := strings.IndexFunc(content, notIdentifier); n >= 0 {
		word = content[:n]
	}
	switch word {
	case "if", "range", "with", "block", "define", "else", "end", "template":
		return word
	default:
		return ""
	}
}

// notIdentifier is text/template's isAlphaNumeric, negated.
func notIdentifier(r rune) bool {
	return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// regionAt returns the action holding pos, and its index.
//
// A parse node's position always falls inside the action that produced
// it, which is how a node is related back to the text it was written as.
func regionAt(found []region, pos int) (int, region, bool) {
	for i, r := range found {
		if pos >= r.start && pos < r.end {
			return i, r, true
		}
	}
	return 0, region{}, false
}

// matchEnd returns the {{end}} closing the action at index open, and the
// {{else}} belonging to it when it has one.
//
// Actions nested inside are skipped by depth, so the else and end
// reported are the ones at the opening action's own level.
func matchEnd(found []region, open int) (endIndex int, elseIndex int, ok bool) {
	if open < 0 || open >= len(found) || !found[open].opensBlock() {
		return 0, 0, false
	}
	elseIndex = -1
	depth := 1
	for i := open + 1; i < len(found); i++ {
		switch {
		case found[i].opensBlock():
			depth++
		case found[i].keyword == "end":
			depth--
			if depth == 0 {
				return i, elseIndex, true
			}
		case found[i].keyword == "else" && depth == 1 && elseIndex < 0:
			elseIndex = i
		}
	}
	return 0, 0, false
}
