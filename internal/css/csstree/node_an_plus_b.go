// Ported from lib/syntax/node/AnPlusB.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

const (
	latinSmallLetterN = 0x006E // U+006E LATIN SMALL LETTER N (n)
	disallowSign      = true
	allowSign         = false
)

func checkInteger(p *Parser, offset int, disallowSign bool) {
	pos := p.TokenStart + offset
	code := p.CharCodeAt(pos)

	if code == plusSign || code == hyphenMinus {
		if disallowSign {
			p.Error("Number sign is not allowed", -1)
		}
		pos++
	}

	for ; pos < p.TokenEnd; pos++ {
		if !tokenizer.IsDigit(p.CharCodeAt(pos)) {
			p.Error("Integer is expected", pos)
		}
	}
}

func checkTokenIsInteger(p *Parser, disallowSign bool) {
	checkInteger(p, 0, disallowSign)
}

func expectCharCode(p *Parser, offset, code int) {
	if !p.CmpChar(p.TokenStart+offset, code) {
		msg := ""

		switch code {
		case latinSmallLetterN:
			msg = "N is expected"
		case hyphenMinus:
			msg = "HyphenMinus is expected"
		}

		p.Error(msg, p.TokenStart+offset)
	}
}

// ... <signed-integer>
// ... ['+' | '-'] <signless-integer>
//
// consumeB returns "" for upstream null.
func consumeB(p *Parser) string {
	offset := 0
	sign := 0
	tokenType := p.TokenType

	for tokenType == tokenizer.WhiteSpace || tokenType == tokenizer.Comment {
		offset++
		tokenType = p.LookupType(offset)
	}

	if tokenType != tokenizer.Number {
		if p.IsDelim(plusSign, offset) ||
			p.IsDelim(hyphenMinus, offset) {
			if p.IsDelim(plusSign, offset) {
				sign = plusSign
			} else {
				sign = hyphenMinus
			}

			for {
				offset++
				tokenType = p.LookupType(offset)
				if tokenType != tokenizer.WhiteSpace && tokenType != tokenizer.Comment {
					break
				}
			}

			if tokenType != tokenizer.Number {
				p.Skip(offset)
				checkTokenIsInteger(p, disallowSign)
			}
		} else {
			return ""
		}
	}

	if offset > 0 {
		p.Skip(offset)
	}

	if sign == 0 {
		code := p.CharCodeAt(p.TokenStart)
		if code != plusSign && code != hyphenMinus {
			p.Error("Number sign is expected", -1)
		}
	}

	checkTokenIsInteger(p, sign != 0)
	if sign == hyphenMinus {
		return "-" + p.Consume(tokenizer.Number)
	}
	return p.Consume(tokenizer.Number)
}

// AnPlusB is the An+B microsyntax https://www.w3.org/TR/css-syntax-3/#anb
type AnPlusB struct {
	Loc *Location
	// A is the coefficient of n; "" is upstream null (no n).
	A string
	// B is the constant; "" is upstream null (no constant).
	B string
}

func (n *AnPlusB) Type() string        { return "AnPlusB" }
func (n *AnPlusB) Location() *Location { return n.Loc }

func (n *AnPlusB) MarshalJSON() ([]byte, error) {
	return marshalNode(n,
		jsonField{"a", nullableString(n.A)},
		jsonField{"b", nullableString(n.B)},
	)
}

func anPlusBNodeType() NodeConfig {
	return NodeConfig{
		Name: "AnPlusB",
		Structure: []StructureField{
			{Name: "a", Types: []FieldType{stringField(), nullField()}},
			{Name: "b", Types: []FieldType{stringField(), nullField()}},
		},
		Generate:        generateAnPlusB,
		Clone:           cloneAnPlusB,
		FromPlainObject: fromPlainObjectAnPlusB,
	}
}

func cloneAnPlusB(node Node, _ func(Node) Node) Node {
	c := *node.(*AnPlusB)
	c.Loc = cloneLocation(c.Loc)
	return &c
}

func fromPlainObjectAnPlusB(r *PlainObjectReader, object map[string]any) Node {
	return &AnPlusB{
		Loc: r.Location(object),
		A:   r.String(object, "a"),
		B:   r.String(object, "b"),
	}
}

// AnPlusB parses an An+B value.
func (p *Parser) AnPlusB() *AnPlusB {
	start := p.TokenStart
	a := ""
	b := ""

	switch {
	// <integer>
	case p.TokenType == tokenizer.Number:
		checkTokenIsInteger(p, allowSign)
		b = p.Consume(tokenizer.Number)

	// -n
	// -n <signed-integer>
	// -n ['+' | '-'] <signless-integer>
	// -n- <signless-integer>
	// <dashndashdigit-ident>
	case p.TokenType == tokenizer.Ident && p.CmpChar(p.TokenStart, hyphenMinus):
		a = "-1"

		expectCharCode(p, 1, latinSmallLetterN)

		switch p.TokenEnd - p.TokenStart {
		// -n
		// -n <signed-integer>
		// -n ['+' | '-'] <signless-integer>
		case 2:
			p.Next()
			b = consumeB(p)

		// -n- <signless-integer>
		case 3:
			expectCharCode(p, 2, hyphenMinus)

			p.Next()
			p.SkipSC()

			checkTokenIsInteger(p, disallowSign)

			b = "-" + p.Consume(tokenizer.Number)

		// <dashndashdigit-ident>
		default:
			expectCharCode(p, 2, hyphenMinus)
			checkInteger(p, 3, disallowSign)
			p.Next()

			b = p.SubstrToCursor(start + 2)
		}

	// '+'? n
	// '+'? n <signed-integer>
	// '+'? n ['+' | '-'] <signless-integer>
	// '+'? n- <signless-integer>
	// '+'? <ndashdigit-ident>
	case p.TokenType == tokenizer.Ident || (p.IsDelim(plusSign, 0) && p.LookupType(1) == tokenizer.Ident):
		sign := 0
		a = "1"

		// just ignore a plus
		if p.IsDelim(plusSign, 0) {
			sign = 1
			p.Next()
		}

		expectCharCode(p, 0, latinSmallLetterN)

		switch p.TokenEnd - p.TokenStart {
		// '+'? n
		// '+'? n <signed-integer>
		// '+'? n ['+' | '-'] <signless-integer>
		case 1:
			p.Next()
			b = consumeB(p)

		// '+'? n- <signless-integer>
		case 2:
			expectCharCode(p, 1, hyphenMinus)

			p.Next()
			p.SkipSC()

			checkTokenIsInteger(p, disallowSign)

			b = "-" + p.Consume(tokenizer.Number)

		// '+'? <ndashdigit-ident>
		default:
			expectCharCode(p, 1, hyphenMinus)
			checkInteger(p, 2, disallowSign)
			p.Next()

			b = p.SubstrToCursor(start + sign + 1)
		}

	// <ndashdigit-dimension>
	// <ndash-dimension> <signless-integer>
	// <n-dimension>
	// <n-dimension> <signed-integer>
	// <n-dimension> ['+' | '-'] <signless-integer>
	case p.TokenType == tokenizer.Dimension:
		code := p.CharCodeAt(p.TokenStart)
		sign := 0
		if code == plusSign || code == hyphenMinus {
			sign = 1
		}
		i := p.TokenStart + sign

		for ; i < p.TokenEnd; i++ {
			if !tokenizer.IsDigit(p.CharCodeAt(i)) {
				break
			}
		}

		if i == p.TokenStart+sign {
			p.Error("Integer is expected", p.TokenStart+sign)
		}

		expectCharCode(p, i-p.TokenStart, latinSmallLetterN)
		a = p.Substring(start, i)

		// <n-dimension>
		// <n-dimension> <signed-integer>
		// <n-dimension> ['+' | '-'] <signless-integer>
		if i+1 == p.TokenEnd {
			p.Next()
			b = consumeB(p)
		} else {
			expectCharCode(p, i-p.TokenStart+1, hyphenMinus)

			// <ndash-dimension> <signless-integer>
			if i+2 == p.TokenEnd {
				p.Next()
				p.SkipSC()
				checkTokenIsInteger(p, disallowSign)
				b = "-" + p.Consume(tokenizer.Number)
			} else {
				// <ndashdigit-dimension>
				checkInteger(p, i-p.TokenStart+2, disallowSign)
				p.Next()
				b = p.SubstrToCursor(i + 1)
			}
		}

	default:
		p.Error("", -1)
	}

	if a != "" && a[0] == plusSign {
		a = a[1:]
	}

	if b != "" && b[0] == plusSign {
		b = b[1:]
	}

	return &AnPlusB{
		Loc: p.GetLocation(start, p.TokenStart),
		A:   a,
		B:   b,
	}
}

func generateAnPlusB(g *Generator, node Node) {
	n := node.(*AnPlusB)
	if n.A != "" {
		var a string
		switch n.A {
		case "+1", "1":
			a = "n"
		case "-1":
			a = "-n"
		default:
			a = n.A + "n"
		}

		if n.B != "" {
			b := n.B
			if n.B[0] != '-' && n.B[0] != '+' {
				b = "+" + n.B
			}
			g.Tokenize(a + b)
		} else {
			g.Tokenize(a)
		}
	} else {
		g.Tokenize(n.B)
	}
}
