// Ported from lib/syntax/node/UnicodeRange.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

const questionMark = 0x003F // U+003F QUESTION MARK (?)

func eatHexSequence(p *Parser, offset int, allowDash bool) int {
	length := 0

	for pos := p.TokenStart + offset; pos < p.TokenEnd; pos++ {
		code := p.CharCodeAt(pos)

		if code == hyphenMinus && allowDash && length != 0 {
			eatHexSequence(p, offset+length+1, false)
			return -1
		}

		if !tokenizer.IsHexDigit(code) {
			var message string
			switch {
			case allowDash && length != 0:
				message = "Hyphen minus"
				if length < 6 {
					message += " or hex digit"
				}
				message += " is expected"
			case length < 6:
				message = "Hex digit is expected"
			default:
				message = "Unexpected input"
			}
			p.Error(message, pos)
		}

		length++
		if length > 6 {
			p.Error("Too many hex digits", pos)
		}
	}

	p.Next()
	return length
}

func eatQuestionMarkSequence(p *Parser, maximum int) {
	count := 0

	for p.IsDelim(questionMark, 0) {
		count++
		if count > maximum {
			p.Error("Too many question marks", -1)
		}

		p.Next()
	}
}

func startsWith(p *Parser, code int) {
	if p.CharCodeAt(p.TokenStart) != code {
		var what string
		if code == plusSign {
			what = "Plus sign"
		} else {
			what = "Hyphen minus"
		}
		p.Error(what+" is expected", -1)
	}
}

// https://drafts.csswg.org/css-syntax/#urange
// Informally, the <urange> production has three forms:
// U+0001
//
//	Defines a range consisting of a single code point, in this case the code point "1".
//
// U+0001-00ff
//
//	Defines a range of codepoints between the first and the second value, in this case
//	the range between "1" and "ff" (255 in decimal) inclusive.
//
// U+00??
//
//	Defines a range of codepoints where the "?" characters range over all hex digits,
//	in this case defining the same as the value U+0000-00ff.
//
// In each form, a maximum of 6 digits is allowed for each hexadecimal number (if you treat "?" as a hexadecimal digit).
//
//	<urange> =
//	  u '+' <ident-token> '?'* |
//	  u <dimension-token> '?'* |
//	  u <number-token> '?'* |
//	  u <number-token> <dimension-token> |
//	  u <number-token> <number-token> |
//	  u '+' '?'+
func scanUnicodeRange(p *Parser) {
	hexLength := 0

	switch p.TokenType {
	case tokenizer.Number:
		// u <number-token> '?'*
		// u <number-token> <dimension-token>
		// u <number-token> <number-token>
		hexLength = eatHexSequence(p, 1, true)

		if p.IsDelim(questionMark, 0) {
			eatQuestionMarkSequence(p, 6-hexLength)
			break
		}

		if p.TokenType == tokenizer.Dimension ||
			p.TokenType == tokenizer.Number {
			startsWith(p, hyphenMinus)
			eatHexSequence(p, 1, false)
			break
		}

	case tokenizer.Dimension:
		// u <dimension-token> '?'*
		hexLength = eatHexSequence(p, 1, true)

		if hexLength > 0 {
			eatQuestionMarkSequence(p, 6-hexLength)
		}

	default:
		// u '+' <ident-token> '?'*
		// u '+' '?'+
		p.EatDelim(plusSign)

		if p.TokenType == tokenizer.Ident {
			hexLength = eatHexSequence(p, 0, true)
			if hexLength > 0 {
				eatQuestionMarkSequence(p, 6-hexLength)
			}
			break
		}

		if p.IsDelim(questionMark, 0) {
			p.Next()
			eatQuestionMarkSequence(p, 5)
			break
		}

		p.Error("Hex digit or question mark is expected", -1)
	}
}

// UnicodeRange is a unicode range, e.g. U+0025-00FF.
type UnicodeRange struct {
	Loc   *Location
	Value string
}

func (n *UnicodeRange) Type() string        { return "UnicodeRange" }
func (n *UnicodeRange) Location() *Location { return n.Loc }

func (n *UnicodeRange) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"value", n.Value},
	)
}

func unicodeRangeNodeType() NodeConfig {
	return NodeConfig{
		Name: "UnicodeRange",
		Structure: []StructureField{
			{Name: "value", Types: []FieldType{stringField()}},
		},
		Generate:        generateUnicodeRange,
		Clone:           cloneUnicodeRange,
		FromPlainObject: fromPlainObjectUnicodeRange,
	}
}

func cloneUnicodeRange(node Node, _ func(Node) Node) Node {
	c := *node.(*UnicodeRange)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectUnicodeRange(r *PlainObjectReader, object map[string]any) Node {
	return &UnicodeRange{
		Loc:   r.Location(object),
		Value: r.String(object, "value"),
	}
}

// UnicodeRange parses a unicode range.
func (p *Parser) UnicodeRange() *UnicodeRange {
	start := p.TokenStart

	// U or u
	p.EatIdent("u")
	scanUnicodeRange(p)

	return &UnicodeRange{
		Loc:   p.GetLocation(start, p.TokenStart),
		Value: p.SubstrToCursor(start),
	}
}

func generateUnicodeRange(g *Generator, node Node) {
	n := node.(*UnicodeRange)
	g.Tokenize(n.Value)
}
