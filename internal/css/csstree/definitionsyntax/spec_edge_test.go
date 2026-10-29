package definitionsyntax_test

// Edge cases from CSS Values and Units Level 4 §2 (Value Definition Syntax):
// combinator precedence, multipliers and their stacking, bracketed range
// notation, functional notation and errors. Every expected value is what
// upstream csstree 3.2.1 gives for the same source: the AST as JSON, the
// generated text (default and compact), or the error message and offset.
// Cases where the port follows the spec instead are in spec_fixes_test.go.

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
)

type specEdgeTest struct {
	source   string
	ast      string // JSON; empty when Parse fails
	generate string
	compact  string
	err      string
	offset   int
}

func runSpecEdgeTest(t *testing.T, tt specEdgeTest) {
	t.Helper()

	ast, err := definitionsyntax.Parse(tt.source)
	if tt.err != "" {
		var syntaxError *definitionsyntax.SyntaxError
		if !errors.As(err, &syntaxError) {
			t.Fatalf("Parse(%q) error = %v, want SyntaxError %q", tt.source, err, tt.err)
		}
		if syntaxError.RawMessage != tt.err || syntaxError.Offset != tt.offset {
			t.Errorf("Parse(%q) error = %q at %d, want %q at %d", tt.source, syntaxError.RawMessage, syntaxError.Offset, tt.err, tt.offset)
		}
		return
	}
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", tt.source, err)
	}

	data, err := json.Marshal(ast)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(tt.ast), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(%q) = %s, want %s", tt.source, data, tt.ast)
	}

	if got := definitionsyntax.Generate(ast, definitionsyntax.GenerateOptions{}); got != tt.generate {
		t.Errorf("Generate(Parse(%q)) = %q, want %q", tt.source, got, tt.generate)
	}
	if got := definitionsyntax.Generate(ast, definitionsyntax.GenerateOptions{Compact: true}); got != tt.compact {
		t.Errorf("Generate(Parse(%q), compact) = %q, want %q", tt.source, got, tt.compact)
	}
}

func TestParseSpecEdges(t *testing.T) {
	for _, tt := range []specEdgeTest{
		{"a{0,5}", `{"type":"Group","terms":[{"type":"Multiplier","comma":false,"min":0,"max":5,"term":{"type":"Keyword","name":"a"}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a{0,5}", "a{0,5}", "", 0},
		{"a{1}", `{"type":"Group","terms":[{"type":"Multiplier","comma":false,"min":1,"max":1,"term":{"type":"Keyword","name":"a"}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a", "a", "", 0},
		{"a{1,1}", `{"type":"Group","terms":[{"type":"Multiplier","comma":false,"min":1,"max":1,"term":{"type":"Keyword","name":"a"}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a", "a", "", 0},
		{"#{1,1}", "", "", "", "Unexpected input", 0},
		{"a#{1}", `{"type":"Group","terms":[{"type":"Multiplier","comma":true,"min":1,"max":1,"term":{"type":"Keyword","name":"a"}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a", "a", "", 0},
		{"a*#", "", "", "", "Unexpected input", 2},
		{"a*?", "", "", "", "Unexpected input", 2},
		{"a??", "", "", "", "Unexpected input", 2},
		{"a+?", "", "", "", "Unexpected input", 2},
		{"a?#", "", "", "", "Unexpected input", 2},
		{"a{2}#", "", "", "", "Unexpected input", 4},
		{"a{2}?", `{"type":"Group","terms":[{"type":"Multiplier","comma":false,"min":0,"max":1,"term":{"type":"Multiplier","comma":false,"min":2,"max":2,"term":{"type":"Keyword","name":"a"}}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a{2}?", "a{2}?", "", 0},
		{"a{1,2}?", `{"type":"Group","terms":[{"type":"Multiplier","comma":false,"min":0,"max":1,"term":{"type":"Multiplier","comma":false,"min":1,"max":2,"term":{"type":"Keyword","name":"a"}}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a{1,2}?", "a{1,2}?", "", 0},
		{"a#?", `{"type":"Group","terms":[{"type":"Multiplier","comma":true,"min":0,"max":0,"term":{"type":"Keyword","name":"a"}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a#?", "a#?", "", 0},
		{"a+#", `{"type":"Group","terms":[{"type":"Multiplier","comma":true,"min":1,"max":0,"term":{"type":"Multiplier","comma":false,"min":1,"max":0,"term":{"type":"Keyword","name":"a"}}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a+#", "a+#", "", 0},
		{"a+#?", `{"type":"Group","terms":[{"type":"Multiplier","comma":true,"min":0,"max":0,"term":{"type":"Multiplier","comma":false,"min":1,"max":0,"term":{"type":"Keyword","name":"a"}}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a+#?", "a+#?", "", 0},
		{"<a(>", "", "", "", "Expect `>`", 2},
		{"<a( )>", "", "", "", "Expect `>`", 2},
		{"<a()>", `{"type":"Group","terms":[{"type":"Type","name":"a()","opts":null}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<a()>", "<a()>", "", 0},
		{"a |", "", "", "", "Unexpected combinator", 0},
		{"a b |", "", "", "", "Unexpected combinator", 0},
		{"a  |  ", "", "", "", "Unexpected combinator", 2},
		{"a | b |", "", "", "", "Unexpected combinator", 0},
		{"{0}", "", "", "", "Unexpected input", 0},
		{"{9}", "", "", "", "Unexpected input", 0},
		{"{/}", `{"type":"Group","terms":[{"type":"Token","value":"{"},{"type":"Token","value":"/"},{"type":"Token","value":"}"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "{ / }", "{ / }", "", 0},
		{"{a", `{"type":"Group","terms":[{"type":"Token","value":"{"},{"type":"Keyword","name":"a"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "{ a", "{ a", "", 0},
		{"a {", `{"type":"Group","terms":[{"type":"Keyword","name":"a"},{"type":"Token","value":"{"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a {", "a {", "", 0},
		{"a\u0001b", `{"type":"Group","terms":[{"type":"Keyword","name":"a"},{"type":"Token","value":"\u0001"},{"type":"Keyword","name":"b"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a \u0001 b", "a \u0001 b", "", 0},
		{"A0 Z9 a-z", `{"type":"Group","terms":[{"type":"Keyword","name":"A0"},{"type":"Keyword","name":"Z9"},{"type":"Keyword","name":"a-z"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "A0 Z9 a-z", "A0 Z9 a-z", "", 0},
		{"a{\r\n1\f,\t2 }", `{"type":"Group","terms":[{"type":"Multiplier","comma":false,"min":1,"max":2,"term":{"type":"Keyword","name":"a"}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a{1,2}", "a{1,2}", "", 0},
		{"<a\n\r\f\t [0,∞]>", `{"type":"Group","terms":[{"type":"Type","name":"a","opts":{"type":"Range","min":0,"max":null}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<a [0,∞]>", "<a [0,∞]>", "", 0},
		{"a{1", "", "", "", "Expect `}`", 3},
		{"a{1,", "", "", "", "Expect a number", 4},
		{"a{", "", "", "", "Expect a number", 2},
		{"'abc", "", "", "", "Expect an apostrophe", 4},
		{"'", "", "", "", "Expect an apostrophe", 1},
		{"<a [0,99999999999999999999999]>", `{"type":"Group","terms":[{"type":"Type","name":"a","opts":{"type":"Range","min":0,"max":1e+23}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<a [0,1e+23]>", "<a [0,1e+23]>", "", 0},
		{"<a [0,123456789012345678901]>", `{"type":"Group","terms":[{"type":"Type","name":"a","opts":{"type":"Range","min":0,"max":123456789012345680000}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<a [0,123456789012345680000]>", "<a [0,123456789012345680000]>", "", 0},
		{"<a [0,99999999999999999999999px]>", `{"type":"Group","terms":[{"type":"Type","name":"a","opts":{"type":"Range","min":0,"max":"1e+23px"}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<a [0,1e+23px]>", "<a [0,1e+23px]>", "", 0},
		{"<a [-0,0]>", `{"type":"Group","terms":[{"type":"Type","name":"a","opts":{"type":"Range","min":0,"max":0}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<a [0,0]>", "<a [0,0]>", "", 0},
		{"<a [-0px,0]>", `{"type":"Group","terms":[{"type":"Type","name":"a","opts":{"type":"Range","min":"0px","max":0}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<a [0px,0]>", "<a [0px,0]>", "", 0},
		{"<a [-5,-1em]>", `{"type":"Group","terms":[{"type":"Type","name":"a","opts":{"type":"Range","min":-5,"max":"-1em"}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<a [-5,-1em]>", "<a [-5,-1em]>", "", 0},
		{"a\u0000", `{"type":"Group","terms":[{"type":"Keyword","name":"a"},{"type":"Token","value":"\u0000"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "a \u0000", "a \u0000", "", 0},
		{"<'a'>{2}", `{"type":"Group","terms":[{"type":"Multiplier","comma":false,"min":2,"max":2,"term":{"type":"Property","name":"a"}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<'a'>{2}", "<'a'>{2}", "", 0},
		{"@a", `{"type":"Group","terms":[{"type":"AtKeyword","name":"a"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "@a", "@a", "", 0},
		{"@", `{"type":"Group","terms":[{"type":"Token","value":"@"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "@", "@", "", 0},
		{"@ b", `{"type":"Group","terms":[{"type":"Token","value":"@"},{"type":"Keyword","name":"b"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "@ b", "@ b", "", 0},
		{"f(a b) c", `{"type":"Group","terms":[{"type":"Function","name":"f"},{"type":"Keyword","name":"a"},{"type":"Keyword","name":"b"},{"type":"Token","value":")"},{"type":"Keyword","name":"c"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "f( a b ) c", "f( a b ) c", "", 0},
		{"f(a | b)", `{"type":"Group","terms":[{"type":"Function","name":"f"},{"type":"Group","terms":[{"type":"Keyword","name":"a"},{"type":"Keyword","name":"b"}],"combinator":"|","disallowEmpty":false,"explicit":false},{"type":"Token","value":")"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "f( a | b )", "f( a|b )", "", 0},
		{"f() a", `{"type":"Group","terms":[{"type":"Function","name":"f"},{"type":"Token","value":")"},{"type":"Keyword","name":"a"}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "f( ) a", "f( ) a", "", 0},
		{"<boolean-expr[ <a> ]>?", `{"type":"Group","terms":[{"type":"Multiplier","comma":false,"min":0,"max":1,"term":{"type":"Boolean","term":{"type":"Type","name":"a","opts":null}}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<boolean-expr[<a>]>?", "<boolean-expr[<a>]>?", "", 0},
		{"a && b || c | d e", `{"type":"Group","terms":[{"type":"Group","terms":[{"type":"Group","terms":[{"type":"Keyword","name":"a"},{"type":"Keyword","name":"b"}],"combinator":"&&","disallowEmpty":false,"explicit":false},{"type":"Keyword","name":"c"}],"combinator":"||","disallowEmpty":false,"explicit":false},{"type":"Group","terms":[{"type":"Keyword","name":"d"},{"type":"Keyword","name":"e"}],"combinator":" ","disallowEmpty":false,"explicit":false}],"combinator":"|","disallowEmpty":false,"explicit":false}`, "a && b || c | d e", "a&&b||c|d e", "", 0},
		{"<length [-∞,0]>", `{"type":"Group","terms":[{"type":"Type","name":"length","opts":{"type":"Range","min":null,"max":0}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<length [-∞,0]>", "<length [-∞,0]>", "", 0},
		// a number too large for a float64 is -Infinity, which JSON writes as null
		{"<a [-" + strings.Repeat("9", 400) + ",1]>", `{"type":"Group","terms":[{"type":"Type","name":"a","opts":{"type":"Range","min":null,"max":1}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<a [-Infinity,1]>", "<a [-Infinity,1]>", "", 0},
		{"<a [0," + strings.Repeat("9", 400) + "px]>", `{"type":"Group","terms":[{"type":"Type","name":"a","opts":{"type":"Range","min":0,"max":"Infinitypx"}}],"combinator":" ","disallowEmpty":false,"explicit":false}`, "<a [0,Infinitypx]>", "<a [0,Infinitypx]>", "", 0},
	} {
		t.Run(tt.source, func(t *testing.T) {
			runSpecEdgeTest(t, tt)
		})
	}
}

// Go only: multiplier bounds are ints, so a bound that does not fit is an
// error; upstream keeps it as an inexact float.
func TestParseMultiplierOutOfRange(t *testing.T) {
	const source = "a{99999999999999999999}"
	_, err := definitionsyntax.Parse(source)
	var syntaxError *definitionsyntax.SyntaxError
	if !errors.As(err, &syntaxError) || syntaxError.RawMessage != "Number is out of range" || syntaxError.Offset != 2 {
		t.Errorf("Parse(%q) error = %v, want %q at 2", source, err, "Number is out of range")
	}
}

// A Range is only a Type's opts; upstream's walk throws for it.
func TestWalkRange(t *testing.T) {
	err := definitionsyntax.Walk(&definitionsyntax.Range{}, definitionsyntax.WalkOptions{Enter: func(definitionsyntax.Node) {}})
	if err == nil || err.Error() != "Unknown type: Range" {
		t.Errorf("Walk(&Range{}) error = %v, want %q", err, "Unknown type: Range")
	}
}

// A Group built without terms encodes them as [], as upstream's always does.
func TestMarshalGroupWithoutTerms(t *testing.T) {
	data, err := json.Marshal(&definitionsyntax.Group{Combinator: " "})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"type":"Group","terms":[],"combinator":" ","disallowEmpty":false,"explicit":false}`
	if string(data) != want {
		t.Errorf("json.Marshal(&Group{}) = %s, want %s", data, want)
	}
}

func TestWalkNilTerm(t *testing.T) {
	for _, node := range []definitionsyntax.Node{
		&definitionsyntax.Boolean{},
		&definitionsyntax.Multiplier{},
		&definitionsyntax.Group{Terms: []definitionsyntax.Node{nil}},
	} {
		var left []string
		err := definitionsyntax.Walk(node, definitionsyntax.WalkOptions{Leave: func(n definitionsyntax.Node) {
			left = append(left, n.Type())
		}})
		if err == nil || err.Error() != "Unknown type: <nil>" {
			t.Errorf("Walk(%s with a nil term) error = %v, want %q", node.Type(), err, "Unknown type: <nil>")
		}
		if len(left) != 0 {
			t.Errorf("Walk(%s with a nil term) left %v, want none", node.Type(), left)
		}
	}
}
