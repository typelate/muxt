// Ported from lib/definition-syntax/scanner.js.

package definitionsyntax

import (
	"strings"
	"unicode/utf8"
)

const (
	tab            = 9  // TAB
	lineFeed       = 10 // N
	formFeed       = 12 // F
	carriageReturn = 13 // R
	space          = 32 // SPACE
)

// isNameChar reports whether code is in /[a-zA-Z0-9\-]/ (upstream NAME_CHAR).
func isNameChar(code byte) bool {
	return 'a' <= code && code <= 'z' ||
		'A' <= code && code <= 'Z' ||
		'0' <= code && code <= '9' ||
		code == '-'
}

type scanner struct {
	str string
	pos int
}

func newScanner(str string) *scanner {
	return &scanner{str: str}
}

// charCodeAt returns 0 out of range; upstream returns NaN for a negative pos,
// which, like 0, matches no character code.
func (s *scanner) charCodeAt(pos int) byte {
	if 0 <= pos && pos < len(s.str) {
		return s.str[pos]
	}
	return 0
}

func (s *scanner) charCode() byte {
	return s.charCodeAt(s.pos)
}

// isNameCharCode is upstream isNameCharCode(code = this.charCode()); call
// it with s.charCode() for the default.
func (s *scanner) isNameCharCode(code byte) bool {
	return code < 128 && isNameChar(code)
}

func (s *scanner) nextCharCode() byte {
	return s.charCodeAt(s.pos + 1)
}

// nextNonWsCode is not ported: nothing upstream calls it.

func (s *scanner) skipWs() {
	s.pos = s.findWsEnd(s.pos)
}

func (s *scanner) findWsEnd(pos int) int {
	for ; pos < len(s.str); pos++ {
		code := s.str[pos]
		if code != carriageReturn && code != lineFeed && code != formFeed && code != space && code != tab {
			break
		}
	}

	return pos
}

func (s *scanner) substringToPos(end int) string {
	result := s.str[s.pos:end]
	s.pos = end
	return result
}

func (s *scanner) eat(code byte) {
	if s.charCode() != code {
		s.error("Expect `" + string(rune(code)) + "`")
	}

	s.pos++
}

// peek returns the code point at pos and moves past it, or "" at the end.
// Upstream returns one UTF-16 code unit; a Go string is read one UTF-8
// encoded code point at a time so a multi-byte character is never split.
func (s *scanner) peek() string {
	if s.pos >= len(s.str) {
		return ""
	}

	_, size := utf8.DecodeRuneInString(s.str[s.pos:])
	result := s.str[s.pos : s.pos+size]
	s.pos += size
	return result
}

// hasPrefix stands in for comparing charCode() with a non-ASCII code point,
// which takes more than one byte in a Go string.
func (s *scanner) hasPrefix(prefix string) bool {
	return strings.HasPrefix(s.str[s.pos:], prefix)
}

// error panics with a *SyntaxError; Parse recovers it.
func (s *scanner) error(message string) {
	panic(newSyntaxError(message, s.str, s.pos))
}

func (s *scanner) scanSpaces() string {
	return s.substringToPos(s.findWsEnd(s.pos))
}

func (s *scanner) scanWord() string {
	end := s.pos

	for ; end < len(s.str); end++ {
		code := s.str[end]
		if code >= 128 || !isNameChar(code) {
			break
		}
	}

	if s.pos == end {
		s.error("Expect a keyword")
	}

	return s.substringToPos(end)
}

func (s *scanner) scanNumber() string {
	end := s.pos

	for ; end < len(s.str); end++ {
		code := s.str[end]

		if code < 48 || code > 57 {
			break
		}
	}

	if s.pos == end {
		s.error("Expect a number")
	}

	return s.substringToPos(end)
}

// scanString is called at an apostrophe, so s.pos < len(s.str).
func (s *scanner) scanString() string {
	end := strings.IndexByte(s.str[s.pos+1:], '\'')

	if end == -1 {
		s.pos = len(s.str)
		s.error("Expect an apostrophe")
	}
	end += s.pos + 1

	return s.substringToPos(end + 1)
}
