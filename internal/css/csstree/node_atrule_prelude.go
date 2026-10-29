// Ported from lib/syntax/node/AtrulePrelude.js.

package csstree

import (
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// AtrulePrelude is the prelude of an at-rule, between its name and its
// block or semicolon.
type AtrulePrelude struct {
	Loc      *Location
	Children *List[Node]
}

func (n *AtrulePrelude) Type() string        { return "AtrulePrelude" }
func (n *AtrulePrelude) Location() *Location { return n.Loc }

func (n *AtrulePrelude) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"children", n.Children},
	)
}

func atrulePreludeNodeType() NodeConfig {
	return NodeConfig{
		Name:        "AtrulePrelude",
		WalkContext: "atrulePrelude",
		Structure: []StructureField{
			{Name: "children", Types: []FieldType{listField()}},
		},
		Generate:        generateAtrulePrelude,
		Walk:            walkAtrulePrelude,
		Clone:           cloneAtrulePrelude,
		FromPlainObject: fromPlainObjectAtrulePrelude,
	}
}

func walkAtrulePrelude(node Node, _ bool, _ func(Node) bool, walkList func(*List[Node]) bool) bool {
	n := node.(*AtrulePrelude)
	return n.Children != nil && walkList(n.Children)
}

func cloneAtrulePrelude(node Node, clone func(Node) Node) Node {
	c := *node.(*AtrulePrelude)
	c.Loc = cloneLocation(c.Loc)
	c.Children = cloneList(c.Children, clone)
	return &c
}

func fromPlainObjectAtrulePrelude(r *PlainObjectReader, object map[string]any) Node {
	return &AtrulePrelude{
		Loc:      r.Location(object),
		Children: r.List(object, "children"),
	}
}

// AtrulePrelude parses the prelude of the at-rule name ("" is upstream
// null) with the at-rule's prelude parser, or the AtrulePrelude scope.
func (p *Parser) AtrulePrelude(name string) *AtrulePrelude {
	var children *List[Node]

	name = toLowerCase(name)

	p.SkipSC()

	if atrule, ok := p.Config.Atrule[name]; ok &&
		atrule.Prelude != nil {
		// custom consumer
		children = atrule.Prelude(p)
	} else {
		// default consumer
		children = p.ReadSequence(p.Config.Scope["AtrulePrelude"])
	}

	p.SkipSC()

	if !p.EOF &&
		p.TokenType != tokenizer.LeftCurlyBracket &&
		p.TokenType != tokenizer.Semicolon {
		p.Error("Semicolon or block is expected", -1)
	}

	return &AtrulePrelude{
		Loc:      p.GetLocationFromList(children),
		Children: children,
	}
}

func generateAtrulePrelude(g *Generator, node Node) {
	n := node.(*AtrulePrelude)
	g.Children(n.Children, nil)
}
