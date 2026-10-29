// Ported from lib/syntax/atrule/media.js.

package csstree

func mediaAtrule() AtruleParser {
	return AtruleParser{
		Prelude: func(p *Parser) *List[Node] {
			return p.CreateSingleNodeList(
				p.MediaQueryList(),
			)
		},
		Block: func(p *Parser, nested bool) *Block {
			return p.Block(nested)
		},
	}
}
