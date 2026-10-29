// Ported from lib/syntax/node/Ratio.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Media Queries Level 3 defines terms of <ratio> as a positive (not zero or negative)
// integers (see https://drafts.csswg.org/mediaqueries-3/#values)
// However, Media Queries Level 4 removes any definition of values
// (see https://drafts.csswg.org/mediaqueries-4/#values) and refers to
// CSS Values and Units for detail. In CSS Values and Units Level 4 a <ratio>
// definition was added (see https://drafts.csswg.org/css-values-4/#ratios) which
// defines ratio as "<number [0,∞]> [ / <number [0,∞]> ]?" and based on it
// any constrains on terms were removed. Parser also doesn't test numbers
// in any way to make possible for linting and fixing them by the tools using CSSTree.
// An additional syntax examination may be applied by a lexer.
func consumeTerm(p *Parser) Node {
	p.SkipSC()

	switch p.TokenType {
	case tokenizer.Number:
		return p.Number()

	case tokenizer.Function:
		return p.Function(p.ReadSequence, p.Config.Scope["Value"])

	default:
		p.Error("Number of function is expected", -1)
	}

	return nil // unreachable: Error panics
}

// Ratio is <number [0,∞]> [ / <number [0,∞]> ]?
type Ratio struct {
	Loc *Location
	// Left is a *Number or a *Function.
	Left Node
	// Right is a *Number, a *Function or nil.
	Right Node
}

func (n *Ratio) Type() string        { return "Ratio" }
func (n *Ratio) Location() *Location { return n.Loc }

func (n *Ratio) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"left", n.Left},
		jsonField{"right", n.Right},
	)
}

func ratioNodeType() NodeConfig {
	return NodeConfig{
		Name: "Ratio",
		Structure: []StructureField{
			{Name: "left", Types: []FieldType{nodeField("Number"), nodeField("Function")}},
			{Name: "right", Types: []FieldType{nodeField("Number"), nodeField("Function"), nullField()}},
		},
		Generate:        generateRatio,
		Walk:            walkRatio,
		Clone:           cloneRatio,
		FromPlainObject: fromPlainObjectRatio,
	}
}

func walkRatio(node Node, reverse bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*Ratio)
	if reverse {
		return n.Right != nil && walkNode(n.Right) ||
			n.Left != nil && walkNode(n.Left)
	}
	return n.Left != nil && walkNode(n.Left) ||
		n.Right != nil && walkNode(n.Right)
}

func cloneRatio(node Node, clone func(Node) Node) Node {
	c := *node.(*Ratio)
	c.Loc = cloneLocation(c.Loc)
	c.Left = clone(c.Left)
	c.Right = clone(c.Right)
	return &c
}

func fromPlainObjectRatio(r *PlainObjectReader, object map[string]any) Node {
	return &Ratio{
		Loc:   r.Location(object),
		Left:  r.Node(object, "left"),
		Right: r.Node(object, "right"),
	}
}

// Ratio parses <number [0,∞]> [ / <number [0,∞]> ]?
func (p *Parser) Ratio() *Ratio {
	start := p.TokenStart
	left := consumeTerm(p)
	var right Node

	p.SkipSC()
	if p.IsDelim(solidus, 0) {
		p.EatDelim(solidus)
		right = consumeTerm(p)
	}

	return &Ratio{
		Loc:   p.GetLocation(start, p.TokenStart),
		Left:  left,
		Right: right,
	}
}

func generateRatio(g *Generator, node Node) {
	n := node.(*Ratio)
	g.Node(n.Left)
	g.Token(tokenizer.Delim, "/")
	if n.Right != nil {
		g.Node(n.Right)
	} else {
		// upstream: this.node(NumberToken, 1), which fails with
		// "Unknown node type: undefined"
		g.Node(nil)
	}
}
