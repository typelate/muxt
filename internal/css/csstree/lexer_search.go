// Ported from lib/lexer/search.js.

package csstree

import "errors"

// Fragment is a run of sibling nodes that matched a syntax: Nodes are
// copied from Parent, the list holding them.
type Fragment struct {
	Parent *List[Node]
	Nodes  *List[Node]
}

// errEmptyMatchFragment is returned for a fragment of a syntax that
// matched no tokens, such as the empty value of a property that allows
// one; upstream throws a TypeError reading its first token.
var errEmptyMatchFragment = errors.New("cannot find the nodes of a syntax that matched no tokens")

func getFirstMatchNode(matchNode *MatchNode) (Node, error) {
	if matchNode.Match == nil {
		return matchNode.Node, nil
	}

	if len(matchNode.Match) == 0 {
		return nil, errEmptyMatchFragment
	}

	return getFirstMatchNode(matchNode.Match[0])
}

func getLastMatchNode(matchNode *MatchNode) (Node, error) {
	if matchNode.Match == nil {
		return matchNode.Node, nil
	}

	if len(matchNode.Match) == 0 {
		return nil, errEmptyMatchFragment
	}

	return getLastMatchNode(matchNode.Match[len(matchNode.Match)-1])
}

func matchFragments(lexer *Lexer, ast Node, match *MatchResult, typ, name string) ([]Fragment, error) {
	var fragments []Fragment

	var findFragments func(matchNode *MatchNode) error
	findFragments = func(matchNode *MatchNode) error {
		if matchNode.Syntax != nil &&
			matchNode.Syntax.Type() == typ &&
			syntaxName(matchNode.Syntax) == name {
			start, err := getFirstMatchNode(matchNode)
			if err != nil {
				return err
			}
			end, err := getLastMatchNode(matchNode)
			if err != nil {
				return err
			}

			err = lexer.syntax.Walk(ast, WalkOptions{
				Enter: func(_ *WalkContext, node Node, item *ListItem[Node], list *List[Node]) WalkAction {
					// upstream reads item.data and throws when start is the
					// root, which has no item
					if node == start && item != nil {
						nodes := new(List[Node])

						for {
							nodes.AppendData(item.Data)

							if item.Data == end {
								break
							}

							item = item.Next
							if item == nil {
								break
							}
						}

						fragments = append(fragments, Fragment{
							Parent: list,
							Nodes:  nodes,
						})
					}
					return WalkContinue
				},
			})
			if err != nil {
				return err
			}
		}

		for _, child := range matchNode.Match {
			if err := findFragments(child); err != nil {
				return err
			}
		}

		return nil
	}

	if match.Matched != nil {
		if err := findFragments(match.Matched); err != nil {
			return nil, err
		}
	}

	return fragments, nil
}
