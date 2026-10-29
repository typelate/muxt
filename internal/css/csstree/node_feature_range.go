// Ported from lib/syntax/node/FeatureRange.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

const lessThanSign = 0x003C // U+003C LESS-THAN SIGN (<)

// FeatureRange is a range feature of a media or container condition, e.g.
// (400px < width <= 700px).
type FeatureRange struct {
	Loc  *Location
	Kind string
	// Left, Middle and Right are an *Identifier, a *Number, a *Dimension, a
	// *Ratio or a *Function; Right may be nil.
	Left           Node
	LeftComparison string
	Middle         Node
	// RightComparison is "" (upstream null) when Right is nil.
	RightComparison string
	Right           Node
}

func (n *FeatureRange) Type() string        { return "FeatureRange" }
func (n *FeatureRange) Location() *Location { return n.Loc }

func (n *FeatureRange) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"kind", n.Kind},
		jsonField{"left", n.Left},
		jsonField{"leftComparison", n.LeftComparison},
		jsonField{"middle", n.Middle},
		jsonField{"rightComparison", nullableString(n.RightComparison)},
		jsonField{"right", n.Right},
	)
}

func featureRangeNodeType() NodeConfig {
	return NodeConfig{
		Name: "FeatureRange",
		Structure: []StructureField{
			{Name: "kind", Types: []FieldType{stringField()}},
			{Name: "left", Types: []FieldType{nodeField("Identifier"), nodeField("Number"), nodeField("Dimension"), nodeField("Ratio"), nodeField("Function")}},
			{Name: "leftComparison", Types: []FieldType{stringField()}},
			{Name: "middle", Types: []FieldType{nodeField("Identifier"), nodeField("Number"), nodeField("Dimension"), nodeField("Ratio"), nodeField("Function")}},
			{Name: "rightComparison", Types: []FieldType{stringField(), nullField()}},
			{Name: "right", Types: []FieldType{nodeField("Identifier"), nodeField("Number"), nodeField("Dimension"), nodeField("Ratio"), nodeField("Function"), nullField()}},
		},
		Generate:        generateFeatureRange,
		Walk:            walkFeatureRange,
		Clone:           cloneFeatureRange,
		FromPlainObject: fromPlainObjectFeatureRange,
	}
}

func walkFeatureRange(node Node, reverse bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*FeatureRange)
	if reverse {
		return n.Right != nil && walkNode(n.Right) ||
			n.Middle != nil && walkNode(n.Middle) ||
			n.Left != nil && walkNode(n.Left)
	}
	return n.Left != nil && walkNode(n.Left) ||
		n.Middle != nil && walkNode(n.Middle) ||
		n.Right != nil && walkNode(n.Right)
}

func cloneFeatureRange(node Node, clone func(Node) Node) Node {
	c := *node.(*FeatureRange)
	c.Loc = cloneLocation(c.Loc)
	c.Left = clone(c.Left)
	c.Middle = clone(c.Middle)
	c.Right = clone(c.Right)
	return &c
}

func fromPlainObjectFeatureRange(r *PlainObjectReader, object map[string]any) Node {
	return &FeatureRange{
		Loc:             r.Location(object),
		Kind:            r.String(object, "kind"),
		Left:            r.Node(object, "left"),
		LeftComparison:  r.String(object, "leftComparison"),
		Middle:          r.Node(object, "middle"),
		RightComparison: r.String(object, "rightComparison"),
		Right:           r.Node(object, "right"),
	}
}

func readTerm(p *Parser) Node {
	p.SkipSC()

	switch p.TokenType {
	case tokenizer.Number:
		if p.IsDelim(solidus, p.LookupOffsetNonSC(1)) {
			return p.Ratio()
		} else {
			return p.Number()
		}

	case tokenizer.Dimension:
		return p.Dimension()

	case tokenizer.Ident:
		return p.Identifier()

	case tokenizer.Function:
		return p.ParseWithFallback(
			func() Node {
				res := p.Function(p.ReadSequence, p.Config.Scope["Value"])

				p.SkipSC()

				if p.IsDelim(solidus, 0) {
					p.Error("", -1)
				}

				return res
			},
			func() Node {
				return p.Ratio()
			},
		)

	default:
		p.Error("Number, dimension, ratio or identifier is expected", -1)
	}

	return nil // unreachable: Error panics
}

func readComparison(p *Parser, expectColon bool) string {
	p.SkipSC()

	if p.IsDelim(lessThanSign, 0) ||
		p.IsDelim(greaterThanSign, 0) {
		value := p.Source[p.TokenStart : p.TokenStart+1]

		p.Next()

		if p.IsDelim(equalsSign, 0) {
			p.Next()
			return value + "="
		}

		return value
	}

	if p.IsDelim(equalsSign, 0) {
		p.Next()
		return "="
	}

	colon := ""
	if expectColon {
		colon = `":", `
	}
	p.Error(`Expected `+colon+`"<", ">", "=" or ")"`, -1)

	return "" // unreachable: Error panics
}

// checkRangeDirection reports a syntax error at the right comparison
// (just read) of a range with two comparisons unless both are < or <=, or
// both are > or >=: there is no = in a range with two comparisons.
// https://drafts.csswg.org/mediaqueries-4/#mq-range-context
func checkRangeDirection(p *Parser, leftComparison, rightComparison string) {
	offset := p.TokenStart - len(rightComparison)
	switch leftComparison[0] {
	case '<':
		if rightComparison[0] != '<' {
			p.Error(`Expected "<", "<=" or ")"`, offset)
		}
	case '>':
		if rightComparison[0] != '>' {
			p.Error(`Expected ">", ">=" or ")"`, offset)
		}
	default:
		p.Error(`")" is expected`, offset)
	}
}

// FeatureRange parses a range feature of a condition of kind ("" means
// "unknown").
func (p *Parser) FeatureRange(kind string) *FeatureRange {
	if kind == "" {
		kind = "unknown"
	}
	start := p.TokenStart

	p.SkipSC()
	p.Eat(tokenizer.LeftParenthesis)

	left := readTerm(p)
	leftComparison := readComparison(p, left.Type() == "Identifier")
	middle := readTerm(p)
	rightComparison := ""
	var right Node

	if p.LookupNonWSType(0) != tokenizer.RightParenthesis {
		rightComparison = readComparison(p, false)
		checkRangeDirection(p, leftComparison, rightComparison)
		right = readTerm(p)
	}

	p.SkipSC()
	p.Eat(tokenizer.RightParenthesis)

	return &FeatureRange{
		Loc:             p.GetLocation(start, p.TokenStart),
		Kind:            kind,
		Left:            left,
		LeftComparison:  leftComparison,
		Middle:          middle,
		RightComparison: rightComparison,
		Right:           right,
	}
}

func generateFeatureRange(g *Generator, node Node) {
	n := node.(*FeatureRange)
	g.Token(tokenizer.LeftParenthesis, "(")
	g.Node(n.Left)
	g.Tokenize(n.LeftComparison)
	g.Node(n.Middle)

	if n.Right != nil {
		g.Tokenize(n.RightComparison)
		g.Node(n.Right)
	}

	g.Token(tokenizer.RightParenthesis, ")")
}
