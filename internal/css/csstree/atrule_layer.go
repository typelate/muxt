// Ported from lib/syntax/atrule/layer.js.

package csstree

func layerAtrule() AtruleParser {
	return AtruleParser{
		Prelude: func(p *Parser) *List[Node] {
			return p.CreateSingleNodeList(
				p.LayerList(),
			)
		},
		Block: func(p *Parser, _ bool) *Block {
			return p.Block(false)
		},
	}
}
