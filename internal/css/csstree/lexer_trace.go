// Ported from lib/lexer/trace.js.

package csstree

// GetTrace returns the syntaxes that matched node, outermost first,
// keeping the Type, Property and Keyword ones. It returns nil when node is
// not in the match (upstream null), and an empty slice when it is but no
// such syntax led to it.
func (r *MatchResult) GetTrace(node Node) []MatchSyntax {
	shouldPutToTrace := func(syntax MatchSyntax) bool {
		if syntax == nil {
			return false
		}

		return syntax.Type() == "Type" ||
			syntax.Type() == "Property" ||
			syntax.Type() == "Keyword"
	}

	var result []MatchSyntax

	var hasMatch func(matchNode *MatchNode) bool
	hasMatch = func(matchNode *MatchNode) bool {
		if matchNode.Match != nil {
			// use for-loop for better perfomance
			for i := 0; i < len(matchNode.Match); i++ {
				if hasMatch(matchNode.Match[i]) {
					if shouldPutToTrace(matchNode.Syntax) {
						result = append([]MatchSyntax{matchNode.Syntax}, result...)
					}

					return true
				}
			}
		} else if matchNode.Node == node {
			if shouldPutToTrace(matchNode.Syntax) {
				result = []MatchSyntax{matchNode.Syntax}
			} else {
				result = []MatchSyntax{}
			}

			return true
		}

		return false
	}

	if r.Matched != nil {
		hasMatch(r.Matched)
	}

	return result
}

// IsType reports whether a syntax of type typ (a Type reference named
// typ) led to node in the match.
func (r *MatchResult) IsType(node Node, typ string) bool {
	return testNode(r, node, func(match MatchSyntax) bool {
		return match.Type() == "Type" && syntaxName(match) == typ
	})
}

// IsProperty reports whether the syntax of property led to node in the
// match.
func (r *MatchResult) IsProperty(node Node, property string) bool {
	return testNode(r, node, func(match MatchSyntax) bool {
		return match.Type() == "Property" && syntaxName(match) == property
	})
}

// IsKeyword reports whether node matched a keyword.
func (r *MatchResult) IsKeyword(node Node) bool {
	return testNode(r, node, func(match MatchSyntax) bool {
		return match.Type() == "Keyword"
	})
}

func testNode(match *MatchResult, node Node, fn func(MatchSyntax) bool) bool {
	trace := match.GetTrace(node)

	if trace == nil {
		return false
	}

	for _, syntax := range trace {
		if fn(syntax) {
			return true
		}
	}

	return false
}
