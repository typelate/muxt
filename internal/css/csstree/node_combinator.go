// Ported from lib/syntax/node/Combinator.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Combinator is a selector combinator: + | > | ~ | || | /deep/ or " " for
// descendant.
type Combinator struct {
	Loc  *Location
	Name string
}

func (n *Combinator) Type() string        { return "Combinator" }
func (n *Combinator) Location() *Location { return n.Loc }

func (n *Combinator) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"name", n.Name},
	)
}

func combinatorNodeType() NodeConfig {
	return NodeConfig{
		Name: "Combinator",
		Structure: []StructureField{
			{Name: "name", Types: []FieldType{stringField()}},
		},
		Generate:        generateCombinator,
		Clone:           cloneCombinator,
		FromPlainObject: fromPlainObjectCombinator,
	}
}

func cloneCombinator(node Node, _ func(Node) Node) Node {
	c := *node.(*Combinator)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectCombinator(r *PlainObjectReader, object map[string]any) Node {
	return &Combinator{
		Loc:  r.Location(object),
		Name: r.String(object, "name"),
	}
}

// Combinator parses + | > | ~ | || | /deep/
func (p *Parser) Combinator() *Combinator {
	start := p.TokenStart
	var name string

	switch p.TokenType {
	case tokenizer.WhiteSpace:
		name = " "

	case tokenizer.Delim:
		switch p.CharCodeAt(p.TokenStart) {
		case greaterThanSign, plusSign, tilde:
			p.Next()

		case verticalLine:
			// column combinator: two adjacent | delims
			p.Next()
			if !p.IsDelim(verticalLine, 0) {
				p.Error(`Delim "|" is expected`, -1)
			}
			p.Next()

		case solidus:
			p.Next()
			p.EatIdent("deep")
			p.EatDelim(solidus)

		default:
			p.Error("Combinator is expected", -1)
		}

		name = p.SubstrToCursor(start)
	}

	return &Combinator{
		Loc:  p.GetLocation(start, p.TokenStart),
		Name: name,
	}
}

func generateCombinator(g *Generator, node Node) {
	n := node.(*Combinator)
	g.Tokenize(n.Name)
}
