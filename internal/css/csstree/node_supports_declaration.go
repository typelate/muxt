// Ported from lib/syntax/node/SupportsDeclaration.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// SupportsDeclaration is a parenthesized declaration in a supports
// condition.
type SupportsDeclaration struct {
	Loc         *Location
	Declaration *Declaration
}

func (n *SupportsDeclaration) Type() string        { return "SupportsDeclaration" }
func (n *SupportsDeclaration) Location() *Location { return n.Loc }

func (n *SupportsDeclaration) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"declaration", n.Declaration},
	)
}

func supportsDeclarationNodeType() NodeConfig {
	return NodeConfig{
		Name: "SupportsDeclaration",
		Structure: []StructureField{
			{Name: "declaration", Types: []FieldType{nodeField("Declaration")}},
		},
		Generate:        generateSupportsDeclaration,
		Walk:            walkSupportsDeclaration,
		Clone:           cloneSupportsDeclaration,
		FromPlainObject: fromPlainObjectSupportsDeclaration,
	}
}

func walkSupportsDeclaration(node Node, _ bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*SupportsDeclaration)
	return n.Declaration != nil && walkNode(n.Declaration)
}

func cloneSupportsDeclaration(node Node, clone func(Node) Node) Node {
	c := *node.(*SupportsDeclaration)
	c.Loc = cloneLocation(c.Loc)
	if c.Declaration != nil {
		c.Declaration, _ = clone(c.Declaration).(*Declaration)
	}
	return &c
}

func fromPlainObjectSupportsDeclaration(r *PlainObjectReader, object map[string]any) Node {
	return &SupportsDeclaration{
		Loc:         r.Location(object),
		Declaration: PlainObjectNode[*Declaration](r, object, "declaration"),
	}
}

// SupportsDeclaration parses ( <declaration> ).
func (p *Parser) SupportsDeclaration() *SupportsDeclaration {
	start := p.TokenStart

	p.Eat(tokenizer.LeftParenthesis)
	p.SkipSC()

	declaration := p.Declaration()

	if !p.EOF {
		p.Eat(tokenizer.RightParenthesis)
	}

	return &SupportsDeclaration{
		Loc:         p.GetLocation(start, p.TokenStart),
		Declaration: declaration,
	}
}

func generateSupportsDeclaration(g *Generator, node Node) {
	n := node.(*SupportsDeclaration)
	g.Token(tokenizer.LeftParenthesis, "(")
	g.Node(n.Declaration)
	g.Token(tokenizer.RightParenthesis, ")")
}
