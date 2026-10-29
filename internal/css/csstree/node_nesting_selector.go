// Ported from lib/syntax/node/NestingSelector.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// NestingSelector is the & selector.
type NestingSelector struct {
	Loc *Location
}

func (n *NestingSelector) Type() string        { return "NestingSelector" }
func (n *NestingSelector) Location() *Location { return n.Loc }

func (n *NestingSelector) MarshalJSON() ([]byte, error) {
	return marshalNode(n)
}

func nestingSelectorNodeType() NodeConfig {
	return NodeConfig{
		Name:            "NestingSelector",
		Structure:       []StructureField{},
		Generate:        generateNestingSelector,
		Clone:           cloneNestingSelector,
		FromPlainObject: fromPlainObjectNestingSelector,
	}
}

func cloneNestingSelector(node Node, _ func(Node) Node) Node {
	c := *node.(*NestingSelector)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectNestingSelector(r *PlainObjectReader, object map[string]any) Node {
	return &NestingSelector{
		Loc: r.Location(object),
	}
}

// NestingSelector parses &.
func (p *Parser) NestingSelector() *NestingSelector {
	start := p.TokenStart

	p.EatDelim(ampersand)

	return &NestingSelector{
		Loc: p.GetLocation(start, p.TokenStart),
	}
}

func generateNestingSelector(g *Generator, _ Node) {
	g.Token(tokenizer.Delim, "&")
}
