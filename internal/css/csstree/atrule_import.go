// Ported from lib/syntax/atrule/import.js.

package csstree

import "github.com/typelate/muxt/internal/css/csstree/tokenizer"

func importParseWithFallback(p *Parser, parse, fallback func() Node) Node {
	if fallback == nil {
		fallback = func() Node { return p.Raw(nil, true) }
	}

	return p.ParseWithFallback(
		func() Node {
			// upstream: try { return parse.call(this); } finally { ... }
			defer func() {
				p.SkipSC()
				if p.LookupNonWSType(0) != tokenizer.RightParenthesis {
					p.Error("", -1)
				}
			}()

			return parse()
		},
		fallback,
	)
}

func isEmptyRaw(node Node) bool {
	raw, ok := node.(*Raw)
	return ok && raw.Value == ""
}

func importParseFunctions() *Recognizer {
	return &Recognizer{
		Functions: map[string]func(p *Parser, recognizer *Recognizer) *List[Node]{
			"layer": func(p *Parser, _ *Recognizer) *List[Node] {
				p.SkipSC()

				children := p.CreateList()
				node := importParseWithFallback(p, func() Node { return p.Layer() }, nil)

				if !isEmptyRaw(node) {
					children.Push(node)
				}

				return children
			},
			"supports": func(p *Parser, _ *Recognizer) *List[Node] {
				p.SkipSC()

				children := p.CreateList()
				node := importParseWithFallback(
					p,
					func() Node { return p.Declaration() },
					func() Node {
						return importParseWithFallback(p, func() Node { return p.Condition("supports") }, nil)
					},
				)

				if !isEmptyRaw(node) {
					children.Push(node)
				}

				return children
			},
		},
	}
}

func importAtrule() AtruleParser {
	return AtruleParser{
		Prelude: func(p *Parser) *List[Node] {
			children := p.CreateList()

			switch p.TokenType {
			case tokenizer.String:
				children.Push(p.String())

			case tokenizer.URL, tokenizer.Function:
				children.Push(p.URL())

			default:
				p.Error("String or url() is expected", -1)
			}

			p.SkipSC()

			if p.TokenType == tokenizer.Ident &&
				p.CmpStr(p.TokenStart, p.TokenEnd, "layer") {
				children.Push(p.Identifier())
			} else if p.TokenType == tokenizer.Function &&
				p.CmpStr(p.TokenStart, p.TokenEnd, "layer(") {
				children.Push(p.Function(nil, importParseFunctions()))
			}

			p.SkipSC()

			if p.TokenType == tokenizer.Function &&
				p.CmpStr(p.TokenStart, p.TokenEnd, "supports(") {
				children.Push(p.Function(nil, importParseFunctions()))
			}

			if p.LookupNonWSType(0) == tokenizer.Ident ||
				p.LookupNonWSType(0) == tokenizer.LeftParenthesis {
				children.Push(p.MediaQueryList())
			}

			return children
		},
		Block: nil,
	}
}
