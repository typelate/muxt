// Ported from lib/syntax/atrule/starting-style.js.

package csstree

func startingStyleAtrule() AtruleParser {
	return AtruleParser{
		Prelude: nil,
		Block: func(p *Parser, nested bool) *Block {
			return p.Block(nested)
		},
	}
}
