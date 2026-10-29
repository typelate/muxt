// Ported from lib/syntax/node/MediaQuery.js.

package csstree

import (
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// MediaQuery is a media query.
type MediaQuery struct {
	Loc *Location
	// Modifier is "not" or "only"; "" is upstream null.
	Modifier string
	// MediaType is the media type; "" is upstream null.
	MediaType string
	Condition *Condition
}

func (n *MediaQuery) Type() string        { return "MediaQuery" }
func (n *MediaQuery) Location() *Location { return n.Loc }

func (n *MediaQuery) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"modifier", nullableString(n.Modifier)},
		jsonField{"mediaType", nullableString(n.MediaType)},
		jsonField{"condition", n.Condition},
	)
}

func mediaQueryNodeType() NodeConfig {
	return NodeConfig{
		Name: "MediaQuery",
		Structure: []StructureField{
			{Name: "modifier", Types: []FieldType{stringField(), nullField()}},
			{Name: "mediaType", Types: []FieldType{stringField(), nullField()}},
			{Name: "condition", Types: []FieldType{nodeField("Condition"), nullField()}},
		},
		Generate:        generateMediaQuery,
		Walk:            walkMediaQuery,
		Clone:           cloneMediaQuery,
		FromPlainObject: fromPlainObjectMediaQuery,
	}
}

func walkMediaQuery(node Node, _ bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*MediaQuery)
	return n.Condition != nil && walkNode(n.Condition)
}

func cloneMediaQuery(node Node, clone func(Node) Node) Node {
	c := *node.(*MediaQuery)
	c.Loc = cloneLocation(c.Loc)
	if c.Condition != nil {
		c.Condition, _ = clone(c.Condition).(*Condition)
	}
	return &c
}

func fromPlainObjectMediaQuery(r *PlainObjectReader, object map[string]any) Node {
	return &MediaQuery{
		Loc:       r.Location(object),
		Modifier:  r.String(object, "modifier"),
		MediaType: r.String(object, "mediaType"),
		Condition: PlainObjectNode[*Condition](r, object, "condition"),
	}
}

// MediaQuery parses a media query.
func (p *Parser) MediaQuery() *MediaQuery {
	start := p.TokenStart
	modifier := ""
	mediaType := ""
	var condition *Condition

	p.SkipSC()

	if p.TokenType == tokenizer.Ident && p.LookupTypeNonSC(1) != tokenizer.LeftParenthesis {
		// [ not | only ]? <media-type>
		ident := p.Consume(tokenizer.Ident)
		identLowerCase := toLowerCase(ident)

		if identLowerCase == "not" || identLowerCase == "only" {
			p.SkipSC()
			modifier = identLowerCase
			mediaType = p.Consume(tokenizer.Ident)
		} else {
			mediaType = ident
		}

		switch p.LookupTypeNonSC(0) {
		case tokenizer.Ident:
			// and <media-condition-without-or>
			p.SkipSC()
			p.EatIdent("and")
			condition = p.condition("media", false)

		case tokenizer.LeftCurlyBracket, tokenizer.Semicolon, tokenizer.Comma, tokenizer.EOF:

		default:
			p.Error("Identifier or parenthesis is expected", -1)
		}
	} else {
		switch p.TokenType {
		case tokenizer.Ident, tokenizer.LeftParenthesis, tokenizer.Function:
			// <media-condition>
			condition = p.Condition("media")

		case tokenizer.LeftCurlyBracket, tokenizer.Semicolon, tokenizer.EOF:

		default:
			p.Error("Identifier or parenthesis is expected", -1)
		}
	}

	return &MediaQuery{
		Loc:       p.GetLocation(start, p.TokenStart),
		Modifier:  modifier,
		MediaType: mediaType,
		Condition: condition,
	}
}

func generateMediaQuery(g *Generator, node Node) {
	n := node.(*MediaQuery)
	if n.MediaType != "" {
		if n.Modifier != "" {
			g.Token(tokenizer.Ident, n.Modifier)
		}

		g.Token(tokenizer.Ident, n.MediaType)

		if n.Condition != nil {
			g.Token(tokenizer.Ident, "and")
			g.Node(n.Condition)
		}
	} else if n.Condition != nil {
		g.Node(n.Condition)
	}
}
