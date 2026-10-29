// Ported from lib/syntax/node/Hash.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Hash is '#' ident in a value, e.g. a hex color.
type Hash struct {
	Loc   *Location
	Value string
}

func (n *Hash) Type() string        { return "Hash" }
func (n *Hash) Location() *Location { return n.Loc }

func (n *Hash) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"value", n.Value},
	)
}

func hashNodeType() NodeConfig {
	return NodeConfig{
		Name: "Hash",
		Structure: []StructureField{
			{Name: "value", Types: []FieldType{stringField()}},
		},
		Generate:        generateHash,
		Clone:           cloneHash,
		FromPlainObject: fromPlainObjectHash,
	}
}

func cloneHash(node Node, _ func(Node) Node) Node {
	c := *node.(*Hash)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectHash(r *PlainObjectReader, object map[string]any) Node {
	return &Hash{
		Loc:   r.Location(object),
		Value: r.String(object, "value"),
	}
}

// Hash parses a hash token.
func (p *Parser) Hash() *Hash {
	start := p.TokenStart

	p.Eat(tokenizer.Hash)

	return &Hash{
		Loc:   p.GetLocation(start, p.TokenStart),
		Value: p.SubstrToCursor(start + 1),
	}
}

func generateHash(g *Generator, node Node) {
	n := node.(*Hash)
	g.Token(tokenizer.Hash, "#"+n.Value)
}
