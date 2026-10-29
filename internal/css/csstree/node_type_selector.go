// Ported from lib/syntax/node/TypeSelector.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

func eatIdentifierOrAsterisk(p *Parser) {
	if p.TokenType != tokenizer.Ident &&
		!p.IsDelim(asterisk, 0) {
		p.Error("Identifier or asterisk is expected", -1)
	}

	p.Next()
}

// TypeSelector is a type (element) selector, with an optional namespace.
type TypeSelector struct {
	Loc  *Location
	Name string
}

func (n *TypeSelector) Type() string        { return "TypeSelector" }
func (n *TypeSelector) Location() *Location { return n.Loc }

func (n *TypeSelector) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"name", n.Name},
	)
}

func typeSelectorNodeType() NodeConfig {
	return NodeConfig{
		Name: "TypeSelector",
		Structure: []StructureField{
			{Name: "name", Types: []FieldType{stringField()}},
		},
		Generate:        generateTypeSelector,
		Clone:           cloneTypeSelector,
		FromPlainObject: fromPlainObjectTypeSelector,
	}
}

func cloneTypeSelector(node Node, _ func(Node) Node) Node {
	c := *node.(*TypeSelector)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectTypeSelector(r *PlainObjectReader, object map[string]any) Node {
	return &TypeSelector{
		Loc:  r.Location(object),
		Name: r.String(object, "name"),
	}
}

// TypeSelector parses
//
//	ident
//	ident|ident
//	ident|*
//	*
//	*|ident
//	*|*
//	|ident
//	|*
func (p *Parser) TypeSelector() *TypeSelector {
	start := p.TokenStart

	if p.IsDelim(verticalLine, 0) {
		p.Next()
		eatIdentifierOrAsterisk(p)
	} else {
		eatIdentifierOrAsterisk(p)

		// a|| is a type selector followed by a column combinator
		if p.IsDelim(verticalLine, 0) && !p.IsDelim(verticalLine, 1) {
			p.Next()
			eatIdentifierOrAsterisk(p)
		}
	}

	return &TypeSelector{
		Loc:  p.GetLocation(start, p.TokenStart),
		Name: p.SubstrToCursor(start),
	}
}

func generateTypeSelector(g *Generator, node Node) {
	n := node.(*TypeSelector)
	g.Tokenize(n.Name)
}
