// Ported from lib/syntax/node/Operator.js.

package csstree

// Operator is '/' | '*' | ',' | ':' | '+' | '-'
type Operator struct {
	Loc   *Location
	Value string
}

func (n *Operator) Type() string        { return "Operator" }
func (n *Operator) Location() *Location { return n.Loc }

func (n *Operator) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"value", n.Value},
	)
}

func operatorNodeType() NodeConfig {
	return NodeConfig{
		Name: "Operator",
		Structure: []StructureField{
			{Name: "value", Types: []FieldType{stringField()}},
		},
		Generate:        generateOperator,
		Clone:           cloneOperator,
		FromPlainObject: fromPlainObjectOperator,
	}
}

func cloneOperator(node Node, _ func(Node) Node) Node {
	c := *node.(*Operator)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectOperator(r *PlainObjectReader, object map[string]any) Node {
	return &Operator{
		Loc:   r.Location(object),
		Value: r.String(object, "value"),
	}
}

// Operator parses the current token as an operator.
func (p *Parser) Operator() *Operator {
	start := p.TokenStart

	p.Next()

	return &Operator{
		Loc:   p.GetLocation(start, p.TokenStart),
		Value: p.SubstrToCursor(start),
	}
}

func generateOperator(g *Generator, node Node) {
	n := node.(*Operator)
	g.Tokenize(n.Value)
}
