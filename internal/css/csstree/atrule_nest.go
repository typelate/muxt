// Ported from lib/syntax/atrule/nest.js.

package csstree

func nestAtrule() AtruleParser {
	return AtruleParser{
		Prelude: func(p *Parser) *List[Node] {
			return p.CreateSingleNodeList(
				p.SelectorList(),
			)
		},
		Block: func(p *Parser, _ bool) *Block {
			return p.Block(true)
		},
	}
}
