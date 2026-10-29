package csstree

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// lexerEdgeCase is a match of a value (CSS text, or parsed as a value when
// ast is set) against a property, type, definition syntax, at-rule prelude
// or at-rule descriptor of the default lexer. The expected results were
// produced by upstream csstree 3.2.1 under Node.js.
type lexerEdgeCase struct {
	kind  string // "property", "type", "syntax", "prelude" or "descriptor"
	name  string
	value string

	ast    bool
	parent string // the at-rule of a descriptor

	matched    bool
	iterations int
	// mismatchOffset is checked for a SyntaxMatchError, -1 to skip
	mismatchOffset int
	// errText, when set, is the text of the error (the raw message of a
	// SyntaxMatchError)
	errText string
}

func (c lexerEdgeCase) run(t *testing.T, lexer *Lexer) *MatchResult {
	t.Helper()

	if c.ast {
		context := "value"
		if c.kind == "prelude" {
			context = "atrulePrelude"
		}
		node := mustParse(t, c.value, ParseOptions{Context: context, Atrule: c.name, Positions: true})
		switch c.kind {
		case "property":
			return lexer.MatchProperty(c.name, node)
		case "type":
			return lexer.MatchType(c.name, node)
		case "syntax":
			return lexer.Match(c.name, node)
		case "prelude":
			return lexer.MatchAtrulePrelude(c.name, node)
		case "descriptor":
			return lexer.MatchAtruleDescriptor(c.parent, c.name, node)
		}
	}

	switch c.kind {
	case "property":
		return lexer.MatchPropertyString(c.name, c.value)
	case "type":
		return lexer.MatchTypeString(c.name, c.value)
	case "syntax":
		return lexer.MatchString(c.name, c.value)
	case "prelude":
		return lexer.MatchAtrulePreludeString(c.name, c.value)
	case "descriptor":
		return lexer.MatchAtruleDescriptorString(c.parent, c.name, c.value)
	}

	t.Fatalf("unknown kind %q", c.kind)
	return nil
}

func runLexerEdgeCases(t *testing.T, lexer *Lexer, cases []lexerEdgeCase) {
	t.Helper()

	for _, c := range cases {
		desc := c.kind + " " + c.name + ": " + c.value
		if c.ast {
			desc += " (ast)"
		}
		t.Run(desc, func(t *testing.T) {
			result := c.run(t, lexer)

			if got := result.Matched != nil; got != c.matched {
				t.Fatalf("matched = %v, want %v (error: %v)", got, c.matched, result.Error)
			}
			if result.Iterations != c.iterations {
				t.Errorf("iterations = %d, want %d", result.Iterations, c.iterations)
			}
			if c.matched {
				if result.Error != nil {
					t.Errorf("error = %v, want nil", result.Error)
				}
				return
			}

			var matchErr *SyntaxMatchError
			isMatchErr := errors.As(result.Error, &matchErr)
			if c.mismatchOffset >= 0 && isMatchErr && matchErr.MismatchOffset != c.mismatchOffset {
				t.Errorf("mismatch offset = %d, want %d", matchErr.MismatchOffset, c.mismatchOffset)
			}
			if c.errText != "" {
				text := ""
				switch {
				case isMatchErr:
					text = matchErr.RawMessage
				case result.Error != nil:
					text = result.Error.Error()
				}
				if text != c.errText {
					t.Errorf("error = %q, want %q", text, c.errText)
				}
			}
		})
	}
}

// TestLexerUnicodeLowerCase checks that keywords, units and custom idents
// are lowercased as JavaScript's toLowerCase does: U+0130 LATIN CAPITAL
// LETTER I WITH DOT ABOVE lowercases to "i\u0307" (i and U+0307), not to an
// ASCII "i", so it never matches an ASCII keyword or unit.
func TestLexerUnicodeLowerCase(t *testing.T) {
	runLexerEdgeCases(t, DefaultLexer(), []lexerEdgeCase{
		// Enum (a | b | c of keywords)
		{kind: "property", name: "display", value: "\u0130nline", iterations: 71},
		{kind: "property", name: "float", value: "r\u0130ght", iterations: 3},
		// <custom-ident> excludes only the ASCII CSS-wide keywords and default
		{kind: "type", name: "custom-ident", value: "\u0130nherit", matched: true, iterations: 3},
		{kind: "type", name: "custom-ident", value: "d\u0130fault", matched: true, iterations: 3},
		{kind: "type", name: "custom-ident", value: "inherit", iterations: 3},
		// the CSS-wide keywords
		{kind: "property", name: "color", value: "\u0130nherit", iterations: 38},
		// units
		{kind: "property", name: "width", value: "1\u0130n", iterations: 29},
		{kind: "type", name: "length", value: "1\u0130N", iterations: 3},
		{kind: "syntax", name: "<attr-unit>", value: "\u0130n", iterations: 5},
		// U+212A KELVIN SIGN lowercases to an ASCII k in JavaScript, but
		// units are ASCII case-insensitive (the port follows the spec)
		{kind: "type", name: "frequency", value: "1\u212Ahz", iterations: 3},
	})
}

// TestLexerKeywordHackLineTerminator checks that dropping the \0 and \9
// hacks from a keyword stops at a line terminator, as JavaScript's
// /\\[09].*$/ does: "." matches no U+2028 or U+2029 and "$" is the end of
// the value.
func TestLexerKeywordHackLineTerminator(t *testing.T) {
	runLexerEdgeCases(t, DefaultLexer(), []lexerEdgeCase{
		{kind: "property", name: "color", value: "red\\9\u2028x", iterations: 38},
		{kind: "property", name: "color", value: "red\\9\u2028x", ast: true, iterations: 38},
		{kind: "property", name: "color", value: "red\\0\u2029", iterations: 38},
		{kind: "syntax", name: "a | b", value: "a\\9\u2028", iterations: 3},
		{kind: "syntax", name: "a", value: "a\\9\u2028", iterations: 3},
		{kind: "property", name: "color", value: "red\\9", matched: true, iterations: 22},
		{kind: "property", name: "color", value: "red\\9x", matched: true, iterations: 22},
	})
}

// TestFindFragmentsEmptyMatch checks that searching the fragments of a
// syntax that matched no tokens (an empty value of a property that allows
// one) fails rather than panics: upstream throws a TypeError reading the
// first token of the empty match.
func TestFindFragmentsEmptyMatch(t *testing.T) {
	lexer := DefaultLexer()
	value := mustParse(t, "", ParseOptions{Context: "value"})

	if result := lexer.MatchProperty("-webkit-box-reflect", value); result.Matched == nil {
		t.Fatalf("MatchProperty(-webkit-box-reflect, empty) error = %v, want a match", result.Error)
	}

	fragments, err := lexer.FindValueFragments("-webkit-box-reflect", value, "Property", "-webkit-box-reflect")
	if err == nil {
		t.Errorf("FindValueFragments(-webkit-box-reflect, empty) = %v, want an error", fragments)
	}

	ast := mustParse(t, "a{-webkit-box-reflect:}", ParseOptions{})
	fragments, err = lexer.FindAllFragments(ast, "Property", "-webkit-box-reflect")
	if err == nil {
		t.Errorf("FindAllFragments(-webkit-box-reflect, empty) = %v, want an error", fragments)
	}

	// other syntaxes of the empty match are searched as before
	fragments, err = lexer.FindValueFragments("-webkit-box-reflect", value, "Type", "length")
	if err != nil || len(fragments) != 0 {
		t.Errorf("FindValueFragments(-webkit-box-reflect, empty, <length>) = %v, %v, want none", fragments, err)
	}
}

// TestJSNumberEdges checks more of jsNumber against Node.js's Number():
// integers too large for 64 bits, separators and whitespace.
func TestJSNumberEdges(t *testing.T) {
	for _, test := range []struct {
		input string
		want  float64
	}{
		{"0x10000000000000000", 18446744073709552000},
		{"0xFFFFFFFFFFFFFFFFFF", 4.722366482869645e+21},
		{"0xfffffffffffffffff", 295147905179352830000},
		{"0b" + strings.Repeat("1", 70), 1.1805916207174113e+21},
		{"0o" + strings.Repeat("7", 30), 1.2379400392853803e+27},
		// rounded once, not digit by digit: 2^69 + 3·2^15 is nearer to
		// 2^69 + 2^17 than to 2^69
		{"0x200000000000018000", 590295810358705782784},
		{"0b1" + strings.Repeat("0", 52) + "11" + strings.Repeat("0", 20), 1.8889465931478585e+22},
		{"0XaB", 171},
		{"0B11", 3},
		{"0O17", 15},
		{"0x_1", math.NaN()},
		{"0x1_0", math.NaN()},
		{"0b102", math.NaN()},
		{"0o8", math.NaN()},
		{"+0x1", math.NaN()},
		{"00x1", math.NaN()},
		{"0b", math.NaN()},
		{"\u00a01\u2028", 1},
		{"\ufeff2", 2},
		{"\u20093", 3},
		{"-1e-400", math.Copysign(0, -1)},
	} {
		got := jsNumber(test.input)
		same := got == test.want && math.Signbit(got) == math.Signbit(test.want) ||
			math.IsNaN(got) && math.IsNaN(test.want)
		if !same {
			t.Errorf("jsNumber(%q) = %v, want %v", test.input, got, test.want)
		}
	}
}

// TestLexerSpecEdgeCases matches values from the edge cases of CSS Values
// and Units 4 and CSS Syntax 3. The expectations are upstream's results,
// except where a comment says the port follows the spec; elsewhere, where
// upstream disagrees with the spec, the comment says what the spec expects
// (the port keeps upstream's behaviour).
func TestLexerSpecEdgeCases(t *testing.T) {
	const mismatch = "Mismatch"

	runLexerEdgeCases(t, DefaultLexer(), []lexerEdgeCase{
		// Values 4 §2.2 component value combinators: juxtaposition binds
		// tighter than &&, && than ||, || than |
		{kind: "syntax", name: "a b | c", value: "a b", matched: true, iterations: 6},
		{kind: "syntax", name: "a b | c", value: "c", matched: true, iterations: 6},
		{kind: "syntax", name: "a b | c", value: "a c", iterations: 8, mismatchOffset: 2, errText: mismatch},
		{kind: "syntax", name: "a || b && c", value: "c b a", matched: true, iterations: 16},
		{kind: "syntax", name: "a || b && c", value: "b a c", iterations: 13, mismatchOffset: 2},
		{kind: "syntax", name: "a || b && c", value: "a b", iterations: 22},
		{kind: "syntax", name: "a | b || c", value: "b c", matched: true, iterations: 10},
		{kind: "syntax", name: "a | b || c", value: "a c", iterations: 11, mismatchOffset: 2},

		// Values 4 §2.4 multipliers and the ! required group
		{kind: "syntax", name: "[a? b?]!", value: "", iterations: 12},
		{kind: "syntax", name: "[a? b?]!", value: "b", matched: true, iterations: 11},
		{kind: "syntax", name: "<length>#{1,3}", value: "1px, 2px, 3px, 4px", iterations: 28, mismatchOffset: 13},
		{kind: "syntax", name: "<length>#{1,3}", value: "1px, 2px, 3px", matched: true, iterations: 27},
		{kind: "syntax", name: "<length>#{2,}", value: "1px", iterations: 17},
		{kind: "syntax", name: "<length>{2}", value: "1px 2px 3px", iterations: 11, mismatchOffset: 8},
		{kind: "syntax", name: "<length>+#", value: "1px 2px, 3px", matched: true, iterations: 49},
		{kind: "syntax", name: "<length>#", value: "1px,", iterations: 14},
		{kind: "syntax", name: "<length>#", value: "1px,,2px", iterations: 20, mismatchOffset: 4},

		// Values 4 §5.1 bracketed range notation; upstream does not check
		// ranges with units (a FIXME), so <angle [0deg,360deg]> takes
		// 400deg and -1deg; the port follows the spec
		{kind: "syntax", name: "<angle [0deg,360deg]>", value: "400deg", iterations: 5},
		{kind: "syntax", name: "<angle [0deg,360deg]>", value: "-1deg", iterations: 5},
		{kind: "syntax", name: "<length [0,∞]>", value: "-1px", iterations: 5},
		{kind: "syntax", name: "<length [0,∞]>", value: "calc(-1px)", matched: true, iterations: 5},
		{kind: "syntax", name: "<integer [1,∞]>", value: "0", iterations: 5},
		{kind: "syntax", name: "<integer [1,∞]>", value: "calc(0)", matched: true, iterations: 5},
		{kind: "syntax", name: "<number [0,1]>", value: "1e309", iterations: 5},
		{kind: "syntax", name: "<number [0,∞]>", value: "1e309", matched: true, iterations: 5},
		{kind: "syntax", name: "<number [-∞,0]>", value: "-1e309", matched: true, iterations: 5},
		{kind: "syntax", name: "<number [0,1]>", value: "-0", matched: true, iterations: 5},

		// Values 4 §4.2 <custom-ident> excludes the CSS-wide keywords and
		// default in any ASCII case, compared by their value: upstream
		// compares the source text, so takes \69nherit (inherit); the port
		// follows the spec
		{kind: "type", name: "custom-ident", value: "DEFAULT", iterations: 3},
		{kind: "type", name: "custom-ident", value: "Revert-Layer", iterations: 3},
		{kind: "type", name: "custom-ident", value: "\\69nherit", iterations: 3},
		{kind: "type", name: "custom-ident", value: "\\64 efault", iterations: 3},
		// keywords are compared by their value too: \72 ed is red, r\ed is
		// r followed by U+00ED
		{kind: "property", name: "color", value: "r\\ed", iterations: 38},
		{kind: "property", name: "color", value: "\\72 ed", matched: true, iterations: 22},
		// Values 4 §4.2 <dashed-ident>, CSS Variables <custom-property-name>
		{kind: "type", name: "dashed-ident", value: "--", matched: true, iterations: 3},
		{kind: "type", name: "dashed-ident", value: "-a", iterations: 3},
		{kind: "type", name: "custom-property-name", value: "--", iterations: 3},

		// Values 4 §5 numbers: <integer> is a number-token of digits
		{kind: "type", name: "integer", value: "1.0", iterations: 3},
		{kind: "type", name: "integer", value: "1e3", iterations: 3},
		{kind: "type", name: "integer", value: "-0", matched: true, iterations: 3},
		{kind: "type", name: "integer", value: "+0", matched: true, iterations: 3},
		{kind: "type", name: "number", value: "+.5", matched: true, iterations: 3},
		{kind: "type", name: "number", value: "1e3", matched: true, iterations: 3},
		{kind: "type", name: "number", value: "1e309", matched: true, iterations: 3},
		// a <number> is a <number-token>: upstream does not check the token
		// type, so it takes any token whose text is a number, the delims +
		// and - and idents like e5; the port follows the spec
		{kind: "type", name: "number", value: "+", iterations: 3},
		{kind: "type", name: "number", value: "-", iterations: 3},
		{kind: "type", name: "number", value: "e5", iterations: 3},
		{kind: "type", name: "number", value: "-E3", iterations: 3},
		{kind: "property", name: "opacity", value: "e5", iterations: 12},
		{kind: "property", name: "opacity", value: "+", iterations: 12},
		{kind: "property", name: "line-height", value: "e5", iterations: 18},
		// Values 4 §6 unitless zero is a <length>, not an <angle> or <time>
		{kind: "type", name: "length", value: "0", matched: true, iterations: 3},
		{kind: "type", name: "length", value: "0.0", matched: true, iterations: 3},
		{kind: "type", name: "length", value: "-0", matched: true, iterations: 3},
		{kind: "type", name: "length", value: "1", iterations: 3},
		{kind: "type", name: "angle", value: "0", iterations: 3},
		{kind: "type", name: "time", value: "0", iterations: 3},

		// Values 4 §10.9 math functions are type checked (upstream
		// consumes them without checking their arguments, so calc(red) is
		// a <length> and hypot(1px) a <number> there); a math function
		// resolving to a <number> is taken for an <integer>
		{kind: "property", name: "width", value: "calc(red)", iterations: 29},
		{kind: "type", name: "number", value: "hypot(1px)", iterations: 3},
		{kind: "type", name: "integer", value: "calc(1.5)", matched: true, iterations: 3},

		// CSS Color 4 <hex-color>
		{kind: "type", name: "hex-color", value: "#ABCDEF", matched: true, iterations: 3},
		{kind: "type", name: "hex-color", value: "#abcde", iterations: 3},

		// Syntax 3 §6.2 the <an+b> grammar includes odd and even, which
		// <an-plus-b> (the AnPlusB node's production) leaves to the
		// syntaxes using it; mdn-data's <an+b> has them, see
		// TestSpecAnPlusBOddEven
		{kind: "type", name: "an-plus-b", value: "odd", iterations: 3},
		{kind: "type", name: "an-plus-b", value: "even", iterations: 3},

		// Syntax 3 §7.1 <urange>: upstream checks the token forms and digit
		// counts, but not that the start is at most the end and at most
		// 10FFFF, that the end of a range is hex digits, or that the
		// first ident is u, so it takes ranges the spec rejects; the port
		// follows the spec
		{kind: "type", name: "urange", value: "U+0-10FFFF", matched: true, iterations: 3},
		{kind: "type", name: "urange", value: "U+110000", iterations: 3},
		{kind: "type", name: "urange", value: "U+FF-00", iterations: 3},
		{kind: "type", name: "urange", value: "U+1-zz", iterations: 3},
		{kind: "type", name: "urange", value: "u+a-", iterations: 3},
		{kind: "type", name: "urange", value: "U+1e-3", iterations: 3},
		{kind: "type", name: "urange", value: "ua+1", iterations: 3},
		{kind: "type", name: "urange", value: "U+1-1234567", iterations: 3},
		{kind: "type", name: "urange", value: "U+??-1", iterations: 4, mismatchOffset: 4},

		// properties: !important is not part of a value, vendor and hack
		// prefixes fall back to the property, custom properties and var()
		// are not matched
		{kind: "property", name: "color", value: "red !important", iterations: 40, mismatchOffset: 4},
		{kind: "property", name: "*zoom", value: "1", matched: true, iterations: 15},
		{kind: "property", name: "_color", value: "red", matched: true, iterations: 22},
		{kind: "property", name: "-webkit-color", value: "red", matched: true, iterations: 22},
		{kind: "property", name: "COLOR", value: "red", matched: true, iterations: 22},
		{kind: "property", name: "--x", value: "red", errText: ErrCustomProperty.Error()},
		{kind: "property", name: "color", value: "var(--x)", errText: ErrVarNotSupported.Error()},
		{kind: "property", name: "color", value: "rgb(VAR(--x))", errText: ErrVarNotSupported.Error()},

		// at-rules: descriptors take no CSS-wide keywords
		{kind: "descriptor", parent: "font-face", name: "font-display", value: "inherit", iterations: 3},
		{kind: "descriptor", parent: "font-face", name: "unicode-range", value: "U+0-7F, U+0400-04FF", matched: true, iterations: 23},
		{kind: "descriptor", parent: "property", name: "syntax", value: "'<length>'", matched: true, iterations: 5},
		{kind: "descriptor", parent: "property", name: "inherits", value: "false", matched: true, iterations: 4},
		{kind: "descriptor", parent: "counter-style", name: "system", value: "fixed 3", matched: true, iterations: 14},
		{kind: "descriptor", parent: "page", name: "size", value: "A4 landscape", matched: true, iterations: 19},
		{kind: "prelude", name: "media", value: "screen and (min-width: 100px)", matched: true, iterations: 562},
		{kind: "prelude", name: "layer", value: "", matched: true, iterations: 12},
		{kind: "prelude", name: "page", value: "", matched: true, iterations: 5},
		{kind: "prelude", name: "font-face", value: "x", errText: "At-rule `@font-face` should not contain a prelude"},
		{kind: "prelude", name: "supports", value: "", errText: "At-rule `@supports` should contain a prelude"},
		{kind: "prelude", name: "supports", value: "(display: grid) and (not (display: inline-grid))", matched: true, iterations: 249},
		{kind: "prelude", name: "import", value: "url(a.css) layer(x) supports(display: grid) screen", matched: true, iterations: 550},
		{kind: "prelude", name: "property", value: "--x", matched: true, iterations: 5},
		{kind: "prelude", name: "property", value: "x", iterations: 5},
		// css-contain-3 excludes none from <container-name>, and
		// css-counter-styles none and decimal from @counter-style names;
		// mdn-data's syntaxes do not
		{kind: "prelude", name: "container", value: "sidebar (min-width: 400px)", matched: true, iterations: 114},
		{kind: "prelude", name: "container", value: "none (min-width: 400px)", matched: true, iterations: 114},
		{kind: "prelude", name: "counter-style", value: "none", matched: true, iterations: 9},
		{kind: "prelude", name: "counter-style", value: "decimal", matched: true, iterations: 9},
		{kind: "prelude", name: "counter-style", value: "inherit", iterations: 9},
	})
}

// TestLexerIterationLimit checks that matching stops after 15000
// iterations with upstream's reason, for a syntax whose alternatives
// backtrack exponentially.
func TestLexerIterationLimit(t *testing.T) {
	const syntax = "[a | a a]* b"

	runLexerEdgeCases(t, DefaultLexer(), []lexerEdgeCase{
		{kind: "syntax", name: syntax, value: "a a a a a a a a a a a a a c", iterations: 14037, mismatchOffset: 26, errText: "Mismatch"},
		{kind: "syntax", name: syntax, value: "a a a a a a a a a a a a a a c", iterations: 15000, mismatchOffset: 28, errText: exitReasonIterationLimit},
	})
}
