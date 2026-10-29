// Ported from lib/syntax/node/MediaQueryList.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// MediaQueryList is a comma separated list of media queries.
type MediaQueryList struct {
	Loc      *Location
	Children *List[Node]
}

func (n *MediaQueryList) Type() string        { return "MediaQueryList" }
func (n *MediaQueryList) Location() *Location { return n.Loc }

func (n *MediaQueryList) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"children", n.Children},
	)
}

func mediaQueryListNodeType() NodeConfig {
	return NodeConfig{
		Name: "MediaQueryList",
		Structure: []StructureField{
			{Name: "children", Types: []FieldType{listField(
				"MediaQuery",
			)}},
		},
		Generate:        generateMediaQueryList,
		Walk:            walkMediaQueryList,
		Clone:           cloneMediaQueryList,
		FromPlainObject: fromPlainObjectMediaQueryList,
	}
}

func walkMediaQueryList(node Node, _ bool, _ func(Node) bool, walkList func(*List[Node]) bool) bool {
	n := node.(*MediaQueryList)
	return n.Children != nil && walkList(n.Children)
}

func cloneMediaQueryList(node Node, clone func(Node) Node) Node {
	c := *node.(*MediaQueryList)
	c.Loc = cloneLocation(c.Loc)
	c.Children = cloneList(c.Children, clone)
	return &c
}

func fromPlainObjectMediaQueryList(r *PlainObjectReader, object map[string]any) Node {
	return &MediaQueryList{
		Loc:      r.Location(object),
		Children: r.List(object, "children"),
	}
}

// MediaQueryList parses a list of media queries.
func (p *Parser) MediaQueryList() *MediaQueryList {
	children := p.CreateList()

	p.SkipSC()

	for !p.EOF {
		children.Push(p.MediaQuery())

		if p.TokenType != tokenizer.Comma {
			break
		}

		p.Next()
	}

	return &MediaQueryList{
		Loc:      p.GetLocationFromList(children),
		Children: children,
	}
}

func generateMediaQueryList(g *Generator, node Node) {
	n := node.(*MediaQueryList)
	g.Children(n.Children, func(Node) { g.Token(tokenizer.Comma, ",") })
}
