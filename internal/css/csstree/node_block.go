// Ported from lib/syntax/node/Block.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

func blockConsumeRaw(p *Parser) Node {
	return p.Raw(nil, true)
}

func blockConsumeRule(p *Parser) Node {
	return p.ParseWithFallback(func() Node { return p.Rule() }, func() Node { return blockConsumeRaw(p) })
}

// isNestedRuleAhead reports whether the tokens at the cursor of a style
// block are a nested style rule. CSS Syntax 3 "consume a block's contents"
// parses a declaration first and a qualified rule when that fails; a
// qualified rule in a block needs a {}-block before the next ;.
// Everything else is a declaration, so that what is neither falls back to
// a Raw declaration as upstream does.
// https://drafts.csswg.org/css-syntax-3/#consume-block-contents
// https://drafts.csswg.org/css-nesting-1/#syntax
func isNestedRuleAhead(p *Parser) bool {
	if p.IsDelim(ampersand, 0) {
		return true
	}
	if isDeclarationAhead(p) {
		return false
	}
	end := p.scanUntilBalanced(p.TokenIndex, tokenizer.Semicolon, tokenizer.LeftCurlyBracket)
	return p.GetTokenType(end) == tokenizer.LeftCurlyBracket
}

func blockConsumeRawDeclaration(p *Parser) Node {
	return p.Raw(p.ConsumeUntilSemicolonIncluded, true)
}

func blockConsumeDeclaration(p *Parser) Node {
	if p.TokenType == tokenizer.Semicolon {
		return blockConsumeRawDeclaration(p)
	}

	node := p.ParseWithFallback(func() Node { return p.Declaration() }, func() Node { return blockConsumeRawDeclaration(p) })

	if p.TokenType == tokenizer.Semicolon {
		p.Next()
	}

	return node
}

// Block is a {}-block of rules or declarations.
type Block struct {
	Loc      *Location
	Children *List[Node]
}

func (n *Block) Type() string        { return "Block" }
func (n *Block) Location() *Location { return n.Loc }

func (n *Block) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"children", n.Children},
	)
}

func blockNodeType() NodeConfig {
	return NodeConfig{
		Name:        "Block",
		WalkContext: "block",
		Structure: []StructureField{
			{Name: "children", Types: []FieldType{listField(
				"Atrule",
				"Rule",
				"Declaration",
			)}},
		},
		Generate:        generateBlock,
		Walk:            walkBlock,
		Clone:           cloneBlock,
		FromPlainObject: fromPlainObjectBlock,
	}
}

func walkBlock(node Node, _ bool, _ func(Node) bool, walkList func(*List[Node]) bool) bool {
	n := node.(*Block)
	return n.Children != nil && walkList(n.Children)
}

func cloneBlock(node Node, clone func(Node) Node) Node {
	c := *node.(*Block)
	c.Loc = cloneLocation(c.Loc)
	c.Children = cloneList(c.Children, clone)
	return &c
}

func fromPlainObjectBlock(r *PlainObjectReader, object map[string]any) Node {
	return &Block{
		Loc:      r.Location(object),
		Children: r.List(object, "children"),
	}
}

// Block parses a {}-block; isStyleBlock makes it a block of declarations
// and nested rules instead of a block of rules.
func (p *Parser) Block(isStyleBlock bool) *Block {
	return p.block(isStyleBlock, isStyleBlock)
}

// descriptorBlock parses a {}-block of declarations (descriptors) of an
// at-rule such as @font-face or @page, where a nested style rule is not
// valid: only a block item that starts with & is read as a rule, as
// upstream does, and what looks like a rule is a declaration.
func (p *Parser) descriptorBlock() *Block {
	return p.block(true, false)
}

// block parses a {}-block; isStyleBlock makes it a block of declarations,
// and nestedRules reads nested style rules in it.
func (p *Parser) block(isStyleBlock, nestedRules bool) *Block {
	consumer := blockConsumeRule
	if isStyleBlock {
		consumer = blockConsumeDeclaration
	}
	start := p.TokenStart
	children := p.CreateList()

	p.Eat(tokenizer.LeftCurlyBracket)

scan:
	for !p.EOF {
		switch p.TokenType {
		case tokenizer.RightCurlyBracket:
			break scan

		case tokenizer.WhiteSpace, tokenizer.Comment:
			p.Next()

		case tokenizer.AtKeyword:
			children.Push(p.ParseWithFallback(func() Node { return p.Atrule(isStyleBlock) }, func() Node { return blockConsumeRaw(p) }))

		default:
			if isStyleBlock && (p.IsDelim(ampersand, 0) || nestedRules && isNestedRuleAhead(p)) {
				children.Push(blockConsumeRule(p))
			} else {
				children.Push(consumer(p))
			}
		}
	}

	if !p.EOF {
		p.Eat(tokenizer.RightCurlyBracket)
	}

	return &Block{
		Loc:      p.GetLocation(start, p.TokenStart),
		Children: children,
	}
}

func generateBlock(g *Generator, node Node) {
	n := node.(*Block)
	g.Token(tokenizer.LeftCurlyBracket, "{")
	g.Children(n.Children, func(prev Node) {
		if prev.Type() == "Declaration" {
			g.Token(tokenizer.Semicolon, ";")
		}
	})
	g.Token(tokenizer.RightCurlyBracket, "}")
}
