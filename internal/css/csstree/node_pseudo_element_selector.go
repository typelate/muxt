// Ported from lib/syntax/node/PseudoElementSelector.js.

package csstree

import (
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// PseudoElementSelector is :: [ <ident> | <function-token> <any-value>? ) ].
type PseudoElementSelector struct {
	Loc  *Location
	Name string
	// Children holds the arguments of a functional pseudo-element; nil for
	// a pseudo-element written without parentheses.
	Children *List[Node]
}

func (n *PseudoElementSelector) Type() string        { return "PseudoElementSelector" }
func (n *PseudoElementSelector) Location() *Location { return n.Loc }

func (n *PseudoElementSelector) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"name", n.Name},
		jsonField{"children", n.Children},
	)
}

func pseudoElementSelectorNodeType() NodeConfig {
	return NodeConfig{
		Name:        "PseudoElementSelector",
		WalkContext: "function",
		Structure: []StructureField{
			{Name: "name", Types: []FieldType{stringField()}},
			{Name: "children", Types: []FieldType{listField("Raw"), nullField()}},
		},
		Generate:        generatePseudoElementSelector,
		Walk:            walkPseudoElementSelector,
		Clone:           clonePseudoElementSelector,
		FromPlainObject: fromPlainObjectPseudoElementSelector,
	}
}

func walkPseudoElementSelector(node Node, _ bool, _ func(Node) bool, walkList func(*List[Node]) bool) bool {
	n := node.(*PseudoElementSelector)
	return n.Children != nil && walkList(n.Children)
}

func clonePseudoElementSelector(node Node, clone func(Node) Node) Node {
	c := *node.(*PseudoElementSelector)
	c.Loc = cloneLocation(c.Loc)
	c.Children = cloneList(c.Children, clone)
	return &c
}

func fromPlainObjectPseudoElementSelector(r *PlainObjectReader, object map[string]any) Node {
	return &PseudoElementSelector{
		Loc:      r.Location(object),
		Name:     r.String(object, "name"),
		Children: r.List(object, "children"),
	}
}

// PseudoElementSelector parses :: [ <ident> | <function-token> <any-value>? ) ]
func (p *Parser) PseudoElementSelector() *PseudoElementSelector {
	start := p.TokenStart
	var children *List[Node]
	var name string

	p.Eat(tokenizer.Colon)
	p.Eat(tokenizer.Colon)

	if p.TokenType == tokenizer.Function {
		name = p.ConsumeFunctionName()
		nameLowerCase := toLowerCase(name)

		if p.LookupNonWSType(0) == tokenizer.RightParenthesis {
			children = p.CreateList()
		} else if pseudo, ok := p.Config.Pseudo[nameLowerCase]; ok {
			p.SkipSC()
			children = pseudo(p)
			p.SkipSC()
		} else {
			children = p.CreateList()
			children.Push(
				p.Raw(nil, false),
			)
		}

		p.Eat(tokenizer.RightParenthesis)
	} else {
		name = p.Consume(tokenizer.Ident)
	}

	return &PseudoElementSelector{
		Loc:      p.GetLocation(start, p.TokenStart),
		Name:     name,
		Children: children,
	}
}

func generatePseudoElementSelector(g *Generator, node Node) {
	n := node.(*PseudoElementSelector)
	g.Token(tokenizer.Colon, ":")
	g.Token(tokenizer.Colon, ":")

	if n.Children == nil {
		g.Token(tokenizer.Ident, n.Name)
	} else {
		g.Token(tokenizer.Function, n.Name+"(")
		g.Children(n.Children, nil)
		g.Token(tokenizer.RightParenthesis, ")")
	}
}
