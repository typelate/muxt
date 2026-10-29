// Ported from lib/syntax/node/Atrule.js.

package csstree

import (
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

func atruleConsumeRaw(p *Parser) Node {
	return p.Raw(p.ConsumeUntilLeftCurlyBracketOrSemicolon, true)
}

func isDeclarationBlockAtrule(p *Parser) bool {
	for offset := 1; ; offset++ {
		tokenType := p.LookupType(offset)
		if tokenType == tokenizer.EOF {
			break
		}

		if tokenType == tokenizer.RightCurlyBracket {
			return true
		}

		if tokenType == tokenizer.LeftCurlyBracket ||
			tokenType == tokenizer.AtKeyword {
			return false
		}
	}

	return false
}

// Atrule is an at-rule such as @media or @import.
type Atrule struct {
	Loc  *Location
	Name string
	// Prelude is an *AtrulePrelude, a *Raw or nil.
	Prelude Node
	// Block is nil for an at-rule ending with a semicolon.
	Block *Block
}

func (n *Atrule) Type() string        { return "Atrule" }
func (n *Atrule) Location() *Location { return n.Loc }

func (n *Atrule) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"name", n.Name},
		jsonField{"prelude", n.Prelude},
		jsonField{"block", n.Block},
	)
}

func atruleNodeType() NodeConfig {
	return NodeConfig{
		Name:        "Atrule",
		WalkContext: "atrule",
		Structure: []StructureField{
			{Name: "name", Types: []FieldType{stringField()}},
			{Name: "prelude", Types: []FieldType{nodeField("AtrulePrelude"), nodeField("Raw"), nullField()}},
			{Name: "block", Types: []FieldType{nodeField("Block"), nullField()}},
		},
		Generate:        generateAtrule,
		Walk:            walkAtrule,
		Clone:           cloneAtrule,
		FromPlainObject: fromPlainObjectAtrule,
	}
}

func walkAtrule(node Node, reverse bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*Atrule)
	if reverse {
		return n.Block != nil && walkNode(n.Block) ||
			n.Prelude != nil && walkNode(n.Prelude)
	}
	return n.Prelude != nil && walkNode(n.Prelude) ||
		n.Block != nil && walkNode(n.Block)
}

func cloneAtrule(node Node, clone func(Node) Node) Node {
	c := *node.(*Atrule)
	c.Loc = cloneLocation(c.Loc)
	c.Prelude = clone(c.Prelude)
	if c.Block != nil {
		c.Block, _ = clone(c.Block).(*Block)
	}
	return &c
}

func fromPlainObjectAtrule(r *PlainObjectReader, object map[string]any) Node {
	return &Atrule{
		Loc:     r.Location(object),
		Name:    r.String(object, "name"),
		Prelude: r.Node(object, "prelude"),
		Block:   PlainObjectNode[*Block](r, object, "block"),
	}
}

// Atrule parses an at-rule; isDeclaration reports that it is nested in a
// declaration block.
func (p *Parser) Atrule(isDeclaration bool) *Atrule {
	start := p.TokenStart
	var prelude Node
	var block *Block

	p.Eat(tokenizer.AtKeyword)

	name := p.SubstrToCursor(start + 1)
	nameLowerCase := toLowerCase(name)
	p.SkipSC()

	// parse prelude
	if !p.EOF &&
		p.TokenType != tokenizer.LeftCurlyBracket &&
		p.TokenType != tokenizer.Semicolon {
		if p.ParseAtrulePrelude {
			prelude = p.ParseWithFallback(func() Node { return p.AtrulePrelude(name) }, func() Node { return atruleConsumeRaw(p) })
		} else {
			prelude = atruleConsumeRaw(p)
		}

		p.SkipSC()
	}

	switch p.TokenType {
	case tokenizer.Semicolon:
		p.Next()

	case tokenizer.LeftCurlyBracket:
		if atrule, ok := p.Config.Atrule[nameLowerCase]; ok &&
			atrule.Block != nil {
			block = atrule.Block(p, isDeclaration)
		} else {
			// TODO: should consume block content as Raw?
			block = p.Block(isDeclarationBlockAtrule(p))
		}
	}

	return &Atrule{
		Loc:     p.GetLocation(start, p.TokenStart),
		Name:    name,
		Prelude: prelude,
		Block:   block,
	}
}

func generateAtrule(g *Generator, node Node) {
	n := node.(*Atrule)
	g.Token(tokenizer.AtKeyword, "@"+n.Name)

	if n.Prelude != nil {
		g.Node(n.Prelude)
	}

	if n.Block != nil {
		g.Node(n.Block)
	} else {
		g.Token(tokenizer.Semicolon, ";")
	}
}
