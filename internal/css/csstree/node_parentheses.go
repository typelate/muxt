// Ported from lib/syntax/node/Parentheses.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Parentheses is a () block in a value.
type Parentheses struct {
	Loc      *Location
	Children *List[Node]
}

func (n *Parentheses) Type() string        { return "Parentheses" }
func (n *Parentheses) Location() *Location { return n.Loc }

func (n *Parentheses) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"children", n.Children},
	)
}

func parenthesesNodeType() NodeConfig {
	return NodeConfig{
		Name: "Parentheses",
		Structure: []StructureField{
			{Name: "children", Types: []FieldType{listField()}},
		},
		Generate:        generateParentheses,
		Walk:            walkParentheses,
		Clone:           cloneParentheses,
		FromPlainObject: fromPlainObjectParentheses,
	}
}

func walkParentheses(node Node, _ bool, _ func(Node) bool, walkList func(*List[Node]) bool) bool {
	n := node.(*Parentheses)
	return n.Children != nil && walkList(n.Children)
}

func cloneParentheses(node Node, clone func(Node) Node) Node {
	c := *node.(*Parentheses)
	c.Loc = cloneLocation(c.Loc)
	c.Children = cloneList(c.Children, clone)
	return &c
}

func fromPlainObjectParentheses(r *PlainObjectReader, object map[string]any) Node {
	return &Parentheses{
		Loc:      r.Location(object),
		Children: r.List(object, "children"),
	}
}

// Parentheses parses a () block, reading its content with readSequence and
// recognizer.
func (p *Parser) Parentheses(readSequence func(recognizer *Recognizer) *List[Node], recognizer *Recognizer) *Parentheses {
	start := p.TokenStart

	p.Eat(tokenizer.LeftParenthesis)

	children := readSequence(recognizer)

	if !p.EOF {
		p.Eat(tokenizer.RightParenthesis)
	}

	return &Parentheses{
		Loc:      p.GetLocation(start, p.TokenStart),
		Children: children,
	}
}

func generateParentheses(g *Generator, node Node) {
	n := node.(*Parentheses)
	g.Token(tokenizer.LeftParenthesis, "(")
	g.Children(n.Children, nil)
	g.Token(tokenizer.RightParenthesis, ")")
}
