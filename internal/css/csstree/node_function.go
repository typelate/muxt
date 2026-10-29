// Ported from lib/syntax/node/Function.js.

package csstree

import (
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// Function is a function in a value: <function-token> <sequence> ).
type Function struct {
	Loc      *Location
	Name     string
	Children *List[Node]
}

func (n *Function) Type() string        { return "Function" }
func (n *Function) Location() *Location { return n.Loc }

func (n *Function) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"name", n.Name},
		jsonField{"children", n.Children},
	)
}

func functionNodeType() NodeConfig {
	return NodeConfig{
		Name:        "Function",
		WalkContext: "function",
		Structure: []StructureField{
			{Name: "name", Types: []FieldType{stringField()}},
			{Name: "children", Types: []FieldType{listField()}},
		},
		Generate:        generateFunction,
		Walk:            walkFunction,
		Clone:           cloneFunction,
		FromPlainObject: fromPlainObjectFunction,
	}
}

func walkFunction(node Node, _ bool, _ func(Node) bool, walkList func(*List[Node]) bool) bool {
	n := node.(*Function)
	return n.Children != nil && walkList(n.Children)
}

func cloneFunction(node Node, clone func(Node) Node) Node {
	c := *node.(*Function)
	c.Loc = cloneLocation(c.Loc)
	c.Children = cloneList(c.Children, clone)
	return &c
}

func fromPlainObjectFunction(r *PlainObjectReader, object map[string]any) Node {
	return &Function{
		Loc:      r.Location(object),
		Name:     r.String(object, "name"),
		Children: r.List(object, "children"),
	}
}

// Function parses <function-token> <sequence> ). The arguments are parsed
// by recognizer.Functions[name] when the recognizer has a parser for the
// function, else by readSequence(recognizer).
func (p *Parser) Function(readSequence func(recognizer *Recognizer) *List[Node], recognizer *Recognizer) *Function {
	start := p.TokenStart
	name := p.ConsumeFunctionName()
	nameLowerCase := toLowerCase(name)
	var children *List[Node]

	if fn, ok := recognizer.Functions[nameLowerCase]; ok {
		children = fn(p, recognizer)
	} else {
		children = readSequence(recognizer)
	}

	if !p.EOF {
		p.Eat(tokenizer.RightParenthesis)
	}

	return &Function{
		Loc:      p.GetLocation(start, p.TokenStart),
		Name:     name,
		Children: children,
	}
}

func generateFunction(g *Generator, node Node) {
	n := node.(*Function)
	g.Token(tokenizer.Function, n.Name+"(")
	g.Children(n.Children, nil)
	g.Token(tokenizer.RightParenthesis, ")")
}
