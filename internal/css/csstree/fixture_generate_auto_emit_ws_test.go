// Ported from lib/__tests/fixture/generate-auto-emit-ws.js.

package csstree

import (
	"fmt"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

func num(postfix string) []string {
	var result []string
	for _, v := range []string{
		"-.5", "-1", "-1e-2", "-1e2", "-1.5",
		"0",
		".5", "1", "1e-2", "1e2", "1.5",
	} {
		result = append(result, v+postfix)
	}
	return result
}

type autoEmitWSToken struct {
	Type  string
	Value string
}

type autoEmitWSFixture struct {
	ID    string
	WS    bool
	Left  autoEmitWSToken
	Right autoEmitWSToken
}

// generateAutoEmitWSFixture builds upstream's `fixture` export.
func generateAutoEmitWSFixture() []autoEmitWSFixture {
	type tokenValues struct {
		Type   string
		Values []string
	}
	tokenTypeKey := map[tokenizer.TokenType]string{
		tokenizer.EOF:                "EOF",
		tokenizer.Ident:              "Ident",
		tokenizer.Function:           "Function",
		tokenizer.AtKeyword:          "AtKeyword",
		tokenizer.Hash:               "Hash",
		tokenizer.String:             "String",
		tokenizer.BadString:          "BadString",
		tokenizer.URL:                "Url",
		tokenizer.BadURL:             "BadUrl",
		tokenizer.Delim:              "Delim",
		tokenizer.Number:             "Number",
		tokenizer.Percentage:         "Percentage",
		tokenizer.Dimension:          "Dimension",
		tokenizer.WhiteSpace:         "WhiteSpace",
		tokenizer.CDO:                "CDO",
		tokenizer.CDC:                "CDC",
		tokenizer.Colon:              "Colon",
		tokenizer.Semicolon:          "Semicolon",
		tokenizer.Comma:              "Comma",
		tokenizer.LeftSquareBracket:  "LeftSquareBracket",
		tokenizer.RightSquareBracket: "RightSquareBracket",
		tokenizer.LeftParenthesis:    "LeftParenthesis",
		tokenizer.RightParenthesis:   "RightParenthesis",
		tokenizer.LeftCurlyBracket:   "LeftCurlyBracket",
		tokenizer.RightCurlyBracket:  "RightCurlyBracket",
		tokenizer.Comment:            "Comment",
	}
	fixtureTokens := []tokenValues{
		{"Ident", []string{"foo", "--foo", "--"}},
		{"Function", []string{"fn(", "--fn(", "--("}},
		{"AtKeyword", []string{"@test"}},
		{"Hash", []string{"#f00"}},
		{"String", []string{`"foo"`, "'foo'"}},
		// BadString: ['"foo', "'foo"],
		{"Url", []string{"url(#)"}},
		// BadUrl: ['url(#'],
		{"Delim", []string{"@", "#", "-", "+", ".", "/", "*", "%"}},
		{"Number", num("")},
		{"Percentage", num("%")},
		{"Dimension", num("px")},
		{"CDO", []string{"<!--"}},
		{"CDC", []string{"-->"}},
		{"Colon", []string{":"}},
		{"Semicolon", []string{";"}},
		{"Comma", []string{","}},
		{"LeftSquareBracket", []string{"["}},
		{"RightSquareBracket", []string{"]"}},
		{"LeftParenthesis", []string{"("}},
		{"RightParenthesis", []string{")"}},
		{"LeftCurlyBracket", []string{"{"}},
		{"RightCurlyBracket", []string{"}"}},
		{"Comment", []string{"/**/"}},
	}
	var fixture []autoEmitWSFixture

	for _, left := range fixtureTokens {
		for leftIndex, leftValue := range left.Values {
			for _, right := range fixtureTokens {
				for rightIndex, rightValue := range right.Values {
					id := fmt.Sprintf("%s[%d]/%s[%d]", left.Type, leftIndex, right.Type, rightIndex)
					source := leftValue + rightValue
					var tokens []autoEmitWSToken

					tokenizer.Tokenize(source, func(tokenType tokenizer.TokenType, start, end int) {
						tokens = append(tokens, autoEmitWSToken{
							Type:  tokenTypeKey[tokenType],
							Value: source[start:end],
						})
					})

					noWsNeeded := len(tokens) == 2 &&
						tokens[0].Type == left.Type &&
						tokens[0].Value == leftValue &&
						tokens[1].Type == right.Type &&
						tokens[1].Value == rightValue

					fixture = append(fixture, autoEmitWSFixture{
						ID: id,
						WS: !noWsNeeded,
						Left: autoEmitWSToken{
							Type:  left.Type,
							Value: leftValue,
						},
						Right: autoEmitWSToken{
							Type:  right.Type,
							Value: rightValue,
						},
					})
				}
			}
		}
	}

	return fixture
}
