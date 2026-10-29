// Ported from lib/syntax/scope/index.js.

package csstree

// scopes returns the parse scopes of the default syntax.
func scopes() map[string]*Recognizer {
	return map[string]*Recognizer{
		"AtrulePrelude": atrulePreludeScope(),
		"Selector":      selectorScope(),
		"Value":         valueScope(),
	}
}
