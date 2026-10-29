// Ported from lib/lexer/match.js.

package csstree

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// match stack item types
const (
	stubItem        = 0
	tokenItem       = 1
	openSyntaxItem  = 2
	closeSyntaxItem = 3
)

const (
	exitReasonMatch          = "Match"
	exitReasonMismatch       = "Mismatch"
	exitReasonIterationLimit = "Maximum iteration number exceeded (please fill an issue on https://github.com/csstree/csstree/issues)"
)

const iterationLimit = 15000

// keywordHack matches the \0 and \9 hacks at the end of a keyword: upstream
// /\\[09].*$/, where "." matches no line terminator and "$" only matches at
// the end of the value.
var keywordHack = regexp.MustCompile(`\\[09][^\n\r\x{2028}\x{2029}]*$`)

// matchSyntaxes resolves the types and properties a match graph refers to
// (upstream the syntaxes argument: an object with types and properties
// dictionaries, usually the lexer).
type matchSyntaxes interface {
	// lookupMatch returns the match graph of the type (isType) or property
	// named name (upstream dictSyntax.matchRef || dictSyntax.match), nil
	// when there is none.
	lookupMatch(isType bool, name string) (*matchGraphNode, error)
}

// matchStackItem is an entry of the match stack: a matched token or the
// opening or closing of a syntax.
type matchStackItem struct {
	typ    int
	syntax definitionsyntax.Node
	token  *matchToken
	prev   *matchStackItem
}

type syntaxStackItem struct {
	syntax         definitionsyntax.Node
	opts           *definitionsyntax.Range
	percentSibling bool
	prev           *syntaxStackItem
}

type thenStackItem struct {
	nextState   *matchGraphNode
	matchStack  *matchStackItem
	syntaxStack *syntaxStackItem
	prev        *thenStackItem
}

type elseStackItem struct {
	nextState   *matchGraphNode
	matchStack  *matchStackItem
	syntaxStack *syntaxStackItem
	thenStack   *thenStackItem
	tokenIndex  int
	prev        *elseStackItem
}

// matchResult is the result of internalMatch, matchAsList and matchAsTree.
// match is set by internalMatch (nil unless the value matched), list by
// matchAsList and tree by matchAsTree.
type matchResult struct {
	tokens       []matchToken
	reason       string
	iterations   int
	match        *matchStackItem
	list         []matchListItem
	tree         *MatchNode
	longestMatch int
}

// matchListItem is an entry of matchAsList's result: the opening or
// closing of a syntax, or a token.
type matchListItem struct {
	typ     int
	syntax  definitionsyntax.Node
	isToken bool
	token   string
	node    Node
}

func reverseList(list *matchStackItem) *matchStackItem {
	var prev *matchStackItem
	var next *matchStackItem
	item := list

	for item != nil {
		next = item.prev
		item.prev = prev
		prev = item
		item = next
	}

	return prev
}

func areStringsEqualCaseInsensitive(testStr, referenceStr string) bool {
	if len(testStr) != len(referenceStr) {
		return false
	}

	for i := 0; i < len(testStr); i++ {
		referenceCode := referenceStr[i]
		testCode := testStr[i]

		// testCode.toLowerCase() for U+0041 LATIN CAPITAL LETTER A (A) .. U+005A LATIN CAPITAL LETTER Z (Z).
		if testCode >= 0x0041 && testCode <= 0x005A {
			testCode = testCode | 32
		}

		if testCode != referenceCode {
			return false
		}
	}

	return true
}

func isContextEdgeDelim(token *matchToken) bool {
	if token.typ != tokenizer.Delim {
		return false
	}

	// Fix matching for unicode-range: U+30??, U+FF00-FF9F
	// Probably we need to check out previous match instead
	return token.value != "?"
}

func isCommaContextStart(token *matchToken) bool {
	if token == nil {
		return true
	}

	return token.typ == tokenizer.Comma ||
		token.typ == tokenizer.Function ||
		token.typ == tokenizer.LeftParenthesis ||
		token.typ == tokenizer.LeftSquareBracket ||
		token.typ == tokenizer.LeftCurlyBracket ||
		isContextEdgeDelim(token)
}

func isCommaContextEnd(token *matchToken) bool {
	if token == nil {
		return true
	}

	return token.typ == tokenizer.RightParenthesis ||
		token.typ == tokenizer.RightSquareBracket ||
		token.typ == tokenizer.RightCurlyBracket ||
		(token.typ == tokenizer.Delim && token.value == "/")
}

// typeOpts returns the opts of a Type syntax node (nil for other nodes,
// whose opts upstream are undefined).
func typeOpts(syntax definitionsyntax.Node) *definitionsyntax.Range {
	if node, ok := syntax.(*definitionsyntax.Type); ok {
		return node.Opts
	}

	return nil
}

// internalMatch runs the matcher. It fails where upstream throws: on a
// reference to an unknown type or property and when building a referenced
// match graph fails.
func internalMatch(tokens []matchToken, state *matchGraphNode, syntaxes matchSyntaxes) (*matchResult, error) {
	var syntaxStack *syntaxStackItem
	var thenStack *thenStackItem
	var elseStack *elseStackItem

	// nil, !syntaxStashDisabled – stashing allowed, nothing stashed
	// nil, syntaxStashDisabled – stashing disabled, nothing stashed
	// anithing else – fail stashable syntaxes, some syntax stashed
	// (upstream: null, false, a state snapshot)
	var syntaxStash *elseStackItem
	syntaxStashDisabled := false

	iterationCount := 0 // count iterations and prevent infinite loop
	exitReason := ""

	var token *matchToken
	tokenIndex := -1
	longestMatch := 0
	matchStack := &matchStackItem{
		typ:    stubItem,
		syntax: nil,
		token:  nil,
		prev:   nil,
	}

	moveToNextToken := func() {
		for {
			tokenIndex++
			token = nil
			if tokenIndex < len(tokens) {
				token = &tokens[tokenIndex]
			}
			if token == nil || (token.typ != tokenizer.WhiteSpace && token.typ != tokenizer.Comment) {
				break
			}
		}
	}

	getNextToken := func(offset int) *matchToken {
		nextIndex := tokenIndex + offset

		if nextIndex >= 0 && nextIndex < len(tokens) {
			return &tokens[nextIndex]
		}

		return nil
	}

	stateSnapshotFromSyntax := func(nextState *matchGraphNode, prev *elseStackItem) *elseStackItem {
		return &elseStackItem{
			nextState:   nextState,
			matchStack:  matchStack,
			syntaxStack: syntaxStack,
			thenStack:   thenStack,
			tokenIndex:  tokenIndex,
			prev:        prev,
		}
	}

	pushThenStack := func(nextState *matchGraphNode) {
		thenStack = &thenStackItem{
			nextState:   nextState,
			matchStack:  matchStack,
			syntaxStack: syntaxStack,
			prev:        thenStack,
		}
	}

	pushElseStack := func(nextState *matchGraphNode) {
		elseStack = stateSnapshotFromSyntax(nextState, elseStack)
	}

	addTokenToMatch := func() {
		matchStack = &matchStackItem{
			typ:    tokenItem,
			syntax: state.syntax,
			token:  token,
			prev:   matchStack,
		}

		moveToNextToken()
		syntaxStash = nil
		syntaxStashDisabled = false

		if tokenIndex > longestMatch {
			longestMatch = tokenIndex
		}
	}

	openSyntax := func() {
		opts := typeOpts(state.syntax)
		if opts == nil && syntaxStack != nil {
			opts = syntaxStack.opts
		}
		syntaxStack = &syntaxStackItem{
			syntax:         state.syntax,
			opts:           opts,
			percentSibling: state.percentSibling,
			prev:           syntaxStack,
		}

		matchStack = &matchStackItem{
			typ:    openSyntaxItem,
			syntax: state.syntax,
			token:  matchStack.token,
			prev:   matchStack,
		}
	}

	closeSyntax := func() {
		if matchStack.typ == openSyntaxItem {
			matchStack = matchStack.prev
		} else {
			matchStack = &matchStackItem{
				typ:    closeSyntaxItem,
				syntax: syntaxStack.syntax,
				token:  matchStack.token,
				prev:   matchStack,
			}
		}

		syntaxStack = syntaxStack.prev
	}

	moveToNextToken()

	for exitReason == "" {
		iterationCount++
		if iterationCount >= iterationLimit {
			break
		}

		switch state.typ {
		case "Match":
			if thenStack == nil {
				// turn to MISMATCH when some tokens left unmatched
				if token != nil {
					// doesn't mismatch if just one token left and it's an IE hack
					if tokenIndex != len(tokens)-1 || (token.value != `\0` && token.value != `\9`) {
						state = mismatchState
						break
					}
				}

				// break the main loop, return a result - MATCH
				exitReason = exitReasonMatch
				break
			}

			// go to next syntax (`then` branch)
			state = thenStack.nextState

			// check match is not empty
			if state == disallowEmptyState {
				if thenStack.matchStack == matchStack {
					state = mismatchState
					break
				} else {
					state = matchState
				}
			}

			// close syntax if needed
			for thenStack.syntaxStack != syntaxStack {
				closeSyntax()
			}

			// pop stack
			thenStack = thenStack.prev

		case "Mismatch":
			// when some syntax is stashed
			if syntaxStash != nil {
				// there is no else branches or a branch reduce match stack
				if elseStack == nil || tokenIndex > elseStack.tokenIndex {
					// restore state from the stash
					elseStack = syntaxStash
					syntaxStash = nil
					syntaxStashDisabled = true // disable stashing
				}
			} else if elseStack == nil {
				// no else branches -> break the main loop
				// return a result - MISMATCH
				exitReason = exitReasonMismatch
				break
			}

			// go to next syntax (`else` branch)
			state = elseStack.nextState

			// restore all the rest stack states
			thenStack = elseStack.thenStack
			syntaxStack = elseStack.syntaxStack
			matchStack = elseStack.matchStack
			tokenIndex = elseStack.tokenIndex
			token = nil
			if tokenIndex < len(tokens) {
				token = &tokens[tokenIndex]
			}

			// pop stack
			elseStack = elseStack.prev

		case "MatchGraph":
			state = state.match

		case "If":
			// IMPORTANT: else stack push must go first,
			// since it stores the state of thenStack before changes
			if state.els != mismatchState {
				pushElseStack(state.els)
			}

			if state.then != matchState {
				pushThenStack(state.then)
			}

			state = state.match

		case "MatchOnce":
			state = &matchGraphNode{
				typ:   "MatchOnceBuffer",
				once:  state,
				index: 0,
				mask:  0,
			}

		case "MatchOnceBuffer":
			terms := state.once.terms

			if state.index == len(terms) {
				// no matches at all or it's required all terms to be matched
				if state.mask == 0 || state.once.all {
					state = mismatchState
					break
				}

				// a partial match is ok
				state = matchState
				break
			}

			// all terms are matched
			if state.mask == (1<<len(terms))-1 {
				state = matchState
				break
			}

			for buffer := state; buffer.index < len(terms); buffer.index++ {
				matchFlag := 1 << buffer.index

				if buffer.mask&matchFlag == 0 {
					// IMPORTANT: else stack push must go first,
					// since it stores the state of thenStack before changes
					pushElseStack(buffer)
					pushThenStack(&matchGraphNode{
						typ:  "AddMatchOnce",
						once: buffer.once,
						mask: buffer.mask | matchFlag,
					})

					// match
					state = terms[buffer.index]
					buffer.index++
					break
				}
			}

		case "AddMatchOnce":
			state = &matchGraphNode{
				typ:   "MatchOnceBuffer",
				once:  state.once,
				index: 0,
				mask:  state.mask,
			}

		case "Enum":
			if token != nil {
				name := toLowerCase(token.value)

				// drop \0 and \9 hack from keyword name
				if strings.Contains(name, `\`) {
					name = keywordHack.ReplaceAllString(name, "")
				}

				if next, ok := state.enumMap[name]; ok {
					state = next
					break
				}

				// the value with escapes decoded, see tokenNameIs
				if next, ok := enumLookup(state.enumMap, decodedNames(token)); ok {
					state = next
					break
				}
			}

			state = mismatchState

		case "Generic":
			var opts *definitionsyntax.Range
			if syntaxStack != nil {
				opts = syntaxStack.opts
			}
			// math functions are type checked (Values 4 §10.9)
			percentSibling := syntaxStack != nil && syntaxStack.percentSibling
			consumed, handled := matchMathFunction(state.name, percentSibling, token, getNextToken, syntaxes)
			if !handled {
				consumed = state.fn(token, getNextToken, opts)
			}
			lastTokenIndex := tokenIndex + consumed

			if lastTokenIndex > tokenIndex {
				for tokenIndex < lastTokenIndex {
					addTokenToMatch()
				}

				state = matchState
			} else {
				state = mismatchState
			}

		case "Type", "Property":
			isType := state.typ == "Type"
			var dictSyntax *matchGraphNode
			if syntaxes != nil {
				var err error
				dictSyntax, err = syntaxes.lookupMatch(isType, state.name)
				if err != nil {
					return nil, err
				}
			}

			if dictSyntax == nil {
				reference := "<'" + state.name + "'>"
				if isType {
					reference = "<" + state.name + ">"
				}
				return nil, errors.New("Bad syntax reference: " + reference)
			}

			// stash a syntax for types with low priority
			if !syntaxStashDisabled && token != nil && isType {
				// https://drafts.csswg.org/css-values-4/#custom-idents
				// When parsing positionally-ambiguous keywords in a property value, a <custom-ident> production
				// can only claim the keyword if no other unfulfilled production can claim it.
				lowPriorityCustomIdent := state.name == "custom-ident" && token.typ == tokenizer.Ident

				// https://drafts.csswg.org/css-values-4/#lengths
				// ... if a `0` could be parsed as either a <number> or a <length> in a property (such as line-height),
				// it must parse as a <number>
				lowPriorityLength := state.name == "length" && token.value == "0"

				lowPriorityMatching := lowPriorityCustomIdent || lowPriorityLength

				if lowPriorityMatching {
					if syntaxStash == nil {
						syntaxStash = stateSnapshotFromSyntax(state, elseStack)
					}

					state = mismatchState
					break
				}
			}

			openSyntax()
			state = dictSyntax

		case "Keyword":
			name := state.name

			if token != nil {
				keywordName := token.value

				// drop \0 and \9 hack from keyword name
				if strings.Contains(keywordName, `\`) {
					keywordName = keywordHack.ReplaceAllString(keywordName, "")
				}

				if areStringsEqualCaseInsensitive(keywordName, name) || tokenNameIs(token, name) {
					addTokenToMatch()
					state = matchState
					break
				}
			}

			state = mismatchState

		case "AtKeyword", "Function":
			if token != nil && (areStringsEqualCaseInsensitive(token.value, state.name) || tokenNameIs(token, state.name)) {
				addTokenToMatch()
				state = matchState
				break
			}

			state = mismatchState

		case "Token":
			if token != nil && token.value == state.value {
				addTokenToMatch()
				state = matchState
				break
			}

			state = mismatchState

		case "Comma":
			if token != nil && token.typ == tokenizer.Comma {
				if isCommaContextStart(matchStack.token) {
					state = mismatchState
				} else {
					addTokenToMatch()
					if isCommaContextEnd(token) {
						state = mismatchState
					} else {
						state = matchState
					}
				}
			} else {
				if isCommaContextStart(matchStack.token) || isCommaContextEnd(token) {
					state = matchState
				} else {
					state = mismatchState
				}
			}

		case "String":
			var str strings.Builder
			lastTokenIndex := tokenIndex

			for ; lastTokenIndex < len(tokens) && str.Len() < len(state.value); lastTokenIndex++ {
				str.WriteString(tokens[lastTokenIndex].value)
			}

			if areStringsEqualCaseInsensitive(str.String(), state.value) {
				for tokenIndex < lastTokenIndex {
					addTokenToMatch()
				}

				state = matchState
			} else {
				state = mismatchState
			}

		default:
			//lint:ignore ST1005 upstream message
			return nil, fmt.Errorf("Unknown node type: %s", state.typ)
		}
	}

	switch exitReason {
	case "":
		// upstream also logs "[csstree-match] BREAK after 15000 iterations"
		// with console.warn
		exitReason = exitReasonIterationLimit
		matchStack = nil

	case exitReasonMatch:
		for syntaxStack != nil {
			closeSyntax()
		}

	default:
		matchStack = nil
	}

	return &matchResult{
		tokens:       tokens,
		reason:       exitReason,
		iterations:   iterationCount,
		match:        matchStack,
		longestMatch: longestMatch,
	}, nil
}

func matchAsList(tokens []matchToken, matchGraph *matchGraphNode, syntaxes matchSyntaxes) (*matchResult, error) {
	matchResult, err := internalMatch(tokens, matchGraph, syntaxes)
	if err != nil {
		return nil, err
	}

	if matchResult.match != nil {
		item := reverseList(matchResult.match).prev

		matchResult.list = []matchListItem{}

		for item != nil {
			switch item.typ {
			case openSyntaxItem, closeSyntaxItem:
				matchResult.list = append(matchResult.list, matchListItem{
					typ:    item.typ,
					syntax: item.syntax,
				})

			default:
				matchResult.list = append(matchResult.list, matchListItem{
					isToken: true,
					token:   item.token.value,
					node:    item.token.node,
				})
			}

			item = item.prev
		}
	}

	return matchResult, nil
}

func matchAsTree(tokens []matchToken, matchGraph *matchGraphNode, syntaxes matchSyntaxes) (*matchResult, error) {
	matchResult, err := internalMatch(tokens, matchGraph, syntaxes)
	if err != nil {
		return nil, err
	}

	if matchResult.match == nil {
		return matchResult, nil
	}

	item := matchResult.match
	host := &MatchNode{
		Match: []*MatchNode{},
	}
	if matchGraph.ref != nil {
		host.Syntax = matchGraph.ref
	}
	matchResult.tree = host
	hostStack := []*MatchNode{host}

	// revert a list and start with 2nd item since 1st is a stub item
	item = reverseList(item).prev

	// build a tree
	for item != nil {
		switch item.typ {
		case openSyntaxItem:
			host.Match = append(host.Match, &MatchNode{
				Syntax: item.syntax,
				Match:  []*MatchNode{},
			})
			host = host.Match[len(host.Match)-1]
			hostStack = append(hostStack, host)

		case closeSyntaxItem:
			hostStack = hostStack[:len(hostStack)-1]
			host = hostStack[len(hostStack)-1]

		default:
			leaf := &MatchNode{
				Token: item.token.value,
				Node:  item.token.node,
			}
			if item.syntax != nil {
				leaf.Syntax = item.syntax
			}
			host.Match = append(host.Match, leaf)
		}

		item = item.prev
	}

	return matchResult, nil
}
