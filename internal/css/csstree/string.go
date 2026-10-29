// Ported from lib/utils/string.js.

package csstree

import (
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

const (
	reverseSolidus = 0x005c // U+005C REVERSE SOLIDUS (\)
	quotationMark  = 0x0022 // "
	apostrophe     = 0x0027 // '
)

// DecodeString returns the value of a CSS string token: str without its
// quotes and with escapes resolved (upstream string.decode). NUL and invalid
// UTF-8, escaped or not, decode to U+FFFD as CSS Syntax §3.3 preprocessing
// replaces them.
func DecodeString(str string) string {
	length := len(str)
	firstChar := tokenizer.CharCodeAt(str, 0)
	start := 0
	if firstChar == quotationMark || firstChar == apostrophe {
		start = 1
	}
	end := length - 1
	if start == 1 && length > 1 && tokenizer.CharCodeAt(str, length-1) == firstChar {
		end = length - 2
	}
	var decoded strings.Builder

	for i := start; i <= end; i++ {
		code := tokenizer.CharCodeAt(str, i)

		if code == reverseSolidus {
			// special case at the ending
			if i == end {
				// if the next input code point is EOF, do nothing
				// otherwise include last quote as escaped
				if i != length-1 {
					decoded.Reset()
					decoded.WriteString(str[i+1:])
				}
				break
			}

			i++
			code = tokenizer.CharCodeAt(str, i)

			// consume escaped
			if tokenizer.IsValidEscape(reverseSolidus, code) {
				escapeStart := i - 1
				escapeEnd := tokenizer.ConsumeEscaped(str, escapeStart)

				i = escapeEnd - 1
				decoded.WriteString(tokenizer.DecodeEscaped(str[escapeStart+1 : escapeEnd]))
			} else {
				// \r\n
				if code == 0x000d && tokenizer.CharCodeAt(str, i+1) == 0x000a {
					i++
				}
			}
		} else {
			i += writeCodePoint(&decoded, str, i) - 1
		}
	}

	return decoded.String()
}

// writeCodePoint writes the code point at str[i:] to decoded as CSS Syntax
// §3.3 preprocessing reads it, with U+FFFD for NUL and for invalid UTF-8, and
// returns its length in bytes (upstream copies the char as is).
func writeCodePoint(decoded *strings.Builder, str string, i int) int {
	r, size := tokenizer.CodePointAt(str, i)
	decoded.WriteRune(r)
	return size
}

// EncodeString serializes str as a CSS string, quoted with an apostrophe
// when apostrophe is true and a quotation mark otherwise (upstream
// string.encode).
//
// https://drafts.csswg.org/cssom/#serialize-a-string
// § 2.1. Common Serializing Idioms
func EncodeString(str string, apostrophe bool) string {
	quote := `"`
	quoteCode := quotationMark
	if apostrophe {
		quote = `'`
		quoteCode = 0x0027
	}
	var encoded strings.Builder
	wsBeforeHexIsNeeded := false

	for i := 0; i < len(str); i++ {
		code := int(str[i])

		// If the character is NULL (U+0000), then the REPLACEMENT CHARACTER (U+FFFD).
		if code == 0x0000 {
			encoded.WriteString("\uFFFD")
			continue
		}

		// If the character is in the range [\1-\1f] (U+0001 to U+001F) or is U+007F,
		// the character escaped as code point.
		// Note: Do not compare with 0x0001 since 0x0000 is precessed before
		if code <= 0x001f || code == 0x007F {
			encoded.WriteString(`\` + strconv.FormatInt(int64(code), 16))
			wsBeforeHexIsNeeded = true
			continue
		}

		// If the character is '"' (U+0022) or "\" (U+005C), the escaped character.
		if code == quoteCode || code == reverseSolidus {
			encoded.WriteString(`\` + str[i:i+1])
			wsBeforeHexIsNeeded = false
		} else {
			if wsBeforeHexIsNeeded && (tokenizer.IsHexDigit(code) || tokenizer.IsWhiteSpace(code)) {
				encoded.WriteString(" ")
			}

			// Otherwise, the character itself.
			encoded.WriteByte(str[i])
			wsBeforeHexIsNeeded = false
		}
	}

	return quote + encoded.String() + quote
}
