// Ported from lib/syntax/scope/default.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

const (
	asterisk          = 0x002A // U+002A ASTERISK (*)
	plusSign          = 0x002B // U+002B PLUS SIGN (+)
	solidus           = 0x002F // U+002F SOLIDUS (/)
	latinSmallLetterU = 0x0075 // U+0075 LATIN SMALL LETTER U (u)
)

func defaultRecognizer(p *Parser, context *SequenceContext) Node {
	switch p.TokenType {
	case tokenizer.Hash:
		return p.Hash()

	case tokenizer.Comma:
		return p.Operator()

	case tokenizer.LeftParenthesis:
		return p.Parentheses(p.ReadSequence, context.Recognizer)

	case tokenizer.LeftSquareBracket:
		return p.Brackets(p.ReadSequence, context.Recognizer)

	case tokenizer.String:
		return p.String()

	case tokenizer.Dimension:
		return p.Dimension()

	case tokenizer.Percentage:
		return p.Percentage()

	case tokenizer.Number:
		return p.Number()

	case tokenizer.Function:
		if p.CmpStr(p.TokenStart, p.TokenEnd, "url(") {
			return p.URL()
		}
		return p.Function(p.ReadSequence, context.Recognizer)

	case tokenizer.URL:
		return p.URL()

	case tokenizer.Semicolon:
		// a ; inside (), [] or a function is a component value of the
		// block; elsewhere it ends the sequence
		if inParenthesesOrBrackets(p) {
			start := p.TokenStart
			p.Next()
			return &Raw{
				Loc:   p.GetLocation(start, p.TokenStart),
				Value: ";",
			}
		}

	case tokenizer.Ident:
		// check for unicode range, it should start with u+ or U+
		if p.CmpChar(p.TokenStart, latinSmallLetterU) &&
			p.CmpChar(p.TokenStart+1, plusSign) {
			return p.UnicodeRange()
		} else {
			return p.Identifier()
		}

	case tokenizer.Delim:
		code := p.CharCodeAt(p.TokenStart)

		if code == solidus ||
			code == asterisk ||
			code == plusSign ||
			code == hyphenMinus {
			return p.Operator() // TODO: replace with Delim
		}

		// TODO: produce a node with Delim node type

		if code == numberSign {
			p.Error("Hex or identifier is expected", p.TokenStart+1)
		}
	}

	return nil
}

// inParenthesesOrBrackets reports whether the current token is inside a
// (), [] or function block that is closed. Balance holds the index of the
// closing token of the enclosing block for a token inside a closed block.
func inParenthesesOrBrackets(p *Parser) bool {
	closer := p.Balance[p.TokenIndex]
	if closer <= p.TokenIndex || closer >= p.TokenCount {
		return false
	}
	switch p.GetTokenType(closer) {
	case tokenizer.RightParenthesis, tokenizer.RightSquareBracket:
		return true
	}
	return false
}
