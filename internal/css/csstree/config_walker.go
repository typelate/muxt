// Ported from lib/syntax/config/walker.js.

package csstree

// walkerConfig returns the walker configuration of the default syntax:
// every node type, of which the walker uses Structure, WalkContext and Walk.
func walkerConfig() *Config {
	return &Config{
		Node: nodeConfigs(),
	}
}
