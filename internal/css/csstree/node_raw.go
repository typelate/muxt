// Ported from lib/syntax/node/Raw.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

func getOffsetExcludeWS(p *Parser) int {
	if p.TokenIndex > 0 {
		if p.LookupType(-1) == tokenizer.WhiteSpace {
			if p.TokenIndex > 1 {
				return p.GetTokenStart(p.TokenIndex - 1)
			}
			return p.GetTokenStart(0) // upstream this.firstCharOffset
		}
	}

	return p.TokenStart
}

// Raw is source text kept as is.
type Raw struct {
	Loc   *Location
	Value string
}

func (n *Raw) Type() string        { return "Raw" }
func (n *Raw) Location() *Location { return n.Loc }

func (n *Raw) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"value", n.Value},
	)
}

func rawNodeType() NodeConfig {
	return NodeConfig{
		Name: "Raw",
		Structure: []StructureField{
			{Name: "value", Types: []FieldType{stringField()}},
		},
		Generate:        generateRaw,
		Clone:           cloneRaw,
		FromPlainObject: fromPlainObjectRaw,
	}
}

func cloneRaw(node Node, _ func(Node) Node) Node {
	c := *node.(*Raw)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectRaw(r *PlainObjectReader, object map[string]any) Node {
	return &Raw{
		Loc:   r.Location(object),
		Value: r.String(object, "value"),
	}
}

// Raw consumes tokens up to the end of the enclosing block, or until
// consumeUntil stops (see ConsumeUntilBalanceEnd); a nil consumeUntil is
// ConsumeUntilBalanceEnd. excludeWhiteSpace leaves trailing whitespace
// out of the value.
func (p *Parser) Raw(consumeUntil func(code int) int, excludeWhiteSpace bool) *Raw {
	startOffset := p.GetTokenStart(p.TokenIndex)
	var endOffset int

	if consumeUntil == nil {
		consumeUntil = p.ConsumeUntilBalanceEnd
	}
	p.SkipUntilBalanced(p.TokenIndex, consumeUntil)

	if excludeWhiteSpace && p.TokenStart > startOffset {
		endOffset = getOffsetExcludeWS(p)
	} else {
		endOffset = p.TokenStart
	}

	return &Raw{
		Loc:   p.GetLocation(startOffset, endOffset),
		Value: p.Substring(startOffset, endOffset),
	}
}

func generateRaw(g *Generator, node Node) {
	n := node.(*Raw)
	g.Tokenize(n.Value)
}
