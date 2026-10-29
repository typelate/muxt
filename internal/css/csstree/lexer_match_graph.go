// Ported from lib/lexer/match-graph.js.

package csstree

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
)

// matchGraphNode is a node of a match graph (upstream a plain object with a
// type field). Only the fields of its type are set:
//
//   - Match, Mismatch, DisallowEmpty: the sentinels below
//   - If: match, then, els
//   - Enum: enumMap
//   - MatchOnce: terms, all
//   - MatchOnceBuffer, AddMatchOnce (matcher states): once (the MatchOnce
//     node, upstream syntax), index, mask
//   - Generic: fn, name (the generic type's name)
//   - Type, Property, Keyword, AtKeyword, Function: name, syntax
//   - Token, String: value, syntax
//   - Comma: syntax (a Comma or a Multiplier node)
//   - MatchGraph: match, ref (upstream syntax), source
type matchGraphNode struct {
	typ string

	match *matchGraphNode
	then  *matchGraphNode
	els   *matchGraphNode

	enumMap map[string]*matchGraphNode

	terms []*matchGraphNode
	all   bool

	once  *matchGraphNode
	index int
	mask  int

	fn genericFunc

	name   string
	value  string
	syntax definitionsyntax.Node

	// percentSibling is set on a numeric Type term of a "|" group that
	// has a <percentage> term (see markPercentSiblings).
	percentSibling bool

	ref    *SyntaxRef
	source definitionsyntax.Node
}

// The sentinel states (upstream MATCH, MISMATCH, DISALLOW_EMPTY). They are
// compared by identity and never changed.
var (
	matchState         = &matchGraphNode{typ: "Match"}
	mismatchState      = &matchGraphNode{typ: "Mismatch"}
	disallowEmptyState = &matchGraphNode{typ: "DisallowEmpty"}
)

// leftParenthesis (40) and rightParenthesis (41) are declared in url.go.

func createCondition(match, thenBranch, elseBranch *matchGraphNode) *matchGraphNode {
	// reduce node count
	if thenBranch == matchState && elseBranch == mismatchState {
		return match
	}

	if match == matchState && thenBranch == matchState && elseBranch == matchState {
		return match
	}

	if match.typ == "If" && match.els == mismatchState && thenBranch == matchState {
		thenBranch = match.then
		match = match.match
	}

	return &matchGraphNode{
		typ:   "If",
		match: match,
		then:  thenBranch,
		els:   elseBranch,
	}
}

func isFunctionType(name string) bool {
	return len(name) > 2 &&
		name[len(name)-2] == leftParenthesis &&
		name[len(name)-1] == rightParenthesis
}

func isEnumCapatible(term *matchGraphNode) bool {
	return term.typ == "Keyword" ||
		term.typ == "AtKeyword" ||
		term.typ == "Function" ||
		term.typ == "Type" && isFunctionType(term.name)
}

func groupNode(terms []definitionsyntax.Node, combinator string, explicit bool) *definitionsyntax.Group {
	return &definitionsyntax.Group{
		Terms:         terms,
		Combinator:    combinator,
		DisallowEmpty: false,
		Explicit:      explicit,
	}
}

func replaceTypeInGraph(node *matchGraphNode, replacements map[string]*matchGraphNode, visited map[*matchGraphNode]bool) *matchGraphNode {
	if !visited[node] {
		visited[node] = true

		switch node.typ {
		case "If":
			node.match = replaceTypeInGraph(node.match, replacements, visited)
			node.then = replaceTypeInGraph(node.then, replacements, visited)
			node.els = replaceTypeInGraph(node.els, replacements, visited)

		case "Type":
			if replacement, ok := replacements[node.name]; ok && replacement != nil {
				return replacement
			}
			return node
		}
	}

	return node
}

// withoutTerm returns terms without term (upstream terms.filter(t => t !== term)).
func withoutTerm(terms []*matchGraphNode, term *matchGraphNode) []*matchGraphNode {
	result := make([]*matchGraphNode, 0, len(terms))
	for _, newGroupTerm := range terms {
		if newGroupTerm != term {
			result = append(result, newGroupTerm)
		}
	}
	return result
}

func buildGroupMatchGraph(combinator string, terms []*matchGraphNode, atLeastOneTermMatched bool) *matchGraphNode {
	switch combinator {
	case " ":
		// Juxtaposing components means that all of them must occur, in the given order.
		//
		// a b c
		// =
		// match a
		//   then match b
		//     then match c
		//       then MATCH
		//       else MISMATCH
		//     else MISMATCH
		//   else MISMATCH
		result := matchState

		for i := len(terms) - 1; i >= 0; i-- {
			term := terms[i]

			result = createCondition(
				term,
				result,
				mismatchState,
			)
		}

		return result

	case "|":
		// A bar (|) separates two or more alternatives: exactly one of them must occur.
		//
		// a | b | c
		// =
		// match a
		//   then MATCH
		//   else match b
		//     then MATCH
		//     else match c
		//       then MATCH
		//       else MISMATCH

		result := mismatchState
		var enumMap map[string]*matchGraphNode

		for i := len(terms) - 1; i >= 0; i-- {
			term := terms[i]

			// reduce sequence of keywords into a Enum
			if isEnumCapatible(term) {
				if enumMap == nil && i > 0 && isEnumCapatible(terms[i-1]) {
					enumMap = make(map[string]*matchGraphNode)
					result = createCondition(
						&matchGraphNode{
							typ:     "Enum",
							enumMap: enumMap,
						},
						matchState,
						result,
					)
				}

				if enumMap != nil {
					key := term.name
					if isFunctionType(term.name) {
						key = term.name[:len(term.name)-1]
					}
					key = toLowerCase(key)
					if _, ok := enumMap[key]; !ok {
						enumMap[key] = term
						continue
					}
				}
			}

			enumMap = nil

			// create a new conditonal node
			result = createCondition(
				term,
				matchState,
				result,
			)
		}

		return result

	case "&&":
		// A double ampersand (&&) separates two or more components,
		// all of which must occur, in any order.

		// Use MatchOnce for groups with a large number of terms,
		// since &&-groups produces at least N!-node trees
		if len(terms) > 5 {
			return &matchGraphNode{
				typ:   "MatchOnce",
				terms: terms,
				all:   true,
			}
		}

		// Use a combination tree for groups with small number of terms
		//
		// a && b && c
		// =
		// match a
		//   then [b && c]
		//   else match b
		//     then [a && c]
		//     else match c
		//       then [a && b]
		//       else MISMATCH
		//
		// a && b
		// =
		// match a
		//   then match b
		//     then MATCH
		//     else MISMATCH
		//   else match b
		//     then match a
		//       then MATCH
		//       else MISMATCH
		//     else MISMATCH
		result := mismatchState

		for i := len(terms) - 1; i >= 0; i-- {
			term := terms[i]
			var thenClause *matchGraphNode

			if len(terms) > 1 {
				thenClause = buildGroupMatchGraph(
					combinator,
					withoutTerm(terms, term),
					false,
				)
			} else {
				thenClause = matchState
			}

			result = createCondition(
				term,
				thenClause,
				result,
			)
		}

		return result

	case "||":
		// A double bar (||) separates two or more options:
		// one or more of them must occur, in any order.

		// Use MatchOnce for groups with a large number of terms,
		// since ||-groups produces at least N!-node trees
		if len(terms) > 5 {
			return &matchGraphNode{
				typ:   "MatchOnce",
				terms: terms,
				all:   false,
			}
		}

		// Use a combination tree for groups with small number of terms
		//
		// a || b || c
		// =
		// match a
		//   then [b || c]
		//   else match b
		//     then [a || c]
		//     else match c
		//       then [a || b]
		//       else MISMATCH
		//
		// a || b
		// =
		// match a
		//   then match b
		//     then MATCH
		//     else MATCH
		//   else match b
		//     then match a
		//       then MATCH
		//       else MATCH
		//     else MISMATCH
		result := mismatchState
		if atLeastOneTermMatched {
			result = matchState
		}

		for i := len(terms) - 1; i >= 0; i-- {
			term := terms[i]
			var thenClause *matchGraphNode

			if len(terms) > 1 {
				thenClause = buildGroupMatchGraph(
					combinator,
					withoutTerm(terms, term),
					true,
				)
			} else {
				thenClause = matchState
			}

			result = createCondition(
				term,
				thenClause,
				result,
			)
		}

		return result
	}

	// upstream returns undefined for an unknown combinator, which the
	// definition syntax parser never produces
	return nil
}

func buildMultiplierMatchGraph(node *definitionsyntax.Multiplier) (*matchGraphNode, error) {
	result := matchState
	matchTerm, err := buildMatchGraphInternal(node.Term)
	if err != nil {
		return nil, err
	}

	if node.Max == definitionsyntax.Unbounded {
		// disable repeating of empty match to prevent infinite loop
		matchTerm = createCondition(
			matchTerm,
			disallowEmptyState,
			mismatchState,
		)

		// an occurrence count is not limited, make a cycle;
		// to collect more terms on each following matching mismatch
		result = createCondition(
			matchTerm,
			nil, // will be a loop
			mismatchState,
		)

		result.then = createCondition(
			matchState,
			matchState,
			result, // make a loop
		)

		if node.Comma {
			result.then.els = createCondition(
				&matchGraphNode{typ: "Comma", syntax: node},
				result,
				mismatchState,
			)
		}
	} else {
		// create a match node chain for [min .. max] interval with optional matches
		for i := max(node.Min, 1); i <= node.Max; i++ {
			if node.Comma && result != matchState {
				result = createCondition(
					&matchGraphNode{typ: "Comma", syntax: node},
					result,
					mismatchState,
				)
			}

			result = createCondition(
				matchTerm,
				createCondition(
					matchState,
					matchState,
					result,
				),
				mismatchState,
			)
		}
	}

	if node.Min == 0 {
		// allow zero match
		result = createCondition(
			matchState,
			matchState,
			result,
		)
	} else {
		// create a match node chain to collect [0 ... min - 1] required matches
		for i := 0; i < node.Min-1; i++ {
			if node.Comma && result != matchState {
				result = createCondition(
					&matchGraphNode{typ: "Comma", syntax: node},
					result,
					mismatchState,
				)
			}

			result = createCondition(
				matchTerm,
				result,
				mismatchState,
			)
		}
	}

	return result, nil
}

// buildGenericMatchGraph is the function case of upstream
// buildMatchGraphInternal: a generic type.
func buildGenericMatchGraph(fn genericFunc) *matchGraphNode {
	return &matchGraphNode{
		typ: "Generic",
		fn:  fn,
	}
}

func buildMatchGraphInternal(node definitionsyntax.Node) (*matchGraphNode, error) {
	switch node := node.(type) {
	case *definitionsyntax.Group:
		terms := make([]*matchGraphNode, len(node.Terms))
		for i, term := range node.Terms {
			graph, err := buildMatchGraphInternal(term)
			if err != nil {
				return nil, err
			}
			terms[i] = graph
		}
		markPercentSiblings(node, terms)

		result := buildGroupMatchGraph(
			node.Combinator,
			terms,
			false,
		)

		if node.DisallowEmpty {
			result = createCondition(
				result,
				disallowEmptyState,
				mismatchState,
			)
		}

		return result, nil

	case *definitionsyntax.Multiplier:
		return buildMultiplierMatchGraph(node)

	// https://drafts.csswg.org/css-values-5/#boolean
	case *definitionsyntax.Boolean:
		term, err := buildMatchGraphInternal(node.Term)
		if err != nil {
			return nil, err
		}
		// <boolean-expr[ <test> ]> = not <boolean-expr-group> | <boolean-expr-group> [ [ and <boolean-expr-group> ]* | [ or <boolean-expr-group> ]* ]
		matchNode, err := buildMatchGraphInternal(groupNode([]definitionsyntax.Node{
			groupNode([]definitionsyntax.Node{
				&definitionsyntax.Keyword{Name: "not"},
				&definitionsyntax.Type{Name: "!boolean-group"},
			}, " ", false),
			groupNode([]definitionsyntax.Node{
				&definitionsyntax.Type{Name: "!boolean-group"},
				groupNode([]definitionsyntax.Node{
					&definitionsyntax.Multiplier{Comma: false, Min: 0, Max: definitionsyntax.Unbounded, Term: groupNode([]definitionsyntax.Node{
						&definitionsyntax.Keyword{Name: "and"},
						&definitionsyntax.Type{Name: "!boolean-group"},
					}, " ", false)},
					&definitionsyntax.Multiplier{Comma: false, Min: 0, Max: definitionsyntax.Unbounded, Term: groupNode([]definitionsyntax.Node{
						&definitionsyntax.Keyword{Name: "or"},
						&definitionsyntax.Type{Name: "!boolean-group"},
					}, " ", false)},
				}, "|", false),
			}, " ", false),
		}, "|", false))
		if err != nil {
			return nil, err
		}
		// <boolean-expr-group> = <test> | ( <boolean-expr[ <test> ]> ) | <general-enclosed>
		booleanGroup, err := buildMatchGraphInternal(
			groupNode([]definitionsyntax.Node{
				&definitionsyntax.Type{Name: "!term"},
				groupNode([]definitionsyntax.Node{
					&definitionsyntax.Token{Value: "("},
					&definitionsyntax.Type{Name: "!self"},
					&definitionsyntax.Token{Value: ")"},
				}, " ", false),
				&definitionsyntax.Type{Name: "general-enclosed"},
			}, "|", false),
		)
		if err != nil {
			return nil, err
		}

		replaceTypeInGraph(booleanGroup, map[string]*matchGraphNode{"!term": term, "!self": matchNode}, make(map[*matchGraphNode]bool))
		replaceTypeInGraph(matchNode, map[string]*matchGraphNode{"!boolean-group": booleanGroup}, make(map[*matchGraphNode]bool))

		return matchNode, nil

	case *definitionsyntax.Type:
		return &matchGraphNode{
			typ:    node.Type(),
			name:   node.Name,
			syntax: node,
		}, nil

	case *definitionsyntax.Property:
		return &matchGraphNode{
			typ:    node.Type(),
			name:   node.Name,
			syntax: node,
		}, nil

	case *definitionsyntax.Keyword:
		return &matchGraphNode{
			typ:    node.Type(),
			name:   toLowerCase(node.Name),
			syntax: node,
		}, nil

	case *definitionsyntax.AtKeyword:
		return &matchGraphNode{
			typ:    node.Type(),
			name:   "@" + toLowerCase(node.Name),
			syntax: node,
		}, nil

	case *definitionsyntax.Function:
		return &matchGraphNode{
			typ:    node.Type(),
			name:   toLowerCase(node.Name) + "(",
			syntax: node,
		}, nil

	case *definitionsyntax.String:
		// convert a one char length String to a Token
		if utf8.RuneCountInString(node.Value) == 3 {
			_, size := utf8.DecodeRuneInString(node.Value[1:])
			return &matchGraphNode{
				typ:    "Token",
				value:  node.Value[1 : 1+size],
				syntax: node,
			}, nil
		}

		// otherwise use it as is
		return &matchGraphNode{
			typ:    node.Type(),
			value:  strings.ReplaceAll(node.Value[1:len(node.Value)-1], `\'`, `'`),
			syntax: node,
		}, nil

	case *definitionsyntax.Token:
		return &matchGraphNode{
			typ:    node.Type(),
			value:  node.Value,
			syntax: node,
		}, nil

	case *definitionsyntax.Comma:
		return &matchGraphNode{
			typ:    node.Type(),
			syntax: node,
		}, nil

	default:
		// upstream: new Error('Unknown node type:', node.type), whose
		// message is the first argument only
		//lint:ignore ST1005 upstream message
		return nil, errors.New("Unknown node type:")
	}
}

// buildMatchGraph returns the match graph of a definition syntax tree; ref
// names the syntax it belongs to (nil is upstream null).
func buildMatchGraph(syntaxTree definitionsyntax.Node, ref *SyntaxRef) (*matchGraphNode, error) {
	match, err := buildMatchGraphInternal(syntaxTree)
	if err != nil {
		return nil, err
	}

	return &matchGraphNode{
		typ:    "MatchGraph",
		match:  match,
		ref:    ref,
		source: syntaxTree,
	}, nil
}

// buildMatchGraphFromString is buildMatchGraph for a definition syntax
// string, which it parses first.
func buildMatchGraphFromString(syntaxTree string, ref *SyntaxRef) (*matchGraphNode, error) {
	tree, err := definitionsyntax.Parse(syntaxTree)
	if err != nil {
		return nil, err
	}

	return buildMatchGraph(tree, ref)
}

// buildGenericMatchGraphRoot is buildMatchGraph for a generic type
// function (upstream passes the function as syntaxTree).
func buildGenericMatchGraphRoot(fn genericFunc, ref *SyntaxRef) *matchGraphNode {
	match := buildGenericMatchGraph(fn)
	if ref != nil {
		match.name = ref.Name
	}
	return &matchGraphNode{
		typ:   "MatchGraph",
		match: match,
		ref:   ref,
	}
}
