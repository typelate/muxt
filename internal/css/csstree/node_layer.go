// Ported from lib/syntax/node/Layer.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Layer is a cascade layer name, e.g. a.b.
type Layer struct {
	Loc  *Location
	Name string
}

func (n *Layer) Type() string        { return "Layer" }
func (n *Layer) Location() *Location { return n.Loc }

func (n *Layer) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"name", n.Name},
	)
}

func layerNodeType() NodeConfig {
	return NodeConfig{
		Name: "Layer",
		Structure: []StructureField{
			{Name: "name", Types: []FieldType{stringField()}},
		},
		Generate:        generateLayer,
		Clone:           cloneLayer,
		FromPlainObject: fromPlainObjectLayer,
	}
}

func cloneLayer(node Node, _ func(Node) Node) Node {
	c := *node.(*Layer)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectLayer(r *PlainObjectReader, object map[string]any) Node {
	return &Layer{
		Loc:  r.Location(object),
		Name: r.String(object, "name"),
	}
}

// Layer parses a layer name.
func (p *Parser) Layer() *Layer {
	tokenStart := p.TokenStart
	name := p.Consume(tokenizer.Ident)

	for p.IsDelim(fullStop, 0) {
		p.Eat(tokenizer.Delim)
		name += "." + p.Consume(tokenizer.Ident)
	}

	return &Layer{
		Loc:  p.GetLocation(tokenStart, p.TokenStart),
		Name: name,
	}
}

func generateLayer(g *Generator, node Node) {
	n := node.(*Layer)
	g.Tokenize(n.Name)
}
