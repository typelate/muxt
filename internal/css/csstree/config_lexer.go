// Ported from lib/syntax/config/lexer.js.

package csstree

import "maps"

// lexerConfig returns the lexer configuration of the default syntax: the
// generic types, the CSS-wide keywords, the definitions of data.go and the
// node types. It fails only if the embedded data is malformed.
func lexerConfig() (*Config, error) {
	definitions, err := defaultDefinitions()
	if err != nil {
		return nil, err
	}

	return &Config{
		Generic:         new(true),
		CSSWideKeywords: cssWideKeywords(),
		Types:           maps.Clone(definitions.Types),
		Atrules:         maps.Clone(definitions.Atrules),
		Properties:      maps.Clone(definitions.Properties),
		Node:            nodeConfigs(),
	}, nil
}

// lexerConfigWithoutDefinitions is lexerConfig without the definitions of
// data.go, for the syntax behind the package-level parse, generate and
// walk functions, which never use them.
func lexerConfigWithoutDefinitions() *Config {
	return &Config{
		Generic:         new(true),
		CSSWideKeywords: cssWideKeywords(),
		Node:            nodeConfigs(),
	}
}
