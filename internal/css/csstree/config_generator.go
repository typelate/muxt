// Ported from lib/syntax/config/generator.js.

package csstree

// generatorConfig returns the generator configuration of the default
// syntax: every node type, of which the generator uses Generate. It is
// upstream's css-tree/generator entry point; Generate uses the default
// syntax, so only the exports test uses it.
func generatorConfig() *Config {
	return &Config{
		Node: nodeConfigs(),
	}
}
