// Ported from lib/syntax/node/Identifier.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Identifier is an identifier.
type Identifier struct {
	Loc  *Location
	Name string
}

func (n *Identifier) Type() string        { return "Identifier" }
func (n *Identifier) Location() *Location { return n.Loc }

func (n *Identifier) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"name", n.Name},
	)
}

func identifierNodeType() NodeConfig {
	return NodeConfig{
		Name: "Identifier",
		Structure: []StructureField{
			{Name: "name", Types: []FieldType{stringField()}},
		},
		Generate:        generateIdentifier,
		Clone:           cloneIdentifier,
		FromPlainObject: fromPlainObjectIdentifier,
	}
}

func cloneIdentifier(node Node, _ func(Node) Node) Node {
	c := *node.(*Identifier)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectIdentifier(r *PlainObjectReader, object map[string]any) Node {
	return &Identifier{
		Loc:  r.Location(object),
		Name: r.String(object, "name"),
	}
}

// Identifier parses an ident token.
func (p *Parser) Identifier() *Identifier {
	loc := p.GetLocation(p.TokenStart, p.TokenEnd)
	return &Identifier{
		Loc:  loc,
		Name: p.Consume(tokenizer.Ident),
	}
}

func generateIdentifier(g *Generator, node Node) {
	n := node.(*Identifier)
	g.Token(tokenizer.Ident, n.Name)
}
