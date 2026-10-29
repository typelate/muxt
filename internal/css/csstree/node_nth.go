// Ported from lib/syntax/node/Nth.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Nth is the argument of :nth-child() and similar pseudo-classes.
type Nth struct {
	Loc *Location
	// Nth is an *AnPlusB or an *Identifier (odd, even).
	Nth Node
	// Selector is the "of" selector list, or nil.
	Selector *SelectorList
}

func (n *Nth) Type() string        { return "Nth" }
func (n *Nth) Location() *Location { return n.Loc }

func (n *Nth) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"nth", n.Nth},
		jsonField{"selector", n.Selector},
	)
}

func nthNodeType() NodeConfig {
	return NodeConfig{
		Name: "Nth",
		Structure: []StructureField{
			{Name: "nth", Types: []FieldType{nodeField("AnPlusB"), nodeField("Identifier")}},
			{Name: "selector", Types: []FieldType{nodeField("SelectorList"), nullField()}},
		},
		Generate:        generateNth,
		Walk:            walkNth,
		Clone:           cloneNth,
		FromPlainObject: fromPlainObjectNth,
	}
}

func walkNth(node Node, reverse bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*Nth)
	if reverse {
		return n.Selector != nil && walkNode(n.Selector) ||
			n.Nth != nil && walkNode(n.Nth)
	}
	return n.Nth != nil && walkNode(n.Nth) ||
		n.Selector != nil && walkNode(n.Selector)
}

func cloneNth(node Node, clone func(Node) Node) Node {
	c := *node.(*Nth)
	c.Loc = cloneLocation(c.Loc)
	c.Nth = clone(c.Nth)
	if c.Selector != nil {
		c.Selector, _ = clone(c.Selector).(*SelectorList)
	}
	return &c
}

func fromPlainObjectNth(r *PlainObjectReader, object map[string]any) Node {
	return &Nth{
		Loc:      r.Location(object),
		Nth:      r.Node(object, "nth"),
		Selector: PlainObjectNode[*SelectorList](r, object, "selector"),
	}
}

// Nth parses An+B [of <selector-list>]?.
func (p *Parser) Nth() *Nth {
	p.SkipSC()

	start := p.TokenStart
	var selector *SelectorList
	var nth Node

	if p.LookupValue(0, "odd") || p.LookupValue(0, "even") {
		nth = p.Identifier()
	} else {
		nth = p.AnPlusB()
	}

	end := p.TokenStart
	p.SkipSC()

	if p.LookupValue(0, "of") {
		p.Next()

		selector = p.SelectorList()
		end = p.TokenStart
	}

	return &Nth{
		Loc:      p.GetLocation(start, end),
		Nth:      nth,
		Selector: selector,
	}
}

func generateNth(g *Generator, node Node) {
	n := node.(*Nth)
	g.Node(n.Nth)
	if n.Selector != nil {
		g.Token(tokenizer.Ident, "of")
		g.Node(n.Selector)
	}
}
