// Ported from lib/syntax/node/AttributeSelector.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

const (
	dollarSign       = 0x0024 // U+0024 DOLLAR SIGN ($)
	equalsSign       = 0x003D // U+003D EQUALS SIGN (=)
	circumflexAccent = 0x005E // U+005E (^)
)

func getAttributeName(p *Parser) *Identifier {
	if p.EOF {
		p.Error("Unexpected end of input", -1)
	}

	start := p.TokenStart
	expectIdent := false

	if p.IsDelim(asterisk, 0) {
		expectIdent = true
		p.Next()
	} else if !p.IsDelim(verticalLine, 0) {
		p.Eat(tokenizer.Ident)
	}

	if p.IsDelim(verticalLine, 0) {
		if p.CharCodeAt(p.TokenStart+1) != equalsSign {
			p.Next()
			p.Eat(tokenizer.Ident)
		} else if expectIdent {
			p.Error("Identifier is expected", p.TokenEnd)
		}
	} else if expectIdent {
		p.Error("Vertical line is expected", -1)
	}

	return &Identifier{
		Loc:  p.GetLocation(start, p.TokenStart),
		Name: p.SubstrToCursor(start),
	}
}

func getOperator(p *Parser) string {
	start := p.TokenStart
	code := p.CharCodeAt(start)

	if code != equalsSign && // =
		code != tilde && // ~=
		code != circumflexAccent && // ^=
		code != dollarSign && // $=
		code != asterisk && // *=
		code != verticalLine { // |=
		p.Error("Attribute selector (=, ~=, ^=, $=, *=, |=) is expected", -1)
	}

	p.Next()

	if code != equalsSign {
		if !p.IsDelim(equalsSign, 0) {
			p.Error("Equal sign is expected", -1)
		}

		p.Next()
	}

	return p.SubstrToCursor(start)
}

// isAttributeModifier reports whether the current ident token is i or s
// (ASCII case-insensitive).
// https://drafts.csswg.org/selectors-4/#attribute-case
func isAttributeModifier(p *Parser) bool {
	return p.TokenEnd-p.TokenStart == 1 &&
		(p.CmpChar(p.TokenStart, 'i') || p.CmpChar(p.TokenStart, 's'))
}

// AttributeSelector is an attribute selector:
//
//	'[' <wq-name> ']'
//	'[' <wq-name> <attr-matcher> [ <string-token> | <ident-token> ] <attr-modifier>? ']'
type AttributeSelector struct {
	Loc  *Location
	Name *Identifier
	// Matcher is the operator ("=", "~=", ...); "" is upstream null.
	Matcher string
	// Value is a *String, an *Identifier or nil.
	Value Node
	// Flags is the modifier; "" is upstream null.
	Flags string
}

func (n *AttributeSelector) Type() string        { return "AttributeSelector" }
func (n *AttributeSelector) Location() *Location { return n.Loc }

func (n *AttributeSelector) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"name", n.Name},
		jsonField{"matcher", nullableString(n.Matcher)},
		jsonField{"value", n.Value},
		jsonField{"flags", nullableString(n.Flags)},
	)
}

func attributeSelectorNodeType() NodeConfig {
	return NodeConfig{
		Name: "AttributeSelector",
		Structure: []StructureField{
			{Name: "name", Types: []FieldType{nodeField("Identifier")}},
			{Name: "matcher", Types: []FieldType{stringField(), nullField()}},
			{Name: "value", Types: []FieldType{nodeField("String"), nodeField("Identifier"), nullField()}},
			{Name: "flags", Types: []FieldType{stringField(), nullField()}},
		},
		Generate:        generateAttributeSelector,
		Walk:            walkAttributeSelector,
		Clone:           cloneAttributeSelector,
		FromPlainObject: fromPlainObjectAttributeSelector,
	}
}

func walkAttributeSelector(node Node, reverse bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*AttributeSelector)
	if reverse {
		return n.Value != nil && walkNode(n.Value) ||
			n.Name != nil && walkNode(n.Name)
	}
	return n.Name != nil && walkNode(n.Name) ||
		n.Value != nil && walkNode(n.Value)
}

func cloneAttributeSelector(node Node, clone func(Node) Node) Node {
	c := *node.(*AttributeSelector)
	c.Loc = cloneLocation(c.Loc)
	if c.Name != nil {
		c.Name, _ = clone(c.Name).(*Identifier)
	}
	c.Value = clone(c.Value)
	return &c
}

func fromPlainObjectAttributeSelector(r *PlainObjectReader, object map[string]any) Node {
	return &AttributeSelector{
		Loc:     r.Location(object),
		Name:    PlainObjectNode[*Identifier](r, object, "name"),
		Matcher: r.String(object, "matcher"),
		Value:   r.Node(object, "value"),
		Flags:   r.String(object, "flags"),
	}
}

// AttributeSelector parses an attribute selector.
func (p *Parser) AttributeSelector() *AttributeSelector {
	start := p.TokenStart
	matcher := ""
	var value Node
	flags := ""

	p.Eat(tokenizer.LeftSquareBracket)
	p.SkipSC()

	name := getAttributeName(p)
	p.SkipSC()

	if p.TokenType != tokenizer.RightSquareBracket {
		// a modifier follows a matcher and a value: [name i] is not an
		// attribute selector
		matcher = getOperator(p)

		p.SkipSC()

		if p.TokenType == tokenizer.String {
			value = p.String()
		} else {
			value = p.Identifier()
		}

		p.SkipSC()

		// attribute modifier: i | s
		if p.TokenType == tokenizer.Ident {
			if !isAttributeModifier(p) {
				p.Error("Attribute selector modifier (i or s) is expected", -1)
			}
			flags = p.Consume(tokenizer.Ident)

			p.SkipSC()
		}
	}

	p.Eat(tokenizer.RightSquareBracket)

	return &AttributeSelector{
		Loc:     p.GetLocation(start, p.TokenStart),
		Name:    name,
		Matcher: matcher,
		Value:   value,
		Flags:   flags,
	}
}

func generateAttributeSelector(g *Generator, node Node) {
	n := node.(*AttributeSelector)
	g.Token(tokenizer.Delim, "[")
	g.Node(n.Name)

	if n.Matcher != "" {
		g.Tokenize(n.Matcher)
		g.Node(n.Value)
	}

	if n.Flags != "" {
		g.Token(tokenizer.Ident, n.Flags)
	}

	g.Token(tokenizer.Delim, "]")
}
