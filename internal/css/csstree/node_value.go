// Ported from lib/syntax/node/Value.js.

package csstree

// Value is a declaration value.
type Value struct {
	Loc      *Location
	Children *List[Node]
}

func (n *Value) Type() string        { return "Value" }
func (n *Value) Location() *Location { return n.Loc }

func (n *Value) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"children", n.Children},
	)
}

func valueNodeType() NodeConfig {
	return NodeConfig{
		Name: "Value",
		Structure: []StructureField{
			{Name: "children", Types: []FieldType{listField()}},
		},
		Generate:        generateValue,
		Walk:            walkValue,
		Clone:           cloneValue,
		FromPlainObject: fromPlainObjectValue,
	}
}

func walkValue(node Node, _ bool, _ func(Node) bool, walkList func(*List[Node]) bool) bool {
	n := node.(*Value)
	return n.Children != nil && walkList(n.Children)
}

func cloneValue(node Node, clone func(Node) Node) Node {
	c := *node.(*Value)
	c.Loc = cloneLocation(c.Loc)
	c.Children = cloneList(c.Children, clone)
	return &c
}

func fromPlainObjectValue(r *PlainObjectReader, object map[string]any) Node {
	return &Value{
		Loc:      r.Location(object),
		Children: r.List(object, "children"),
	}
}

// Value parses a value with the Value scope.
func (p *Parser) Value() *Value {
	start := p.TokenStart
	children := p.ReadSequence(p.Config.Scope["Value"])

	return &Value{
		Loc:      p.GetLocation(start, p.TokenStart),
		Children: children,
	}
}

func generateValue(g *Generator, node Node) {
	n := node.(*Value)
	g.Children(n.Children, nil)
}
