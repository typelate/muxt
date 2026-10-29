// Ported from lib/syntax/node/StyleSheet.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

func styleSheetConsumeRaw(p *Parser) Node {
	return p.Raw(nil, false)
}

// StyleSheet is a style sheet.
type StyleSheet struct {
	Loc      *Location
	Children *List[Node]
}

func (n *StyleSheet) Type() string        { return "StyleSheet" }
func (n *StyleSheet) Location() *Location { return n.Loc }

func (n *StyleSheet) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"children", n.Children},
	)
}

func styleSheetNodeType() NodeConfig {
	return NodeConfig{
		Name:        "StyleSheet",
		WalkContext: "stylesheet",
		Structure: []StructureField{
			{Name: "children", Types: []FieldType{listField(
				"Comment",
				"CDO",
				"CDC",
				"Atrule",
				"Rule",
				"Raw",
			)}},
		},
		Generate:        generateStyleSheet,
		Walk:            walkStyleSheet,
		Clone:           cloneStyleSheet,
		FromPlainObject: fromPlainObjectStyleSheet,
	}
}

func walkStyleSheet(node Node, _ bool, _ func(Node) bool, walkList func(*List[Node]) bool) bool {
	n := node.(*StyleSheet)
	return n.Children != nil && walkList(n.Children)
}

func cloneStyleSheet(node Node, clone func(Node) Node) Node {
	c := *node.(*StyleSheet)
	c.Loc = cloneLocation(c.Loc)
	c.Children = cloneList(c.Children, clone)
	return &c
}

func fromPlainObjectStyleSheet(r *PlainObjectReader, object map[string]any) Node {
	return &StyleSheet{
		Loc:      r.Location(object),
		Children: r.List(object, "children"),
	}
}

// StyleSheet parses a style sheet.
func (p *Parser) StyleSheet() *StyleSheet {
	start := p.TokenStart
	children := p.CreateList()
	var child Node

	for !p.EOF {
		switch p.TokenType {
		case tokenizer.WhiteSpace:
			p.Next()
			continue

		case tokenizer.Comment:
			// ignore comments except exclamation comments (i.e. /*! .. */) on top level
			if p.CharCodeAt(p.TokenStart+2) != exclamationMark {
				p.Next()
				continue
			}

			child = p.Comment()

		case tokenizer.CDO: // <!--
			child = p.CDO()

		case tokenizer.CDC: // -->
			child = p.CDC()

		// CSS Syntax Module Level 3
		// §2.2 Error handling
		// At the "top level" of a stylesheet, an <at-keyword-token> starts an at-rule.
		case tokenizer.AtKeyword:
			child = p.ParseWithFallback(func() Node { return p.Atrule(false) }, func() Node { return styleSheetConsumeRaw(p) })

		// Anything else starts a qualified rule ...
		default:
			child = p.ParseWithFallback(func() Node { return p.Rule() }, func() Node { return styleSheetConsumeRaw(p) })
		}

		children.Push(child)
	}

	return &StyleSheet{
		Loc:      p.GetLocation(start, p.TokenStart),
		Children: children,
	}
}

func generateStyleSheet(g *Generator, node Node) {
	n := node.(*StyleSheet)
	g.Children(n.Children, nil)
}
