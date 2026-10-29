package csstree

import (
	"errors"
	"testing"
)

// specMatchCase is a value matched against a property, a type or a
// definition syntax, with whether the spec says it matches.
type specMatchCase struct {
	kind  string // "property", "type" or "syntax"
	name  string
	value string
	want  bool
}

func runSpecMatchCases(t *testing.T, cases []specMatchCase) {
	t.Helper()

	lexer := DefaultLexer()
	for _, c := range cases {
		var (
			result *MatchResult
			call   string
		)
		switch c.kind {
		case "property":
			result, call = lexer.MatchPropertyString(c.name, c.value), "MatchPropertyString"
		case "type":
			result, call = lexer.MatchTypeString(c.name, c.value), "MatchTypeString"
		case "syntax":
			result, call = lexer.MatchString(c.name, c.value), "MatchString"
		default:
			t.Fatalf("unknown kind %q", c.kind)
		}

		if got := result.Matched != nil; got != c.want {
			t.Errorf("%s(%q, %q) = %v, want %v (error: %v)", call, c.name, c.value, got, c.want, result.Error)
		}
	}
}

// TestSpecEscapedKeywords checks that keywords, units and function names
// are compared by the value of the identifier with its escapes decoded,
// ASCII case-insensitively (CSS Syntax 3 §4.3.11 consume a name, §8
// "Tokens match if their value is a match"; CSS Values 4 §2.1, §4).
func TestSpecEscapedKeywords(t *testing.T) {
	runSpecMatchCases(t, []specMatchCase{
		// keywords (Keyword and Enum)
		// \ed is the hex escape of U+00ED (í), so r\ed is rí, not red
		{"property", "color", `r\ed`, false},
		{"property", "color", `r\65 d`, true},
		{"property", "color", `r\45 D`, true},
		{"property", "color", `\72 ed`, true},
		{"property", "color", `\52 ED`, true},
		{"property", "color", `re\64`, true},
		{"property", "color", `\000072 ed`, true},
		{"property", "display", `\62lock`, true},
		{"property", "display", `\62 lock`, true},
		{"property", "display", `\62 lock\9`, true},
		{"syntax", "a", `\61`, true},
		{"syntax", "a", `\41`, true},
		{"syntax", "a | b", `\62`, true},
		{"syntax", "a", `\62`, false},
		{"property", "color", `'red'`, false},
		{"property", "color", `r\65 d x`, false},
		// the \0 and \9 hacks are still dropped
		{"property", "color", `red\9`, true},
		{"property", "color", `red\0`, true},
		// CSS-wide keywords
		{"property", "color", `\69nherit`, true},
		{"property", "color", `\49 NITIAL`, true},
		// ASCII case-insensitive only: U+212A KELVIN SIGN is not k
		{"property", "display", "blocK", false},
		{"property", "display", `bloc\212A`, false},
		{"syntax", "k", "K", false},
		// units
		{"property", "width", `1p\x`, true},
		{"property", "width", `1\70 x`, true},
		{"property", "width", `1P\58`, true},
		{"property", "width", `1p\x\9`, true},
		{"property", "width", `1px\9`, true},
		{"property", "width", `1p\y`, false},
		{"type", "frequency", "1Khz", false},
		{"type", "frequency", `1\212A hz`, false},
		{"type", "frequency", `1\6B hz`, true},
		{"syntax", "<attr-unit>", `p\x`, true},
		{"syntax", "<attr-unit>", `\25`, false}, // an ident %, not the delim %
		{"syntax", "<attr-unit>", "Khz", false},
		// function names
		{"property", "color", `\72 gb(0 0 0)`, true},
		{"property", "color", `RG\42 (0 0 0)`, true},
		{"property", "width", `c\61lc(1px)`, true},
		{"property", "width", `\63 alc(1px)`, true},
		{"syntax", "fn( <number> )", `f\6e(1)`, true},
		{"syntax", "fn( <number> )", `f\6f(1)`, false},
		// at-keywords
		{"syntax", "@foo", `@f\6fo`, true},
		{"syntax", "@foo", `@f\6f`, false},
		// <custom-ident> excludes the CSS-wide keywords and default after
		// decoding
		{"type", "custom-ident", `\69nherit`, false},
		{"type", "custom-ident", `\64 efault`, false},
		{"type", "custom-ident", `\44 EFAULT`, false},
		{"type", "custom-ident", `unse\74`, false},
		{"type", "custom-ident", `\66oo`, true},
		{"type", "custom-ident", `inherit\9`, true},
		// <dashed-ident> and <custom-property-name> check the decoded value
		{"type", "dashed-ident", `\2d-foo`, true},
		{"type", "dashed-ident", `-\2d foo`, true},
		{"type", "dashed-ident", `\2d foo`, false},
		{"type", "custom-property-name", `\2d-foo`, true},
		{"type", "custom-property-name", `-\2d`, false},
		{"type", "custom-property-name", `\2d\2d`, false},
	})
}

// TestSpecEscapedVar checks that var() is found by its decoded name.
func TestSpecEscapedVar(t *testing.T) {
	lexer := DefaultLexer()
	for _, value := range []string{`v\61r(--x)`, `\76 AR(--x)`, `rgb(V\41R(--x))`} {
		result := lexer.MatchPropertyString("color", value)
		if !errors.Is(result.Error, ErrVarNotSupported) {
			t.Errorf("MatchPropertyString(%q, %q) error = %v, want %v", "color", value, result.Error, ErrVarNotSupported)
		}
	}
}

// TestSpecNumberTokens checks that <number> is a <number-token> and
// <integer> a <number-token> of digits (CSS Values 4 §5.2, §5.3): delims
// and idents whose text reads as a number are not numbers.
func TestSpecNumberTokens(t *testing.T) {
	runSpecMatchCases(t, []specMatchCase{
		{"type", "number", "+", false},
		{"type", "number", "-", false},
		{"type", "number", "e5", false},
		{"type", "number", "-E3", false},
		{"type", "number", "e-5", false},
		{"type", "number", "+e5", false},
		{"property", "opacity", "e5", false},
		{"property", "opacity", "+", false},
		{"property", "line-height", "e5", false},
		{"syntax", "<number [0,∞]>", "e5", false},
		{"type", "number", "1", true},
		{"type", "number", "+.5", true},
		{"type", "number", "-1e-3", true},
		{"type", "number", "1e309", true},
		{"property", "opacity", ".5", true},
		{"property", "line-height", "1.5", true},
		// the IE postfix hack is still taken
		{"type", "number", `1\9`, true},
		{"type", "number", `1\0`, true},
		{"type", "number", `1px`, false},
		{"type", "number", `1%`, false},
		// an integer is written with digits only
		{"type", "integer", "1", true},
		{"type", "integer", "-1", true},
		{"type", "integer", "+0", true},
		{"type", "integer", "1.0", false},
		{"type", "integer", "1e3", false},
		{"type", "integer", "1E0", false},
		{"type", "integer", "e5", false},
		{"type", "integer", "+", false},
		{"property", "z-index", "1e3", false},
		{"property", "z-index", "1.0", false},
		{"property", "z-index", "10", true},
	})
}

// TestSpecRangesWithUnits checks the bracketed range notation of CSS
// Values 4 §5.1 for bounds with units: values of a unit that converts to
// the unit of the bound are compared in a canonical unit, and relative
// units are compared only by sign.
func TestSpecRangesWithUnits(t *testing.T) {
	runSpecMatchCases(t, []specMatchCase{
		{"syntax", "<angle [0deg,360deg]>", "0deg", true},
		{"syntax", "<angle [0deg,360deg]>", "360deg", true},
		{"syntax", "<angle [0deg,360deg]>", "360DEG", true},
		{"syntax", "<angle [0deg,360deg]>", "400deg", false},
		{"syntax", "<angle [0deg,360deg]>", "-1deg", false},
		{"syntax", "<angle [0deg,360deg]>", "-0deg", true},
		{"syntax", "<angle [0deg,360deg]>", "1turn", true},
		{"syntax", "<angle [0deg,360deg]>", "1.01turn", false},
		{"syntax", "<angle [0deg,360deg]>", "400grad", true},
		{"syntax", "<angle [0deg,360deg]>", "401grad", false},
		{"syntax", "<angle [0deg,360deg]>", "6.28rad", true},
		{"syntax", "<angle [0deg,360deg]>", "6.3rad", false},
		{"syntax", "<angle [0deg,360deg]>", `400d\65 g`, false},
		{"syntax", "<time [0s,∞]>", "0s", true},
		{"syntax", "<time [0s,∞]>", "1ms", true},
		{"syntax", "<time [0s,∞]>", "-1s", false},
		{"syntax", "<time [0s,∞]>", "-1ms", false},
		{"syntax", "<time [0s,∞]>", "1e309s", true},
		{"syntax", "<time [100ms,1s]>", "0.1s", true},
		{"syntax", "<time [100ms,1s]>", "99ms", false},
		{"syntax", "<time [100ms,1s]>", "1000ms", true},
		{"syntax", "<time [100ms,1s]>", "1001ms", false},
		{"syntax", "<time [100ms,1s]>", "1s", true},
		{"property", "animation-duration", "-1s", false},
		{"property", "animation-duration", "1s, -1ms", false},
		{"property", "animation-duration", "1s, 0ms", true},
		{"syntax", "<length [0,1in]>", "96px", true},
		{"syntax", "<length [0,1in]>", "97px", false},
		{"syntax", "<length [0,1in]>", "2.54cm", true},
		{"syntax", "<length [0,1in]>", "2.6cm", false},
		{"syntax", "<length [0,1in]>", "25.4mm", true},
		{"syntax", "<length [0,1in]>", "101.6q", true},
		{"syntax", "<length [0,1in]>", "102Q", false},
		{"syntax", "<length [0,1in]>", "72pt", true},
		{"syntax", "<length [0,1in]>", "73pt", false},
		{"syntax", "<length [0,1in]>", "6pc", true},
		{"syntax", "<length [0,1in]>", "7pc", false},
		{"syntax", "<length [0,1in]>", "0", true},
		// relative lengths are not comparable at parse time, except by
		// sign: every unit is a non-negative multiple of the canonical one
		{"syntax", "<length [0,1in]>", "100em", true},
		{"syntax", "<length [0,1in]>", "100vw", true},
		{"syntax", "<length [0,1in]>", "-1em", false},
		{"syntax", "<length [1px,∞]>", "1px", true},
		{"syntax", "<length [1px,∞]>", "0.5px", false},
		{"syntax", "<length [1px,∞]>", "0", false},
		{"syntax", "<length [1px,∞]>", "0em", false},
		{"syntax", "<length [1px,∞]>", "-1em", false},
		{"syntax", "<length [1px,∞]>", "0.01em", true},
		{"syntax", "<length [1em,2em]>", "1.5em", true},
		{"syntax", "<length [1em,2em]>", "3em", false},
		{"syntax", "<length [1em,2em]>", "3px", true},
		{"syntax", "<length [0,∞]>", "-1px", false},
		{"syntax", "<length [0,∞]>", "-1vw", false},
		{"syntax", "<length-percentage [0,∞]>", "-1%", false},
		{"syntax", "<resolution [0dpi,∞]>", "-1x", false},
		{"syntax", "<resolution [0dpi,∞]>", "0x", true},
		{"syntax", "<resolution [1dppx,192dpi]>", "1x", true},
		{"syntax", "<resolution [1dppx,192dpi]>", "96dpi", true},
		{"syntax", "<resolution [1dppx,192dpi]>", "95dpi", false},
		{"syntax", "<resolution [1dppx,192dpi]>", "37.8dpcm", true},
		{"syntax", "<resolution [1dppx,192dpi]>", "37.7dpcm", false},
		{"syntax", "<resolution [1dppx,192dpi]>", "2dppx", true},
		{"syntax", "<resolution [1dppx,192dpi]>", "2.1dppx", false},
		{"syntax", "<frequency [0hz,1khz]>", "1000Hz", true},
		{"syntax", "<frequency [0hz,1khz]>", "1001hz", false},
		{"syntax", "<frequency [0hz,1khz]>", "-1khz", false},
		{"syntax", "<flex [0,∞]>", "-1fr", false},
		{"syntax", "<flex [1fr,∞]>", "0.5fr", false},
		{"syntax", "<percentage [0,100]>", "101%", false},
		{"syntax", "<dimension [0px,1in]>", "97px", false},
		{"syntax", "<dimension [0px,1in]>", "1foo", true},
		{"syntax", "<dimension [0px,1in]>", "-1foo", false},
		// the IE postfix hack is not part of the unit
		{"syntax", "<time [0s,∞]>", `-1s\9`, false},
		{"syntax", "<time [0s,∞]>", `1s\9`, true},
	})
}

// TestSpecURange checks <urange> against CSS Syntax 3 §7.1 (Candidate
// Recommendation Draft, 24 December 2021): the first token is an ident u,
// the representations of the other tokens are interpreted as a range, and
// the range is invalid when its end is above U+10FFFF or below its start.
func TestSpecURange(t *testing.T) {
	runSpecMatchCases(t, []specMatchCase{
		{"type", "urange", "U+0-10FFFF", true},
		{"type", "urange", "U+10FFFF", true},
		{"type", "urange", "U+110000", false},
		{"type", "urange", "U+0-110000", false},
		{"type", "urange", "U+10FFF?", true},
		{"type", "urange", "U+11????", false},
		{"type", "urange", "U+??????", false},
		{"type", "urange", "U+0?????", true},
		{"type", "urange", "U+FF-00", false},
		{"type", "urange", "U+FF-FF", true},
		{"type", "urange", "U+1-zz", false},
		{"type", "urange", "u+a-", false},
		{"type", "urange", "U+1e-3", false},
		{"type", "urange", "U+1e-1e", true},
		{"type", "urange", "U+1e3", true},
		{"type", "urange", "u+12e-130", true},
		{"type", "urange", "ua+1", false},
		{"type", "urange", "Ua+1", false},
		{"type", "urange", "UU+1", false},
		{"type", "urange", `\75+1`, true},
		{"type", "urange", `\55 +1`, true},
		{"type", "urange", "U+1?-2", false},
		{"type", "urange", "u+?", true},
		{"type", "urange", "U+0-7F", true},
		{"type", "urange", "u+0025-00FF", true},
		{"type", "urange", "U+4??", true},
		{"syntax", "<urange>#", "U+0-7F, U+110000", false},
		{"property", "unicode-range", "U+0-7F", false}, // a descriptor, not a property
	})

	lexer := DefaultLexer()
	for _, c := range []struct {
		value string
		want  bool
	}{
		{"U+0-7F, U+0400-04FF", true},
		{"U+0-7F, U+FF-00", false},
		{"U+110000", false},
	} {
		result := lexer.MatchAtruleDescriptorString("font-face", "unicode-range", c.value)
		if got := result.Matched != nil; got != c.want {
			t.Errorf("MatchAtruleDescriptorString(%q, %q, %q) = %v, want %v (error: %v)", "font-face", "unicode-range", c.value, got, c.want, result.Error)
		}
	}
}

// TestSpecAnPlusBOddEven checks where odd and even of the An+B grammar
// (CSS Syntax 3 §6.2) match. <an+b>, mdn-data's syntax of the spec's
// production, takes them, in any ASCII case and with escapes. The generic
// <an-plus-b> is the production of the AnPlusB node, which holds A and B
// only (the parser keeps odd and even in an Identifier of the Nth node),
// so it leaves odd and even to the syntaxes using it, as upstream does.
func TestSpecAnPlusBOddEven(t *testing.T) {
	runSpecMatchCases(t, []specMatchCase{
		{"type", "an+b", "odd", true},
		{"type", "an+b", "EVEN", true},
		{"type", "an+b", `\6f dd`, true},
		{"type", "an+b", "2n+1", true},
		{"type", "an-plus-b", "2n+1", true},
		{"type", "an-plus-b", "odd", false},
		{"type", "an-plus-b", "even", false},
		{"syntax", "<an-plus-b> | odd | even", "odd", true},
		{"syntax", "<an-plus-b> | odd | even", "Even", true},
	})
}
