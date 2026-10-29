// Ported from lib/tokenizer/utils.js.

package tokenizer

import "unicode/utf8"

// replacementCharacter is U+FFFD REPLACEMENT CHARACTER.
const replacementCharacter = 0xFFFD

func getCharCode(source string, offset int) int {
	if offset >= 0 && offset < len(source) {
		if code := source[offset]; code != 0 {
			return int(code)
		}
		// § 3.3. Preprocessing the input stream
		// Replace any U+0000 NULL ... code points in input with U+FFFD REPLACEMENT CHARACTER.
		return replacementCharacter
	}
	return 0
}

// CharCodeAt is the Go form of source.charCodeAt(offset): the byte at offset,
// or 0 when offset is out of range (where JavaScript returns NaN, which
// compares unequal to every code these helpers test for). It is not an
// upstream export.
//
// A NUL byte reads as U+FFFD REPLACEMENT CHARACTER, as CSS Syntax §3.3
// preprocessing replaces it (upstream returns 0, which its helpers then take
// for EOF), so 0 always means out of range. Every other byte >= 0x80 is
// returned as is: it is part of a non-ASCII code point, or of an invalid
// UTF-8 sequence that decodes to U+FFFD, and either classifies as a name
// code point.
func CharCodeAt(source string, offset int) int {
	return getCharCode(source, offset)
}

// CodePointAt returns the code point that starts at byte offset in source and
// its length in bytes, after CSS Syntax §3.3 preprocessing: a NUL byte is
// U+FFFD, and so is each invalid UTF-8 sequence, measured as the WHATWG
// Encoding Standard's UTF-8 decoder does (the longest prefix of a valid
// sequence, at least one byte). It returns 0, 0 when offset is out of range.
// Newlines are not normalized. It is not an upstream export.
func CodePointAt(source string, offset int) (rune, int) {
	if offset < 0 || offset >= len(source) {
		return 0, 0
	}

	code := source[offset]
	if code == 0 {
		return replacementCharacter, 1
	}
	if code < utf8.RuneSelf {
		return rune(code), 1
	}

	r, size := utf8.DecodeRuneInString(source[offset:])
	if r != utf8.RuneError || size != 1 {
		return r, size
	}

	// An invalid sequence: count the bytes that could still start a valid one.
	need := 0
	lower, upper := byte(0x80), byte(0xBF)
	switch {
	case 0xC2 <= code && code <= 0xDF:
		need = 1
	case 0xE0 <= code && code <= 0xEF:
		need = 2
		if code == 0xE0 {
			lower = 0xA0
		} else if code == 0xED {
			upper = 0x9F // no surrogates
		}
	case 0xF0 <= code && code <= 0xF4:
		need = 3
		if code == 0xF0 {
			lower = 0x90
		} else if code == 0xF4 {
			upper = 0x8F
		}
	}

	size = 1
	for size <= need && offset+size < len(source) {
		if next := source[offset+size]; next < lower || next > upper {
			break
		}
		size++
		lower, upper = 0x80, 0xBF
	}

	return replacementCharacter, size
}

// GetNewlineLength returns 2 for "\r\n" at offset, else 1. code is the char
// code at offset.
func GetNewlineLength(source string, offset, code int) int {
	if code == 13 /* \r */ && getCharCode(source, offset+1) == 10 /* \n */ {
		return 2
	}

	return 1
}

// CmpChar reports whether the char code at offset, lowercased for A..Z,
// equals referenceCode.
func CmpChar(testStr string, offset, referenceCode int) bool {
	code := CharCodeAt(testStr, offset)

	// code.toLowerCase() for A..Z
	if IsUppercaseLetter(code) {
		code = code | 32
	}

	return code == referenceCode
}

// CmpStr reports whether testStr[start:end], lowercased for A..Z, equals
// referenceStr. referenceStr is expected to be lowercase.
func CmpStr(testStr string, start, end int, referenceStr string) bool {
	if end-start != len(referenceStr) {
		return false
	}

	if start < 0 || end > len(testStr) {
		return false
	}

	for i := start; i < end; i++ {
		referenceCode := int(referenceStr[i-start])
		testCode := int(testStr[i])

		// testCode.toLowerCase() for A..Z
		if IsUppercaseLetter(testCode) {
			testCode = testCode | 32
		}

		if testCode != referenceCode {
			return false
		}
	}

	return true
}

// FindWhiteSpaceStart scans backwards from offset and returns the offset of
// the first char of the whitespace run that ends at offset.
func FindWhiteSpaceStart(source string, offset int) int {
	for ; offset >= 0; offset-- {
		if !IsWhiteSpace(CharCodeAt(source, offset)) {
			break
		}
	}

	return offset + 1
}

// FindWhiteSpaceEnd returns the offset just past the whitespace run starting
// at offset.
func FindWhiteSpaceEnd(source string, offset int) int {
	for ; offset < len(source); offset++ {
		if !IsWhiteSpace(CharCodeAt(source, offset)) {
			break
		}
	}

	return offset
}

// FindDecimalNumberEnd returns the offset just past the digits starting at
// offset.
func FindDecimalNumberEnd(source string, offset int) int {
	for ; offset < len(source); offset++ {
		if !IsDigit(CharCodeAt(source, offset)) {
			break
		}
	}

	return offset
}

// ConsumeEscaped returns the offset just past the escape that starts with the
// U+005C REVERSE SOLIDUS (\) at offset.
//
// § 4.3.7. Consume an escaped code point
func ConsumeEscaped(source string, offset int) int {
	// It assumes that the U+005C REVERSE SOLIDUS (\) has already been consumed and
	// that the next input code point has already been verified to be part of a valid escape.
	offset++

	// anything else
	// Return the current input code point.
	// (Go only: the whole code point; upstream consumes one UTF-16 code unit,
	// and offset+1 past the end, where this returns that too.)
	if !IsHexDigit(getCharCode(source, offset)) {
		_, size := CodePointAt(source, offset)
		return offset + max(size, 1)
	}

	// hex digit
	// Consume as many hex digits as possible, but no more than 5.
	// Note that this means 1-6 hex digits have been consumed in total.
	offset++
	for maxOffset := min(len(source), offset+5); offset < maxOffset; offset++ {
		if !IsHexDigit(getCharCode(source, offset)) {
			break
		}
	}

	// If the next input code point is whitespace, consume it as well.
	code := getCharCode(source, offset)
	if IsWhiteSpace(code) {
		offset += GetNewlineLength(source, offset, code)
	}

	return offset
}

// ConsumeName returns the offset just past the name starting at offset.
//
// §4.3.11. Consume a name
// Note: This algorithm does not do the verification of the first few code points that are necessary
// to ensure the returned code points would constitute an <ident-token>. If that is the intended use,
// ensure that the stream starts with an identifier before calling this algorithm.
func ConsumeName(source string, offset int) int {
	// Let result initially be an empty string.
	// Repeatedly consume the next input code point from the stream:
	for ; offset < len(source); offset++ {
		code := getCharCode(source, offset)

		// name code point
		if IsName(code) {
			// Append the code point to result.
			continue
		}

		// the stream starts with a valid escape
		if IsValidEscape(code, getCharCode(source, offset+1)) {
			// Consume an escaped code point. Append the returned code point to result.
			offset = ConsumeEscaped(source, offset) - 1
			continue
		}

		// anything else
		// Reconsume the current input code point. Return result.
		break
	}

	return offset
}

// ConsumeNumber returns the offset just past the number starting at offset.
//
// §4.3.12. Consume a number
func ConsumeNumber(source string, offset int) int {
	code := CharCodeAt(source, offset)

	// 2. If the next input code point is U+002B PLUS SIGN (+) or U+002D HYPHEN-MINUS (-),
	// consume it and append it to repr.
	if code == 0x002B || code == 0x002D {
		offset += 1
		code = CharCodeAt(source, offset)
	}

	// 3. While the next input code point is a digit, consume it and append it to repr.
	if IsDigit(code) {
		offset = FindDecimalNumberEnd(source, offset+1)
		code = CharCodeAt(source, offset)
	}

	// 4. If the next 2 input code points are U+002E FULL STOP (.) followed by a digit, then:
	if code == 0x002E && IsDigit(CharCodeAt(source, offset+1)) {
		// 4.1 Consume them.
		// 4.2 Append them to repr.
		offset += 2

		// 4.3 Set type to "number".
		// TODO

		// 4.4 While the next input code point is a digit, consume it and append it to repr.

		offset = FindDecimalNumberEnd(source, offset)
	}

	// 5. If the next 2 or 3 input code points are U+0045 LATIN CAPITAL LETTER E (E)
	// or U+0065 LATIN SMALL LETTER E (e), ... , followed by a digit, then:
	if CmpChar(source, offset, 101 /* e */) {
		sign := 0
		code = CharCodeAt(source, offset+1)

		// ... optionally followed by U+002D HYPHEN-MINUS (-) or U+002B PLUS SIGN (+) ...
		if code == 0x002D || code == 0x002B {
			sign = 1
			code = CharCodeAt(source, offset+2)
		}

		// ... followed by a digit
		if IsDigit(code) {
			// 5.1 Consume them.
			// 5.2 Append them to repr.

			// 5.3 Set type to "number".
			// TODO

			// 5.4 While the next input code point is a digit, consume it and append it to repr.
			offset = FindDecimalNumberEnd(source, offset+1+sign+1)
		}
	}

	return offset
}

// ConsumeBadURLRemnants returns the offset just past the remnants of a bad url
// starting at offset.
//
// § 4.3.14. Consume the remnants of a bad url
// ... its sole use is to consume enough of the input stream to reach a recovery point
// where normal tokenizing can resume.
func ConsumeBadURLRemnants(source string, offset int) int {
	// Repeatedly consume the next input code point from the stream:
	for ; offset < len(source); offset++ {
		code := getCharCode(source, offset)

		// U+0029 RIGHT PARENTHESIS ())
		// EOF
		if code == 0x0029 {
			// Return.
			offset++
			break
		}

		if IsValidEscape(code, getCharCode(source, offset+1)) {
			// Consume an escaped code point.
			// Note: This allows an escaped right parenthesis ("\)") to be encountered
			// without ending the <bad-url-token>. This is otherwise identical to
			// the "anything else" clause.
			// (upstream does not subtract one, so the loop skips the code
			// point after the escape)
			offset = ConsumeEscaped(source, offset) - 1
		}
	}

	return offset
}

// DecodeEscaped returns the code point an escape stands for, as a string.
//
// § 4.3.7. Consume an escaped code point
// Note: This algorithm assumes that escaped is valid without leading U+005C REVERSE SOLIDUS (\)
func DecodeEscaped(escaped string) string {
	// Single char escaped that's not a hex digit
	// (upstream counts one UTF-16 code unit; a non-ASCII char is more than
	// one byte here, so count code points, read as §3.3 preprocessing does:
	// NUL and invalid UTF-8 are U+FFFD)
	if r, size := CodePointAt(escaped, 0); size > 0 && size == len(escaped) && !IsHexDigit(int(escaped[0])) {
		return string(r)
	}

	// Interpret the hex digits as a hexadecimal number.
	code, ok := parseHexPrefix(escaped)

	if !ok || // (Go only: upstream's parseInt returns NaN and String.fromCodePoint throws)
		(code == 0) || // If this number is zero,
		(code >= 0xD800 && code <= 0xDFFF) || // or is for a surrogate,
		(code > 0x10FFFF) { // or is greater than the maximum allowed code point
		// ... return U+FFFD REPLACEMENT CHARACTER
		code = 0xFFFD
	}

	// Otherwise, return the code point with that value.
	return string(rune(code))
}

// parseHexPrefix mirrors parseInt(s, 16): it reads the leading hex digits of
// s (an escape may end with whitespace) and reports false when there are none.
// Values above 0x10FFFF are clamped to 0x110000 so they cannot overflow.
func parseHexPrefix(s string) (int, bool) {
	code := 0
	i := 0
	for ; i < len(s) && IsHexDigit(int(s[i])); i++ {
		code = code<<4 | hexValue(s[i])
		if code > utf8.MaxRune {
			code = utf8.MaxRune + 1
		}
	}
	return code, i > 0
}

func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}
