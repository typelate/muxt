// Ported from lib/syntax/node/IdSelector.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// IDSelector is an id selector, '#' ident (upstream IdSelector).
type IDSelector struct {
	Loc  *Location
	Name string
}

func (n *IDSelector) Type() string        { return "IdSelector" }
func (n *IDSelector) Location() *Location { return n.Loc }

func (n *IDSelector) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"name", n.Name},
	)
}

func idSelectorNodeType() NodeConfig {
	return NodeConfig{
		Name: "IdSelector",
		Structure: []StructureField{
			{Name: "name", Types: []FieldType{stringField()}},
		},
		Generate:        generateIDSelector,
		Clone:           cloneIDSelector,
		FromPlainObject: fromPlainObjectIDSelector,
	}
}

func cloneIDSelector(node Node, _ func(Node) Node) Node {
	c := *node.(*IDSelector)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectIDSelector(r *PlainObjectReader, object map[string]any) Node {
	return &IDSelector{
		Loc:  r.Location(object),
		Name: r.String(object, "name"),
	}
}

// IDSelector parses an id selector (upstream IdSelector).
func (p *Parser) IDSelector() *IDSelector {
	start := p.TokenStart

	// TODO: check value is an ident
	p.Eat(tokenizer.Hash)

	return &IDSelector{
		Loc:  p.GetLocation(start, p.TokenStart),
		Name: p.SubstrToCursor(start + 1),
	}
}

func generateIDSelector(g *Generator, node Node) {
	n := node.(*IDSelector)
	// Using Delim instead of Hash is a hack to avoid for a whitespace between ident and id-selector
	// in safe mode (e.g. "a#id"), because IE11 doesn't allow a sequence <ident-token> <hash-token>
	// without a whitespace in values (e.g. "1px solid#000")
	g.Token(tokenizer.Delim, "#"+n.Name)
}
