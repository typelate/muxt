// Ported from lib/syntax/node/Declaration.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

// consumeValueRaw consumes the tokens up to the token index end as a Raw
// node, without trailing whitespace when excludeWhiteSpace is set.
func consumeValueRaw(p *Parser, end int, excludeWhiteSpace bool) *Raw {
	startOffset := p.GetTokenStart(p.TokenIndex)
	p.Skip(end - p.TokenIndex)

	endOffset := p.TokenStart
	if excludeWhiteSpace && p.TokenStart > startOffset {
		endOffset = getOffsetExcludeWS(p)
	}

	return &Raw{
		Loc:   p.GetLocation(startOffset, endOffset),
		Value: p.Substring(startOffset, endOffset),
	}
}

// consumeValue parses the tokens up to the token index end as a Value.
func consumeValue(p *Parser, end int) Node {
	value := p.Value()

	if value.Type() != "Raw" && p.TokenIndex != end {
		p.Error("", -1)
	}

	return value
}

// declarationValueEnd returns the index of the token that ends the value
// of a declaration that starts at the token index start and whether the value is followed by
// ! and an identifier. CSS Syntax 3 "consume a declaration" takes the
// component values up to a ; (or the end of the block) and removes the
// last two non-whitespace ones when they are ! and important; the port
// also splits off ! and any other identifier, for upstream's support of
// hacks like !ie.
// https://drafts.csswg.org/css-syntax-3/#consume-declaration
func declarationValueEnd(p *Parser, start int) (end int, important bool) {
	end = p.scanUntilBalanced(start, tokenizer.Semicolon)

	ident := lastNonSC(p, start, end)
	if ident < 0 || p.GetTokenType(ident) != tokenizer.Ident {
		return end, false
	}
	bang := lastNonSC(p, start, ident)
	if bang < 0 || !p.IsDelim(exclamationMark, bang-p.TokenIndex) {
		return end, false
	}
	return bang, true
}

// lastNonSC returns the index of the last token in [start, end) that is
// not whitespace or a comment, or -1.
func lastNonSC(p *Parser, start, end int) int {
	for i := end - 1; i >= start; i-- {
		switch p.GetTokenType(i) {
		case tokenizer.WhiteSpace, tokenizer.Comment:
		default:
			return i
		}
	}
	return -1
}

// Declaration is a property declaration.
type Declaration struct {
	Loc *Location
	// Important is "" for no !important (upstream false), "important" for
	// !important (upstream true), or the text after ! when it differs from
	// "important", e.g. "IMPORTANT" or "ie" (upstream keeps that string).
	Important string
	Property  string
	// Value is a *Value or a *Raw.
	Value Node
}

func (n *Declaration) Type() string        { return "Declaration" }
func (n *Declaration) Location() *Location { return n.Loc }

func (n *Declaration) MarshalJSON() ([]byte, error) {
	var important any
	switch n.Important {
	case "":
		important = false
	case "important":
		important = true
	default:
		important = n.Important
	}
	return marshalNode(n,
		jsonField{"important", important},
		jsonField{"property", n.Property},
		jsonField{"value", n.Value},
	)
}

func declarationNodeType() NodeConfig {
	return NodeConfig{
		Name:        "Declaration",
		WalkContext: "declaration",
		Structure: []StructureField{
			{Name: "important", Types: []FieldType{booleanField(), stringField()}},
			{Name: "property", Types: []FieldType{stringField()}},
			{Name: "value", Types: []FieldType{nodeField("Value"), nodeField("Raw")}},
		},
		Generate:        generateDeclaration,
		Walk:            walkDeclaration,
		Clone:           cloneDeclaration,
		FromPlainObject: fromPlainObjectDeclaration,
	}
}

func walkDeclaration(node Node, _ bool, walkNode func(Node) bool, _ func(*List[Node]) bool) bool {
	n := node.(*Declaration)
	return n.Value != nil && walkNode(n.Value)
}

func cloneDeclaration(node Node, clone func(Node) Node) Node {
	c := *node.(*Declaration)
	c.Loc = cloneLocation(c.Loc)
	c.Value = clone(c.Value)
	return &c
}

func fromPlainObjectDeclaration(r *PlainObjectReader, object map[string]any) Node {
	return &Declaration{
		Loc:       r.Location(object),
		Important: plainObjectImportant(r, object),
		Property:  r.String(object, "property"),
		Value:     r.Node(object, "value"),
	}
}

// plainObjectImportant reads Declaration.important: false, true or a
// string.
func plainObjectImportant(r *PlainObjectReader, object map[string]any) string {
	switch value := object["important"].(type) {
	case nil:
		return ""
	case bool:
		if value {
			return "important"
		}
		return ""
	case string:
		return value
	default:
		r.fieldError(object, "important", "a boolean or a string")
		return ""
	}
}

// Declaration parses a declaration.
func (p *Parser) Declaration() *Declaration {
	start := p.TokenStart
	startToken := p.TokenIndex
	property := readProperty(p)
	customProperty := IsCustomProperty(property, 0)
	parseValue := p.ParseValue
	if customProperty {
		parseValue = p.ParseCustomProperty
	}
	important := ""
	var value Node

	p.SkipSC()
	p.Eat(tokenizer.Colon)

	valueStart := p.TokenIndex

	if !customProperty {
		p.SkipSC()
	}

	valueEnd, hasImportant := declarationValueEnd(p, p.TokenIndex)
	consumeRaw := func() Node { return consumeValueRaw(p, valueEnd, !customProperty) }

	if parseValue {
		value = p.ParseWithFallback(func() Node { return consumeValue(p, valueEnd) }, consumeRaw)
	} else {
		value = consumeRaw()
	}

	if value, ok := value.(*Value); customProperty && ok && value.Children.IsEmpty() {
		for offset := valueStart - p.TokenIndex; offset <= 0; offset++ {
			if p.LookupType(offset) == tokenizer.WhiteSpace {
				value.Children.AppendData(&WhiteSpace{
					Loc:   nil,
					Value: " ",
				})
				break
			}
		}
	}

	if hasImportant {
		important = getImportant(p)
		p.SkipSC()
	}

	// Do not include semicolon to range per spec
	// https://drafts.csswg.org/css-syntax/#declaration-diagram

	if !p.EOF &&
		p.TokenType != tokenizer.Semicolon &&
		!p.IsBalanceEdge(startToken) {
		p.Error("", -1)
	}

	return &Declaration{
		Loc:       p.GetLocation(start, p.TokenStart),
		Important: important,
		Property:  property,
		Value:     value,
	}
}

func generateDeclaration(g *Generator, node Node) {
	n := node.(*Declaration)
	g.Token(tokenizer.Ident, n.Property)
	g.Token(tokenizer.Colon, ":")
	g.Node(n.Value)

	if n.Important != "" {
		g.Token(tokenizer.Delim, "!")
		g.Token(tokenizer.Ident, n.Important)
	}
}

// isDeclarationAhead reports whether the tokens at the cursor are a
// declaration as CSS Syntax 3 "consume a declaration" reads one, to tell a
// declaration from a nested rule: a property name (an identifier, or a
// hack readProperty accepts), a colon and a value that does not mix a
// {}-block with anything else (any value for a custom property). A
// trailing ! and an identifier are not part of the value.
// https://drafts.csswg.org/css-syntax-3/#consume-declaration
func isDeclarationAhead(p *Parser) bool {
	i := p.TokenIndex

	// hacks, as readProperty reads them
	if p.GetTokenType(i) == tokenizer.Delim {
		switch p.CharCodeAt(p.GetTokenStart(i)) {
		case asterisk, dollarSign, plusSign, numberSign, ampersand:
			i++

		case solidus:
			i++
			if p.GetTokenType(i) == tokenizer.Delim && p.CharCodeAt(p.GetTokenStart(i)) == solidus {
				i++
			}
		}
	}

	if tokenType := p.GetTokenType(i); tokenType != tokenizer.Ident && tokenType != tokenizer.Hash {
		return false
	}
	property := p.Substring(p.TokenStart, p.GetTokenEnd(i))

	i = nextNonSC(p, i+1)
	if p.GetTokenType(i) != tokenizer.Colon {
		return false
	}

	if IsCustomProperty(property, 0) {
		return true
	}

	// a {}-block may only be the whole value
	valueEnd, _ := declarationValueEnd(p, i+1)
	block, other := false, false
	for j := i + 1; j < valueEnd; j++ {
		tokenType := p.GetTokenType(j)
		switch tokenType {
		case tokenizer.WhiteSpace, tokenizer.Comment:
			continue
		case tokenizer.LeftCurlyBracket:
			block = true
		default:
			other = true
		}
		// skip the rest of a block
		if p.IsBlockOpenerTokenType(tokenType) && p.Balance[j] > j {
			j = p.Balance[j]
		}
	}

	return !block || !other
}

// nextNonSC returns the index of the first token from index that is not
// whitespace or a comment.
func nextNonSC(p *Parser, index int) int {
	for index < p.TokenCount {
		switch p.GetTokenType(index) {
		case tokenizer.WhiteSpace, tokenizer.Comment:
			index++
		default:
			return index
		}
	}
	return p.TokenCount
}

func readProperty(p *Parser) string {
	start := p.TokenStart

	// hacks
	if p.TokenType == tokenizer.Delim {
		switch p.CharCodeAt(p.TokenStart) {
		case asterisk, dollarSign, plusSign, numberSign, ampersand:
			p.Next()

		// TODO: not sure we should support this hack
		case solidus:
			p.Next()
			if p.IsDelim(solidus, 0) {
				p.Next()
			}
		}
	}

	if p.TokenType == tokenizer.Hash {
		p.Eat(tokenizer.Hash)
	} else {
		p.Eat(tokenizer.Ident)
	}

	return p.SubstrToCursor(start)
}

// ! ws* important
func getImportant(p *Parser) string {
	p.Eat(tokenizer.Delim)
	p.SkipSC()

	important := p.Consume(tokenizer.Ident)

	// store original value in case it differ from `important`
	// for better original source restoring and hacks like `!ie` support
	// (upstream returns true for "important"; Declaration.Important keeps
	// the string)
	return important
}
