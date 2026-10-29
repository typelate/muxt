// Ported from lib/syntax/scope/selector.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

const (
	ampersand       = 0x0026 // U+0026 AMPERSAND (&)
	fullStop        = 0x002E // U+002E FULL STOP (.)
	greaterThanSign = 0x003E // U+003E GREATER-THAN SIGN (>)
	verticalLine    = 0x007C // U+007C VERTICAL LINE (|)
	tilde           = 0x007E // U+007E TILDE (~)
)

func selectorOnWhiteSpace(_ *Parser, next Node, children *List[Node], _ *SequenceContext) {
	if last := children.Last(); last != nil && last.Type() != "Combinator" &&
		next != nil && next.Type() != "Combinator" {
		children.Push(&Combinator{ // FIXME: this.Combinator() should be used instead
			Loc:  nil,
			Name: " ",
		})
	}
}

func selectorGetNode(p *Parser, _ *SequenceContext) Node {
	switch p.TokenType {
	case tokenizer.LeftSquareBracket:
		return p.AttributeSelector()

	case tokenizer.Hash:
		return p.IDSelector()

	case tokenizer.Colon:
		if p.LookupType(1) == tokenizer.Colon {
			return p.PseudoElementSelector()
		} else {
			return p.PseudoClassSelector()
		}

	case tokenizer.Ident:
		return p.TypeSelector()

	case tokenizer.Number, tokenizer.Percentage:
		return p.Percentage()

	case tokenizer.Dimension:
		// throws when .123ident
		if p.CharCodeAt(p.TokenStart) == fullStop {
			p.Error("Identifier is expected", p.TokenStart+1)
		}

	case tokenizer.Delim:
		code := p.CharCodeAt(p.TokenStart)

		switch code {
		case plusSign, greaterThanSign, tilde, solidus: // /deep/
			return p.Combinator()

		case fullStop:
			return p.ClassSelector()

		case verticalLine:
			// || is the column combinator, |name a type selector
			if p.IsDelim(verticalLine, 1) {
				return p.Combinator()
			}
			return p.TypeSelector()

		case asterisk:
			return p.TypeSelector()

		case numberSign:
			return p.IDSelector()

		case ampersand:
			return p.NestingSelector()
		}
	}

	return nil
}

func selectorScope() *Recognizer {
	return &Recognizer{
		OnWhiteSpace: selectorOnWhiteSpace,
		GetNode:      selectorGetNode,
	}
}
