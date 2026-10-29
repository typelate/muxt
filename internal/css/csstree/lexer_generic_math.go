// Type checking of math functions, which upstream csstree consumes without
// looking at their arguments.
//
// CSS Values and Units Level 4 §10 https://drafts.csswg.org/css-values-4/#math
// CSS Typed OM Level 1 §4.3.2 (types) https://drafts.css-houdini.org/css-typed-om-1/#numeric-typing

package csstree

import (
	"strings"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// maxCalcDepth is the deepest nesting of parentheses and math functions a
// calculation may have. §10.8 requires at least 32 levels and makes a
// calculation with more nesting than supported invalid.
const maxCalcDepth = 256

// calcBase is a base type of a math function type (Typed OM: "length",
// "angle", "time", "frequency", "resolution" and "percent"). <flex> is
// left out: CSS Grid 2 §7.2.4 says fr values cannot be represented in
// calc(), so a flex unit in a calculation is a failure like any other
// unknown unit.
type calcBase int8

const (
	calcLength calcBase = iota
	calcAngle
	calcTime
	calcFrequency
	calcResolution
	calcPercent
	calcBaseCount

	// calcNoHint is a null percent hint. As a calculation context it
	// means percentages are not allowed.
	calcNoHint calcBase = -1
)

// calcType is the type of a calculation: an exponent per base type and a
// percent hint.
type calcType struct {
	exp  [calcBaseCount]int
	hint calcBase
}

// numberCalcType is «[ ]», the type of a <number>.
func numberCalcType() calcType {
	return calcType{hint: calcNoHint}
}

func baseCalcType(base calcBase) calcType {
	t := numberCalcType()
	t.exp[base] = 1
	return t
}

// percentageCalcType is the type of a <percentage-token> in the
// calculation context ctx (§10.9): the type percentages resolve against
// with that type as percent hint, else «[ "percent" → 1 ]» with a percent
// hint of "percent".
func percentageCalcType(ctx calcBase) calcType {
	if ctx == calcNoHint || ctx == calcPercent {
		ctx = calcPercent
	}
	t := baseCalcType(ctx)
	t.hint = ctx
	return t
}

// applyPercentHint is Typed OM "apply the percent hint".
func (t calcType) applyPercentHint(hint calcBase) calcType {
	t.hint = hint
	if hint != calcPercent {
		t.exp[hint] += t.exp[calcPercent]
		t.exp[calcPercent] = 0
	}
	return t
}

// sharePercentHints is the common first step of Typed OM "add two types"
// and "multiply two types": it fails on different percent hints and
// applies one type's hint to the other.
func sharePercentHints(a, b calcType) (calcType, calcType, bool) {
	switch {
	case a.hint != calcNoHint && b.hint != calcNoHint && a.hint != b.hint:
		return a, b, false
	case a.hint != calcNoHint && b.hint == calcNoHint:
		b = b.applyPercentHint(a.hint)
	case b.hint != calcNoHint && a.hint == calcNoHint:
		a = a.applyPercentHint(b.hint)
	}
	return a, b, true
}

// addCalcTypes is Typed OM "add two types".
func addCalcTypes(a, b calcType) (calcType, bool) {
	a, b, ok := sharePercentHints(a, b)
	if !ok {
		return calcType{}, false
	}

	if a.exp == b.exp {
		return a, true
	}

	hasPercent := a.exp[calcPercent] != 0 || b.exp[calcPercent] != 0
	hasOther := a.hasNonPercentEntry() || b.hasNonPercentEntry()
	if !hasPercent || !hasOther {
		return calcType{}, false
	}

	for hint := range calcPercent {
		ha, hb := a.applyPercentHint(hint), b.applyPercentHint(hint)
		if ha.exp == hb.exp {
			return ha, true
		}
	}

	return calcType{}, false
}

func (t calcType) hasNonPercentEntry() bool {
	for base := range calcPercent {
		if t.exp[base] != 0 {
			return true
		}
	}
	return false
}

// multiplyCalcTypes is Typed OM "multiply two types".
func multiplyCalcTypes(a, b calcType) (calcType, bool) {
	a, b, ok := sharePercentHints(a, b)
	if !ok {
		return calcType{}, false
	}

	for base := range calcBaseCount {
		a.exp[base] += b.exp[base]
	}
	return a, true
}

// invert is Typed OM "invert a type".
func (t calcType) invert() calcType {
	for base := range calcBaseCount {
		t.exp[base] = -t.exp[base]
	}
	return t
}

// madeConsistentWith is Values 4 "make a type consistent" with t as the
// base.
func (t calcType) madeConsistentWith(input calcType) (calcType, bool) {
	if t.hint != calcNoHint && input.hint != calcNoHint && t.hint != input.hint {
		return calcType{}, false
	}
	if t.hint == calcNoHint {
		t.hint = input.hint
	}
	return t, true
}

// calcWant is the production a math function has to resolve to where it
// is used.
type calcWant int8

const (
	calcWantNumber calcWant = iota // <number>, and <integer> (§10.9)
	calcWantPercentage
	calcWantDimension // a single base type other than percent
	calcWantLength
	calcWantAngle
	calcWantTime
	calcWantFrequency
	calcWantResolution
)

func (w calcWant) base() calcBase {
	switch w {
	case calcWantLength:
		return calcLength
	case calcWantAngle:
		return calcAngle
	case calcWantTime:
		return calcTime
	case calcWantFrequency:
		return calcFrequency
	case calcWantResolution:
		return calcResolution
	case calcWantPercentage:
		return calcPercent
	}
	return calcNoHint
}

// isNumber reports whether t has no non-zero entry.
func (t calcType) isNumber() bool {
	return t.exp == [calcBaseCount]int{}
}

// onlyEntry reports whether base → 1 is the only non-zero entry of t.
func (t calcType) onlyEntry(base calcBase) bool {
	for b := range calcBaseCount {
		want := 0
		if b == base {
			want = 1
		}
		if t.exp[b] != want {
			return false
		}
	}
	return true
}

// matches is Typed OM "a type matches a CSS production" in the calculation
// context ctx, the type percentages resolve against (calcPercent when
// they are allowed but resolve against nothing or a <number>, calcNoHint
// when they are not allowed).
func (t calcType) matches(want calcWant, ctx calcBase) bool {
	switch want {
	case calcWantNumber:
		return t.isNumber() && (t.hint == calcNoHint || (ctx != calcNoHint && t.hint == ctx))

	case calcWantPercentage:
		return t.onlyEntry(calcPercent) && (t.hint == calcNoHint || t.hint == calcPercent)

	case calcWantDimension:
		if t.hint != calcNoHint {
			return false
		}
		for base := range calcPercent {
			if t.onlyEntry(base) {
				return true
			}
		}
		return false
	}

	base := want.base()
	return t.onlyEntry(base) && (t.hint == calcNoHint || (ctx == base && t.hint == base))
}

// mathFunctionGeneric returns the production a math function used for
// the generic type named generic has to resolve to, and the calculation
// context: percentSibling is set when the type is an alternative of a
// <percentage> in a "|" group, as in <length-percentage>. ok is false for
// generic types that do not take math functions at all.
func mathFunctionGeneric(generic string, percentSibling bool) (want calcWant, ctx calcBase, ok bool) {
	mixed := func(base calcBase) calcBase {
		if percentSibling {
			return base
		}
		return calcNoHint
	}

	switch generic {
	case "number", "integer":
		return calcWantNumber, mixed(calcPercent), true
	case "percentage":
		return calcWantPercentage, calcPercent, true
	case "dimension":
		return calcWantDimension, calcNoHint, true
	case "length":
		return calcWantLength, mixed(calcLength), true
	case "angle":
		return calcWantAngle, mixed(calcAngle), true
	case "time":
		return calcWantTime, mixed(calcTime), true
	case "frequency":
		return calcWantFrequency, mixed(calcFrequency), true
	case "resolution":
		return calcWantResolution, calcNoHint, true
	}
	return 0, 0, false
}

// isMathGeneric reports whether upstream lets the generic type named
// generic take math functions (the types built with mathType).
func isMathGeneric(generic string) bool {
	switch generic {
	case "number", "integer", "percentage", "dimension",
		"length", "angle", "time", "frequency", "resolution",
		"flex", "decibel", "semitones":
		return true
	}
	return false
}

// isMathFunctionName reports whether name, a lowercased function token
// value, is a math function (§10.8, and the -moz- and -webkit- prefixed
// calc() upstream takes).
func isMathFunctionName(name string) bool {
	switch name {
	case "calc(", "-moz-calc(", "-webkit-calc(",
		"min(", "max(", "clamp(",
		"round(", "mod(", "rem(",
		"sin(", "cos(", "tan(", "asin(", "acos(", "atan(", "atan2(",
		"pow(", "sqrt(", "hypot(", "log(", "exp(",
		"abs(", "sign(":
		return true
	}
	return false
}

// isSubstitutionFunctionName reports whether name, a lowercased function
// token value, is an arbitrary substitution function (CSS Values 5 §7),
// whose value is unknown until it is substituted.
func isSubstitutionFunctionName(name string) bool {
	switch name {
	case "var(", "env(", "attr(", "if(", "inherit(":
		return true
	}
	return false
}

// isLengthFunctionName reports whether name, a lowercased function token
// value, is a non-math function that resolves to a <length> in a
// calculation: anchor() and anchor-size() (CSS Anchor Positioning 1).
func isLengthFunctionName(name string) bool {
	return name == "anchor(" || name == "anchor-size("
}

// cssName returns an identifier, function name or unit as it compares
// with keywords: escapes resolved (CSS Syntax 3 §4.3.7) and ASCII
// lowercased, as CSS names are ASCII case-insensitive.
func cssName(s string) string {
	if strings.IndexByte(s, '\\') != -1 {
		if name, ok := strings.CutSuffix(s, "("); ok {
			return asciiLower(DecodeIdent(name)) + "("
		}
		s = DecodeIdent(s)
	}
	return asciiLower(s)
}

// asciiLower lowercases A..Z only.
func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if 'A' <= s[i] && s[i] <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if 'A' <= b[j] && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

// calcUnitSource looks up the base type of a dimension unit; the Lexer
// implements it with its unit groups.
type calcUnitSource interface {
	calcUnitBase(unit string) (calcBase, bool)
}

func (l *Lexer) calcUnitBase(unit string) (calcBase, bool) {
	return calcUnitBaseIn(l.units, unit)
}

func calcUnitBaseIn(units map[string][]string, unit string) (calcBase, bool) {
	groups := []struct {
		name string
		base calcBase
	}{
		{"length", calcLength},
		{"angle", calcAngle},
		{"time", calcTime},
		{"frequency", calcFrequency},
		{"resolution", calcResolution},
	}

	unit = cssName(unit)
	for _, group := range groups {
		for _, u := range units[group.name] {
			if u == unit {
				return group.base, true
			}
		}
	}
	return 0, false
}

// matchMathFunction matches a math function token for the generic type
// named generic. handled is false when token is not a math function or
// the type does not take math functions, leaving the token to the type's
// own matching. Otherwise it returns how many tokens the function takes, 0
// when its calculation is invalid or does not resolve to the type
// (§10.9). A function containing an arbitrary substitution function is
// not checked and is consumed as upstream does.
func matchMathFunction(generic string, percentSibling bool, token *matchToken, getNextToken func(offset int) *matchToken, syntaxes matchSyntaxes) (consumed int, handled bool) {
	if token == nil || token.typ != tokenizer.Function || !isMathGeneric(generic) {
		return 0, false
	}
	if !isMathFunctionName(cssName(token.value)) {
		return 0, false
	}

	length := consumeFunction(token, getNextToken)
	for i := 1; i < length; i++ {
		t := getNextToken(i)
		if t != nil && t.typ == tokenizer.Function && isSubstitutionFunctionName(cssName(t.value)) {
			return length, true
		}
	}

	want, ctx, ok := mathFunctionGeneric(generic, percentSibling)
	if !ok {
		return 0, true
	}

	units, _ := syntaxes.(calcUnitSource)
	p := &calcParser{next: getNextToken, ctx: ctx, units: units}
	t, ok := p.mathFunction()
	// a type matching want also resolves to one of the §10.9 productions
	if !ok || !t.matches(want, ctx) {
		return 0, true
	}
	return p.pos, true
}

// calcParser parses a math function from the tokens at offsets from the
// function token (§10.8) and computes its type (§10.9).
type calcParser struct {
	next  func(offset int) *matchToken
	pos   int
	depth int
	ctx   calcBase
	units calcUnitSource
}

// tok returns the token at pos after skipping comments, nil at the end.
func (p *calcParser) tok() *matchToken {
	for {
		t := p.next(p.pos)
		if t == nil || t.typ != tokenizer.Comment {
			return t
		}
		p.pos++
	}
}

// skipWhiteSpace skips whitespace and comments and reports whether there
// was whitespace.
func (p *calcParser) skipWhiteSpace() bool {
	seen := false
	for {
		t := p.next(p.pos)
		if t == nil || (t.typ != tokenizer.WhiteSpace && t.typ != tokenizer.Comment) {
			return seen
		}
		seen = seen || t.typ == tokenizer.WhiteSpace
		p.pos++
	}
}

func (p *calcParser) isDelim(t *matchToken, delim string) bool {
	return t != nil && t.typ == tokenizer.Delim && t.value == delim
}

// enter counts a level of nesting; it fails past maxCalcDepth.
func (p *calcParser) enter() bool {
	p.depth++
	return p.depth <= maxCalcDepth
}

func (p *calcParser) leave() {
	p.depth--
}

// closeParenthesis consumes the ) closing a function or a parenthesized
// sum; the end of the input closes it too (CSS Syntax 3 §5.4.9).
func (p *calcParser) closeParenthesis() bool {
	p.skipWhiteSpace()
	t := p.tok()
	if t == nil {
		return true
	}
	if t.typ != tokenizer.RightParenthesis {
		return false
	}
	p.pos++
	return true
}

// sum is <calc-sum> = <calc-product> [ [ '+' | '-' ] <calc-product> ]*,
// with whitespace required on both sides of + and -.
func (p *calcParser) sum() (calcType, bool) {
	t, ok := p.product()
	if !ok {
		return calcType{}, false
	}

	for {
		start := p.pos
		spaceBefore := p.skipWhiteSpace()
		op := p.tok()
		if !p.isDelim(op, "+") && !p.isDelim(op, "-") {
			p.pos = start
			return t, true
		}
		if !spaceBefore {
			return calcType{}, false
		}
		p.pos++
		if !p.skipWhiteSpace() {
			return calcType{}, false
		}

		operand, ok := p.product()
		if !ok {
			return calcType{}, false
		}
		if t, ok = addCalcTypes(t, operand); !ok {
			return calcType{}, false
		}
	}
}

// product is <calc-product> = <calc-value> [ [ '*' | / ] <calc-value> ]*.
func (p *calcParser) product() (calcType, bool) {
	t, ok := p.value()
	if !ok {
		return calcType{}, false
	}

	for {
		start := p.pos
		p.skipWhiteSpace()
		op := p.tok()
		divide := p.isDelim(op, "/")
		if !divide && !p.isDelim(op, "*") {
			p.pos = start
			return t, true
		}
		p.pos++
		p.skipWhiteSpace()

		operand, ok := p.value()
		if !ok {
			return calcType{}, false
		}
		if divide {
			operand = operand.invert()
		}
		if t, ok = multiplyCalcTypes(t, operand); !ok {
			return calcType{}, false
		}
	}
}

// value is <calc-value> = <number> | <dimension> | <percentage> |
// <calc-keyword> | ( <calc-sum> ), where the numeric types include math
// functions.
func (p *calcParser) value() (calcType, bool) {
	t := p.tok()
	if t == nil {
		return calcType{}, false
	}

	switch t.typ {
	case tokenizer.Number:
		p.pos++
		return numberCalcType(), true

	case tokenizer.Percentage:
		p.pos++
		return percentageCalcType(p.ctx), true

	case tokenizer.Dimension:
		unit := t.value[min(tokenizer.ConsumeNumber(t.value, 0), len(t.value)):]
		base, ok := p.unitBase(unit)
		if !ok {
			return calcType{}, false
		}
		p.pos++
		return baseCalcType(base), true

	case tokenizer.Ident:
		// §10.7 <calc-keyword> = e | pi | infinity | -infinity | NaN
		switch cssName(t.value) {
		case "e", "pi", "infinity", "-infinity", "nan":
			p.pos++
			return numberCalcType(), true
		}
		return calcType{}, false

	case tokenizer.LeftParenthesis:
		if !p.enter() {
			return calcType{}, false
		}
		defer p.leave()
		p.pos++
		p.skipWhiteSpace()
		inner, ok := p.sum()
		if !ok || !p.closeParenthesis() {
			return calcType{}, false
		}
		return inner, true

	case tokenizer.Function:
		name := cssName(t.value)
		if isMathFunctionName(name) {
			return p.mathFunction()
		}
		if isLengthFunctionName(name) {
			p.pos += consumeFunction(t, func(offset int) *matchToken { return p.next(p.pos + offset) })
			return baseCalcType(calcLength), true
		}
	}

	return calcType{}, false
}

func (p *calcParser) unitBase(unit string) (calcBase, bool) {
	if p.units == nil {
		return calcUnitBaseIn(defaultUnits(), unit)
	}
	return p.units.calcUnitBase(unit)
}

// calcArg is an argument of a math function: a calculation, or a keyword
// standing alone (none in clamp(), a rounding strategy in round()).
type calcArg struct {
	typ     calcType
	keyword string
}

// arguments parses the comma-separated arguments of a math function up
// to and including its closing parenthesis. isKeyword tells whether a
// lowercased identifier standing alone as argument index is a keyword.
func (p *calcParser) arguments(isKeyword func(index int, keyword string) bool) ([]calcArg, bool) {
	var args []calcArg
	for {
		p.skipWhiteSpace()
		if keyword, ok := p.loneKeyword(); ok && isKeyword(len(args), keyword) {
			args = append(args, calcArg{keyword: keyword})
		} else {
			t, ok := p.sum()
			if !ok {
				return nil, false
			}
			args = append(args, calcArg{typ: t})
		}

		p.skipWhiteSpace()
		t := p.tok()
		switch {
		case t == nil:
			return args, true
		case t.typ == tokenizer.Comma:
			p.pos++
		case t.typ == tokenizer.RightParenthesis:
			p.pos++
			return args, true
		default:
			return nil, false
		}
	}
}

// loneKeyword consumes an identifier followed by a comma, a ) or the end
// of the input, and returns it lowercased.
func (p *calcParser) loneKeyword() (string, bool) {
	t := p.tok()
	if t == nil || t.typ != tokenizer.Ident {
		return "", false
	}

	start := p.pos
	p.pos++
	p.skipWhiteSpace()
	after := p.tok()
	if after != nil && after.typ != tokenizer.Comma && after.typ != tokenizer.RightParenthesis {
		p.pos = start
		return "", false
	}
	p.pos = start + 1
	return cssName(t.value), true
}

func noKeywords(int, string) bool {
	return false
}

// sumOf adds the types of calculations (§10.9 for min(), max(), clamp(),
// hypot(), round(), mod(), rem()).
func sumOf(args []calcArg) (calcType, bool) {
	var sum calcType
	first := true
	for _, arg := range args {
		if arg.keyword != "" {
			continue
		}
		if first {
			sum, first = arg.typ, false
			continue
		}
		var ok bool
		if sum, ok = addCalcTypes(sum, arg.typ); !ok {
			return calcType{}, false
		}
	}
	return sum, !first
}

// mathFunction parses the math function at pos (§10.1-§10.6) and returns
// its type (§10.9).
func (p *calcParser) mathFunction() (calcType, bool) {
	if !p.enter() {
		return calcType{}, false
	}
	defer p.leave()

	name := cssName(p.tok().value)
	p.pos++

	switch name {
	case "calc(", "-moz-calc(", "-webkit-calc(", "abs(":
		args, ok := p.arguments(noKeywords)
		if !ok || len(args) != 1 {
			return calcType{}, false
		}
		return args[0].typ, true

	case "min(", "max(", "hypot(":
		args, ok := p.arguments(noKeywords)
		if !ok {
			return calcType{}, false
		}
		return sumOf(args)

	case "clamp(":
		// clamp( [ <calc-sum> | none ], <calc-sum>, [ <calc-sum> | none ] )
		args, ok := p.arguments(func(index int, keyword string) bool {
			return index != 1 && keyword == "none"
		})
		if !ok || len(args) != 3 {
			return calcType{}, false
		}
		return sumOf(args)

	case "round(":
		return p.round()

	case "mod(", "rem(":
		args, ok := p.arguments(noKeywords)
		if !ok || len(args) != 2 {
			return calcType{}, false
		}
		return sumOf(args)

	case "sin(", "cos(", "tan(":
		// a <number> or an <angle>, returning a <number>
		arg, ok := p.singleArgument()
		if !ok || (!arg.matches(calcWantNumber, p.ctx) && !arg.matches(calcWantAngle, p.ctx)) {
			return calcType{}, false
		}
		return numberCalcType().madeConsistentWith(arg)

	case "asin(", "acos(", "atan(":
		// a <number>, returning an <angle>
		arg, ok := p.singleArgument()
		if !ok || !arg.matches(calcWantNumber, p.ctx) {
			return calcType{}, false
		}
		return baseCalcType(calcAngle).madeConsistentWith(arg)

	case "atan2(":
		args, ok := p.arguments(noKeywords)
		if !ok || len(args) != 2 {
			return calcType{}, false
		}
		sum, ok := sumOf(args)
		if !ok {
			return calcType{}, false
		}
		return baseCalcType(calcAngle).madeConsistentWith(sum)

	case "pow(", "log(":
		// pow(A, B), log(A, B?): <number>s of a consistent type
		args, ok := p.arguments(noKeywords)
		if !ok || len(args) > 2 || (name == "pow(" && len(args) != 2) {
			return calcType{}, false
		}
		for _, arg := range args {
			if !arg.typ.matches(calcWantNumber, p.ctx) {
				return calcType{}, false
			}
		}
		sum, ok := sumOf(args)
		if !ok {
			return calcType{}, false
		}
		return numberCalcType().madeConsistentWith(sum)

	case "sqrt(", "exp(":
		arg, ok := p.singleArgument()
		if !ok || !arg.matches(calcWantNumber, p.ctx) {
			return calcType{}, false
		}
		return numberCalcType().madeConsistentWith(arg)

	case "sign(":
		arg, ok := p.singleArgument()
		if !ok {
			return calcType{}, false
		}
		return numberCalcType().madeConsistentWith(arg)
	}

	return calcType{}, false
}

func (p *calcParser) singleArgument() (calcType, bool) {
	args, ok := p.arguments(noKeywords)
	if !ok || len(args) != 1 {
		return calcType{}, false
	}
	return args[0].typ, true
}

// round parses round( <rounding-strategy>?, <calc-sum>, <calc-sum>? )
// (§10.3).
func (p *calcParser) round() (calcType, bool) {
	args, ok := p.arguments(func(index int, keyword string) bool {
		if index != 0 {
			return false
		}
		switch keyword {
		case "nearest", "up", "down", "to-zero", "line-width":
			return true
		}
		return false
	})
	if !ok {
		return calcType{}, false
	}

	strategy := "nearest"
	if len(args) > 0 && args[0].keyword != "" {
		strategy = args[0].keyword
		args = args[1:]
	}
	if len(args) == 0 || len(args) > 2 {
		return calcType{}, false
	}

	a := args[0].typ
	if strategy == "line-width" && !a.matches(calcWantLength, p.ctx) {
		return calcType{}, false
	}
	// B may be omitted when A is a <number> or for line-width
	if len(args) == 1 && strategy != "line-width" && !a.matches(calcWantNumber, p.ctx) {
		return calcType{}, false
	}
	return sumOf(args)
}

// hasPercentSiblingMeaning reports whether a math function for the
// generic type named name mixes with percentages when a <percentage> is
// an alternative to it: the <length-percentage>, <angle-percentage>,
// <time-percentage>, <frequency-percentage> and <number-percentage>
// pattern.
func hasPercentSiblingMeaning(name string) bool {
	switch name {
	case "length", "angle", "time", "frequency", "number", "integer":
		return true
	}
	return false
}

// markPercentSiblings marks the numeric Type terms of a "|" group that
// also has a <percentage> term, so that a math function matched for them
// resolves percentages against their type (§10.9.1 calculation contexts).
func markPercentSiblings(group *definitionsyntax.Group, terms []*matchGraphNode) {
	if group.Combinator != "|" {
		return
	}

	hasPercentage := false
	for _, term := range group.Terms {
		if t, ok := term.(*definitionsyntax.Type); ok && t.Name == "percentage" {
			hasPercentage = true
			break
		}
	}
	if !hasPercentage {
		return
	}

	for _, term := range terms {
		if term.typ == "Type" && hasPercentSiblingMeaning(term.name) {
			term.percentSibling = true
		}
	}
}
