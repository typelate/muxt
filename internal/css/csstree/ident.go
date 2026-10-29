// Ported from lib/utils/ident.js.

package csstree

import (
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// DecodeIdent returns the value of a CSS identifier with escapes resolved
// (upstream ident.decode). NUL and invalid UTF-8, escaped or not, decode to
// U+FFFD as CSS Syntax §3.3 preprocessing replaces them.
func DecodeIdent(str string) string {
	end := len(str) - 1
	var decoded strings.Builder

	for i := 0; i < len(str); i++ {
		code := tokenizer.CharCodeAt(str, i)

		if code == reverseSolidus {
			// special case at the ending
			if i == end {
				// if the next input code point is EOF, do nothing
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

// EncodeIdent serializes str as a CSS identifier (upstream ident.encode).
//
// https://drafts.csswg.org/cssom/#serialize-an-identifier
// § 2.1. Common Serializing Idioms
func EncodeIdent(str string) string {
	var encoded strings.Builder

	// If the character is the first character and is a "-" (U+002D),
	// and there is no second character, then the escaped character.
	// Note: That's means a single dash string "-" return as escaped dash,
	// so move the condition out of the main loop
	if len(str) == 1 && str[0] == 0x002D {
		return `\-`
	}

	// To serialize an identifier means to create a string represented
	// by the concatenation of, for each character of the identifier:
	for i := 0; i < len(str); i++ {
		code := int(str[i])

		// If the character is NULL (U+0000), then the REPLACEMENT CHARACTER (U+FFFD).
		if code == 0x0000 {
			encoded.WriteString("\uFFFD")
			continue
		}

		if code <= 0x001F || code == 0x007F || // If the character is in the range [\1-\1f] (U+0001 to U+001F) or is U+007F ...
			// Note: Do not compare with 0x0001 since 0x0000 is precessed before
			// [or] ... is in the range [0-9] (U+0030 to U+0039),
			(code >= 0x0030 && code <= 0x0039 && (
			// If the character is the first character ...
			i == 0 ||
				// If the character is the second character ... and the first character is a "-" (U+002D)
				i == 1 && str[0] == 0x002D)) {
			// ... then the character escaped as code point.
			encoded.WriteString(`\` + strconv.FormatInt(int64(code), 16) + " ")
			continue
		}

		// If the character is not handled by one of the above rules and is greater
		// than or equal to U+0080, is "-" (U+002D) or "_" (U+005F), or is in one
		// of the ranges [0-9] (U+0030 to U+0039), [A-Z] (U+0041 to U+005A),
		// or \[a-z] (U+0061 to U+007A), then the character itself.
		if tokenizer.IsName(code) {
			encoded.WriteByte(str[i])
		} else {
			// Otherwise, the escaped character.
			encoded.WriteString(`\` + str[i:i+1])
		}
	}

	return encoded.String()
}
