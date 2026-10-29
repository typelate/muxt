// Ported from lib/syntax/node/Rule.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

func ruleConsumeRaw(p *Parser) Node {
	return p.Raw(p.ConsumeUntilLeftCurlyBracket, true)
}

func consumePrelude(p *Parser) Node {
	prelude := p.SelectorList()

	if prelude.Type() != "Raw" &&
		!p.EOF &&
		p.TokenType != tokenizer.LeftCurlyBracket {
		p.Error("", -1)
	}

	return prelude
}

// Rule is a qualified rule: a prelude and a block.
type Rule struct {
	Loc *Location
	// Prelude is a *SelectorList or a *Raw.
	Prelude Node
	Block   *Block
}

func (n *Rule) Type() string        { return "Rule" }
func (n *Rule) Location() *Location { return n.Loc }

func (n *Rule) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"prelude", n.Prelude},
		jsonField{"block", n.Block},
	)
}

func ruleNodeType() NodeConfig {
	return NodeConfig{
		Name:        "Rule",
		WalkContext: "rule",
		Structure: []StructureField{
			{Name: "prelude", Types: []FieldType{nodeField("SelectorList"), nodeField("Raw")}},
			{Name: "block", Types: []FieldType{nodeField("Block")}},
		},
		Generate:        generateRule,
		Walk:            walkRule,
		Clone:           cloneRule,
		FromPlainObject: fromPlainObjectRule,
	}
}

func walkRule(node Node, reverse bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*Rule)
	if reverse {
		return n.Block != nil && walkNode(n.Block) ||
			n.Prelude != nil && walkNode(n.Prelude)
	}
	return n.Prelude != nil && walkNode(n.Prelude) ||
		n.Block != nil && walkNode(n.Block)
}

func cloneRule(node Node, clone func(Node) Node) Node {
	c := *node.(*Rule)
	c.Loc = cloneLocation(c.Loc)
	c.Prelude = clone(c.Prelude)
	if c.Block != nil {
		c.Block, _ = clone(c.Block).(*Block)
	}
	return &c
}

func fromPlainObjectRule(r *PlainObjectReader, object map[string]any) Node {
	return &Rule{
		Loc:     r.Location(object),
		Prelude: r.Node(object, "prelude"),
		Block:   PlainObjectNode[*Block](r, object, "block"),
	}
}

// Rule parses a qualified rule.
func (p *Parser) Rule() *Rule {
	startOffset := p.TokenStart
	var prelude Node

	if p.ParseRulePrelude {
		prelude = p.ParseWithFallback(func() Node { return consumePrelude(p) }, func() Node { return ruleConsumeRaw(p) })
	} else {
		prelude = ruleConsumeRaw(p)
	}

	block := p.Block(true)

	return &Rule{
		Loc:     p.GetLocation(startOffset, p.TokenStart),
		Prelude: prelude,
		Block:   block,
	}
}

func generateRule(g *Generator, node Node) {
	n := node.(*Rule)
	g.Node(n.Prelude)
	g.Node(n.Block)
}
