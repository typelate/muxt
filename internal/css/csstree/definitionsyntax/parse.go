// Ported from lib/definition-syntax/parse.js.

package definitionsyntax

import (
	"maps"
	"slices"
	"strconv"
)

const (
	exclamationMark    = 33  // !
	numberSign         = 35  // #
	ampersand          = 38  // &
	apostrophe         = 39  // '
	leftParenthesis    = 40  // (
	rightParenthesis   = 41  // )
	asterisk           = 42  // *
	plusSign           = 43  // +
	comma              = 44  // ,
	hyphenMinus        = 45  // -
	lessThanSign       = 60  // <
	greaterThanSign    = 62  // >
	questionMark       = 63  // ?
	commercialAt       = 64  // @
	leftSquareBracket  = 91  // [
	rightSquareBracket = 93  // ]
	leftCurlyBracket   = 123 // {
	verticalLine       = 124 // |
	rightCurlyBracket  = 125 // }
	infinity           = "∞" // U+221E, more than one byte in a Go string
	minusSign          = "−" // U+2212, as CSS Values §2.4.1 writes −∞
)

// combinatorPrecedence is upstream COMBINATOR_PRECEDENCE.
func combinatorPrecedence(combinator string) int {
	switch combinator {
	case " ":
		return 1
	case "&&":
		return 2
	case "||":
		return 3
	case "|":
		return 4
	}
	return 0
}

// combinatorNode and spacesNode are the transient { type: 'Combinator' } and
// { type: 'Spaces' } tokens; they never appear in a parsed AST.
type combinatorNode struct {
	value string
}

type spacesNode struct {
	value string
}

func (*combinatorNode) Type() string { return "Combinator" }
func (*spacesNode) Type() string     { return "Spaces" }
func (*combinatorNode) node()        {}
func (*spacesNode) node()            {}

func isCombinator(node Node) bool {
	_, ok := node.(*combinatorNode)
	return ok
}

// scanInt reads a number for a multiplier range.
func scanInt(s *scanner) int {
	start := s.pos
	digits := s.scanNumber()
	value, err := strconv.Atoi(digits)
	if err != nil {
		// digits are 0-9 only, so the number is too large for an int;
		// upstream would keep it as an inexact float
		s.pos = start
		s.error("Number is out of range")
	}
	return value
}

// scanFloat reads a number for a bracketed range; like Number(), it gives
// ±Inf for a number too large for a float64.
func scanFloat(s *scanner) float64 {
	value, err := strconv.ParseFloat(s.scanNumber(), 64)
	if err != nil {
		// digits are 0-9 only, so the only possible error is ErrRange,
		// for which value is already ±Inf as Number() returns
		return value
	}
	return value
}

func readMultiplierRange(s *scanner) (min, max int) {
	s.eat(leftCurlyBracket)
	s.skipWs()

	min = scanInt(s)
	s.skipWs()

	if s.charCode() == comma {
		s.pos++
		s.skipWs()

		max = Unbounded
		if s.charCode() != rightCurlyBracket {
			// {A,B}: "at least A and at most B"; upstream accepts B < A
			maxStart := s.pos
			max = scanInt(s)
			if max < min {
				s.pos = maxStart
				s.error("Wrong multiplier range")
			}
			s.skipWs()
		}
	} else {
		max = min
	}

	s.eat(rightCurlyBracket)

	return min, max
}

func readMultiplier(s *scanner) *Multiplier {
	var min, max int
	isComma := false

	switch s.charCode() {
	case asterisk:
		s.pos++

		min, max = 0, Unbounded

	case plusSign:
		s.pos++

		min, max = 1, Unbounded

	case questionMark:
		s.pos++

		min, max = 0, 1

	case numberSign:
		s.pos++

		isComma = true

		if s.charCode() == leftCurlyBracket {
			min, max = readMultiplierRange(s)
		} else if s.charCode() == questionMark {
			// https://www.w3.org/TR/css-values-4/#component-multipliers
			// > the # and ? multipliers may be stacked as #?
			// In this case just treat "#?" as a single multiplier
			// { min: 0, max: 0, comma: true }
			s.pos++
			min, max = 0, Unbounded
		} else {
			min, max = 1, Unbounded
		}

	case leftCurlyBracket:
		min, max = readMultiplierRange(s)

	default:
		return nil
	}

	return &Multiplier{
		Comma: isComma,
		Min:   min,
		Max:   max,
		Term:  nil,
	}
}

func maybeMultiplied(s *scanner, node Node) Node {
	multiplier := readMultiplier(s)

	if multiplier != nil {
		multiplier.Term = node

		// https://www.w3.org/TR/css-values-4/#component-multipliers
		// > The + and # multipliers may be stacked as +#;
		// Represent "+#" as nested multipliers:
		// { ...<multiplier #>,
		//   term: {
		//     ...<multipler +>,
		//     term: node
		//   }
		// }
		if s.charCode() == numberSign &&
			s.charCodeAt(s.pos-1) == plusSign {
			return maybeMultiplied(s, multiplier)
		}

		// https://www.w3.org/TR/css-values-4/#component-multipliers
		// > the # and ? multipliers, {A} and ? multipliers, and {A,B} and ? multipliers
		// > may be stacked as #?, {A}?, and {A,B}?, respectively
		// Represent "{}?" as nested multipliers as well as "+#".
		// The "#?" case is already handled above, in maybeMultiplied()
		if s.charCode() == questionMark &&
			s.charCodeAt(s.pos-1) == rightCurlyBracket {
			return maybeMultiplied(s, multiplier)
		}

		return multiplier
	}

	return node
}

func maybeToken(s *scanner) Node {
	ch := s.peek()

	if ch == "" {
		return nil
	}

	return maybeMultiplied(s, &Token{
		Value: ch,
	})
}

func readProperty(s *scanner) Node {
	s.eat(lessThanSign)
	s.eat(apostrophe)

	name := s.scanWord()

	s.eat(apostrophe)
	s.eat(greaterThanSign)

	return maybeMultiplied(s, &Property{
		Name: name,
	})
}

// https://drafts.csswg.org/css-values-3/#numeric-ranges
// 4.1. Range Restrictions and Range Definition Notation
//
// Range restrictions can be annotated in the numeric type notation using CSS bracketed
// range notation—[min,max]—within the angle brackets, after the identifying keyword,
// indicating a closed range between (and including) min and max.
// For example, <integer [0, 10]> indicates an integer between 0 and 10, inclusive.
func readTypeRange(s *scanner) *Range {
	// use nil for Infinity to make AST format JSON serializable/deserializable
	var min *RangeBound // -Infinity
	var max *RangeBound // Infinity
	sign := 1.0

	s.eat(leftSquareBracket)

	sign = readSign(s)

	if sign == -1 && s.hasPrefix(infinity) {
		s.peek()
	} else {
		min = &RangeBound{Value: sign * scanFloat(s)}

		if s.isNameCharCode(s.charCode()) {
			min.Unit = s.scanWord()
		}
	}

	s.skipWs()
	s.eat(comma)
	s.skipWs()

	if s.hasPrefix(infinity) {
		s.peek()
	} else {
		sign = readSign(s)

		max = &RangeBound{Value: sign * scanFloat(s)}

		if s.isNameCharCode(s.charCode()) {
			max.Unit = s.scanWord()
		}
	}

	s.eat(rightSquareBracket)

	return &Range{
		Min: min,
		Max: max,
	}
}

// readSign moves past a U+002D HYPHEN-MINUS or a U+2212 MINUS SIGN and
// returns -1, or returns 1 when there is neither (upstream accepts only the
// hyphen-minus).
func readSign(s *scanner) float64 {
	if s.charCode() == hyphenMinus {
		s.pos++
		return -1
	}
	if s.hasPrefix(minusSign) {
		s.pos += len(minusSign)
		return -1
	}
	return 1
}

func readType(s *scanner) Node {
	var opts *Range

	s.eat(lessThanSign)
	name := s.scanWord()

	// https://drafts.csswg.org/css-values-5/#boolean
	if name == "boolean-expr" {
		s.eat(leftSquareBracket)

		implicitGroup := readImplicitGroup(s, rightSquareBracket)

		s.eat(rightSquareBracket)
		s.eat(greaterThanSign)

		var term Node = implicitGroup
		if len(implicitGroup.Terms) == 1 {
			term = implicitGroup.Terms[0]
		}

		return maybeMultiplied(s, &Boolean{
			Term: term,
		})
	}

	if s.charCode() == leftParenthesis &&
		s.nextCharCode() == rightParenthesis {
		s.pos += 2
		name += "()"
	}

	if s.charCodeAt(s.findWsEnd(s.pos)) == leftSquareBracket {
		s.skipWs()
		opts = readTypeRange(s)
	}

	s.eat(greaterThanSign)

	return maybeMultiplied(s, &Type{
		Name: name,
		Opts: opts,
	})
}

func readKeywordOrFunction(s *scanner) Node {
	name := s.scanWord()

	if s.charCode() == leftParenthesis {
		s.pos++

		return &Function{
			Name: name,
		}
	}

	return maybeMultiplied(s, &Keyword{
		Name: name,
	})
}

// regroupTerms returns the regrouped terms, since a Go slice cannot be
// spliced in place, and the lowest precedence combinator ("" for none).
func regroupTerms(terms []Node, combinators map[string]bool) ([]Node, string) {
	createGroup := func(terms []Node, combinator string) *Group {
		return &Group{
			Terms:         terms,
			Combinator:    combinator,
			DisallowEmpty: false,
			Explicit:      false,
		}
	}

	var combinator string

	sorted := slices.SortedFunc(maps.Keys(combinators), func(a, b string) int {
		return combinatorPrecedence(a) - combinatorPrecedence(b)
	})

	for len(sorted) > 0 {
		combinator = sorted[0]
		sorted = sorted[1:]

		i := 0
		subgroupStart := 0

		for ; i < len(terms); i++ {
			term, ok := terms[i].(*combinatorNode)

			if ok {
				if term.value == combinator {
					if subgroupStart == -1 {
						subgroupStart = i - 1
					}
					terms = slices.Delete(terms, i, i+1)
					i--
				} else {
					if subgroupStart != -1 && i-subgroupStart > 1 {
						terms = slices.Replace(terms,
							subgroupStart,
							i,
							Node(createGroup(slices.Clone(terms[subgroupStart:i]), combinator)),
						)
						i = subgroupStart + 1
					}
					subgroupStart = -1
				}
			}
		}

		if subgroupStart != -1 && len(sorted) > 0 {
			terms = slices.Replace(terms,
				subgroupStart,
				i,
				Node(createGroup(slices.Clone(terms[subgroupStart:i]), combinator)),
			)
		}
	}

	return terms, combinator
}

// readImplicitGroup reads terms until stopCharCode; -1 reads to the end.
func readImplicitGroup(s *scanner, stopCharCode int) *Group {
	combinators := make(map[string]bool)
	terms := []Node{}
	var prevToken Node
	prevTokenPos := s.pos
	prevTokenIsFunction := false

	for int(s.charCode()) != stopCharCode {
		var token Node
		if prevTokenIsFunction {
			token = readImplicitGroup(s, rightParenthesis)
		} else {
			token = peek(s)
		}

		if token == nil {
			break
		}

		if _, ok := token.(*spacesNode); ok {
			continue
		}

		if prevTokenIsFunction {
			group := token.(*Group)

			if len(group.Terms) == 0 {
				prevTokenIsFunction = false
				continue
			}

			if group.Combinator == " " {
				for len(group.Terms) > 1 {
					combinators[" "] = true // a b
					terms = append(terms, &combinatorNode{
						value: " ",
					}, group.Terms[0])
					group.Terms = group.Terms[1:]
				}

				token = group.Terms[0]
			}
		}

		if combinator, ok := token.(*combinatorNode); ok {
			// check for combinator in group beginning and double combinator sequence
			if prevToken == nil || isCombinator(prevToken) {
				s.pos = prevTokenPos
				s.error("Unexpected combinator")
			}

			combinators[combinator.value] = true
		} else if prevToken != nil && !isCombinator(prevToken) {
			combinators[" "] = true // a b
			terms = append(terms, &combinatorNode{
				value: " ",
			})
		}

		terms = append(terms, token)
		prevToken = token
		prevTokenPos = s.pos
		_, prevTokenIsFunction = token.(*Function)
	}

	// check for combinator in group ending
	if prevToken != nil && isCombinator(prevToken) {
		s.pos -= prevTokenPos
		s.error("Unexpected combinator")
	}

	terms, combinator := regroupTerms(terms, combinators)
	if combinator == "" {
		combinator = " "
	}

	return &Group{
		Terms:         terms,
		Combinator:    combinator,
		DisallowEmpty: false,
		Explicit:      false,
	}
}

func readGroup(s *scanner) *Group {
	s.eat(leftSquareBracket)
	result := readImplicitGroup(s, rightSquareBracket)
	s.eat(rightSquareBracket)

	result.Explicit = true

	if s.charCode() == exclamationMark {
		s.pos++
		result.DisallowEmpty = true
	}

	return result
}

// peek returns nil where upstream returns undefined.
func peek(s *scanner) Node {
	code := s.charCode()

	switch code {
	case rightSquareBracket:
		// don't eat, stop scan a group

	case leftSquareBracket:
		return maybeMultiplied(s, readGroup(s))

	case lessThanSign:
		if s.nextCharCode() == apostrophe {
			return readProperty(s)
		}
		return readType(s)

	case verticalLine:
		end := s.pos + 1
		if s.nextCharCode() == verticalLine {
			end = s.pos + 2
		}
		return &combinatorNode{
			value: s.substringToPos(end),
		}

	case ampersand:
		s.pos++
		s.eat(ampersand)

		return &combinatorNode{
			value: "&&",
		}

	case comma:
		s.pos++
		return &Comma{}

	case apostrophe:
		return maybeMultiplied(s, &String{
			Value: s.scanString(),
		})

	case space, tab, lineFeed, carriageReturn, formFeed:
		return &spacesNode{
			value: s.scanSpaces(),
		}

	case commercialAt:
		code = s.nextCharCode()

		if s.isNameCharCode(code) {
			s.pos++
			return &AtKeyword{
				Name: s.scanWord(),
			}
		}

		return maybeToken(s)

	case asterisk, plusSign, questionMark, numberSign, exclamationMark:
		// prohibited tokens (used as a multiplier start)

	case leftCurlyBracket:
		// LEFTCURLYBRACKET is allowed since mdn/data uses it w/o quoting
		// check next char isn't a number, because it's likely a disjoined multiplier
		code = s.nextCharCode()

		if code < 48 || code > 57 {
			return maybeToken(s)
		}

	default:
		if s.isNameCharCode(code) {
			return readKeywordOrFunction(s)
		}

		return maybeToken(s)
	}

	return nil
}

// Parse parses a value definition syntax. The result is always a Group. A
// malformed source gives a *SyntaxError.
func Parse(source string) (result *Group, err error) {
	defer func() {
		if r := recover(); r != nil {
			syntaxError, ok := r.(*SyntaxError)
			if !ok {
				panic(r)
			}
			result, err = nil, syntaxError
		}
	}()

	s := newScanner(source)
	result = readImplicitGroup(s, -1)

	if s.pos != len(source) {
		s.error("Unexpected input")
	}

	// reduce redundant groups with single group term
	if len(result.Terms) == 1 {
		if group, ok := result.Terms[0].(*Group); ok {
			return group, nil
		}
	}

	return result, nil
}
