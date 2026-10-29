// Ported from lib/lexer/generic-urange.js.

package csstree

import (
	"strings"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

const (
	urangePlusSign     = 0x002B // U+002B PLUS SIGN (+)
	urangeHyphenMinus  = 0x002D // U+002D HYPHEN-MINUS (-)
	urangeQuestionMark = 0x003F // U+003F QUESTION MARK (?)
)

func tokenStartsWith(token *matchToken, code int) bool {
	return tokenizer.CharCodeAt(token.value, 0) == code
}

func hexSequence(token *matchToken, offset int, allowDash bool) int {
	hexlen := 0

	for pos := offset; pos < len(token.value); pos++ {
		code := tokenizer.CharCodeAt(token.value, pos)

		if code == urangeHyphenMinus && allowDash && hexlen != 0 {
			hexSequence(token, offset+hexlen+1, false)
			return 6 // dissallow following question marks
		}

		if !tokenizer.IsHexDigit(code) {
			return 0 // not a hex digit
		}

		hexlen++
		if hexlen > 6 {
			return 0 // too many hex digits
		}
	}

	return hexlen
}

func withQuestionMarkSequence(consumed, length int, getNextToken func(offset int) *matchToken) int {
	if consumed == 0 {
		return 0 // nothing consumed
	}

	for isDelimToken(getNextToken(length), urangeQuestionMark) {
		consumed++
		if consumed > 6 {
			return 0 // too many question marks
		}

		length++
	}

	return length
}

// https://drafts.csswg.org/css-syntax/#urange
// Informally, the <urange> production has three forms:
// U+0001
//
//	Defines a range consisting of a single code point, in this case the code point "1".
//
// U+0001-00ff
//
//	Defines a range of codepoints between the first and the second value, in this case
//	the range between "1" and "ff" (255 in decimal) inclusive.
//
// U+00??
//
//	Defines a range of codepoints where the "?" characters range over all hex digits,
//	in this case defining the same as the value U+0000-00ff.
//
// In each form, a maximum of 6 digits is allowed for each hexadecimal number (if you treat "?" as a hexadecimal digit).
//
// <urange> =
//
//	u '+' <ident-token> '?'* |
//	u <dimension-token> '?'* |
//	u <number-token> '?'* |
//	u <number-token> <dimension-token> |
//	u <number-token> <number-token> |
//	u '+' '?'+
//
// The tokens must then interpret into a valid range, see validURange.
func urange(token *matchToken, getNextToken func(offset int) *matchToken, _ *definitionsyntax.Range) int {
	length := urangeTokens(token, getNextToken)
	if length == 0 {
		return 0
	}

	// Skipping the first u token, concatenate the representations of all
	// the tokens in the production together.
	var text strings.Builder
	for i := 1; i < length; i++ {
		text.WriteString(getNextToken(i).value)
	}

	if !validURange(text.String()) {
		return 0
	}

	return length
}

// urangeTokens returns how many tokens starting at token have the forms
// of <urange>, 0 for none.
func urangeTokens(token *matchToken, getNextToken func(offset int) *matchToken) int {
	length := 0

	// should start with the ident u (upstream: an ident starting with
	// `u` or `U`, so ua+1 was a <urange>)
	if token == nil || token.typ != tokenizer.Ident || !eqStr(token.value, "u") {
		return 0
	}

	length++
	token = getNextToken(length)
	if token == nil {
		return 0
	}

	// u '+' <ident-token> '?'*
	// u '+' '?'+
	if isDelimToken(token, urangePlusSign) {
		length++
		token = getNextToken(length)
		if token == nil {
			return 0
		}

		if token.typ == tokenizer.Ident {
			// u '+' <ident-token> '?'*
			length++
			return withQuestionMarkSequence(hexSequence(token, 0, true), length, getNextToken)
		}

		if isDelimToken(token, urangeQuestionMark) {
			// u '+' '?'+
			length++
			return withQuestionMarkSequence(1, length, getNextToken)
		}

		// Hex digit or question mark is expected
		return 0
	}

	// u <number-token> '?'*
	// u <number-token> <dimension-token>
	// u <number-token> <number-token>
	if token.typ == tokenizer.Number {
		consumedHexLength := hexSequence(token, 1, true)
		if consumedHexLength == 0 {
			return 0
		}

		length++
		token = getNextToken(length)
		if token == nil {
			// u <number-token> <eof>
			return length
		}

		if token.typ == tokenizer.Dimension || token.typ == tokenizer.Number {
			// u <number-token> <dimension-token>
			// u <number-token> <number-token>
			if !tokenStartsWith(token, urangeHyphenMinus) || hexSequence(token, 1, false) == 0 {
				return 0
			}

			return length + 1
		}

		// u <number-token> '?'*
		return withQuestionMarkSequence(consumedHexLength, length, getNextToken)
	}

	// u <dimension-token> '?'*
	if token.typ == tokenizer.Dimension {
		length++
		return withQuestionMarkSequence(hexSequence(token, 1, true), length, getNextToken)
	}

	return 0
}

// maxCodePoint is the maximum allowed code point (CSS Syntax 3 §4.2).
const maxCodePoint = 0x10FFFF

// validURange reports whether text, the representations of the tokens of
// a <urange> after its u, is a valid range: it interprets text into a
// start and an end value as CSS Syntax 3 §7.1 says (Candidate
// Recommendation Draft, 24 December 2021), and the range is invalid when
// the end is greater than the maximum allowed code point or the start is
// greater than the end.
func validURange(text string) bool {
	// If the first character of text is U+002B PLUS SIGN, consume it.
	// Otherwise, this is an invalid <urange>.
	if !strings.HasPrefix(text, "+") {
		return false
	}
	text = text[1:]

	// Consume as many hex digits from text as possible, then consume as
	// many U+003F QUESTION MARK (?) code points as possible. If zero code
	// points were consumed, or more than six code points were consumed,
	// this is an invalid <urange>.
	digits := hexDigitsPrefix(text)
	questionMarks := len(text[digits:]) - len(strings.TrimLeft(text[digits:], "?"))
	consumed := text[:digits+questionMarks]
	text = text[len(consumed):]
	if len(consumed) == 0 || len(consumed) > 6 {
		return false
	}

	var start, end int64
	switch {
	case questionMarks > 0:
		// If there are any code points left in text, this is an invalid
		// <urange>. The start value is the consumed code points with the ?
		// replaced by 0, the end value with the ? replaced by F.
		if text != "" {
			return false
		}
		start = parseHex(strings.ReplaceAll(consumed, "?", "0"))
		end = parseHex(strings.ReplaceAll(consumed, "?", "F"))

	case text == "":
		// If there are no code points left in text, the end value is the
		// same as the start value.
		start = parseHex(consumed)
		end = start

	default:
		start = parseHex(consumed)

		// If the next code point in text is U+002D HYPHEN-MINUS (-),
		// consume it. Otherwise, this is an invalid <urange>.
		if text[0] != '-' {
			return false
		}
		text = text[1:]

		// Consume as many hex digits as possible from text. If zero hex
		// digits were consumed, or more than 6 hex digits were consumed,
		// or there are any code points left in text, this is an invalid
		// <urange>.
		digits := hexDigitsPrefix(text)
		if digits == 0 || digits > 6 || digits != len(text) {
			return false
		}
		end = parseHex(text)
	}

	// If end value is greater than the maximum allowed code point, or
	// start value is greater than end value, the <urange> is invalid.
	return end <= maxCodePoint && start <= end
}

// hexDigitsPrefix returns the number of hex digits at the start of s.
func hexDigitsPrefix(s string) int {
	n := 0
	for n < len(s) && tokenizer.IsHexDigit(int(s[n])) {
		n++
	}
	return n
}

// parseHex returns the value of at most six hex digits.
func parseHex(digits string) int64 {
	var value int64
	for i := 0; i < len(digits); i++ {
		value = value<<4 | int64(digitValue(digits[i]))
	}
	return value
}
