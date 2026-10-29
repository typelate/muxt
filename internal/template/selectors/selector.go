package selectors

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/template"
)

// Selector is a compiled selector. The zero value is not usable; obtain one
// from [Compile] or [MustCompile].
type Selector struct {
	match matcher
}

// matcher is the internal interface implemented by every selector AST node.
// It reports, for a node in a template tree, whether the selector matches.
type matcher interface {
	match(n template.Node) Trit
}

// Compile parses a selector string into a [Selector].
func Compile(s string) (Selector, error) {
	m, err := parse(s)
	if err != nil {
		return Selector{}, err
	}
	return Selector{match: m}, nil
}

// MustCompile is like [Compile] but panics if the selector cannot be parsed.
func MustCompile(s string) Selector {
	sel, err := Compile(s)
	if err != nil {
		panic("selectors: " + err.Error())
	}
	return sel
}

// Matches reports whether the selector matches n. ok is true when the match is
// possible (the Trit is [TritYes] or [TritMaybe]) and false when it is
// statically impossible ([TritNo]). The Trit gives the certainty: [TritYes] if
// n statically matches, [TritMaybe] if the result depends on runtime data.
func (s Selector) Matches(n template.Node) (ok bool, certainty Trit) {
	t := s.trit(n)
	return t != TritNo, t
}

// trit returns the raw match certainty for n.
func (s Selector) trit(n template.Node) Trit {
	if s.match == nil {
		return TritNo
	}
	return s.match.match(n)
}

// parse compiles a selector string into a matcher.
func parse(s string) (matcher, error) {
	p := &parser{s: s}
	m, err := p.parseSelectorList()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.i < len(p.s) {
		return nil, fmt.Errorf("unexpected %q at offset %d", p.s[p.i:], p.i)
	}
	return m, nil
}

// parseSelectorList parses one or more complex selectors separated by commas.
func (p *parser) parseSelectorList() (matcher, error) {
	p.skipSpace()
	first, err := p.parseComplex()
	if err != nil {
		return nil, err
	}
	alternatives := []matcher{first}
	for {
		p.skipSpace()
		if p.peek() != ',' {
			break
		}
		p.i++ // consume ','
		p.skipSpace()
		next, err := p.parseComplex()
		if err != nil {
			return nil, err
		}
		alternatives = append(alternatives, next)
	}
	if len(alternatives) == 1 {
		return alternatives[0], nil
	}
	return selectorList{alternatives: alternatives}, nil
}

// parseComplex parses compound selectors joined by combinators, building a
// left-associative chain so the rightmost compound is the match subject.
func (p *parser) parseComplex() (matcher, error) {
	left, err := p.parseCompound()
	if err != nil {
		return nil, err
	}
	for {
		sawSpace := p.skipSpaceReport()
		var comb combinator
		switch c := p.peek(); c {
		case '>', '+', '~':
			comb = combinator(c)
			p.i++
			p.skipSpace()
		case ',', 0:
			return left, nil
		default:
			if !sawSpace || !isCompoundStart(c) {
				return left, nil
			}
			comb = descendant
		}
		right, err := p.parseCompound()
		if err != nil {
			return nil, err
		}
		left = combinatorSelector{left: left, comb: comb, right: right}
	}
}

// skipSpaceReport skips whitespace and reports whether any was consumed.
func (p *parser) skipSpaceReport() bool {
	start := p.i
	p.skipSpace()
	return p.i > start
}

func isCompoundStart(c byte) bool {
	return isNameStart(c) || c == '*' || c == '#' || c == '.' || c == '[' || c == ':'
}

// parser is a small recursive-descent selector parser.
type parser struct {
	s string
	i int
}

func (p *parser) skipSpace() {
	for p.i < len(p.s) && isHTMLSpace(rune(p.s[p.i])) {
		p.i++
	}
}

func (p *parser) peek() byte {
	if p.i < len(p.s) {
		return p.s[p.i]
	}
	return 0
}

// parseCompound parses a sequence of simple selectors applied to one element.
func (p *parser) parseCompound() (matcher, error) {
	var simples []matcher

	// Optional leading type or universal selector.
	switch p.peek() {
	case '*':
		p.i++
		simples = append(simples, universalSelector{})
	default:
		if isNameStart(p.peek()) {
			simples = append(simples, typeSelector{tag: p.readName()})
		}
	}

	for {
		switch p.peek() {
		case '#':
			p.i++
			simples = append(simples, idSelector{id: p.readName()})
		case '.':
			p.i++
			simples = append(simples, classSelector{class: p.readName()})
		case '[':
			a, err := p.parseAttr()
			if err != nil {
				return nil, err
			}
			simples = append(simples, a)
		case ':':
			p.i++
			ps, err := p.parsePseudo()
			if err != nil {
				return nil, err
			}
			simples = append(simples, ps)
		default:
			if len(simples) == 0 {
				return nil, fmt.Errorf("expected a selector at offset %d", p.i)
			}
			if len(simples) == 1 {
				return simples[0], nil
			}
			return compoundSelector{simples: simples}, nil
		}
	}
}

// parseHasArg consumes the argument of :has(): a comma-separated list of
// relative selectors, each an optional leading combinator (>, +, ~; default
// descendant) followed by a single compound selector. A relative selector with
// an internal combinator or descendant chain (e.g. ".a .b" or ".a > .b") is not
// supported — scoping the ancestor traversal to the :has host is not yet
// implemented — and is rejected with an error.
func (p *parser) parseHasArg() (matcher, error) {
	if p.peek() != '(' {
		return nil, fmt.Errorf("expected '(' after :has at offset %d", p.i)
	}
	p.i++
	var alternatives []matcher
	for {
		p.skipSpace()
		comb := descendant
		switch p.peek() {
		case '>', '+', '~':
			comb = combinator(p.peek())
			p.i++
			p.skipSpace()
		}
		rel, err := p.parseCompound()
		if err != nil {
			return nil, err
		}
		alternatives = append(alternatives, hasPseudo{comb: comb, rel: rel})
		p.skipSpace()
		switch p.peek() {
		case ',':
			p.i++
			continue
		case ')':
			p.i++
			if len(alternatives) == 1 {
				return alternatives[0], nil
			}
			return selectorList{alternatives: alternatives}, nil
		default:
			return nil, fmt.Errorf("unsupported complex relative selector in :has() at offset %d", p.i)
		}
	}
}

// parsePseudo parses a pseudo-class following ':'.
func (p *parser) parsePseudo() (matcher, error) {
	name := p.readName()
	switch name {
	case "first-child":
		return firstChild{}, nil
	case "last-child":
		return lastChild{}, nil
	case "empty":
		return empty{}, nil
	case "root":
		return root{}, nil
	case "only-child":
		return onlyChild{}, nil
	case "not":
		if p.peek() != '(' {
			return nil, fmt.Errorf("expected '(' after :not at offset %d", p.i)
		}
		p.i++ // consume '('
		inner, err := p.parseSelectorList()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.peek() != ')' {
			return nil, fmt.Errorf("expected ')' to close :not at offset %d", p.i)
		}
		p.i++ // consume ')'
		return pseudoNot{inner: inner}, nil
	case "is", "where":
		if p.peek() != '(' {
			return nil, fmt.Errorf("expected '(' after :%s at offset %d", name, p.i)
		}
		p.i++
		inner, err := p.parseSelectorList()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.peek() != ')' {
			return nil, fmt.Errorf("expected ')' to close :%s at offset %d", name, p.i)
		}
		p.i++
		return isPseudo{inner: inner}, nil
	case "nth-child", "nth-last-child":
		a, b, err := p.parseNthArg()
		if err != nil {
			return nil, err
		}
		return nthChild{a: a, b: b, fromEnd: name == "nth-last-child"}, nil
	case "nth-of-type", "nth-last-of-type":
		a, b, err := p.parseNthArg()
		if err != nil {
			return nil, err
		}
		return nthOfType{a: a, b: b, fromEnd: name == "nth-last-of-type"}, nil
	case "first-of-type":
		return nthOfType{a: 0, b: 1}, nil
	case "last-of-type":
		return nthOfType{a: 0, b: 1, fromEnd: true}, nil
	case "only-of-type":
		return compoundSelector{simples: []matcher{
			nthOfType{a: 0, b: 1},
			nthOfType{a: 0, b: 1, fromEnd: true},
		}}, nil
	case "has":
		return p.parseHasArg()
	default:
		if interactivePseudos[name] {
			if err := p.skipFunctionalArg(); err != nil {
				return nil, err
			}
			return maybePseudo{}, nil
		}
		return nil, fmt.Errorf("unsupported pseudo-class :%s", name)
	}
}

// parseAttr parses an attribute selector starting at '['.
func (p *parser) parseAttr() (matcher, error) {
	p.i++ // consume '['
	p.skipSpace()
	name := p.readName()
	if name == "" {
		return nil, fmt.Errorf("expected attribute name at offset %d", p.i)
	}
	p.skipSpace()
	if p.peek() == ']' {
		p.i++
		return attrSelector{name: name, op: attrExists}, nil
	}
	op, err := p.readAttrOp()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	val, err := p.readValue()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	ci := false
	switch p.peek() {
	case 'i', 'I':
		ci = true
		p.i++
		p.skipSpace()
	case 's', 'S':
		p.i++
		p.skipSpace()
	}
	if p.peek() != ']' {
		return nil, fmt.Errorf("expected ']' at offset %d", p.i)
	}
	p.i++
	return attrSelector{name: name, op: op, val: val, ci: ci}, nil
}

// readAttrOp reads an attribute operator ('=', '~=', '|=', '^=', '$=', '*=').
func (p *parser) readAttrOp() (attrOp, error) {
	switch c := p.peek(); c {
	case '=':
		p.i++
		return attrEquals, nil
	case '~', '|', '^', '$', '*':
		if p.i+1 >= len(p.s) || p.s[p.i+1] != '=' {
			return 0, fmt.Errorf("expected '=' after %q at offset %d", c, p.i)
		}
		p.i += 2
		switch c {
		case '~':
			return attrIncludes, nil
		case '|':
			return attrDashMatch, nil
		case '^':
			return attrPrefix, nil
		case '$':
			return attrSuffix, nil
		default: // '*'
			return attrSubstring, nil
		}
	default:
		return 0, fmt.Errorf("unsupported attribute operator at offset %d", p.i)
	}
}

// readEscape consumes a CSS escape sequence after a backslash: either up to six
// hex digits (optionally followed by one whitespace), or a single literal
// character. It assumes p.s[p.i] == '\\'. ok is false at end of input.
func (p *parser) readEscape() (rune, bool) {
	p.i++ // consume '\'
	if p.i >= len(p.s) {
		return 0, false
	}
	c := p.s[p.i]
	if isHexDigit(c) {
		start := p.i
		for p.i < len(p.s) && p.i-start < 6 && isHexDigit(p.s[p.i]) {
			p.i++
		}
		v, _ := strconv.ParseInt(p.s[start:p.i], 16, 32)
		if p.i < len(p.s) && isHTMLSpace(rune(p.s[p.i])) {
			p.i++ // one trailing whitespace is part of the escape
		}
		return rune(v), true
	}
	p.i++
	return rune(c), true
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// readName reads an identifier (tag, id, class, or attribute name).
func (p *parser) readName() string {
	var sb strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '\\':
			if r, ok := p.readEscape(); ok {
				sb.WriteRune(r)
			}
		case isNameStart(c) || c >= '0' && c <= '9' || c == '-':
			sb.WriteByte(c)
			p.i++
		default:
			return sb.String()
		}
	}
	return sb.String()
}

// readValue reads an attribute value: a quoted string or a bare value.
func (p *parser) readValue() (string, error) {
	q := p.peek()
	if q == '"' || q == '\'' {
		p.i++
		start := p.i
		for p.i < len(p.s) && p.s[p.i] != q {
			p.i++
		}
		if p.i >= len(p.s) {
			return "", fmt.Errorf("unterminated string in attribute selector")
		}
		val := p.s[start:p.i]
		p.i++ // consume closing quote
		return val, nil
	}
	// Read bare attribute value: any character except whitespace, quotes, or ']'
	// Also honors CSS escape sequences.
	var sb strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if isHTMLSpace(rune(c)) || c == '"' || c == '\'' || c == ']' {
			break
		}
		if c == '\\' {
			if r, ok := p.readEscape(); ok {
				sb.WriteRune(r)
			}
		} else {
			sb.WriteByte(c)
			p.i++
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("expected attribute value at offset %d", p.i)
	}
	return sb.String(), nil
}

func isNameStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

// parseAnB parses the An+B microsyntax (Selectors L4 §6.6.3): "odd", "even",
// an integer B, "n", "An", "An+B", "An-B", "-n+B", tolerant of internal
// whitespace. index positions are 1-based.
func parseAnB(s string) (a, b int, err error) {
	t := strings.TrimSpace(s)
	switch strings.ToLower(t) {
	case "odd":
		return 2, 1, nil
	case "even":
		return 2, 0, nil
	case "":
		return 0, 0, fmt.Errorf("empty An+B")
	}
	// Split into the "n" term and the trailing constant.
	lower := strings.ToLower(t)
	// Detect invalid UTF-8 or character expansion that would break indexing.
	// If ToLower changes the byte length, we have non-ASCII characters causing issues.
	if len(lower) != len(t) {
		return 0, 0, fmt.Errorf("invalid An+B %q", s)
	}
	ni := strings.IndexByte(lower, 'n')
	if ni < 0 {
		// Pure integer B.
		v, perr := strconv.Atoi(strings.ReplaceAll(t, " ", ""))
		if perr != nil {
			return 0, 0, fmt.Errorf("invalid An+B %q", s)
		}
		return 0, v, nil
	}
	// Coefficient A is the text before 'n'.
	coeff := strings.TrimSpace(t[:ni])
	switch coeff {
	case "", "+":
		a = 1
	case "-":
		a = -1
	default:
		v, perr := strconv.Atoi(coeff)
		if perr != nil {
			return 0, 0, fmt.Errorf("invalid An+B coefficient %q", s)
		}
		a = v
	}
	rest := strings.TrimSpace(t[ni+1:])
	if rest == "" {
		return a, 0, nil
	}
	// rest is a signed integer, possibly with a space after the sign.
	rest = strings.ReplaceAll(rest, " ", "")
	if rest == "+" || rest == "-" {
		return 0, 0, fmt.Errorf("invalid An+B constant %q", s)
	}
	v, perr := strconv.Atoi(rest)
	if perr != nil {
		return 0, 0, fmt.Errorf("invalid An+B constant %q", s)
	}
	return a, v, nil
}

// anbMatches reports whether 1-based index satisfies index == a*k + b for some
// integer k >= 0.
func anbMatches(a, b, index int) bool {
	if index < 1 {
		return false
	}
	if a == 0 {
		return index == b
	}
	diff := index - b
	if diff == 0 {
		return true
	}
	if diff%a != 0 {
		return false
	}
	return diff/a > 0
}

// anbMatchesAll reports whether every index >= 1 matches (e.g. "n").
func anbMatchesAll(a, b int) bool {
	return a == 1 && b <= 1
}

// anbMatchesNone reports whether no index >= 1 can match.
func anbMatchesNone(a, b int) bool {
	return a <= 0 && b <= 0
}

// parseNthArg consumes "( An+B )" and returns the coefficients.
func (p *parser) parseNthArg() (a, b int, err error) {
	if p.peek() != '(' {
		return 0, 0, fmt.Errorf("expected '(' at offset %d", p.i)
	}
	p.i++
	start := p.i
	for p.i < len(p.s) && p.s[p.i] != ')' {
		p.i++
	}
	if p.i >= len(p.s) {
		return 0, 0, fmt.Errorf("unterminated nth argument")
	}
	arg := p.s[start:p.i]
	p.i++ // consume ')'
	return parseAnB(arg)
}

// skipFunctionalArg consumes a balanced "( ... )" argument, ignoring its
// contents. Used for interactive functional pseudo-classes like :lang(en).
func (p *parser) skipFunctionalArg() error {
	if p.peek() != '(' {
		return nil // no argument
	}
	depth := 0
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case '(':
			depth++
		case ')':
			depth--
			p.i++
			if depth == 0 {
				return nil
			}
			continue
		}
		p.i++
	}
	return fmt.Errorf("unterminated functional pseudo-class argument")
}
