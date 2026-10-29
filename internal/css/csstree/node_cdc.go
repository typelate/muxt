// Ported from lib/syntax/node/CDC.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// CDC is a --> token at the top level of a style sheet.
type CDC struct {
	Loc *Location
}

func (n *CDC) Type() string        { return "CDC" }
func (n *CDC) Location() *Location { return n.Loc }

func (n *CDC) MarshalJSON() ([]byte, error) {
	return marshalNode(n)
}

func cdcNodeType() NodeConfig {
	return NodeConfig{
		Name:            "CDC",
		Structure:       []StructureField{},
		Generate:        generateCDC,
		Clone:           cloneCDC,
		FromPlainObject: fromPlainObjectCDC,
	}
}

func cloneCDC(node Node, _ func(Node) Node) Node {
	c := *node.(*CDC)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectCDC(r *PlainObjectReader, object map[string]any) Node {
	return &CDC{
		Loc: r.Location(object),
	}
}

// CDC parses a --> token.
func (p *Parser) CDC() *CDC {
	start := p.TokenStart

	p.Eat(tokenizer.CDC) // -->

	return &CDC{
		Loc: p.GetLocation(start, p.TokenStart),
	}
}

func generateCDC(g *Generator, _ Node) {
	g.Token(tokenizer.CDC, "-->")
}
