// Ported from lib/syntax/node/WhiteSpace.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// WhiteSpace is whitespace kept in a value.
type WhiteSpace struct {
	Loc   *Location
	Value string
}

func (n *WhiteSpace) Type() string        { return "WhiteSpace" }
func (n *WhiteSpace) Location() *Location { return n.Loc }

func (n *WhiteSpace) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"value", n.Value},
	)
}

func whiteSpaceNodeType() NodeConfig {
	return NodeConfig{
		Name: "WhiteSpace",
		Structure: []StructureField{
			{Name: "value", Types: []FieldType{stringField()}},
		},
		Generate:        generateWhiteSpace,
		Clone:           cloneWhiteSpace,
		FromPlainObject: fromPlainObjectWhiteSpace,
	}
}

func cloneWhiteSpace(node Node, _ func(Node) Node) Node {
	c := *node.(*WhiteSpace)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectWhiteSpace(r *PlainObjectReader, object map[string]any) Node {
	return &WhiteSpace{
		Loc:   r.Location(object),
		Value: r.String(object, "value"),
	}
}

// WhiteSpace parses a whitespace token. Upstream returns one frozen shared
// node; the port returns a new node each time.
func (p *Parser) WhiteSpace() *WhiteSpace {
	p.Eat(tokenizer.WhiteSpace)
	return &WhiteSpace{
		Loc:   nil,
		Value: " ",
	}

	// return {
	//     type: 'WhiteSpace',
	//     loc: this.getLocation(this.tokenStart, this.tokenEnd),
	//     value: this.consume(WHITESPACE)
	// };
}

func generateWhiteSpace(g *Generator, node Node) {
	n := node.(*WhiteSpace)
	g.Token(tokenizer.WhiteSpace, n.Value)
}
