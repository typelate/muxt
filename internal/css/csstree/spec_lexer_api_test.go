package csstree

import (
	"encoding/json"
	"errors"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/definitionsyntax"
)

// The expectations in this file are upstream csstree 3.2.1's results under
// Node.js.

// TestNewLexerCustomUnits checks a lexer whose config replaces unit
// groups: a list replaces the group (units compare lowercased against the
// list as given), nil keeps the default and an empty list leaves the group
// without units. <attr-unit> takes every unit lowercased.
func TestNewLexerCustomUnits(t *testing.T) {
	lexer, err := NewLexer(&Config{
		Generic: new(true),
		Units: map[string][]string{
			"length": {"foo", "BAR"},
			"angle":  nil,
			"time":   {},
		},
		Types: map[string]string{"a": "<length>"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	runLexerEdgeCases(t, lexer, []lexerEdgeCase{
		{kind: "type", name: "length", value: "1foo", matched: true, iterations: 3},
		{kind: "type", name: "length", value: "1FOO", matched: true, iterations: 3},
		{kind: "type", name: "length", value: "1bar", iterations: 3},
		{kind: "type", name: "length", value: "1px", iterations: 3},
		{kind: "type", name: "length", value: "0", matched: true, iterations: 3},
		{kind: "type", name: "length", value: "1deg", iterations: 3},
		{kind: "type", name: "a", value: "1foo", matched: true, iterations: 5},
		{kind: "type", name: "angle", value: "1deg", matched: true, iterations: 3},
		{kind: "type", name: "time", value: "1s", iterations: 3},
		{kind: "type", name: "time", value: "1ms", iterations: 3},
		{kind: "syntax", name: "<attr-unit>", value: "foo", matched: true, iterations: 5},
		{kind: "syntax", name: "<attr-unit>", value: "BAR", matched: true, iterations: 5},
		{kind: "syntax", name: "<attr-unit>", value: "px", iterations: 5},
		{kind: "syntax", name: "<attr-unit>", value: "deg", matched: true, iterations: 5},
		{kind: "syntax", name: "<attr-unit>", value: "%", matched: true, iterations: 5},
		{kind: "syntax", name: "<attr-unit>", value: "s", iterations: 5},
	})

	want := defaultUnits()
	want["length"] = []string{"foo", "BAR"}
	want["time"] = []string{}
	if got := lexer.Units(); !maps.EqualFunc(got, want, slices.Equal) {
		t.Errorf("Units() = %v, want %v", got, want)
	}
}

func TestLexerAccessors(t *testing.T) {
	lexer := DefaultLexer()

	t.Run("CSSWideKeywords", func(t *testing.T) {
		want := []string{"initial", "inherit", "unset", "revert", "revert-layer"}
		if got := lexer.CSSWideKeywords(); !slices.Equal(got, want) {
			t.Errorf("CSSWideKeywords() = %v, want %v", got, want)
		}
	})

	t.Run("StructureDocs", func(t *testing.T) {
		for nodeType, want := range map[string]map[string]string{
			"Dimension":   {"type": `"Dimension"`, "value": "string", "unit": "string"},
			"Declaration": {"type": `"Declaration"`, "important": "boolean | string", "property": "string", "value": "Value | Raw"},
			"Nope":        nil,
		} {
			if got := lexer.StructureDocs(nodeType); !maps.Equal(got, want) {
				t.Errorf("StructureDocs(%q) = %v, want %v", nodeType, got, want)
			}
		}
	})

	t.Run("GetAtrulePrelude", func(t *testing.T) {
		for _, test := range []struct {
			name     string
			fallback bool
			want     bool
		}{
			{"media", true, true},
			{"font-face", true, false},
			{"nope", true, false},
			{"-webkit-media", true, true},
			{"-webkit-media", false, false},
		} {
			prelude := lexer.GetAtrulePrelude(test.name, test.fallback)
			if got := prelude != nil; got != test.want {
				t.Errorf("GetAtrulePrelude(%q, %v) = %v, want a prelude: %v", test.name, test.fallback, prelude, test.want)
			}
			if prelude != nil && (prelude.Type != "AtrulePrelude" || prelude.Name != "media") {
				t.Errorf("GetAtrulePrelude(%q, %v) = %s %s, want AtrulePrelude media", test.name, test.fallback, prelude.Type, prelude.Name)
			}
		}
	})

	t.Run("GetAtruleDescriptor", func(t *testing.T) {
		// upstream throws a TypeError for every call
		if got := lexer.GetAtruleDescriptor("font-face", "src"); got != nil {
			t.Errorf("GetAtruleDescriptor(font-face, src) = %v, want nil", got)
		}
	})

	t.Run("Dump", func(t *testing.T) {
		dump, err := lexer.Dump(false)
		if err != nil {
			t.Fatal(err)
		}
		media := dump.Atrules["media"]
		if media.Prelude != "<media-query-list>" || media.Descriptors != nil {
			t.Errorf("Dump(false).Atrules[media] = %+v, want prelude <media-query-list> and no descriptors", media)
		}
		fontFace := dump.Atrules["font-face"]
		if fontFace.Prelude != "" || len(fontFace.Descriptors) != 13 {
			t.Errorf("Dump(false).Atrules[font-face] = %+v, want no prelude and 13 descriptors", fontFace)
		}
		if _, ok := dump.Atrules["nest"]; ok {
			t.Errorf("Dump(false).Atrules has nest, want none")
		}

		pretty, err := lexer.Dump(true)
		if err != nil {
			t.Fatal(err)
		}
		const wantImport = "[ <string> | <url> ] [ layer | layer( <layer-name> ) ]? [ supports( [ <supports-condition> | <declaration> ] ) ]? <media-query-list>?"
		if got := pretty.Atrules["import"].Prelude; got != wantImport {
			t.Errorf("Dump(true).Atrules[import].Prelude = %q, want %q", got, wantImport)
		}
	})

	t.Run("String", func(t *testing.T) {
		var dump struct {
			Generic bool              `json:"generic"`
			Types   map[string]string `json:"types"`
			Atrules map[string]struct {
				Prelude     *string           `json:"prelude"`
				Descriptors map[string]string `json:"descriptors"`
			} `json:"atrules"`
		}
		if err := json.Unmarshal([]byte(lexer.String()), &dump); err != nil {
			t.Fatalf("String() is not JSON: %v", err)
		}
		if !dump.Generic || dump.Types["ratio"] != "<number [0,∞]> [/ <number [0,∞]>]?" {
			t.Errorf("String() generic = %v, types.ratio = %q", dump.Generic, dump.Types["ratio"])
		}
		if prelude := dump.Atrules["font-face"].Prelude; prelude != nil {
			t.Errorf("String() atrules.font-face.prelude = %q, want null", *prelude)
		}
	})
}

// TestCheckPlainObjectStructureUnknownType checks that a node type that is
// not a string is written as JavaScript's string conversion writes it, and
// a missing one as undefined.
func TestCheckPlainObjectStructureUnknownType(t *testing.T) {
	lexer := DefaultLexer()

	for _, test := range []struct {
		node map[string]any
		want string
	}{
		{map[string]any{}, "undefined"},
		{map[string]any{"type": nil}, "null"},
		{map[string]any{"type": "x"}, "x"},
		{map[string]any{"type": true}, "true"},
		{map[string]any{"type": 1.0}, "1"},
		{map[string]any{"type": 1.5}, "1.5"},
		{map[string]any{"type": -1.5}, "-1.5"},
		{map[string]any{"type": math.Copysign(0, -1)}, "0"},
		{map[string]any{"type": 0.1}, "0.1"},
		{map[string]any{"type": 100.0}, "100"},
		{map[string]any{"type": 1e6}, "1000000"},
		{map[string]any{"type": 12345.678}, "12345.678"},
		{map[string]any{"type": 1e20}, "100000000000000000000"},
		{map[string]any{"type": 123456789012345680000.0}, "123456789012345680000"},
		{map[string]any{"type": 1e21}, "1e+21"},
		{map[string]any{"type": -1e21}, "-1e+21"},
		{map[string]any{"type": 0.000001}, "0.000001"},
		{map[string]any{"type": 1e-7}, "1e-7"},
		{map[string]any{"type": 1.2345678901234568e-7}, "1.2345678901234568e-7"},
		{map[string]any{"type": 5e-324}, "5e-324"},
		{map[string]any{"type": math.MaxFloat64}, "1.7976931348623157e+308"},
		{map[string]any{"type": map[string]any{}}, "[object Object]"},
		{map[string]any{"type": []any{}}, ""},
		{map[string]any{"type": []any{nil}}, ""},
		{map[string]any{"type": []any{1.0, 2.0}}, "1,2"},
		{map[string]any{"type": []any{[]any{1.0, []any{2.0}}, nil, "a", map[string]any{}, true}}, "1,2,,a,[object Object],true"},
	} {
		warnings := lexer.CheckPlainObjectStructure(test.node)
		want := "Unknown node type `" + test.want + "`"
		if len(warnings) != 1 || warnings[0].Message != want {
			t.Errorf("CheckPlainObjectStructure(%v) = %v, want [%s]", test.node, warnings, want)
		}
	}

	// the structure is looked up by the type converted to a string
	for _, test := range []struct {
		node map[string]any
		want []string
	}{
		{
			map[string]any{"type": []any{"Dimension"}, "loc": nil, "value": "1", "unit": "px"},
			[]string{"Wrong node type `Dimension`, expected `Dimension`"},
		},
		{
			map[string]any{"type": []any{[]any{"Dimension"}}, "value": "1", "unit": "px"},
			[]string{"Wrong node type `Dimension`, expected `Dimension`", "Field `Dimension.loc` is missed"},
		},
		{
			map[string]any{"type": "Dimension", "loc": "x", "value": "1", "unit": "px"},
			[]string{"Bad value for `Dimension.loc`"},
		},
	} {
		var got []string
		for _, warning := range lexer.CheckPlainObjectStructure(test.node) {
			got = append(got, warning.Message)
		}
		if !slices.Equal(got, test.want) {
			t.Errorf("CheckPlainObjectStructure(%v) = %q, want %q", test.node, got, test.want)
		}
	}
}

// TestSyntaxMatchErrorLocation checks the location of a mismatch after
// line breaks (\n, \r\n and \f each end a line).
func TestSyntaxMatchErrorLocation(t *testing.T) {
	lexer := DefaultLexer()

	for _, test := range []struct {
		value      string
		ast        bool
		start, end tokenizerPosition
	}{
		{"red\nblue", false, tokenizerPosition{4, 2, 1}, tokenizerPosition{8, 2, 5}},
		{"red\r\n\fblue", false, tokenizerPosition{6, 3, 1}, tokenizerPosition{10, 3, 5}},
		{"red \n  blue", false, tokenizerPosition{7, 2, 3}, tokenizerPosition{11, 2, 7}},
		{"red\n  blue", true, tokenizerPosition{6, 2, 3}, tokenizerPosition{10, 2, 7}},
	} {
		var result *MatchResult
		if test.ast {
			result = lexer.MatchProperty("color", mustParse(t, test.value, ParseOptions{Context: "value", Positions: true}))
		} else {
			result = lexer.MatchPropertyString("color", test.value)
		}

		var matchErr *SyntaxMatchError
		if !errors.As(result.Error, &matchErr) {
			t.Fatalf("MatchProperty(color, %q) error = %v, want a SyntaxMatchError", test.value, result.Error)
		}
		start := tokenizerPosition{matchErr.Loc.Start.Offset, matchErr.Loc.Start.Line, matchErr.Loc.Start.Column}
		end := tokenizerPosition{matchErr.Loc.End.Offset, matchErr.Loc.End.Line, matchErr.Loc.End.Column}
		fields := tokenizerPosition{matchErr.Offset, matchErr.Line, matchErr.Column}
		if start != test.start || end != test.end || fields != test.start || matchErr.Loc.Source != "<unknown>" {
			t.Errorf("MatchProperty(color, %q) mismatch at %v-%v (fields %v, source %q), want %v-%v", test.value, start, end, fields, matchErr.Loc.Source, test.start, test.end)
		}
	}
}

// tokenizerPosition is an offset, line and column.
type tokenizerPosition struct {
	offset, line, column int
}

// TestLexerGenericEdges matches generic types at their edges: a range of a
// number whose text is not a number (Number("+") is NaN, which is out of
// any range) and <attr-unit> without a token.
func TestLexerGenericEdges(t *testing.T) {
	runLexerEdgeCases(t, DefaultLexer(), []lexerEdgeCase{
		{kind: "syntax", name: "<number [0,∞]>", value: "+", iterations: 5},
		{kind: "syntax", name: "<number [0,∞]>", value: "e5", iterations: 5},
		{kind: "syntax", name: "<number [-∞,∞]>", value: "-", iterations: 5},
		{kind: "syntax", name: "<attr-unit>", value: "", iterations: 5},
	})
}

// TestLexerFragmentEdges finds fragments of syntaxes without a name, of
// keywords, and of a syntax ending outside the list it starts in.
func TestLexerFragmentEdges(t *testing.T) {
	nodeNames := func(fragments []Fragment) []string {
		var names []string
		for _, fragment := range fragments {
			for node := range fragment.Nodes.All() {
				name := node.Type()
				switch node := node.(type) {
				case *Operator:
					name += ":" + node.Value
				case *Identifier:
					name += ":" + node.Name
				}
				names = append(names, name)
			}
		}
		return names
	}

	lexer := DefaultLexer()
	for _, test := range []struct {
		property, value, typ, name string
		want                       []string
	}{
		// a name "" is upstream's omitted name, which matches the syntaxes
		// without one
		{"font", "12px/1.5 serif", "Token", "", []string{"Operator:/"}},
		{"border", "1px solid red", "Keyword", "solid", []string{"Identifier:solid"}},
	} {
		value := mustParse(t, test.value, ParseOptions{Context: "value"})
		fragments, err := lexer.FindValueFragments(test.property, value, test.typ, test.name)
		if err != nil {
			t.Fatal(err)
		}
		if got := nodeNames(fragments); !slices.Equal(got, test.want) {
			t.Errorf("FindValueFragments(%s, %q, %s %q) = %v, want %v", test.property, test.value, test.typ, test.name, got, test.want)
		}
	}

	// <a> starts at 1px in the parentheses and ends at their ")"
	custom := mustFork(t, ExtendFunc(func(prev *Config) *Config {
		prev.Types = map[string]string{"a": "<length> ')'"}
		prev.Properties = map[string]string{"foo": "'(' <a>"}
		return prev
	}))
	value := mustParse(t, "(1px)", ParseOptions{Context: "value"})
	if result := custom.Lexer.MatchProperty("foo", value); result.Matched == nil {
		t.Fatalf("MatchProperty(foo, (1px)) error = %v", result.Error)
	}
	fragments, err := custom.Lexer.FindValueFragments("foo", value, "Type", "a")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := nodeNames(fragments), []string{"Dimension"}; !slices.Equal(got, want) {
		t.Errorf("FindValueFragments(foo, (1px), <a>) = %v, want %v", got, want)
	}
}

// TestLexerBrokenReference checks that matching a syntax that refers to a
// type whose syntax does not parse fails (upstream throws the parse
// error).
func TestLexerBrokenReference(t *testing.T) {
	custom := mustFork(t, ExtendFunc(func(prev *Config) *Config {
		prev.Types = map[string]string{"a": "<b>", "b": "[ x"}
		return prev
	}))

	for _, name := range []string{"a", "b"} {
		result := custom.Lexer.MatchTypeString(name, "x")
		if result.Matched != nil || result.Error == nil || !strings.HasPrefix(result.Error.Error(), "Expect `]`") {
			t.Errorf("MatchTypeString(%s, x) error = %v, want Expect `]`", name, result.Error)
		}
	}
}

// TestLexerMatchNodeInputs matches ASTs that are not parsed values: no
// node (upstream null, which throws reading its type, except where there
// is nothing to match), single nodes, and declarations.
func TestLexerMatchNodeInputs(t *testing.T) {
	lexer := DefaultLexer()

	for _, test := range []struct {
		desc       string
		result     *MatchResult
		matched    bool
		iterations int
		err        string // "*" is any error
	}{
		{"property nil", lexer.MatchProperty("color", nil), false, 0, "*"},
		{"type nil", lexer.MatchType("length", nil), false, 0, "*"},
		{"descriptor nil", lexer.MatchAtruleDescriptor("font-face", "src", nil), false, 0, "*"},
		{"prelude nil", lexer.MatchAtrulePrelude("media", nil), false, 0, "At-rule `@media` should contain a prelude"},
		{"prelude nil layer", lexer.MatchAtrulePrelude("layer", nil), true, 12, ""},
		{"prelude nil font-face", lexer.MatchAtrulePrelude("font-face", nil), false, 0, ""},
		{"declaration", lexer.MatchDeclaration(mustParse(t, "color: red", ParseOptions{Context: "declaration"})), true, 22, ""},
		{"declaration important", lexer.MatchDeclaration(mustParse(t, "color: red !important", ParseOptions{Context: "declaration"})), true, 22, ""},
		{"declaration custom", lexer.MatchDeclaration(mustParse(t, "--x: red", ParseOptions{Context: "declaration"})), false, 0, ErrCustomProperty.Error()},
		{"not a declaration", lexer.MatchDeclaration(mustParse(t, "red", ParseOptions{Context: "value"})), false, 0, ErrNotDeclaration.Error()},
		{"raw", lexer.MatchProperty("color", &Raw{Value: " red "}), true, 22, ""},
		{"identifier", lexer.MatchProperty("color", &Identifier{Name: "red"}), true, 22, ""},
		{"number", lexer.MatchType("number", &Number{Value: "1"}), true, 3, ""},
	} {
		result := test.result
		if got := result.Matched != nil; got != test.matched || result.Iterations != test.iterations {
			t.Errorf("%s: matched = %v after %d iterations, want %v after %d (error %v)", test.desc, got, result.Iterations, test.matched, test.iterations, result.Error)
		}
		switch {
		case test.err == "*" && result.Error == nil:
			t.Errorf("%s: error = nil, want an error", test.desc)
		case test.err == "" && result.Error != nil:
			t.Errorf("%s: error = %v, want nil", test.desc, result.Error)
		case test.err != "" && test.err != "*" && (result.Error == nil || result.Error.Error() != test.err):
			t.Errorf("%s: error = %v, want %q", test.desc, result.Error, test.err)
		}
	}
}

// TestLexerMatchDescriptorEdges checks matching against a parsed definition
// syntax, an empty one and missing syntaxes.
func TestLexerMatchDescriptorEdges(t *testing.T) {
	lexer := DefaultLexer()

	tree, err := definitionsyntax.Parse("<length> | auto")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := lexer.CreateDescriptorFromTree(tree, "Type", "anonymous", "")
	for _, test := range []struct {
		value      string
		matched    bool
		iterations int
	}{
		{"auto", true, 8},
		{"1px", true, 6},
		{"none", false, 8},
	} {
		result := lexer.MatchDescriptorString(descriptor, test.value)
		if got := result.Matched != nil; got != test.matched || result.Iterations != test.iterations {
			t.Errorf("MatchDescriptorString(<length> | auto, %q) = %v, %d iterations, want %v, %d", test.value, got, result.Iterations, test.matched, test.iterations)
		}
		var matchErr *SyntaxMatchError
		if errors.As(result.Error, &matchErr) && matchErr.Syntax != "<length> | auto" {
			t.Errorf("MatchDescriptorString(<length> | auto, %q) syntax = %q", test.value, matchErr.Syntax)
		}
	}

	result := lexer.MatchString("", "x")
	var matchErr *SyntaxMatchError
	if !errors.As(result.Error, &matchErr) || matchErr.RawMessage != "Mismatch" || matchErr.Syntax != "" || result.Iterations != 3 {
		t.Errorf(`MatchString("", "x") = %v, %d iterations, want a Mismatch of "" after 3`, result.Error, result.Iterations)
	}

	for _, test := range []struct {
		desc   string
		result *MatchResult
		want   string
	}{
		{"unknown type reference", lexer.MatchString("<unknown-type>", "x"), "Bad syntax reference: <unknown-type>"},
		{"nil descriptor", lexer.MatchDescriptorString(nil, "x"), "Bad syntax"},
		{"unknown type", lexer.MatchTypeString("nope", "x"), "Unknown type `nope`"},
		{"unknown property", lexer.MatchPropertyString("nope", "x"), "Unknown property `nope`"},
		{"media descriptor", lexer.MatchAtruleDescriptorString("media", "x", "x"), "At-rule `@media` has no known descriptors"},
		{"unknown descriptor", lexer.MatchAtruleDescriptorString("font-face", "nope", "x"), "Unknown at-rule descriptor `nope`"},
		{"unknown at-rule", lexer.MatchAtrulePreludeString("nope", "x"), "Unknown at-rule `@nope`"},
	} {
		if test.result.Matched != nil || test.result.Error == nil || test.result.Error.Error() != test.want {
			t.Errorf("%s: error = %v, want %q", test.desc, test.result.Error, test.want)
		}
	}

	var refErr *SyntaxReferenceError
	if result := lexer.MatchTypeString("nope", "x"); !errors.As(result.Error, &refErr) || refErr.Reference != "nope" {
		t.Errorf(`MatchTypeString("nope", "x") error = %v, want a SyntaxReferenceError for nope`, result.Error)
	}

	if result := lexer.MatchAtrulePreludeString("font-face", ""); result.Matched != nil || result.Error != nil {
		t.Errorf(`MatchAtrulePreludeString("font-face", "") = %v, %v, want no match and no error`, result.Matched, result.Error)
	}
}

// TestLexerFunctionFragmentsAndTrace finds the fragments and traces of
// function syntaxes.
func TestLexerFunctionFragmentsAndTrace(t *testing.T) {
	lexer := DefaultLexer()
	value := mustParse(t, "translate(1px) rotate(1deg)", ParseOptions{Context: "value"})

	fragmentNames := func(fragments []Fragment) []string {
		var names []string
		for _, fragment := range fragments {
			for node := range fragment.Nodes.All() {
				names = append(names, node.Type()+":"+node.(*Function).Name)
			}
		}
		return names
	}

	for _, test := range []struct {
		typ, name string
		want      []string
	}{
		{"Function", "translate", []string{"Function:translate"}},
		{"Type", "transform-function", []string{"Function:translate", "Function:rotate"}},
		{"Keyword", "translate", nil},
	} {
		fragments, err := lexer.FindValueFragments("transform", value, test.typ, test.name)
		if err != nil {
			t.Fatal(err)
		}
		if got := fragmentNames(fragments); !slices.Equal(got, test.want) {
			t.Errorf("FindValueFragments(transform, %s %s) = %v, want %v", test.typ, test.name, got, test.want)
		}
	}

	match := lexer.MatchProperty("transform", value)
	translate := value.(*Value).Children.First()
	length := translate.(*Function).Children.First()

	traceNames := func(trace []MatchSyntax) []string {
		names := []string{}
		for _, syntax := range trace {
			names = append(names, syntax.Type()+":"+syntaxName(syntax))
		}
		return names
	}

	wantTranslate := []string{"Property:transform", "Type:transform-list", "Type:transform-function", "Type:translate()"}
	if got := traceNames(match.GetTrace(translate)); !slices.Equal(got, wantTranslate) {
		t.Errorf("GetTrace(translate()) = %v, want %v", got, wantTranslate)
	}
	wantLength := append(slices.Clone(wantTranslate), "Type:length-percentage", "Type:length")
	if got := traceNames(match.GetTrace(length)); !slices.Equal(got, wantLength) {
		t.Errorf("GetTrace(1px) = %v, want %v", got, wantLength)
	}
	if got := match.GetTrace(mustParse(t, "1px", ParseOptions{Context: "value"})); got != nil {
		t.Errorf("GetTrace(another node) = %v, want nil", got)
	}

	for _, test := range []struct {
		desc string
		got  bool
		want bool
	}{
		{"IsType(translate(), transform-function)", match.IsType(translate, "transform-function"), true},
		{"IsKeyword(translate())", match.IsKeyword(translate), false},
		{"IsProperty(translate(), transform)", match.IsProperty(translate, "transform"), true},
		{"IsProperty(translate(), width)", match.IsProperty(translate, "width"), false},
		{"IsType(1px, length)", match.IsType(length, "length"), true},
		{"IsType(1px, length-percentage)", match.IsType(length, "length-percentage"), true},
		{"IsType(1px, angle)", match.IsType(length, "angle"), false},
	} {
		if test.got != test.want {
			t.Errorf("%s = %v, want %v", test.desc, test.got, test.want)
		}
	}
}
