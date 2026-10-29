// Ported from lib/tokenizer/char-code-definitions.js.

package tokenizer

// Char codes are ints. In Go they are bytes of a UTF-8 string (see CharCodeAt);
// upstream reads UTF-16 code units. Every byte >= 0x80 is part of a non-ASCII
// code point, so it classifies the same way as the code unit upstream sees.

// eofCode is upstream's local EOF constant; renamed so it does not clash with
// the EOF token type.
const eofCode = 0

// https://drafts.csswg.org/css-syntax-3/
// § 4.2. Definitions

// IsDigit reports whether code is a digit.
//
// digit
// A code point between U+0030 DIGIT ZERO (0) and U+0039 DIGIT NINE (9).
func IsDigit(code int) bool {
	return code >= 0x0030 && code <= 0x0039
}

// IsHexDigit reports whether code is a hex digit.
//
// hex digit
// A digit, or a code point between U+0041 LATIN CAPITAL LETTER A (A) and U+0046 LATIN CAPITAL LETTER F (F),
// or a code point between U+0061 LATIN SMALL LETTER A (a) and U+0066 LATIN SMALL LETTER F (f).
func IsHexDigit(code int) bool {
	return IsDigit(code) || // 0 .. 9
		(code >= 0x0041 && code <= 0x0046) || // A .. F
		(code >= 0x0061 && code <= 0x0066) // a .. f
}

// IsUppercaseLetter reports whether code is an uppercase letter.
//
// uppercase letter
// A code point between U+0041 LATIN CAPITAL LETTER A (A) and U+005A LATIN CAPITAL LETTER Z (Z).
func IsUppercaseLetter(code int) bool {
	return code >= 0x0041 && code <= 0x005A
}

// IsLowercaseLetter reports whether code is a lowercase letter.
//
// lowercase letter
// A code point between U+0061 LATIN SMALL LETTER A (a) and U+007A LATIN SMALL LETTER Z (z).
func IsLowercaseLetter(code int) bool {
	return code >= 0x0061 && code <= 0x007A
}

// IsLetter reports whether code is a letter.
//
// letter
// An uppercase letter or a lowercase letter.
func IsLetter(code int) bool {
	return IsUppercaseLetter(code) || IsLowercaseLetter(code)
}

// IsNonASCII reports whether code is a non-ASCII code point.
//
// non-ASCII code point
// A code point with a value equal to or greater than U+0080 <control>.
//
// 2024-09-02: The latest spec narrows the range for non-ASCII characters (see https://github.com/csstree/csstree/issues/188).
// However, all modern browsers support a wider range, and strictly following the latest spec could result
// in some CSS being parsed incorrectly, even though it works in the browser. Therefore, this function adheres
// to the previous, broader definition of non-ASCII characters.
func IsNonASCII(code int) bool {
	return code >= 0x0080
}

// IsNameStart reports whether code is a name-start code point.
//
// name-start code point
// A letter, a non-ASCII code point, or U+005F LOW LINE (_).
func IsNameStart(code int) bool {
	return IsLetter(code) || IsNonASCII(code) || code == 0x005F
}

// IsName reports whether code is a name code point.
//
// name code point
// A name-start code point, a digit, or U+002D HYPHEN-MINUS (-).
func IsName(code int) bool {
	return IsNameStart(code) || IsDigit(code) || code == 0x002D
}

// IsNonPrintable reports whether code is a non-printable code point.
//
// non-printable code point
// A code point between U+0000 NULL and U+0008 BACKSPACE, or U+000B LINE TABULATION,
// or a code point between U+000E SHIFT OUT and U+001F INFORMATION SEPARATOR ONE, or U+007F DELETE.
func IsNonPrintable(code int) bool {
	return (code >= 0x0000 && code <= 0x0008) ||
		(code == 0x000B) ||
		(code >= 0x000E && code <= 0x001F) ||
		(code == 0x007F)
}

// IsNewline reports whether code is a newline.
//
// newline
// U+000A LINE FEED. Note that U+000D CARRIAGE RETURN and U+000C FORM FEED are not included in this definition,
// as they are converted to U+000A LINE FEED during preprocessing.
// TODO: we doesn't do a preprocessing, so check a code point for U+000D CARRIAGE RETURN and U+000C FORM FEED
func IsNewline(code int) bool {
	return code == 0x000A || code == 0x000D || code == 0x000C
}

// IsWhiteSpace reports whether code is whitespace.
//
// whitespace
// A newline, U+0009 CHARACTER TABULATION, or U+0020 SPACE.
func IsWhiteSpace(code int) bool {
	return IsNewline(code) || code == 0x0020 || code == 0x0009
}

// IsValidEscape reports whether two code points are a valid escape.
//
// § 4.3.8. Check if two code points are a valid escape
func IsValidEscape(first, second int) bool {
	// If the first code point is not U+005C REVERSE SOLIDUS (\), return false.
	if first != 0x005C {
		return false
	}

	// Otherwise, if the second code point is a newline or EOF, return false.
	if IsNewline(second) || second == eofCode {
		return false
	}

	// Otherwise, return true.
	return true
}

// IsIdentifierStart reports whether three code points would start an identifier.
//
// § 4.3.9. Check if three code points would start an identifier
func IsIdentifierStart(first, second, third int) bool {
	// Look at the first code point:

	// U+002D HYPHEN-MINUS
	if first == 0x002D {
		// If the second code point is a name-start code point or a U+002D HYPHEN-MINUS,
		// or the second and third code points are a valid escape, return true. Otherwise, return false.
		return IsNameStart(second) ||
			second == 0x002D ||
			IsValidEscape(second, third)
	}

	// name-start code point
	if IsNameStart(first) {
		// Return true.
		return true
	}

	// U+005C REVERSE SOLIDUS (\)
	if first == 0x005C {
		// If the first and second code points are a valid escape, return true. Otherwise, return false.
		return IsValidEscape(first, second)
	}

	// anything else
	// Return false.
	return false
}

// IsNumberStart reports whether three code points would start a number. It
// returns 0 when they would not, otherwise the number of code points (1..3)
// that were looked at to decide.
//
// § 4.3.10. Check if three code points would start a number
func IsNumberStart(first, second, third int) int {
	// Look at the first code point:

	// U+002B PLUS SIGN (+)
	// U+002D HYPHEN-MINUS (-)
	if first == 0x002B || first == 0x002D {
		// If the second code point is a digit, return true.
		if IsDigit(second) {
			return 2
		}

		// Otherwise, if the second code point is a U+002E FULL STOP (.)
		// and the third code point is a digit, return true.
		// Otherwise, return false.
		if second == 0x002E && IsDigit(third) {
			return 3
		}
		return 0
	}

	// U+002E FULL STOP (.)
	if first == 0x002E {
		// If the second code point is a digit, return true. Otherwise, return false.
		if IsDigit(second) {
			return 2
		}
		return 0
	}

	// digit
	if IsDigit(first) {
		// Return true.
		return 1
	}

	// anything else
	// Return false.
	return 0
}

//
// Misc
//

// IsBOM detects a byte order mark (https://en.wikipedia.org/wiki/Byte_order_mark).
// code is a code point (decode it with utf8.DecodeRuneInString); the result is
// the number of bytes to skip, which is 3 for a BOM (upstream returns 1, the
// number of UTF-16 code units), else 0.
//
// Only U+FEFF is a BOM. A Go string is already decoded, and a UTF-16LE BOM
// decodes to U+FEFF too; upstream also skips U+FFFE, a noncharacter that CSS
// Syntax §3.2 treats as ordinary input.
func IsBOM(code int) int {
	if code == 0xFEFF {
		return 3
	}

	return 0
}

// Fast code category
// Only ASCII code points has a special meaning, that's why we define a maps for 0..127 codes only
const (
	EOFCategory          = 0x80
	WhiteSpaceCategory   = 0x82
	DigitCategory        = 0x83
	NameStartCategory    = 0x84
	NonPrintableCategory = 0x85
)

// category is a read-only table built once at package initialization.
var category = func() (category [0x80]int) {
	for i := range category {
		switch {
		case IsWhiteSpace(i):
			category[i] = WhiteSpaceCategory
		case IsDigit(i):
			category[i] = DigitCategory
		case IsNameStart(i):
			category[i] = NameStartCategory
		case IsNonPrintable(i):
			category[i] = NonPrintableCategory
		case i != 0:
			category[i] = i
		default:
			category[i] = EOFCategory
		}
	}
	return category
}()

// CharCodeCategory returns one of the *Category constants for code, or code
// itself when it has no category.
func CharCodeCategory(code int) int {
	if code < 0x80 {
		return category[code]
	}
	return NameStartCategory
}
