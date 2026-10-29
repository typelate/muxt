// Ported from lib/syntax/function/var.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// var( <ident> , <value>? )
func varFunction(p *Parser, _ *Recognizer) *List[Node] {
	children := p.CreateList()

	p.SkipSC()

	// NOTE: Don't check more than a first argument is an ident, rest checks are for lexer
	children.Push(p.Identifier())

	p.SkipSC()

	if p.TokenType == tokenizer.Comma {
		children.Push(p.Operator())

		startIndex := p.TokenIndex
		var value Node
		if p.ParseCustomProperty {
			value = p.Value()
		} else {
			value = p.Raw(p.ConsumeUntilExclamationMarkOrSemicolon, false)
		}

		if value, ok := value.(*Value); ok && value.Children.IsEmpty() {
			for offset := startIndex - p.TokenIndex; offset <= 0; offset++ {
				if p.LookupType(offset) == tokenizer.WhiteSpace {
					value.Children.AppendData(&WhiteSpace{
						Loc:   nil,
						Value: " ",
					})
					break
				}
			}
		}

		children.Push(value)
	}

	return children
}
