// Ported from lib/lexer/generic.js.

package csstree

import (
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// genericFunc is a generic type: it returns how many tokens starting at
// token (nil at the end of the input) it consumes, 0 for a mismatch.
// getNextToken(offset) returns the token offset tokens ahead of token, nil
// past the end. opts is the range restriction of the type reference being
// matched, nil when there is none.
type genericFunc func(token *matchToken, getNextToken func(offset int) *matchToken, opts *definitionsyntax.Range) int

// genericType is a generic type with its name; createGenericTypes returns
// them in upstream's object key order.
type genericType struct {
	name string
	fn   genericFunc
}

// CSS mathematical functions categorized by return type behavior
// See: https://www.w3.org/TR/css-values-4/#math

// Calculation functions that return different types depending on input
func calcFunctionNames() []string {
	return []string{
		"calc(",
		"-moz-calc(",
		"-webkit-calc(",
	}
}

// Comparison functions that return different types depending on input
func comparisonFunctionNames() []string {
	return []string{
		"min(",
		"max(",
		"clamp(",
	}
}

// Functions that return a stepped value, i.e. a value that is rounded to the nearest step
func steppedValueFunctionNames() []string {
	return []string{
		"round(",
		"mod(",
		"rem(",
	}
}

// Trigonometrical functions that return a <number>
func trigNumberFunctionNames() []string {
	return []string{
		"sin(",
		"cos(",
		"tan(",
	}
}

// Trigonometrical functions that return a <angle>
func trigAngleFunctionNames() []string {
	return []string{
		"asin(",
		"acos(",
		"atan(",
		"atan2(",
	}
}

// Other functions that return a <number>
func otherNumberFunctionNames() []string {
	return []string{
		"pow(",
		"sqrt(",
		"log(",
		"exp(",
		"sign(",
	}
}

// Exponential functions that return a <number> or <dimension> or <percentage>
func expNumberDimensionPercentageFunctionNames() []string {
	return []string{
		"hypot(",
	}
}

// Return the same type as the input
func signFunctionNames() []string {
	return []string{
		"abs(",
	}
}

func numberFunctionNames() []string {
	return concatStrings(
		calcFunctionNames(),
		comparisonFunctionNames(),
		steppedValueFunctionNames(),
		trigNumberFunctionNames(),
		otherNumberFunctionNames(),
		expNumberDimensionPercentageFunctionNames(),
		signFunctionNames(),
	)
}

func percentageFunctionNames() []string {
	return concatStrings(
		calcFunctionNames(),
		comparisonFunctionNames(),
		steppedValueFunctionNames(),
		expNumberDimensionPercentageFunctionNames(),
		signFunctionNames(),
	)
}

func dimensionFunctionNames() []string {
	return concatStrings(
		calcFunctionNames(),
		comparisonFunctionNames(),
		steppedValueFunctionNames(),
		trigAngleFunctionNames(),
		expNumberDimensionPercentageFunctionNames(),
		signFunctionNames(),
	)
}

func concatStrings(lists ...[]string) []string {
	var result []string
	for _, list := range lists {
		result = append(result, list...)
	}
	return result
}

// balancePair returns the closing token type for an opening one (upstream
// the balancePair Map), 0 for other types.
func balancePair(tokenType tokenizer.TokenType) tokenizer.TokenType {
	switch tokenType {
	case tokenizer.Function, tokenizer.LeftParenthesis:
		return tokenizer.RightParenthesis
	case tokenizer.LeftSquareBracket:
		return tokenizer.RightSquareBracket
	case tokenizer.LeftCurlyBracket:
		return tokenizer.RightCurlyBracket
	}
	return 0
}

// safe char code getter
func charCodeAt(str string, index int) int {
	if index < len(str) {
		return int(str[index])
	}
	return 0
}

// eqStr reports whether the name actual (an identifier or a function
// name, escapes decoded) is an ASCII case-insensitive match for the
// lowercase expected.
func eqStr(actual, expected string) bool {
	if strings.Contains(actual, `\`) {
		actual = DecodeIdent(actual)
	}

	return tokenizer.CmpStr(actual, 0, len(actual), expected)
}

func eqStrAny(actual string, expected []string) bool {
	for i := 0; i < len(expected); i++ {
		if eqStr(actual, expected[i]) {
			return true
		}
	}

	return false
}

// IE postfix hack, i.e. 123\0 or 123px\9
func isPostfixIeHack(str string, offset int) bool {
	if offset != len(str)-2 {
		return false
	}

	return charCodeAt(str, offset) == 0x005C && // U+005C REVERSE SOLIDUS (\)
		tokenizer.IsDigit(charCodeAt(str, offset+1))
}

// jsNumber converts str to a number as JavaScript's Number(str) does: it
// trims whitespace, reads "" as 0, accepts decimal literals with an
// optional sign and exponent, Infinity, and 0x/0o/0b integers, and returns
// NaN for anything else. strconv.ParseFloat alone would also accept
// "inf", "nan", hexadecimal floats and underscores.
func jsNumber(str string) float64 {
	str = strings.TrimFunc(str, isJSWhiteSpace)

	if str == "" {
		return 0
	}

	switch str {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}

	if len(str) > 2 && str[0] == '0' {
		base := 0
		switch str[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			digits := str[2:]
			for i := 0; i < len(digits); i++ {
				if !isDigitOfBase(digits[i], base) {
					return math.NaN()
				}
			}
			// the exact integer rounded once to the nearest float64, as
			// JavaScript rounds the value of the literal
			value, ok := new(big.Int).SetString(digits, base)
			if !ok {
				return math.NaN()
			}
			f, _ := new(big.Float).SetInt(value).Float64()
			return f
		}
	}

	if !isJSDecimalLiteral(str) {
		return math.NaN()
	}

	value, err := strconv.ParseFloat(str, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return math.NaN()
	}

	// out of range values are ±Inf or ±0, as in JavaScript
	return value
}

func isJSWhiteSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	}
	return 99
}

func isDigitOfBase(c byte, base int) bool {
	return digitValue(c) < base
}

// isJSDecimalLiteral reports whether str is a StrDecimalLiteral with an
// optional sign: digits with an optional fraction, or a fraction alone,
// then an optional exponent.
func isJSDecimalLiteral(str string) bool {
	i := 0
	if i < len(str) && (str[i] == '+' || str[i] == '-') {
		i++
	}

	intDigits := 0
	for i < len(str) && str[i] >= '0' && str[i] <= '9' {
		i++
		intDigits++
	}

	fracDigits := 0
	if i < len(str) && str[i] == '.' {
		i++
		for i < len(str) && str[i] >= '0' && str[i] <= '9' {
			i++
			fracDigits++
		}
	}

	if intDigits == 0 && fracDigits == 0 {
		return false
	}

	if i < len(str) && (str[i] == 'e' || str[i] == 'E') {
		i++
		if i < len(str) && (str[i] == '+' || str[i] == '-') {
			i++
		}
		expDigits := 0
		for i < len(str) && str[i] >= '0' && str[i] <= '9' {
			i++
			expDigits++
		}
		if expDigits == 0 {
			return false
		}
	}

	return i == len(str)
}

// outOfRange reports whether the number at the start of value is outside
// opts. numEnd < 0 is upstream's omitted numEnd.
func outOfRange(opts *definitionsyntax.Range, value string, numEnd int) bool {
	return outOfRangeUnit(opts, value, numEnd, "")
}

// outOfRangeUnit reports whether the number at the start of value, of the
// lowercase unit ("" for a number), is outside opts. Upstream skips the
// bounds with a unit (a FIXME: a type conversion is not implemented);
// compareToBound compares them as CSS Values 4 §5.1 says.
func outOfRangeUnit(opts *definitionsyntax.Range, value string, numEnd int, unit string) bool {
	if opts != nil {
		numStr := value
		if numEnd >= 0 && numEnd != len(value) {
			numStr = value[:numEnd]
		}
		num := jsNumber(numStr)

		if math.IsNaN(num) {
			return true
		}

		if opts.Min != nil {
			if order, ok := compareToBound(num, unit, opts.Min); ok && order < 0 {
				return true
			}
		}

		if opts.Max != nil {
			if order, ok := compareToBound(num, unit, opts.Max); ok && order > 0 {
				return true
			}
		}
	}

	return false
}

func consumeFunction(token *matchToken, getNextToken func(offset int) *matchToken) int {
	var balanceCloseType tokenizer.TokenType
	var balanceStash []tokenizer.TokenType
	length := 0

	// balanced token consuming
scan:
	for token != nil {
		switch token.typ {
		case tokenizer.RightCurlyBracket, tokenizer.RightParenthesis, tokenizer.RightSquareBracket:
			if token.typ != balanceCloseType {
				break scan
			}

			balanceCloseType, balanceStash = popTokenType(balanceStash)

			if len(balanceStash) == 0 {
				length++
				break scan
			}

		case tokenizer.Function, tokenizer.LeftParenthesis, tokenizer.LeftSquareBracket, tokenizer.LeftCurlyBracket:
			balanceStash = append(balanceStash, balanceCloseType)
			balanceCloseType = balancePair(token.typ)
		}

		length++
		token = getNextToken(length)
	}

	return length
}

// popTokenType is balanceStash.pop(): it returns the last item (0 for an
// empty stack, upstream undefined) and the shortened stack.
func popTokenType(stack []tokenizer.TokenType) (tokenizer.TokenType, []tokenizer.TokenType) {
	if len(stack) == 0 {
		return 0, stack
	}
	return stack[len(stack)-1], stack[:len(stack)-1]
}

// TODO: implement
// can be used wherever <length>, <frequency>, <angle>, <time>, <percentage>, <number>, or <integer> values are allowed
// https://drafts.csswg.org/css-values/#calc-notation
// mathType is upstream math (renamed: Go code imports package math). The
// matcher type checks math functions with matchMathFunction before it
// calls a generic type, so the function branch is only reached directly.
func mathType(next genericFunc, functionNames []string) genericFunc {
	return func(token *matchToken, getNextToken func(offset int) *matchToken, opts *definitionsyntax.Range) int {
		if token == nil {
			return 0
		}

		if token.typ == tokenizer.Function && eqStrAny(token.value, functionNames) {
			return consumeFunction(token, getNextToken)
		}

		return next(token, getNextToken, opts)
	}
}

func tokenType(expectedTokenType tokenizer.TokenType) genericFunc {
	return func(token *matchToken, _ func(offset int) *matchToken, _ *definitionsyntax.Range) int {
		if token == nil || token.typ != expectedTokenType {
			return 0
		}

		return 1
	}
}

// =========================
// Complex types
//

// https://drafts.csswg.org/css-values-4/#custom-idents
// 4.2. Author-defined Identifiers: the <custom-ident> type
// Some properties accept arbitrary author-defined identifiers as a component value.
// This generic data type is denoted by <custom-ident>, and represents any valid CSS identifier
// that would not be misinterpreted as a pre-defined keyword in that property’s value definition.
//
// See also: https://developer.mozilla.org/en-US/docs/Web/CSS/custom-ident
func customIdent(token *matchToken, _ func(offset int) *matchToken, _ *definitionsyntax.Range) int {
	if token == nil || token.typ != tokenizer.Ident {
		return 0
	}

	// the identifier's value: escapes decoded, compared ASCII
	// case-insensitively
	name := tokenValue(token)

	// The CSS-wide keywords are not valid <custom-ident>s
	if eqStrAny(name, cssWideKeywords()) {
		return 0
	}

	// The default keyword is reserved and is also not a valid <custom-ident>
	if eqStr(name, "default") {
		return 0
	}

	// TODO: ignore property specific keywords (as described https://developer.mozilla.org/en-US/docs/Web/CSS/custom-ident)
	// Specifications using <custom-ident> must specify clearly what other keywords
	// are excluded from <custom-ident>, if any—for example by saying that any pre-defined keywords
	// in that property’s value definition are excluded. Excluded keywords are excluded
	// in all ASCII case permutations.

	return 1
}

// https://drafts.csswg.org/css-values-4/#dashed-idents
// The <dashed-ident> production is a <custom-ident>, with all the case-sensitivity that implies,
// with the additional restriction that it must start with two dashes (U+002D HYPHEN-MINUS).
func dashedIdent(token *matchToken, _ func(offset int) *matchToken, _ *definitionsyntax.Range) int {
	if token == nil || token.typ != tokenizer.Ident {
		return 0
	}

	// ... must start with two dashes (U+002D HYPHEN-MINUS), in its value
	// (escapes decoded)
	value := tokenValue(token)
	if charCodeAt(value, 0) != 0x002D || charCodeAt(value, 1) != 0x002D {
		return 0
	}

	return 1
}

// https://drafts.csswg.org/css-variables/#typedef-custom-property-name
// A custom property is any property whose name starts with two dashes (U+002D HYPHEN-MINUS), like --foo.
// The <custom-property-name> production corresponds to this: it’s defined as any <dashed-ident>
// (a valid identifier that starts with two dashes), except -- itself, which is reserved for future use by CSS.
func customPropertyName(token *matchToken, getNextToken func(offset int) *matchToken, opts *definitionsyntax.Range) int {
	// ... it’s defined as any <dashed-ident>
	if dashedIdent(token, getNextToken, opts) == 0 {
		return 0
	}

	// ... except -- itself, which is reserved for future use by CSS
	if tokenValue(token) == "--" {
		return 0
	}

	return 1
}

// https://drafts.csswg.org/css-color-4/#hex-notation
// The syntax of a <hex-color> is a <hash-token> token whose value consists of 3, 4, 6, or 8 hexadecimal digits.
// In other words, a hex color is written as a hash character, "#", followed by some number of digits 0-9 or
// letters a-f (the case of the letters doesn’t matter - #00ff00 is identical to #00FF00).
func hexColor(token *matchToken, _ func(offset int) *matchToken, _ *definitionsyntax.Range) int {
	if token == nil || token.typ != tokenizer.Hash {
		return 0
	}

	length := len(token.value)

	// valid values (length): #rgb (4), #rgba (5), #rrggbb (7), #rrggbbaa (9)
	if length != 4 && length != 5 && length != 7 && length != 9 {
		return 0
	}

	for i := 1; i < length; i++ {
		if !tokenizer.IsHexDigit(charCodeAt(token.value, i)) {
			return 0
		}
	}

	return 1
}

func idSelector(token *matchToken, _ func(offset int) *matchToken, _ *definitionsyntax.Range) int {
	if token == nil || token.typ != tokenizer.Hash {
		return 0
	}

	if !tokenizer.IsIdentifierStart(charCodeAt(token.value, 1), charCodeAt(token.value, 2), charCodeAt(token.value, 3)) {
		return 0
	}

	return 1
}

// https://drafts.csswg.org/css-syntax/#any-value
// It represents the entirety of what a valid declaration can have as its value.
func declarationValue(token *matchToken, getNextToken func(offset int) *matchToken, _ *definitionsyntax.Range) int {
	if token == nil {
		return 0
	}

	var balanceCloseType tokenizer.TokenType
	var balanceStash []tokenizer.TokenType
	length := 0

	// The <declaration-value> production matches any sequence of one or more tokens,
	// so long as the sequence does not contain ...
scan:
	for token != nil {
		switch token.typ {
		// ... <bad-string-token>, <bad-url-token>,
		case tokenizer.BadString, tokenizer.BadURL:
			break scan

		// ... unmatched <)-token>, <]-token>, or <}-token>,
		case tokenizer.RightCurlyBracket, tokenizer.RightParenthesis, tokenizer.RightSquareBracket:
			if token.typ != balanceCloseType {
				break scan
			}

			balanceCloseType, balanceStash = popTokenType(balanceStash)

		// ... or top-level <semicolon-token> tokens
		case tokenizer.Semicolon:
			if balanceCloseType == 0 {
				break scan
			}

		// ... or <delim-token> tokens with a value of "!"
		case tokenizer.Delim:
			if balanceCloseType == 0 && token.value == "!" {
				break scan
			}

		case tokenizer.Function, tokenizer.LeftParenthesis, tokenizer.LeftSquareBracket, tokenizer.LeftCurlyBracket:
			balanceStash = append(balanceStash, balanceCloseType)
			balanceCloseType = balancePair(token.typ)
		}

		length++
		token = getNextToken(length)
	}

	return length
}

// https://drafts.csswg.org/css-syntax/#any-value
// The <any-value> production is identical to <declaration-value>, but also
// allows top-level <semicolon-token> tokens and <delim-token> tokens
// with a value of "!". It represents the entirety of what valid CSS can be in any context.
func anyValue(token *matchToken, getNextToken func(offset int) *matchToken, _ *definitionsyntax.Range) int {
	if token == nil {
		return 0
	}

	var balanceCloseType tokenizer.TokenType
	var balanceStash []tokenizer.TokenType
	length := 0

	// The <any-value> production matches any sequence of one or more tokens,
	// so long as the sequence ...
scan:
	for token != nil {
		switch token.typ {
		// ... does not contain <bad-string-token>, <bad-url-token>,
		case tokenizer.BadString, tokenizer.BadURL:
			break scan

		// ... unmatched <)-token>, <]-token>, or <}-token>,
		case tokenizer.RightCurlyBracket, tokenizer.RightParenthesis, tokenizer.RightSquareBracket:
			if token.typ != balanceCloseType {
				break scan
			}

			balanceCloseType, balanceStash = popTokenType(balanceStash)

		case tokenizer.Function, tokenizer.LeftParenthesis, tokenizer.LeftSquareBracket, tokenizer.LeftCurlyBracket:
			balanceStash = append(balanceStash, balanceCloseType)
			balanceCloseType = balancePair(token.typ)
		}

		length++
		token = getNextToken(length)
	}

	return length
}

// =========================
// Dimensions
//

// dimension returns a generic type for dimension tokens whose unit is in
// units; nil units (upstream null) accepts any unit.
func dimension(units []string) genericFunc {
	var unitSet map[string]bool
	if units != nil {
		unitSet = make(map[string]bool, len(units))
		for _, unit := range units {
			unitSet[unit] = true
		}
	}

	return func(token *matchToken, _ func(offset int) *matchToken, opts *definitionsyntax.Range) int {
		if token == nil || token.typ != tokenizer.Dimension {
			return 0
		}

		numberEnd := tokenizer.ConsumeNumber(token.value, 0)

		// the unit without the IE postfix hack, i.e. 123px\0 or 123px\9,
		// and with escapes decoded, compared ASCII case-insensitively
		unit := asciiLowerCase(dimensionUnit(token.value, numberEnd))

		// check unit
		if unitSet != nil && !unitSet[unit] {
			return 0
		}

		// check range if specified
		if outOfRangeUnit(opts, token.value, numberEnd, unit) {
			return 0
		}

		return 1
	}
}

// =========================
// Percentage
//

// §5.5. Percentages: the <percentage> type
// https://drafts.csswg.org/css-values-4/#percentages
func percentage(token *matchToken, _ func(offset int) *matchToken, opts *definitionsyntax.Range) int {
	// ... corresponds to the <percentage-token> production
	if token == nil || token.typ != tokenizer.Percentage {
		return 0
	}

	// check range if specified
	if outOfRangeUnit(opts, token.value, len(token.value)-1, "%") {
		return 0
	}

	return 1
}

// =========================
// Numeric
//

// https://drafts.csswg.org/css-values-4/#numbers
// The value <zero> represents a literal number with the value 0. Expressions that merely
// evaluate to a <number> with the value 0 (for example, calc(0)) do not match <zero>;
// only literal <number-token>s do.
func zero(next genericFunc) genericFunc {
	if next == nil {
		next = func(*matchToken, func(offset int) *matchToken, *definitionsyntax.Range) int {
			return 0
		}
	}

	return func(token *matchToken, getNextToken func(offset int) *matchToken, opts *definitionsyntax.Range) int {
		if token != nil && token.typ == tokenizer.Number {
			// a zero is out of a range such as [1px,∞]
			if jsNumber(token.value) == 0 && !outOfRange(opts, "0", -1) {
				return 1
			}
		}

		return next(token, getNextToken, opts)
	}
}

// § 5.3. Real Numbers: the <number> type
// https://drafts.csswg.org/css-values-4/#numbers
// Number values are denoted by <number>, and represent real numbers, possibly with a fractional component.
// ... It corresponds to the <number-token> production
func number(token *matchToken, _ func(offset int) *matchToken, opts *definitionsyntax.Range) int {
	// ... a <number-token>, or the <dimension-token> of a number with the
	// IE postfix hack (123\9); upstream reads any token whose text is a
	// number, such as the delims + and - or the ident e5
	if token == nil || token.typ != tokenizer.Number && token.typ != tokenizer.Dimension {
		return 0
	}

	numberEnd := tokenizer.ConsumeNumber(token.value, 0)
	isNumber := token.typ == tokenizer.Number && numberEnd == len(token.value)
	if !isNumber && (token.typ != tokenizer.Dimension || !isPostfixIeHack(token.value, numberEnd)) {
		return 0
	}

	// check range if specified
	if outOfRange(opts, token.value, numberEnd) {
		return 0
	}

	return 1
}

// §5.2. Integers: the <integer> type
// https://drafts.csswg.org/css-values-4/#integers
func integer(token *matchToken, _ func(offset int) *matchToken, opts *definitionsyntax.Range) int {
	// ... corresponds to a subset of the <number-token> production
	if token == nil || token.typ != tokenizer.Number {
		return 0
	}

	// The first digit of an integer may be immediately preceded by `-` or `+` to indicate the integer’s sign.
	i := 0
	if charCodeAt(token.value, 0) == 0x002B || // U+002B PLUS SIGN (+)
		charCodeAt(token.value, 0) == 0x002D { // U+002D HYPHEN-MINUS (-)
		i = 1
	}

	// When written literally, an integer is one or more decimal digits 0 through 9 ...
	for ; i < len(token.value); i++ {
		if !tokenizer.IsDigit(charCodeAt(token.value, i)) {
			return 0
		}
	}

	// check range if specified
	if outOfRange(opts, token.value, i) {
		return 0
	}

	return 1
}

// token types
func tokenTypes() []genericType {
	return []genericType{
		{"ident-token", tokenType(tokenizer.Ident)},
		{"function-token", tokenType(tokenizer.Function)},
		{"at-keyword-token", tokenType(tokenizer.AtKeyword)},
		{"hash-token", tokenType(tokenizer.Hash)},
		{"string-token", tokenType(tokenizer.String)},
		{"bad-string-token", tokenType(tokenizer.BadString)},
		{"url-token", tokenType(tokenizer.URL)},
		{"bad-url-token", tokenType(tokenizer.BadURL)},
		{"delim-token", tokenType(tokenizer.Delim)},
		{"number-token", tokenType(tokenizer.Number)},
		{"percentage-token", tokenType(tokenizer.Percentage)},
		{"dimension-token", tokenType(tokenizer.Dimension)},
		{"whitespace-token", tokenType(tokenizer.WhiteSpace)},
		{"CDO-token", tokenType(tokenizer.CDO)},
		{"CDC-token", tokenType(tokenizer.CDC)},
		{"colon-token", tokenType(tokenizer.Colon)},
		{"semicolon-token", tokenType(tokenizer.Semicolon)},
		{"comma-token", tokenType(tokenizer.Comma)},
		{"[-token", tokenType(tokenizer.LeftSquareBracket)},
		{"]-token", tokenType(tokenizer.RightSquareBracket)},
		{"(-token", tokenType(tokenizer.LeftParenthesis)},
		{")-token", tokenType(tokenizer.RightParenthesis)},
		{"{-token", tokenType(tokenizer.LeftCurlyBracket)},
		{"}-token", tokenType(tokenizer.RightCurlyBracket)},
	}
}

// token production types
func productionTypes() []genericType {
	return []genericType{
		// token type aliases
		{"string", tokenType(tokenizer.String)},
		{"ident", tokenType(tokenizer.Ident)},

		// percentage
		{"percentage", mathType(percentage, percentageFunctionNames())},

		// numeric
		{"zero", zero(nil)},
		{"number", mathType(number, numberFunctionNames())},
		{"integer", mathType(integer, numberFunctionNames())},

		// complex types
		{"custom-ident", customIdent},
		{"dashed-ident", dashedIdent},
		{"custom-property-name", customPropertyName},
		{"hex-color", hexColor},
		{"id-selector", idSelector}, // element( <id-selector> )
		{"an-plus-b", anPlusB},
		{"urange", urange},
		{"declaration-value", declarationValue},
		{"any-value", anyValue},
	}
}

func unitGroups() []string {
	return []string{
		"length",
		"angle",
		"time",
		"frequency",
		"resolution",
		"flex",
		"decibel",
		"semitones",
	}
}

// dimensions types depend on units set
func createDemensionTypes(units map[string][]string) []genericType {
	angle := units["angle"]
	decibel := units["decibel"]
	frequency := units["frequency"]
	flex := units["flex"]
	length := units["length"]
	resolution := units["resolution"]
	semitones := units["semitones"]
	time := units["time"]

	return []genericType{
		{"dimension", mathType(dimension(nil), dimensionFunctionNames())},
		{"angle", mathType(dimension(nonNilStrings(angle)), dimensionFunctionNames())},
		{"decibel", mathType(dimension(nonNilStrings(decibel)), dimensionFunctionNames())},
		{"frequency", mathType(dimension(nonNilStrings(frequency)), dimensionFunctionNames())},
		{"flex", mathType(dimension(nonNilStrings(flex)), dimensionFunctionNames())},
		{"length", mathType(zero(dimension(nonNilStrings(length))), dimensionFunctionNames())},
		{"resolution", mathType(dimension(nonNilStrings(resolution)), dimensionFunctionNames())},
		{"semitones", mathType(dimension(nonNilStrings(semitones)), dimensionFunctionNames())},
		{"time", mathType(dimension(nonNilStrings(time)), dimensionFunctionNames())},
	}
}

// nonNilStrings returns units, or an empty slice for nil, so that a unit
// group without units accepts no unit (dimension(nil) accepts any).
func nonNilStrings(units []string) []string {
	if units == nil {
		return []string{}
	}
	return units
}

// The <attr-unit> production matches any identifier that is an ASCII case-insensitive
// match for the name of a CSS dimension unit, such as px, or the <delim-token> %.
func createAttrUnit(units map[string][]string) genericFunc {
	unitSet := make(map[string]bool)

	for _, group := range unitGroups() {
		if groupUnits, ok := units[group]; ok && groupUnits != nil {
			for _, unit := range groupUnits {
				unitSet[toLowerCase(unit)] = true
			}
		}
	}

	return func(token *matchToken, _ func(offset int) *matchToken, _ *definitionsyntax.Range) int {
		if token == nil {
			return 0
		}

		if token.typ == tokenizer.Delim && token.value == "%" {
			return 1
		}

		if token.typ == tokenizer.Ident && unitSet[asciiLowerCase(tokenValue(token))] {
			return 1
		}

		return 0
	}
}

func createGenericTypes(units map[string][]string) []genericType {
	return concatGenericTypes(
		tokenTypes(),
		productionTypes(),
		createDemensionTypes(units),
		[]genericType{{"attr-unit", createAttrUnit(units)}},
	)
}

func concatGenericTypes(lists ...[]genericType) []genericType {
	var result []genericType
	for _, list := range lists {
		result = append(result, list...)
	}
	return result
}
