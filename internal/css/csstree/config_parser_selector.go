// Ported from lib/syntax/config/parser-selector.js.

package csstree

// parserSelectorConfig returns the configuration of the selector-only
// parser (ParseSelector).
func parserSelectorConfig() *Config {
	return &Config{
		ParseContext: map[string]func(p *Parser, options ParseOptions) Node{
			"default":      func(p *Parser, _ ParseOptions) Node { return p.SelectorList() },
			"selectorList": func(p *Parser, _ ParseOptions) Node { return p.SelectorList() },
			"selector":     func(p *Parser, _ ParseOptions) Node { return p.Selector() },
		},
		Scope:  map[string]*Recognizer{"Selector": selectorScope()},
		Atrule: map[string]AtruleParser{},
		Pseudo: pseudos(),
		Node:   nodeConfigs(),
	}
}
