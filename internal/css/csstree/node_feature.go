// Ported from lib/syntax/node/Feature.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Feature is a media or container feature: ( <name> [: <value>]? ).
type Feature struct {
	Loc  *Location
	Kind string
	Name string
	// Value is an *Identifier, a *Number, a *Dimension, a *Ratio, a
	// *Function or nil.
	Value Node
}

func (n *Feature) Type() string        { return "Feature" }
func (n *Feature) Location() *Location { return n.Loc }

func (n *Feature) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"kind", n.Kind},
		jsonField{"name", n.Name},
		jsonField{"value", n.Value},
	)
}

func featureNodeType() NodeConfig {
	return NodeConfig{
		Name: "Feature",
		Structure: []StructureField{
			{Name: "kind", Types: []FieldType{stringField()}},
			{Name: "name", Types: []FieldType{stringField()}},
			{Name: "value", Types: []FieldType{nodeField("Identifier"), nodeField("Number"), nodeField("Dimension"), nodeField("Ratio"), nodeField("Function"), nullField()}},
		},
		Generate:        generateFeature,
		Walk:            walkFeature,
		Clone:           cloneFeature,
		FromPlainObject: fromPlainObjectFeature,
	}
}

func walkFeature(node Node, _ bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*Feature)
	return n.Value != nil && walkNode(n.Value)
}

func cloneFeature(node Node, clone func(Node) Node) Node {
	c := *node.(*Feature)
	c.Loc = cloneLocation(c.Loc)
	c.Value = clone(c.Value)
	return &c
}

func fromPlainObjectFeature(r *PlainObjectReader, object map[string]any) Node {
	return &Feature{
		Loc:   r.Location(object),
		Kind:  r.String(object, "kind"),
		Name:  r.String(object, "name"),
		Value: r.Node(object, "value"),
	}
}

// Feature parses a feature of a condition of kind.
func (p *Parser) Feature(kind string) *Feature {
	start := p.TokenStart
	var value Node

	p.Eat(tokenizer.LeftParenthesis)
	p.SkipSC()

	name := p.Consume(tokenizer.Ident)
	p.SkipSC()

	if p.TokenType != tokenizer.RightParenthesis {
		p.Eat(tokenizer.Colon)
		p.SkipSC()

		switch p.TokenType {
		case tokenizer.Number:
			if p.LookupNonWSType(1) == tokenizer.Delim {
				value = p.Ratio()
			} else {
				value = p.Number()
			}

		case tokenizer.Dimension:
			value = p.Dimension()

		case tokenizer.Ident:
			value = p.Identifier()

		case tokenizer.Function:
			value = p.ParseWithFallback(
				func() Node {
					res := p.Function(p.ReadSequence, p.Config.Scope["Value"])

					p.SkipSC()

					if p.IsDelim(solidus, 0) {
						p.Error("", -1)
					}

					return res
				},
				func() Node {
					return p.Ratio()
				},
			)

		default:
			p.Error("Number, dimension, ratio or identifier is expected", -1)
		}

		p.SkipSC()
	}

	if !p.EOF {
		p.Eat(tokenizer.RightParenthesis)
	}

	return &Feature{
		Loc:   p.GetLocation(start, p.TokenStart),
		Kind:  kind,
		Name:  name,
		Value: value,
	}
}

func generateFeature(g *Generator, node Node) {
	n := node.(*Feature)
	g.Token(tokenizer.LeftParenthesis, "(")
	g.Token(tokenizer.Ident, n.Name)

	if n.Value != nil {
		g.Token(tokenizer.Colon, ":")
		g.Node(n.Value)
	}

	g.Token(tokenizer.RightParenthesis, ")")
}
