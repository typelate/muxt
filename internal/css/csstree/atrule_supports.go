// Ported from lib/syntax/atrule/supports.js.

package csstree

func supportsAtrule() AtruleParser {
	return AtruleParser{
		Prelude: func(p *Parser) *List[Node] {
			return p.CreateSingleNodeList(
				p.Condition("supports"),
			)
		},
		Block: func(p *Parser, nested bool) *Block {
			return p.Block(nested)
		},
	}
}
