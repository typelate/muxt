// Ported from lib/syntax/node/Comment.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// Comment is a /* */ comment kept in the AST (exclamation comments at the
// top level of a style sheet).
type Comment struct {
	Loc   *Location
	Value string
}

func (n *Comment) Type() string        { return "Comment" }
func (n *Comment) Location() *Location { return n.Loc }

func (n *Comment) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"value", n.Value},
	)
}

func commentNodeType() NodeConfig {
	return NodeConfig{
		Name: "Comment",
		Structure: []StructureField{
			{Name: "value", Types: []FieldType{stringField()}},
		},
		Generate:        generateComment,
		Clone:           cloneComment,
		FromPlainObject: fromPlainObjectComment,
	}
}

func cloneComment(node Node, _ func(Node) Node) Node {
	c := *node.(*Comment)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectComment(r *PlainObjectReader, object map[string]any) Node {
	return &Comment{
		Loc:   r.Location(object),
		Value: r.String(object, "value"),
	}
}

// Comment parses a comment token.
func (p *Parser) Comment() *Comment {
	start := p.TokenStart
	end := p.TokenEnd

	p.Eat(tokenizer.Comment)

	if (end-start+2) >= 2 &&
		p.CharCodeAt(end-2) == asterisk &&
		p.CharCodeAt(end-1) == solidus {
		end -= 2
	}

	return &Comment{
		Loc:   p.GetLocation(start, p.TokenStart),
		Value: p.Substring(start+2, end),
	}
}

func generateComment(g *Generator, node Node) {
	n := node.(*Comment)
	g.Token(tokenizer.Comment, "/*"+n.Value+"*/")
}
