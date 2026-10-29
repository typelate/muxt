// Ported from lib/lexer/generic-an-plus-b.js.

package csstree

import (
	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

const (
	anPlusBPlusSign     = 0x002B // U+002B PLUS SIGN (+)
	anPlusBHyphenMinus  = 0x002D // U+002D HYPHEN-MINUS (-)
	anPlusBN            = 0x006E // U+006E LATIN SMALL LETTER N (n)
	anPlusBDisallowSign = true
	anPlusBAllowSign    = false
)

func isDelimToken(token *matchToken, code int) bool {
	return token != nil && token.typ == tokenizer.Delim && tokenizer.CharCodeAt(token.value, 0) == code
}

func skipSC(token *matchToken, offset int, getNextToken func(offset int) *matchToken) int {
	for token != nil && (token.typ == tokenizer.WhiteSpace || token.typ == tokenizer.Comment) {
		offset++
		token = getNextToken(offset)
	}

	return offset
}

func checkAnPlusBInteger(token *matchToken, valueOffset int, disallowSign bool, offset int) int {
	if token == nil {
		return 0
	}

	code := tokenizer.CharCodeAt(token.value, valueOffset)

	if code == anPlusBPlusSign || code == anPlusBHyphenMinus {
		if disallowSign {
			// Number sign is not allowed
			return 0
		}
		valueOffset++
	}

	for ; valueOffset < len(token.value); valueOffset++ {
		if !tokenizer.IsDigit(tokenizer.CharCodeAt(token.value, valueOffset)) {
			// Integer is expected
			return 0
		}
	}

	return offset + 1
}

// ... <signed-integer>
// ... ['+' | '-'] <signless-integer>
func consumeAnPlusBB(token *matchToken, offset_ int, getNextToken func(offset int) *matchToken) int {
	sign := false
	offset := skipSC(token, offset_, getNextToken)

	token = getNextToken(offset)

	if token == nil {
		return offset_
	}

	if token.typ != tokenizer.Number {
		if isDelimToken(token, anPlusBPlusSign) || isDelimToken(token, anPlusBHyphenMinus) {
			sign = true
			offset++
			offset = skipSC(getNextToken(offset), offset, getNextToken)
			token = getNextToken(offset)

			if token == nil || token.typ != tokenizer.Number {
				return 0
			}
		} else {
			return offset_
		}
	}

	if !sign {
		code := tokenizer.CharCodeAt(token.value, 0)
		if code != anPlusBPlusSign && code != anPlusBHyphenMinus {
			// Number sign is expected
			return 0
		}
	}

	checkOffset := 1
	if sign {
		checkOffset = 0
	}

	return checkAnPlusBInteger(token, checkOffset, sign, offset)
}

// An+B microsyntax https://www.w3.org/TR/css-syntax-3/#anb
func anPlusB(token *matchToken, getNextToken func(offset int) *matchToken, _ *definitionsyntax.Range) int {
	offset := 0

	if token == nil {
		return 0
	}

	// <integer>
	if token.typ == tokenizer.Number {
		return checkAnPlusBInteger(token, 0, anPlusBAllowSign, offset) // b
	} else if token.typ == tokenizer.Ident && tokenizer.CharCodeAt(token.value, 0) == anPlusBHyphenMinus {
		// -n
		// -n <signed-integer>
		// -n ['+' | '-'] <signless-integer>
		// -n- <signless-integer>
		// <dashndashdigit-ident>

		// expect 1st char is N
		if !tokenizer.CmpChar(token.value, 1, anPlusBN) {
			return 0
		}

		switch len(token.value) {
		// -n
		// -n <signed-integer>
		// -n ['+' | '-'] <signless-integer>
		case 2:
			offset++
			return consumeAnPlusBB(getNextToken(offset), offset, getNextToken)

		// -n- <signless-integer>
		case 3:
			if tokenizer.CharCodeAt(token.value, 2) != anPlusBHyphenMinus {
				return 0
			}

			offset++
			offset = skipSC(getNextToken(offset), offset, getNextToken)
			token = getNextToken(offset)

			return checkAnPlusBInteger(token, 0, anPlusBDisallowSign, offset)

		// <dashndashdigit-ident>
		default:
			if tokenizer.CharCodeAt(token.value, 2) != anPlusBHyphenMinus {
				return 0
			}

			return checkAnPlusBInteger(token, 3, anPlusBDisallowSign, offset)
		}
	} else if token.typ == tokenizer.Ident || (isDelimToken(token, anPlusBPlusSign) && isIdentToken(getNextToken(offset+1))) {
		// '+'? n
		// '+'? n <signed-integer>
		// '+'? n ['+' | '-'] <signless-integer>
		// '+'? n- <signless-integer>
		// '+'? <ndashdigit-ident>

		// just ignore a plus
		if token.typ != tokenizer.Ident {
			offset++
			token = getNextToken(offset)
		}

		if token == nil || !tokenizer.CmpChar(token.value, 0, anPlusBN) {
			return 0
		}

		switch len(token.value) {
		// '+'? n
		// '+'? n <signed-integer>
		// '+'? n ['+' | '-'] <signless-integer>
		case 1:
			offset++
			return consumeAnPlusBB(getNextToken(offset), offset, getNextToken)

		// '+'? n- <signless-integer>
		case 2:
			if tokenizer.CharCodeAt(token.value, 1) != anPlusBHyphenMinus {
				return 0
			}

			offset++
			offset = skipSC(getNextToken(offset), offset, getNextToken)
			token = getNextToken(offset)

			return checkAnPlusBInteger(token, 0, anPlusBDisallowSign, offset)

		// '+'? <ndashdigit-ident>
		default:
			if tokenizer.CharCodeAt(token.value, 1) != anPlusBHyphenMinus {
				return 0
			}

			return checkAnPlusBInteger(token, 2, anPlusBDisallowSign, offset)
		}
	} else if token.typ == tokenizer.Dimension {
		// <ndashdigit-dimension>
		// <ndash-dimension> <signless-integer>
		// <n-dimension>
		// <n-dimension> <signed-integer>
		// <n-dimension> ['+' | '-'] <signless-integer>
		code := tokenizer.CharCodeAt(token.value, 0)
		sign := 0
		if code == anPlusBPlusSign || code == anPlusBHyphenMinus {
			sign = 1
		}
		i := sign

		for ; i < len(token.value); i++ {
			if !tokenizer.IsDigit(tokenizer.CharCodeAt(token.value, i)) {
				break
			}
		}

		if i == sign {
			// Integer is expected
			return 0
		}

		if !tokenizer.CmpChar(token.value, i, anPlusBN) {
			return 0
		}

		// <n-dimension>
		// <n-dimension> <signed-integer>
		// <n-dimension> ['+' | '-'] <signless-integer>
		if i+1 == len(token.value) {
			offset++
			return consumeAnPlusBB(getNextToken(offset), offset, getNextToken)
		} else {
			if tokenizer.CharCodeAt(token.value, i+1) != anPlusBHyphenMinus {
				return 0
			}

			// <ndash-dimension> <signless-integer>
			if i+2 == len(token.value) {
				offset++
				offset = skipSC(getNextToken(offset), offset, getNextToken)
				token = getNextToken(offset)

				return checkAnPlusBInteger(token, 0, anPlusBDisallowSign, offset)
			} else {
				// <ndashdigit-dimension>
				return checkAnPlusBInteger(token, i+2, anPlusBDisallowSign, offset)
			}
		}
	}

	return 0
}

// isIdentToken is upstream's getNextToken(offset + 1).type === Ident, which
// throws a TypeError when there is no next token; here a missing token is
// not an ident.
func isIdentToken(token *matchToken) bool {
	return token != nil && token.typ == tokenizer.Ident
}
