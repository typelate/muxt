// Ported from lib/syntax/node/CDO.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// CDO is a <!-- token at the top level of a style sheet.
type CDO struct {
	Loc *Location
}

func (n *CDO) Type() string        { return "CDO" }
func (n *CDO) Location() *Location { return n.Loc }

func (n *CDO) MarshalJSON() ([]byte, error) {
	return marshalNode(n)
}

func cdoNodeType() NodeConfig {
	return NodeConfig{
		Name:            "CDO",
		Structure:       []StructureField{},
		Generate:        generateCDO,
		Clone:           cloneCDO,
		FromPlainObject: fromPlainObjectCDO,
	}
}

func cloneCDO(node Node, _ func(Node) Node) Node {
	c := *node.(*CDO)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectCDO(r *PlainObjectReader, object map[string]any) Node {
	return &CDO{
		Loc: r.Location(object),
	}
}

// CDO parses a <!-- token.
func (p *Parser) CDO() *CDO {
	start := p.TokenStart

	p.Eat(tokenizer.CDO) // <!--

	return &CDO{
		Loc: p.GetLocation(start, p.TokenStart),
	}
}

func generateCDO(g *Generator, _ Node) {
	g.Token(tokenizer.CDO, "<!--")
}
