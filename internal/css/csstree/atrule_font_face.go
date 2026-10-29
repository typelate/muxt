// Ported from lib/syntax/atrule/font-face.js.

package csstree

func fontFaceAtrule() AtruleParser {
	return AtruleParser{
		Prelude: nil,
		Block: func(p *Parser, _ bool) *Block {
			return p.descriptorBlock()
		},
	}
}
