// Ported from lib/syntax/node/Url.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// URL is a url, <url-token> | <function-token> <string> ) (upstream Url);
// Value is decoded.
type URL struct {
	Loc   *Location
	Value string
}

func (n *URL) Type() string        { return "Url" }
func (n *URL) Location() *Location { return n.Loc }

func (n *URL) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"value", n.Value},
	)
}

func urlNodeType() NodeConfig {
	return NodeConfig{
		Name: "Url",
		Structure: []StructureField{
			{Name: "value", Types: []FieldType{stringField()}},
		},
		Generate:        generateURL,
		Clone:           cloneURL,
		FromPlainObject: fromPlainObjectURL,
	}
}

func cloneURL(node Node, _ func(Node) Node) Node {
	c := *node.(*URL)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectURL(r *PlainObjectReader, object map[string]any) Node {
	return &URL{
		Loc:   r.Location(object),
		Value: r.String(object, "value"),
	}
}

// URL parses <url-token> | <function-token> <string> ) (upstream Url).
func (p *Parser) URL() *URL {
	start := p.TokenStart
	var value string

	switch p.TokenType {
	case tokenizer.URL:
		value = DecodeURL(p.Consume(tokenizer.URL))

	case tokenizer.Function:
		if !p.CmpStr(p.TokenStart, p.TokenEnd, "url(") {
			p.Error("Function name must be `url`", -1)
		}

		p.Eat(tokenizer.Function)
		p.SkipSC()
		value = DecodeString(p.Consume(tokenizer.String))
		p.SkipSC()
		if !p.EOF {
			p.Eat(tokenizer.RightParenthesis)
		}

	default:
		p.Error("Url or Function is expected", -1)
	}

	return &URL{
		Loc:   p.GetLocation(start, p.TokenStart),
		Value: value,
	}
}

func generateURL(g *Generator, node Node) {
	n := node.(*URL)
	g.Token(tokenizer.URL, EncodeURL(n.Value))
}
