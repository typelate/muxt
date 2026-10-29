// Ported from lib/syntax/node/ClassSelector.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// ClassSelector is '.' ident.
type ClassSelector struct {
	Loc  *Location
	Name string
}

func (n *ClassSelector) Type() string        { return "ClassSelector" }
func (n *ClassSelector) Location() *Location { return n.Loc }

func (n *ClassSelector) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"name", n.Name},
	)
}

func classSelectorNodeType() NodeConfig {
	return NodeConfig{
		Name: "ClassSelector",
		Structure: []StructureField{
			{Name: "name", Types: []FieldType{stringField()}},
		},
		Generate:        generateClassSelector,
		Clone:           cloneClassSelector,
		FromPlainObject: fromPlainObjectClassSelector,
	}
}

func cloneClassSelector(node Node, _ func(Node) Node) Node {
	c := *node.(*ClassSelector)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectClassSelector(r *PlainObjectReader, object map[string]any) Node {
	return &ClassSelector{
		Loc:  r.Location(object),
		Name: r.String(object, "name"),
	}
}

// ClassSelector parses a class selector.
func (p *Parser) ClassSelector() *ClassSelector {
	p.EatDelim(fullStop)

	loc := p.GetLocation(p.TokenStart-1, p.TokenEnd)
	return &ClassSelector{
		Loc:  loc,
		Name: p.Consume(tokenizer.Ident),
	}
}

func generateClassSelector(g *Generator, node Node) {
	n := node.(*ClassSelector)
	g.Token(tokenizer.Delim, ".")
	g.Token(tokenizer.Ident, n.Name)
}
