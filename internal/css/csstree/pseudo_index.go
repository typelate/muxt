// Ported from lib/syntax/pseudo/index.js.

package csstree

func selectorListPseudo(p *Parser) *List[Node] {
	return p.CreateSingleNodeList(
		p.SelectorList(),
	)
}

func selectorPseudo(p *Parser) *List[Node] {
	return p.CreateSingleNodeList(
		p.Selector(),
	)
}

func identListPseudo(p *Parser) *List[Node] {
	return p.CreateSingleNodeList(
		p.Identifier(),
	)
}

func nthPseudo(p *Parser) *List[Node] {
	return p.CreateSingleNodeList(
		p.Nth(),
	)
}

// pseudos returns the pseudo-class and pseudo-element argument parsers of
// the default syntax.
func pseudos() map[string]func(p *Parser) *List[Node] {
	return map[string]func(p *Parser) *List[Node]{
		"dir":              identListPseudo,
		"has":              selectorListPseudo,
		"lang":             parseLanguageRangeList,
		"matches":          selectorListPseudo,
		"is":               selectorListPseudo,
		"-moz-any":         selectorListPseudo,
		"-webkit-any":      selectorListPseudo,
		"where":            selectorListPseudo,
		"not":              selectorListPseudo,
		"nth-child":        nthPseudo,
		"nth-last-child":   nthPseudo,
		"nth-last-of-type": nthPseudo,
		"nth-of-type":      nthPseudo,
		"slotted":          selectorPseudo,
		"host":             selectorPseudo,
		"host-context":     selectorPseudo,
	}
}
