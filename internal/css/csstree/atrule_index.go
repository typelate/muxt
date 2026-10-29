// Ported from lib/syntax/atrule/index.js.

package csstree

// atrules returns the at-rule parsers of the default syntax.
func atrules() map[string]AtruleParser {
	return map[string]AtruleParser{
		"container":      containerAtrule(),
		"font-face":      fontFaceAtrule(),
		"import":         importAtrule(),
		"layer":          layerAtrule(),
		"media":          mediaAtrule(),
		"nest":           nestAtrule(),
		"page":           pageAtrule(),
		"scope":          scopeAtrule(),
		"starting-style": startingStyleAtrule(),
		"supports":       supportsAtrule(),
	}
}
