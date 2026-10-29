// Ported from lib/syntax/node/Scope.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Scope is the prelude of @scope: [(<scope-start>)]? [to (<scope-end>)]?
type Scope struct {
	Loc *Location
	// Root and Limit are a *SelectorList, a *Raw or nil.
	Root  Node
	Limit Node
}

func (n *Scope) Type() string        { return "Scope" }
func (n *Scope) Location() *Location { return n.Loc }

func (n *Scope) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"root", n.Root},
		jsonField{"limit", n.Limit},
	)
}

func scopeNodeType() NodeConfig {
	return NodeConfig{
		Name: "Scope",
		Structure: []StructureField{
			{Name: "root", Types: []FieldType{nodeField("SelectorList"), nodeField("Raw"), nullField()}},
			{Name: "limit", Types: []FieldType{nodeField("SelectorList"), nodeField("Raw"), nullField()}},
		},
		Generate:        generateScope,
		Walk:            walkScope,
		Clone:           cloneScope,
		FromPlainObject: fromPlainObjectScope,
	}
}

func walkScope(node Node, reverse bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*Scope)
	if reverse {
		return n.Limit != nil && walkNode(n.Limit) ||
			n.Root != nil && walkNode(n.Root)
	}
	return n.Root != nil && walkNode(n.Root) ||
		n.Limit != nil && walkNode(n.Limit)
}

func cloneScope(node Node, clone func(Node) Node) Node {
	c := *node.(*Scope)
	c.Loc = cloneLocation(c.Loc)
	c.Root = clone(c.Root)
	c.Limit = clone(c.Limit)
	return &c
}

func fromPlainObjectScope(r *PlainObjectReader, object map[string]any) Node {
	return &Scope{
		Loc:   r.Location(object),
		Root:  r.Node(object, "root"),
		Limit: r.Node(object, "limit"),
	}
}

// Scope parses the prelude of @scope.
func (p *Parser) Scope() *Scope {
	var root Node
	var limit Node

	p.SkipSC()

	startOffset := p.TokenStart
	if p.TokenType == tokenizer.LeftParenthesis {
		p.Next()
		p.SkipSC()
		root = p.ParseWithFallback(
			func() Node { return p.SelectorList() },
			func() Node { return p.Raw(nil, true) },
		)
		p.SkipSC()
		p.Eat(tokenizer.RightParenthesis)
	}

	if p.LookupNonWSType(0) == tokenizer.Ident {
		p.SkipSC()
		p.EatIdent("to")
		p.SkipSC()
		p.Eat(tokenizer.LeftParenthesis)
		p.SkipSC()
		limit = p.ParseWithFallback(
			func() Node { return p.SelectorList() },
			func() Node { return p.Raw(nil, true) },
		)
		p.SkipSC()
		p.Eat(tokenizer.RightParenthesis)
	}

	return &Scope{
		Loc:   p.GetLocation(startOffset, p.TokenStart),
		Root:  root,
		Limit: limit,
	}
}

func generateScope(g *Generator, node Node) {
	n := node.(*Scope)
	if n.Root != nil {
		g.Token(tokenizer.LeftParenthesis, "(")
		g.Node(n.Root)
		g.Token(tokenizer.RightParenthesis, ")")
	}

	if n.Limit != nil {
		g.Token(tokenizer.Ident, "to")
		g.Token(tokenizer.LeftParenthesis, "(")
		g.Node(n.Limit)
		g.Token(tokenizer.RightParenthesis, ")")
	}
}
