package csstree

import (
	"slices"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// Tests of CSS Values and Units Level 4 §10 math functions, where this port
// type checks calc() and friends instead of consuming any balanced
// function as upstream csstree does.
// https://drafts.csswg.org/css-values-4/#math

type mathMatchCase struct {
	name  string // property name, or type name for MatchTypeString
	value string
	want  bool
}

func TestSpecMathMatchProperty(t *testing.T) {
	cases := []mathMatchCase{
		// §10.1 calc() takes a <calc-sum>; width is a <length-percentage>
		{"width", "calc(1px + 2px)", true},
		{"width", "calc(100% - 2em)", true},
		{"width", "calc(100% / 3 - 2 * 1em - 2 * 1px)", true},
		{"width", "calc((1px + 2px) * 3)", true},
		{"width", "calc(calc(1px + 2px) * 3)", true},
		{"width", "calc( 1px )", true},
		{"width", "calc(1px*2)", true},
		{"width", "calc(1px/2)", true},
		{"width", "calc(1px /* note */ + 2px)", true},
		{"width", "calc(10%)", true},
		{"width", "calc(1px * (2 + 3))", true},
		{"width", "calc((1px + 2px) / (1 + 1))", true},
		{"width", "calc(1px / (2px / 1px))", true},
		{"width", "calc(1px / 1px * 1px)", true},
		{"width", "calc(10px * 10px / 1px)", true},
		{"width", "calc(1px / 0)", true},
		{"width", "calc(min(1px, max(2px, 3%)) + clamp(1px, 2px, 3px))", true},
		// a function name is ASCII case-insensitive
		{"width", "CALC(1px)", true},
		{"width", "Calc(1px + 1%)", true},
		{"width", "-webkit-calc(1px + 2%)", true},
		{"width", "-moz-calc(1px + 2%)", true},
		// and may be escaped (CSS Syntax 3 §4.3.7), as may keywords and
		// units
		{"width", `c\61lc(1px)`, true},
		{"width", `c\61lc(red)`, false},
		{"width", `calc(1px * p\69)`, true},
		{"width", `calc(1p\78)`, true},
		{"width", `clamp(n\6fne, 1px, none)`, true},
		// §10.2 comparison functions
		{"width", "min(10px, 5%)", true},
		{"width", "max(10px, 5%, 1em)", true},
		{"width", "clamp(10px, 5%, 100px)", true},
		{"width", "clamp(none, 5%, 100px)", true},
		{"width", "clamp(10px, 5%, NONE)", true},
		{"width", "clamp(none, 5%, none)", true},
		// §10.3 stepped value functions
		{"width", "round(10.5px, 1px)", true},
		{"width", "round(up, 10.5px, 1px)", true},
		{"width", "round(TO-ZERO, 10.5px, 1px)", true},
		{"width", "round(line-width, 1.5px)", true},
		{"width", "round(line-width, 1.5px, 1px)", true},
		{"width", "mod(18px, 5px)", true},
		{"width", "rem(18px, 5%)", true},
		// §10.4 and §10.5 functions of numbers used as factors
		{"width", "calc(1px * sin(30deg))", true},
		{"width", "calc(1px * cos(1))", true},
		{"width", "calc(1px * tan(0.5turn))", true},
		{"width", "calc(1px * pow(2, 3))", true},
		{"width", "calc(1px * sqrt(4))", true},
		{"width", "calc(1px * exp(1) / log(2))", true},
		{"width", "calc(1px * log(8, 2))", true},
		{"width", "calc(1px * pow(10% / 1px, 2))", true},
		{"width", "hypot(30px, 40px)", true},
		{"width", "hypot(30px, 4%)", true},
		// §10.6 sign-related functions
		{"width", "abs(-1px)", true},
		{"width", "calc(1px * sign(-5%))", true},
		// §10.7 numeric constants are ASCII case-insensitive <number>s
		{"width", "calc(1px * e)", true},
		{"width", "calc(1px * PI)", true},
		{"width", "calc(infinity * 1px)", true},
		{"width", "calc(-INFINITY * 1px)", true},
		{"width", "calc(NaN * 1px)", true},
		{"width", "calc(nan * 1px)", true},

		// mismatches: the arguments are not a <calc-sum>
		{"width", "calc(red)", false},
		{"width", "calc()", false},
		{"width", "min()", false},
		{"width", "calc(1px, 2px)", false},
		{"width", "calc(none)", false},
		{"width", "calc(1px +)", false},
		{"width", "calc(+ 1px)", false},
		{"width", "calc(1px *)", false},
		{"width", "calc(1px / )", false},
		{"width", "calc(1px ** 2)", false},
		{"width", "calc(1px 2px)", false},
		{"width", "calc(-(1px))", false},
		{"width", "calc(-pi * 1px)", false},
		{"width", "calc(\"1px\")", false},
		{"width", "calc([1px])", false},
		{"width", "calc(1px + foo(2px))", false},
		// §10.8 + and - need whitespace on both sides
		{"width", "calc(1px+2px)", false},
		{"width", "calc(1px +2px)", false},
		{"width", "calc(1px- 2px)", false},
		{"width", "calc(1px/**/+ 2px)", false},
		// §10.2, §10.3 argument counts and keywords
		{"width", "clamp(1px, 2px)", false},
		{"width", "clamp(1px, none, 2px)", false},
		{"width", "clamp(1px, 2px, 3px, 4px)", false},
		{"width", "round(1.5px)", false},
		{"width", "round(nearest 1px, 1px)", false},
		{"width", "round(sideways, 1px, 1px)", false},
		{"width", "round(up, 1px, 1px, 1px)", false},
		{"width", "round(line-width, 1.5)", false},
		{"width", "mod(1px)", false},
		{"width", "hypot()", false},
		// §10.9 type checking
		{"width", "calc(1px + 2s)", false},
		{"width", "calc(1px - 1px + 1s)", false},
		{"width", "calc(0 * 5px + 10s)", false},
		{"width", "calc(1px * 1px)", false},
		{"width", "calc(1 / 1px)", false},
		{"width", "calc(0)", false},
		{"width", "calc(0 + 5px)", false},
		{"width", "calc(5)", false},
		{"width", "calc(pi)", false},
		{"width", "calc(1px + (2s))", false},
		{"width", "calc(10% * 10%)", false},
		{"width", "min(1px, 1s)", false},
		{"width", "calc(sin(1px) * 1px)", false},
		{"width", "calc(sqrt(4px))", false},
		{"width", "calc(pow(2px, 2))", false},
		{"width", "asin(0.5)", false},
		{"width", "atan2(1px, 2px)", false},
		{"width", "sign(1px)", false},
		// <flex> values are not allowed in math functions (CSS Grid 2 §7.2.4)
		{"width", "calc(1fr)", false},

		// opacity is <number> | <percentage>: percentages resolve against a
		// <number> and do not combine with numbers
		{"opacity", "calc(0.5)", true},
		{"opacity", "calc(50%)", true},
		{"opacity", "calc(100% / 3)", true},
		{"opacity", "calc(50% / 1%)", true},
		{"opacity", "calc(1 * 1%)", true},
		{"opacity", "sin(1)", true},
		{"opacity", "sign(-1)", true},
		{"opacity", "calc(1px)", false},
		{"opacity", "calc(.25 + 25%)", false},
		{"opacity", "hypot(1px)", false},

		// line-height is normal | <number> | <length> | <percentage>
		{"line-height", "calc(1 + 1)", true},
		{"line-height", "calc(1em + 10%)", true},
		{"line-height", "calc(1 + 1em)", false},
		{"line-height", "calc(1 + 10%)", false},

		// rotate takes an <angle>
		{"rotate", "calc(45deg + 0.5turn)", true},
		{"rotate", "atan2(1, 1)", true},
		{"rotate", "atan2(1px, 1%)", false},
		{"rotate", "asin(0.5)", true},
		{"rotate", "calc(1rad * pi)", true},
		{"rotate", "x calc(90deg)", true},
		{"rotate", "calc(45deg + 10%)", false},
		{"rotate", "asin(1deg)", false},
		{"rotate", "calc(45)", false},
		{"rotate", "sin(45deg)", false},

		// transition-duration takes a <time>
		{"transition-duration", "calc(1s + 100ms)", true},
		{"transition-duration", "calc(1s * 2)", true},
		{"transition-duration", "min(1s, 500ms), 2s", true},
		{"transition-duration", "calc(1s + 1px)", false},
		{"transition-duration", "calc(2 / 1s)", false},
		{"transition-duration", "calc(10%)", false},

		// flex-grow takes a <number>; §10.12 a math function out of the
		// range is clamped, not rejected
		{"flex-grow", "calc(1 + 1)", true},
		{"flex-grow", "calc(-1)", true},
		{"flex-grow", "calc(1px)", false},
		{"flex-grow", "calc(1%)", false},

		// §10.9 a math function resolving to <number> can be used where an
		// <integer> is accepted
		{"z-index", "calc(1 + 1)", true},
		{"z-index", "calc(1.5)", true},
		{"z-index", "round(1.5)", true},
		{"z-index", "round(down, 1.5, 1)", true},
		{"z-index", "calc(1px)", false},
		{"z-index", "calc(1%)", false},
		{"counter-increment", "foo calc(1 + 1)", true},
		{"tab-size", "calc(4)", true},
		{"tab-size", "calc(4px)", true},
		{"tab-size", "calc(4%)", false},

		// font-weight takes a <number [1,1000]>
		{"font-weight", "calc(400 + 100)", true},
		{"font-weight", "calc(2000)", true},
		{"font-weight", "calc(400px)", false},
		// font-stretch takes a <percentage>
		{"font-stretch", "calc(50% + 10%)", true},
		{"font-stretch", "calc(1)", false},
		{"font-stretch", "calc(1px + 10%)", false},

		// margin and padding take <length-percentage>s
		{"margin", "calc(-10% - 1px)", true},
		{"margin", "calc(1px) calc(2%)", true},
		{"margin", "calc(1px + 1s)", false},
		{"padding", "calc(1em + 10%)", true},
		// border-width takes a <length>, not a percentage
		{"border-width", "calc(2px)", true},
		{"border-width", "calc(10%)", false},
		{"border-width", "calc(1px + 10%)", false},
		{"font-size", "clamp(1rem, 2.5vw, 2rem)", true},
		{"font-size", "calc(1em + 1vw)", true},
		{"background-position", "calc(100% - 10px) calc(100% - 10px)", true},

		// grid tracks: a <flex> is not allowed in a math function
		{"grid-template-columns", "calc(100px)", true},
		{"grid-template-columns", "repeat(2, calc(10% + 1px))", true},
		{"grid-template-columns", "minmax(calc(10px), 1fr)", true},
		{"grid-template-columns", "calc(1fr)", false},
		{"grid-template-columns", "calc(1fr + 1px)", false},

		// aspect-ratio takes a <ratio> of <number>s
		{"aspect-ratio", "calc(16 / 9)", true},
		{"aspect-ratio", "16 / calc(9)", true},
		{"aspect-ratio", "calc(16px / 9)", false},

		// math functions in other functions
		{"transform", "translate(calc(50% + 10px), 0)", true},
		{"transform", "rotate(calc(1turn / 4))", true},
		{"transform", "scale(calc(1 + 0.5))", true},
		{"transform", "scale(calc(1px))", false},
		{"color", "rgb(calc(255 / 2) 0 0)", true},
		{"color", "rgb(calc(1px) 0 0)", false},
		{"color", "hsl(calc(120deg + 10deg) 50% 50%)", true},

		// arbitrary substitution functions are not type checked, as
		// upstream; anchor-size() and anchor() are <length>s
		{"width", "calc(1px + env(safe-area-inset-top))", true},
		{"width", "calc(attr(data-width px))", true},
		{"width", "calc(1px + anchor-size(width))", true},
		{"top", "calc(anchor(--a bottom) + 1em)", true},
		{"width", "calc(1s + anchor-size(width))", false},
		{"voice-volume", "calc(1db + env(x))", true},
		{"grid-template-columns", "calc(1fr * env(x))", true},
		{"voice-volume", "calc(1db)", false},
	}

	lexer := DefaultLexer()
	for _, tc := range cases {
		result := lexer.MatchPropertyString(tc.name, tc.value)
		if got := result.Matched != nil; got != tc.want {
			t.Errorf("MatchPropertyString(%q, %q) matched = %t, want %t (error: %v)", tc.name, tc.value, got, tc.want, result.Error)
		}
	}

	// a parsed value is matched as its text is
	for _, tc := range cases {
		node, err := Parse(tc.value, ParseOptions{Context: "value"})
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", tc.value, err)
		}
		result := lexer.MatchProperty(tc.name, node)
		got := result.Matched != nil
		want := tc.want
		// the parser drops the whitespace of "1px +2px", giving the
		// tokens of "1px +2px" without whitespace, still not a sum
		if got != want {
			t.Errorf("MatchProperty(%q, Parse(%q)) matched = %t, want %t (error: %v)", tc.name, tc.value, got, want, result.Error)
		}
	}
}

func TestSpecMathMatchType(t *testing.T) {
	cases := []mathMatchCase{
		{"length", "calc(1px)", true},
		{"length", "calc(1px + 1em)", true},
		{"length", "calc(10%)", false},
		{"length", "calc(1px + 1%)", false},
		{"length", "asin(0.5)", false},
		{"length-percentage", "calc(10%)", true},
		{"length-percentage", "calc(1px + 1%)", true},
		{"percentage", "calc(10% + 1%)", true},
		{"percentage", "calc(10% + 1px)", false},
		{"number", "calc(1)", true},
		{"number", "calc(1px / 1px)", true},
		{"number", "calc(1% / 1%)", false},
		{"number", "round(down, 1px, 2px)", false},
		{"number", "mod(1px, 2px)", false},
		{"number", "sign(-1px)", true},
		{"number-percentage", "calc(1% / 1%)", true},
		{"integer", "calc(1.5)", true},
		{"integer", "calc(1px)", false},
		{"angle", "calc(1deg + 1grad)", true},
		{"angle", "calc(1deg + 0)", false},
		{"angle-percentage", "calc(1deg + 10%)", true},
		{"time", "calc(1s - 1ms)", true},
		{"time-percentage", "calc(1s - 10%)", true},
		{"frequency", "calc(1hz + 1khz)", true},
		{"frequency", "calc(1 / 1s)", false},
		{"frequency-percentage", "calc(1khz * 10%)", false},
		{"resolution", "calc(1dppx * 2)", true},
		{"resolution", "calc(1x + 1dpi)", true},
		{"dimension", "calc(1px)", true},
		{"dimension", "calc(1deg * 2)", true},
		{"dimension", "calc(1)", false},
		{"dimension", "calc(1%)", false},
		{"dimension", "calc(1foo)", false},
		// <flex>, <decibel> and <semitones> are not math function types
		{"flex", "calc(1fr)", false},
		{"flex", "calc(2 * 1fr)", false},
		{"decibel", "calc(2 * 1db)", false},
		{"semitones", "calc(2 * 1st)", false},
		// <zero> takes a literal 0 only
		{"zero", "calc(0)", false},
		// non-numeric types still do not take math functions
		{"custom-ident", "calc(1)", false},
		{"any-value", "calc(red)", true},
		// an unclosed function at the end of the value is closed, as in
		// CSS Syntax 3 §5.4.9
		{"length", "calc(1px + (2px", true},
		{"length", "calc(1px +", false},
	}

	lexer := DefaultLexer()
	for _, tc := range cases {
		result := lexer.MatchTypeString(tc.name, tc.value)
		if got := result.Matched != nil; got != tc.want {
			t.Errorf("MatchTypeString(%q, %q) matched = %t, want %t (error: %v)", tc.name, tc.value, got, tc.want, result.Error)
		}
	}
}

func TestSpecMathMatchSyntax(t *testing.T) {
	cases := []struct {
		syntax string
		value  string
		want   bool
	}{
		// a <percentage> alternative of a numeric type lets percentages
		// mix with that type, as in <length-percentage>
		{"<length> | <percentage>", "calc(1px + 10%)", true},
		{"<percentage> | <length>", "calc(1px + 10%)", true},
		{"<length> | <percentage> | auto", "calc(1px + 10%)", true},
		{"[ <length> | <percentage> ]{2}", "calc(1px + 10%) calc(10% - 1em)", true},
		{"<length> <percentage>", "calc(1px + 10%) 1%", false},
		{"<length> || <percentage>", "calc(1px + 10%)", false},
		{"<length>", "calc(1px + 10%)", false},
		{"<time> | <percentage>", "calc(1s + 10%)", true},
		{"<angle> | <percentage>", "calc(1deg + 10%)", true},
		{"<frequency> | <percentage>", "calc(1hz + 10%)", true},
		{"<resolution> | <percentage>", "calc(1dppx + 10%)", false},
		// §10.12 ranges are not checked for math functions
		{"<length [0,∞]>", "calc(-1px)", true},
		{"<integer [1,∞]>", "calc(0)", true},
		{"<number [0,1]>", "calc(2)", true},
	}

	lexer := DefaultLexer()
	for _, tc := range cases {
		result := lexer.MatchString(tc.syntax, tc.value)
		if got := result.Matched != nil; got != tc.want {
			t.Errorf("MatchString(%q, %q) matched = %t, want %t (error: %v)", tc.syntax, tc.value, got, tc.want, result.Error)
		}
	}
}

// TestSpecMathTrace checks that a math function is matched as one token
// run by the type it resolves to.
func TestSpecMathTrace(t *testing.T) {
	lexer := DefaultLexer()
	node, err := Parse("calc(1px + 10%) 2px", ParseOptions{Context: "value"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	result := lexer.MatchProperty("margin", node)
	if result.Matched == nil {
		t.Fatalf("MatchProperty(margin) error = %v", result.Error)
	}
	fragments, err := lexer.FindValueFragments("margin", node, "Type", "length")
	if err != nil {
		t.Fatalf("FindValueFragments() error = %v", err)
	}
	if len(fragments) != 2 {
		t.Fatalf("FindValueFragments(margin, <length>) = %d fragments, want 2", len(fragments))
	}
}

// TestSpecMathIterations checks that type checking keeps the matcher from
// iterating more for nested math functions.
func TestSpecMathIterations(t *testing.T) {
	deep := strings.Repeat("calc(", 30) + "1px" + strings.Repeat(")", 30)
	cases := []struct {
		property string
		value    string
		want     bool
	}{
		{"width", deep, true},
		{"width", "calc(" + strings.Repeat("(", 200) + "1px" + strings.Repeat(")", 200) + ")", true},
		{"width", "calc(" + strings.Repeat("1px + ", 500) + "1px)", true},
		{"width", "min(" + strings.Repeat("calc(1px + 2%), ", 100) + "1px)", true},
		{"grid-template-columns", strings.Repeat("calc(10% + 1px) ", 20), true},
		{"transition-duration", strings.Repeat("calc(1s * 2), ", 50) + "1s", true},
		{"margin", "calc(1px + 1s) calc(1px + 1s) calc(1px + 1s) calc(1px + 1s)", false},
	}

	lexer := DefaultLexer()
	for _, tc := range cases {
		result := lexer.MatchPropertyString(tc.property, tc.value)
		if got := result.Matched != nil; got != tc.want {
			t.Errorf("MatchPropertyString(%q, %.40q...) matched = %t, want %t (error: %v)", tc.property, tc.value, got, tc.want, result.Error)
		}
		if result.Iterations >= iterationLimit {
			t.Errorf("MatchPropertyString(%q, %.40q...) iterations = %d, want below %d", tc.property, tc.value, result.Iterations, iterationLimit)
		}
	}
}

// TestSpecMathNestingLimit checks that a math function nested deeper than
// the supported depth is invalid rather than exhausting the stack (§10.8:
// a calculation with more nesting than supported is invalid).
func TestSpecMathNestingLimit(t *testing.T) {
	lexer := DefaultLexer()
	n := maxCalcDepth + 1
	for _, value := range []string{
		strings.Repeat("calc(", n) + "1px" + strings.Repeat(")", n),
		"calc(" + strings.Repeat("(", n) + "1px" + strings.Repeat(")", n) + ")",
	} {
		if result := lexer.MatchPropertyString("width", value); result.Matched != nil {
			t.Errorf("MatchPropertyString(width, %d levels of nesting) matched, want a mismatch", n)
		}
	}
}

// specMathFixtureInvalid lists the values of upstream's
// definition-syntax-match fixtures that are valid upstream but invalid by
// Values 4 §10.9, by fixture key.
var specMathFixtureInvalid = map[string][]string{
	// percentages are not allowed in a <length> that is not a
	// <length-percentage>
	"<length> || <size>": {"calc(1px + (2px / 3) + 10%) normal"},
	// <flex> is not a math function type (CSS Grid 2 §7.2.4), nor are the
	// CSS Speech <decibel> and <semitones>
	"<flex>":      {"calc(2 * 1fr)"},
	"<decibel>":   {"calc(2 * 1db)"},
	"<semitones>": {"calc(2 * 1st)"},
	"math-length": {
		// percentages are not allowed in <length>
		"calc(1px + 1%)", "CALC(1px + 1%)",
		"-moz-calc(1px + 1%)", "-MOZ-CALC(1px + 1%)",
		"-webkit-calc(1px + 1%)", "-WEBKIT-CALC(1px + 1%)",
		// these resolve to <angle>
		"asin(0.5)", "acos(0.5)", "atan(1)", "atan2(1px, 2px)",
	},
	// these resolve to <length>
	"math-number": {"round(down, 1px, 2px)", "mod(1px, 2px)", "rem(1px, 2px)"},
}

// specMathFixtureValues moves the values specMathFixtureInvalid lists for
// the fixture key from valid to invalid.
func specMathFixtureValues(key string, valid, invalid []string) ([]string, []string) {
	moved := specMathFixtureInvalid[key]
	if len(moved) == 0 {
		return valid, invalid
	}

	var stillValid []string
	invalid = slices.Clone(invalid)
	for _, value := range valid {
		if slices.Contains(moved, value) {
			invalid = append(invalid, value)
			continue
		}
		stillValid = append(stillValid, value)
	}
	return stillValid, invalid
}

// TestSpecMathFixtureOverrides checks that every value
// specMathFixtureInvalid moves is a valid value of its fixture, so that
// the list does not go stale.
func TestSpecMathFixtureOverrides(t *testing.T) {
	seen := make(map[string]map[string]bool)
	forEachDefinitionSyntaxMatchTest(t, func(_ *testing.T, key string, test definitionSyntaxMatchTest) {
		if _, ok := specMathFixtureInvalid[key]; !ok {
			return
		}
		if seen[key] == nil {
			seen[key] = make(map[string]bool)
		}
		for _, value := range test.Valid {
			seen[key][value] = true
		}
	})

	for key, values := range specMathFixtureInvalid {
		for _, value := range values {
			if !seen[key][value] {
				t.Errorf("specMathFixtureInvalid[%q] has %q, which is not a valid value of that fixture", key, value)
			}
		}
	}
}

// FuzzMathFunction matches arbitrary math functions against properties
// taking numeric types: it must never panic, a mismatch must come with an
// error, and nesting a valid math function in calc() keeps it valid.
func FuzzMathFunction(f *testing.F) {
	for _, seed := range []struct{ property, value string }{
		{"width", "calc(100% / 3 - 2 * 1em - 2 * 1px)"},
		{"width", "clamp(none, 5%, 100px)"},
		{"width", "round(line-width, 1.5px)"},
		{"width", "calc(1px * pow(10% / 1px, 2))"},
		{"width", "calc((1px + 2px) / (1 + 1))"},
		{"width", "calc(1px + env(x))"},
		{"opacity", "calc(50% / 1%)"},
		{"rotate", "atan2(1px, 1%)"},
		{"z-index", "round(down, 1.5, 1)"},
		{"line-height", "calc(1em + 10%)"},
		{"grid-template-columns", "repeat(2, calc(10% + 1px))"},
		{"transition-duration", "min(1s, 500ms), 2s"},
	} {
		f.Add(seed.property, seed.value)
	}

	f.Fuzz(func(t *testing.T, property, value string) {
		lexer := DefaultLexer()
		result := lexer.MatchPropertyString(property, value)
		if (result.Matched == nil) == (result.Error == nil) {
			t.Fatalf("MatchPropertyString(%q, %q) matched = %t, error = %v", property, value, result.Matched != nil, result.Error)
		}

		if result.Matched == nil || !isSingleMathFunction(value) || strings.Count(value, "(") >= maxCalcDepth/2 {
			return
		}
		nested := "calc(" + value + ")"
		if lexer.MatchPropertyString(property, nested).Matched == nil {
			t.Errorf("MatchPropertyString(%q, %q) matched, but not %q", property, value, nested)
		}
	})
}

// isSingleMathFunction reports whether value is one math function closed
// by its ) and nothing else, without substitution functions.
func isSingleMathFunction(value string) bool {
	tokens := stringToTokens(value)
	if len(tokens) == 0 || tokens[0].typ != tokenizer.Function || !isMathFunctionName(cssName(tokens[0].value)) {
		return false
	}
	if tokens[len(tokens)-1].typ != tokenizer.RightParenthesis {
		return false
	}
	for _, token := range tokens {
		// a function with substitution functions is matched as upstream
		if token.typ == tokenizer.Function && isSubstitutionFunctionName(cssName(token.value)) {
			return false
		}
	}
	n := consumeFunction(&tokens[0], func(offset int) *matchToken {
		if offset < len(tokens) {
			return &tokens[offset]
		}
		return nil
	})
	return n == len(tokens)
}
