// Ported from lib/generator/token-before.js.

package csstree

import (
	"sync"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// code:
// 0xxxxxxx x0000000 - char code (0x80 for non-ASCII) or delim value
// 00000000 0xxxxxx0 - token type (0 for delim, 1 for token)
// 00000000 0000000x - reserved for carriage emit flag (0 for no space, 1 for space)
func code(tokenType tokenizer.TokenType, value string) int {
	if tokenType == tokenizer.Delim {
		return charPairCode(tokenizer.CharCodeAt(value, 0))
	}

	return tokenPairCode(tokenType)
}

// tokenPairCode is code(type) for a token type of a pair table.
func tokenPairCode(tokenType tokenizer.TokenType) int {
	return int(tokenType) << 1
}

// charPairCode is code(char) for a delim char of a pair table.
func charPairCode(char int) int {
	return min(char, 0x80) << 6 << 1 // replace non-ASCII with 0x80
}

// https://www.w3.org/TR/css-syntax-3/#serialization
// The only requirement for serialization is that it must "round-trip" with parsing,
// that is, parsing the stylesheet must produce the same data structures as parsing,
// serializing, and parsing again, except for consecutive <whitespace-token>s,
// which may be collapsed into a single token.

func specPairs() [][2]int {
	var (
		ident          = tokenPairCode(tokenizer.Ident)
		functionToken  = tokenPairCode(tokenizer.Function)
		url            = tokenPairCode(tokenizer.URL)
		badURL         = tokenPairCode(tokenizer.BadURL)
		numberToken    = tokenPairCode(tokenizer.Number)
		percentage     = tokenPairCode(tokenizer.Percentage)
		dimension      = tokenPairCode(tokenizer.Dimension)
		cdc            = tokenPairCode(tokenizer.CDC)
		leftParen      = tokenPairCode(tokenizer.LeftParenthesis)
		atKeyword      = tokenPairCode(tokenizer.AtKeyword)
		hash           = tokenPairCode(tokenizer.Hash)
		hyphen         = charPairCode('-')
		numberSignChar = charPairCode('#')
		percentSign    = charPairCode('%')
		commercialAt   = charPairCode('@')
		fullStopChar   = charPairCode('.')
		plusSignChar   = charPairCode('+')
		solidusChar    = charPairCode('/')
		asteriskChar   = charPairCode('*')
	)

	return [][2]int{
		{ident, ident},
		{ident, functionToken},
		{ident, url},
		{ident, badURL},
		{ident, hyphen},
		{ident, numberToken},
		{ident, percentage},
		{ident, dimension},
		{ident, cdc},
		{ident, leftParen},

		{atKeyword, ident},
		{atKeyword, functionToken},
		{atKeyword, url},
		{atKeyword, badURL},
		{atKeyword, hyphen},
		{atKeyword, numberToken},
		{atKeyword, percentage},
		{atKeyword, dimension},
		{atKeyword, cdc},

		{hash, ident},
		{hash, functionToken},
		{hash, url},
		{hash, badURL},
		{hash, hyphen},
		{hash, numberToken},
		{hash, percentage},
		{hash, dimension},
		{hash, cdc},

		{dimension, ident},
		{dimension, functionToken},
		{dimension, url},
		{dimension, badURL},
		{dimension, hyphen},
		{dimension, numberToken},
		{dimension, percentage},
		{dimension, dimension},
		{dimension, cdc},

		{numberSignChar, ident},
		{numberSignChar, functionToken},
		{numberSignChar, url},
		{numberSignChar, badURL},
		{numberSignChar, hyphen},
		{numberSignChar, numberToken},
		{numberSignChar, percentage},
		{numberSignChar, dimension},
		{numberSignChar, cdc}, // https://github.com/w3c/csswg-drafts/pull/6874

		{hyphen, ident},
		{hyphen, functionToken},
		{hyphen, url},
		{hyphen, badURL},
		{hyphen, hyphen},
		{hyphen, numberToken},
		{hyphen, percentage},
		{hyphen, dimension},
		{hyphen, cdc}, // https://github.com/w3c/csswg-drafts/pull/6874

		{numberToken, ident},
		{numberToken, functionToken},
		{numberToken, url},
		{numberToken, badURL},
		{numberToken, numberToken},
		{numberToken, percentage},
		{numberToken, dimension},
		{numberToken, percentSign},
		{numberToken, cdc}, // https://github.com/w3c/csswg-drafts/pull/6874

		{commercialAt, ident},
		{commercialAt, functionToken},
		{commercialAt, url},
		{commercialAt, badURL},
		{commercialAt, hyphen},
		{commercialAt, cdc}, // https://github.com/w3c/csswg-drafts/pull/6874

		{fullStopChar, numberToken},
		{fullStopChar, percentage},
		{fullStopChar, dimension},

		{plusSignChar, numberToken},
		{plusSignChar, percentage},
		{plusSignChar, dimension},

		{solidusChar, asteriskChar},
	}
}

// validate with scripts/generate-safe
func safePairs() [][2]int {
	var (
		ident         = tokenPairCode(tokenizer.Ident)
		functionToken = tokenPairCode(tokenizer.Function)
		percentage    = tokenPairCode(tokenizer.Percentage)
		dimension     = tokenPairCode(tokenizer.Dimension)
		leftParen     = tokenPairCode(tokenizer.LeftParenthesis)
		rightParen    = tokenPairCode(tokenizer.RightParenthesis)
		atKeyword     = tokenPairCode(tokenizer.AtKeyword)
		hash          = tokenPairCode(tokenizer.Hash)
		stringToken   = tokenPairCode(tokenizer.String)
		colon         = tokenPairCode(tokenizer.Colon)
		hyphen        = charPairCode('-')
	)

	return append(specPairs(), [][2]int{
		{ident, hash},

		{dimension, hash},

		{hash, hash},

		{atKeyword, leftParen},
		{atKeyword, stringToken},
		{atKeyword, colon},

		{percentage, percentage},
		{percentage, dimension},
		{percentage, functionToken},
		{percentage, hyphen},

		{rightParen, ident},
		{rightParen, functionToken},
		{rightParen, percentage},
		{rightParen, dimension},
		{rightParen, hash},
		{rightParen, hyphen},
	}...)
}

// TokenBeforeFunc decides whether the generator emits whitespace before a
// token: given the code returned for the previous token, it returns the
// code of this token with bit 0 set when whitespace is needed.
type TokenBeforeFunc func(prevCode int, tokenType tokenizer.TokenType, value string) int

func createMap(pairs [][2]int) TokenBeforeFunc {
	isWhiteSpaceRequired := make(map[int]struct{}, len(pairs))
	for _, pair := range pairs {
		isWhiteSpaceRequired[pair[0]<<16|pair[1]] = struct{}{}
	}

	return func(prevCode int, tokenType tokenizer.TokenType, value string) int {
		nextCode := code(tokenType, value)
		nextCharCode := tokenizer.CharCodeAt(value, 0)
		var emitWs bool
		if (nextCharCode == hyphenMinus &&
			tokenType != tokenizer.Ident &&
			tokenType != tokenizer.Function &&
			tokenType != tokenizer.CDC) ||
			(nextCharCode == plusSign) {
			_, emitWs = isWhiteSpaceRequired[(prevCode&0xFFFE)<<16|nextCharCode<<7]
		} else {
			_, emitWs = isWhiteSpaceRequired[(prevCode&0xFFFE)<<16|nextCode]
		}

		if emitWs {
			return nextCode | 1
		}
		return nextCode
	}
}

// specTokenBefore and safeTokenBefore are upstream's spec and safe exports;
// their tables are built once and only read.
var (
	specTokenBefore = sync.OnceValue(func() TokenBeforeFunc { return createMap(specPairs()) })
	safeTokenBefore = sync.OnceValue(func() TokenBeforeFunc { return createMap(safePairs()) })
)
