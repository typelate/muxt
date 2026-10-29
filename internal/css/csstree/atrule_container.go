// Ported from lib/syntax/atrule/container.js.

package csstree

import (
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// https://drafts.csswg.org/css-contain-3/#container-rule
// The keywords `none`, `and`, `not`, and `or` are excluded from the <custom-ident> above.
func isNonContainerNameKeyword(name string) bool {
	switch name {
	case "none", "and", "not", "or":
		return true
	}
	return false
}

func containerAtrule() AtruleParser {
	return AtruleParser{
		Prelude: func(p *Parser) *List[Node] {
			children := p.CreateList()

			if p.TokenType == tokenizer.Ident {
				name := p.Substring(p.TokenStart, p.TokenEnd)

				if !isNonContainerNameKeyword(toLowerCase(name)) {
					children.Push(p.Identifier())
				}
			}

			children.Push(p.Condition("container"))

			return children
		},
		Block: func(p *Parser, nested bool) *Block {
			return p.Block(nested)
		},
	}
}
