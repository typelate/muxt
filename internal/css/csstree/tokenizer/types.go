// Ported from lib/tokenizer/types.js.

package tokenizer

import "strconv"

// TokenType identifies the kind of a token. Upstream token types are plain
// numbers stored in the top 8 bits of a Uint32Array cell; TokenType is an int
// so it can index tables such as TokenNames directly.
type TokenType int

// CSS Syntax Module Level 3
// https://www.w3.org/TR/css-syntax-3/
const (
	EOF                TokenType = 0  // <EOF-token>
	Ident              TokenType = 1  // <ident-token>
	Function           TokenType = 2  // <function-token>
	AtKeyword          TokenType = 3  // <at-keyword-token>
	Hash               TokenType = 4  // <hash-token>
	String             TokenType = 5  // <string-token>
	BadString          TokenType = 6  // <bad-string-token>
	URL                TokenType = 7  // <url-token>
	BadURL             TokenType = 8  // <bad-url-token>
	Delim              TokenType = 9  // <delim-token>
	Number             TokenType = 10 // <number-token>
	Percentage         TokenType = 11 // <percentage-token>
	Dimension          TokenType = 12 // <dimension-token>
	WhiteSpace         TokenType = 13 // <whitespace-token>
	CDO                TokenType = 14 // <CDO-token>
	CDC                TokenType = 15 // <CDC-token>
	Colon              TokenType = 16 // <colon-token>     :
	Semicolon          TokenType = 17 // <semicolon-token> ;
	Comma              TokenType = 18 // <comma-token>     ,
	LeftSquareBracket  TokenType = 19 // <[-token>
	RightSquareBracket TokenType = 20 // <]-token>
	LeftParenthesis    TokenType = 21 // <(-token>
	RightParenthesis   TokenType = 22 // <)-token>
	LeftCurlyBracket   TokenType = 23 // <{-token>
	RightCurlyBracket  TokenType = 24 // <}-token>
	Comment            TokenType = 25
)

// String returns the upstream token name (see TokenNames), e.g. "ident-token".
func (t TokenType) String() string {
	if t >= 0 && int(t) < len(TokenNames) {
		return TokenNames[t]
	}
	return "TokenType(" + strconv.Itoa(int(t)) + ")"
}
