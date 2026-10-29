package csstree

import (
	"errors"
	"strings"
	"testing"
)

// enterTypes returns the types of the nodes of ast in walk order.
func enterTypes(t *testing.T, ast Node) string {
	t.Helper()
	var types []string
	mustWalk(t, ast, enterOnly(func(node Node) { types = append(types, node.Type()) }))
	return strings.Join(types, " ")
}

// Upstream looks names up after String.prototype.toLowerCase, which maps
// U+0130 (İ) to i and U+0307 (two code points), so a name spelled with
// İ is not the ASCII name with an i (strings.ToLower maps İ to i).
func TestParseNameWithDottedCapitalI(t *testing.T) {
	for _, tt := range []struct {
		source string
		types  string
		error  string
	}{
		{source: `@İmport "a";`, types: "StyleSheet Atrule AtrulePrelude String"},
		{source: `:İs(a){}`, types: "StyleSheet Rule SelectorList Selector PseudoClassSelector Raw Block"},
		{source: `:nth-chİld(2n+1){}`, types: "StyleSheet Rule SelectorList Selector PseudoClassSelector Raw Block"},
		{source: `::slİtted(a){}`, types: "StyleSheet Rule SelectorList Selector PseudoElementSelector Raw Block"},
		{source: `a{b:expressİon(1)}`, types: "StyleSheet Rule SelectorList Selector TypeSelector Block Declaration Value Function Number"},
		{source: `@medİa (width){}`, types: "StyleSheet Atrule AtrulePrelude Parentheses Identifier Block"},
		{source: `@media nİt screen{}`, types: "StyleSheet Atrule Raw Block", error: `Identifier "and" is expected`},
		{source: `@supports selectİr(a){}`, types: "StyleSheet Atrule AtrulePrelude Condition GeneralEnclosed Identifier Block", error: "Unknown feature selecti\u0307r()"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			var parseErrors []string
			ast, err := Parse(tt.source, ParseOptions{OnParseError: func(err error, _ Node) error {
				parseErrors = append(parseErrors, err.Error())
				return nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			if got := enterTypes(t, ast); got != tt.types {
				t.Errorf("Parse(%q) node types = %q, want %q", tt.source, got, tt.types)
			}
			if got := strings.Join(parseErrors, "; "); got != tt.error {
				t.Errorf("Parse(%q) errors = %q, want %q", tt.source, got, tt.error)
			}
			if css := mustGenerate(t, ast); css != tt.source {
				t.Errorf("Generate(Parse(%q)) = %q", tt.source, css)
			}
		})
	}
}

func mustGenerate(t *testing.T, ast Node) string {
	t.Helper()
	css, err := Generate(ast, GenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return css
}

// Keyword and Property lowercase names as String.prototype.toLowerCase does
// (expected values are upstream's).
func TestNamesLowercaseAsJavaScript(t *testing.T) {
	for _, tt := range []struct {
		name string
		want string
	}{
		{name: "ΑΣ", want: "ας"},
		{name: "ΑΣ-", want: "ας-"},
		{name: "-ΑΣ-Σ", want: "-ας-σ"},
		{name: "Σ", want: "σ"},
		{name: "ΣΣ", want: "σς"},
		{name: "ΣA", want: "σa"},
		{name: "aΣb", want: "aσb"},
		{name: "AΣ1", want: "aς1"},
		{name: "A Σ", want: "a σ"},
		{name: "1Σ", want: "1σ"},
		{name: "ǅΣ", want: "ǆς"},
		{name: "ªΣ", want: "ªς"},
		{name: "ʰΣ", want: "ʰσ"},
		{name: "AΣʰ", want: "aςʰ"},
		{name: "a\u0301Σ", want: "a\u0301ς"},
		{name: "aΣ\u0301b", want: "aσ\u0301b"},
		{name: "aΣ\u0345", want: "aς\u0345"},
		{name: "a.Σ", want: "a.ς"},
		{name: "aΣ.b", want: "aσ.b"},
		{name: "A\u00adΣ", want: "a\u00adς"},
		{name: "A:Σ", want: "a:ς"},
		{name: "AΣ'", want: "aς'"},
		{name: "\u2160Σ", want: "\u2170ς"},
		{name: "İ", want: "i\u0307"},
		{name: "-İ-x", want: "-i\u0307-x"},
		{name: "\u212a", want: "k"},
		{name: "ABC", want: "abc"},
	} {
		if got := Keyword(tt.name).Name; got != tt.want {
			t.Errorf("Keyword(%q).Name = %q, want %q", tt.name, got, tt.want)
		}
		if got := Property(tt.name).Name; got != tt.want {
			t.Errorf("Property(%q).Name = %q, want %q", tt.name, got, tt.want)
		}
	}

	if got, want := Keyword("-İ-x"), (KeywordDescriptor{Basename: "x", Name: "-i\u0307-x", Prefix: "-i\u0307-", Vendor: "-i\u0307-"}); got != want {
		t.Errorf("Keyword(%q) = %+v, want %+v", "-İ-x", got, want)
	}
}

// A panic in OnParseError ends the parse, as an exception thrown by
// upstream's onParseError reaches the caller of parse, even from a nested
// parseWithFallback (upstream: one call, then "boom" is thrown).
func TestParseOnParseErrorPanics(t *testing.T) {
	const source = "@media ((a:])){}"

	errBoom := errors.New("boom")
	for _, tt := range []struct {
		name  string
		value any
		check func(err error) bool
	}{
		{name: "error", value: errBoom, check: func(err error) bool { return errors.Is(err, errBoom) }},
		{name: "string", value: "boom", check: func(err error) bool { return err.Error() == "boom" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			ast, err := Parse(source, ParseOptions{OnParseError: func(error, Node) error {
				calls++
				panic(tt.value)
			}})
			if ast != nil || err == nil || !tt.check(err) {
				t.Errorf("Parse(%q) = %v, %v; want nil, boom", source, ast, err)
			}
			if calls != 1 {
				t.Errorf("OnParseError called %d times, want 1", calls)
			}
		})
	}

	var messages []string
	if _, err := Parse(source, ParseOptions{OnParseError: func(err error, _ Node) error {
		messages = append(messages, err.Error())
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	// upstream ends with `")" is expected`: it reads the a of (a:]) as a
	// condition, which the port rejects (see TestSpecConditionGrammar)
	want := `Number, dimension, ratio or identifier is expected; Number, dimension, ratio or identifier is expected; Unexpected input; Parenthesis or function is expected`
	if got := strings.Join(messages, "; "); got != want {
		t.Errorf("parse errors = %q, want %q", got, want)
	}
}

// An unknown condition kind fails where upstream's parentheses[kind] is not
// a function; upstream reports a TypeError, the port a syntax error. Upstream
// then reads (a) as a nested condition of the identifier a; the port
// rejects a lone identifier as a condition (see TestSpecConditionGrammar)
// and falls back to a GeneralEnclosed.
func TestParseConditionUnknownKind(t *testing.T) {
	var parseErrors []error
	ast, err := Parse("(a)", ParseOptions{Context: "condition", Kind: "unknown", OnParseError: func(err error, _ Node) error {
		parseErrors = append(parseErrors, err)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := enterTypes(t, ast), "Condition GeneralEnclosed Identifier"; got != want {
		t.Errorf("node types = %q, want %q", got, want)
	}
	var syntaxError *SyntaxError
	if len(parseErrors) != 2 || !errors.As(parseErrors[0], &syntaxError) {
		t.Fatalf("parse errors = %v, want two, the first a *SyntaxError", parseErrors)
	}
	if got, want := parseErrors[0].Error(), "Unknown condition kind `unknown`"; got != want {
		t.Errorf("parse error = %q, want %q", got, want)
	}
}
