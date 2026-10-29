// Ported from lib/syntax/pseudo/lang.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

func parseLanguageRangeList(p *Parser) *List[Node] {
	children := p.CreateList()

	p.SkipSC()

loop:
	for !p.EOF {
		switch p.TokenType {
		case tokenizer.Ident:
			children.Push(p.Identifier())

		case tokenizer.String:
			children.Push(p.String())

		case tokenizer.Comma:
			children.Push(p.Operator())

		case tokenizer.RightParenthesis:
			break loop

		default:
			p.Error("Identifier, string or comma is expected", -1)
		}

		p.SkipSC()
	}

	return children
}
