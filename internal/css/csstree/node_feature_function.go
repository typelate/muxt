// Ported from lib/syntax/node/FeatureFunction.js.

package csstree

import (
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// FeatureFunction is a functional feature of a condition, e.g.
// selector() in @supports or style() in @container.
type FeatureFunction struct {
	Loc     *Location
	Kind    string
	Feature string
	// Value is a *Declaration, a *Selector (or a *Raw when the value does
	// not parse).
	Value Node
}

func (n *FeatureFunction) Type() string        { return "FeatureFunction" }
func (n *FeatureFunction) Location() *Location { return n.Loc }

func (n *FeatureFunction) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"kind", n.Kind},
		jsonField{"feature", n.Feature},
		jsonField{"value", n.Value},
	)
}

func featureFunctionNodeType() NodeConfig {
	return NodeConfig{
		Name: "FeatureFunction",
		Structure: []StructureField{
			{Name: "kind", Types: []FieldType{stringField()}},
			{Name: "feature", Types: []FieldType{stringField()}},
			{Name: "value", Types: []FieldType{nodeField("Declaration"), nodeField("Selector")}},
		},
		Generate:        generateFeatureFunction,
		Walk:            walkFeatureFunction,
		Clone:           cloneFeatureFunction,
		FromPlainObject: fromPlainObjectFeatureFunction,
	}
}

func walkFeatureFunction(node Node, _ bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*FeatureFunction)
	return n.Value != nil && walkNode(n.Value)
}

func cloneFeatureFunction(node Node, clone func(Node) Node) Node {
	c := *node.(*FeatureFunction)
	c.Loc = cloneLocation(c.Loc)
	c.Value = clone(c.Value)
	return &c
}

func fromPlainObjectFeatureFunction(r *PlainObjectReader, object map[string]any) Node {
	return &FeatureFunction{
		Loc:     r.Location(object),
		Kind:    r.String(object, "kind"),
		Feature: r.String(object, "feature"),
		Value:   r.Node(object, "value"),
	}
}

func getFeatureParser(p *Parser, kind, name string) func(p *Parser) Node {
	featuresOfKind := p.Config.Features[kind]
	parser := featuresOfKind[name]

	if parser == nil {
		p.Error("Unknown feature "+name+"()", -1)
	}

	return parser
}

// FeatureFunction parses a functional feature of a condition of kind (""
// means "unknown").
func (p *Parser) FeatureFunction(kind string) *FeatureFunction {
	if kind == "" {
		kind = "unknown"
	}
	start := p.TokenStart
	functionName := p.ConsumeFunctionName()
	valueParser := getFeatureParser(p, kind, toLowerCase(functionName))

	p.SkipSC()

	value := p.ParseWithFallback(
		func() Node {
			startValueToken := p.TokenIndex
			value := valueParser(p)

			if !p.EOF &&
				!p.IsBalanceEdge(startValueToken) {
				p.Error("", -1)
			}

			return value
		},
		func() Node { return p.Raw(nil, false) },
	)

	if !p.EOF {
		p.Eat(tokenizer.RightParenthesis)
	}

	return &FeatureFunction{
		Loc:     p.GetLocation(start, p.TokenStart),
		Kind:    kind,
		Feature: functionName,
		Value:   value,
	}
}

func generateFeatureFunction(g *Generator, node Node) {
	n := node.(*FeatureFunction)
	g.Token(tokenizer.Function, n.Feature+"(")
	g.Node(n.Value)
	g.Token(tokenizer.RightParenthesis, ")")
}
