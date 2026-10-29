// Ported from lib/syntax/config/parser.js.

package csstree

// parserConfig returns the parser configuration of the default syntax.
func parserConfig() *Config {
	return &Config{
		ParseContext: map[string]func(p *Parser, options ParseOptions) Node{
			"default":    func(p *Parser, _ ParseOptions) Node { return p.StyleSheet() },
			"stylesheet": func(p *Parser, _ ParseOptions) Node { return p.StyleSheet() },
			"atrule":     func(p *Parser, _ ParseOptions) Node { return p.Atrule(false) },
			"atrulePrelude": func(p *Parser, options ParseOptions) Node {
				return p.AtrulePrelude(options.Atrule)
			},
			"mediaQueryList": func(p *Parser, _ ParseOptions) Node { return p.MediaQueryList() },
			"mediaQuery":     func(p *Parser, _ ParseOptions) Node { return p.MediaQuery() },
			"condition": func(p *Parser, options ParseOptions) Node {
				return p.Condition(options.Kind)
			},
			"rule":         func(p *Parser, _ ParseOptions) Node { return p.Rule() },
			"selectorList": func(p *Parser, _ ParseOptions) Node { return p.SelectorList() },
			"selector":     func(p *Parser, _ ParseOptions) Node { return p.Selector() },
			"block": func(p *Parser, _ ParseOptions) Node {
				return p.Block(true)
			},
			"declarationList": func(p *Parser, _ ParseOptions) Node { return p.DeclarationList() },
			"declaration":     func(p *Parser, _ ParseOptions) Node { return p.Declaration() },
			"value":           func(p *Parser, _ ParseOptions) Node { return p.Value() },
		},
		Features: map[string]map[string]func(p *Parser) Node{
			"supports": {
				"selector": func(p *Parser) Node {
					return p.Selector()
				},
			},
			"container": {
				"style": func(p *Parser) Node {
					return p.Declaration()
				},
			},
		},
		Scope:  scopes(),
		Atrule: atrules(),
		Pseudo: pseudos(),
		Node:   nodeConfigs(),
	}
}
