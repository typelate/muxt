package csstree

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The tests in this file cover where the parser and the generator follow
// the CSS specifications instead of upstream csstree.

// parseCollectingErrors parses source and returns the AST and the messages
// of the errors the parser recovered from.
func parseCollectingErrors(t *testing.T, source string, options ParseOptions) (Node, []string) {
	t.Helper()
	var parseErrors []string
	options.OnParseError = func(err error, _ Node) error {
		parseErrors = append(parseErrors, err.Error())
		return nil
	}
	ast, err := Parse(source, options)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", source, err)
	}
	return ast, parseErrors
}

// plainJSON returns the JSON of node without locations.
func plainJSON(t *testing.T, node Node) string {
	t.Helper()
	data, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	var plain any
	if err := json.Unmarshal(data, &plain); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(dropLocations(plain)); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

func dropLocations(value any) any {
	switch value := value.(type) {
	case map[string]any:
		delete(value, "loc")
		for key, item := range value {
			value[key] = dropLocations(item)
		}
	case []any:
		for i, item := range value {
			value[i] = dropLocations(item)
		}
	}
	return value
}

// specParseTest is a source parsed in a context, with the expected plain
// object (JSON without locations; "" skips the check), node types in walk
// order ("" skips the check), generated CSS and recovered errors.
type specParseTest struct {
	source   string
	context  string
	json     string
	types    string
	generate string
	errors   string
}

func runSpecParseTests(t *testing.T, tests []specParseTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			ast, parseErrors := parseCollectingErrors(t, tt.source, ParseOptions{Context: tt.context})
			if tt.json != "" {
				if got := plainJSON(t, ast); got != tt.json {
					t.Errorf("Parse(%q) = %s, want %s", tt.source, got, tt.json)
				}
			}
			if tt.types != "" {
				if got := enterTypes(t, ast); got != tt.types {
					t.Errorf("Parse(%q) node types = %q, want %q", tt.source, got, tt.types)
				}
			}
			if got := mustGenerate(t, ast); got != tt.generate {
				t.Errorf("Generate(Parse(%q)) = %q, want %q", tt.source, got, tt.generate)
			}
			if got := strings.Join(parseErrors, "; "); got != tt.errors {
				t.Errorf("Parse(%q) errors = %q, want %q", tt.source, got, tt.errors)
			}
		})
	}
}

// Selectors 4 column combinator: https://drafts.csswg.org/selectors-4/#the-column-combinator
func TestSpecColumnCombinator(t *testing.T) {
	runSpecParseTests(t, []specParseTest{
		{
			source:   "a||b",
			context:  "selector",
			json:     `{"children":[{"name":"a","type":"TypeSelector"},{"name":"||","type":"Combinator"},{"name":"b","type":"TypeSelector"}],"type":"Selector"}`,
			generate: "a||b",
		},
		{
			source:   "a || b",
			context:  "selector",
			json:     `{"children":[{"name":"a","type":"TypeSelector"},{"name":"||","type":"Combinator"},{"name":"b","type":"TypeSelector"}],"type":"Selector"}`,
			generate: "a||b",
		},
		{
			source:   "col.selected||td",
			context:  "selector",
			types:    "Selector TypeSelector ClassSelector Combinator TypeSelector",
			generate: "col.selected||td",
		},
		{
			source:   "*||ns|b",
			context:  "selector",
			json:     `{"children":[{"name":"*","type":"TypeSelector"},{"name":"||","type":"Combinator"},{"name":"ns|b","type":"TypeSelector"}],"type":"Selector"}`,
			generate: "*||ns|b",
		},
		{
			source:   "ns|a||*|b",
			context:  "selector",
			json:     `{"children":[{"name":"ns|a","type":"TypeSelector"},{"name":"||","type":"Combinator"},{"name":"*|b","type":"TypeSelector"}],"type":"Selector"}`,
			generate: "ns|a||*|b",
		},
		{
			source:   "|| b",
			context:  "selector",
			json:     `{"children":[{"name":"||","type":"Combinator"},{"name":"b","type":"TypeSelector"}],"type":"Selector"}`,
			generate: "||b",
		},
		{
			source:   ":has(|| td){}",
			types:    "StyleSheet Rule SelectorList Selector PseudoClassSelector SelectorList Selector Combinator TypeSelector Block",
			generate: ":has(||td){}",
		},
		{
			// a namespace prefix is still read when one | follows
			source:   "a|b",
			context:  "selector",
			json:     `{"children":[{"name":"a|b","type":"TypeSelector"}],"type":"Selector"}`,
			generate: "a|b",
		},
		{
			source:   "|b",
			context:  "selector",
			json:     `{"children":[{"name":"|b","type":"TypeSelector"}],"type":"Selector"}`,
			generate: "|b",
		},
	})
}

// Selectors 4 attribute selector modifiers:
// https://drafts.csswg.org/selectors-4/#attribute-case
//
//	<attr-modifier> = i | s
//
// and the modifier follows a matcher and a value.
func TestSpecAttributeSelectorFlags(t *testing.T) {
	runSpecParseTests(t, []specParseTest{
		{
			source:   "[a=b i]",
			context:  "selector",
			json:     `{"children":[{"flags":"i","matcher":"=","name":{"name":"a","type":"Identifier"},"type":"AttributeSelector","value":{"name":"b","type":"Identifier"}}],"type":"Selector"}`,
			generate: "[a=b i]",
		},
		{
			source:   "[a=b s]",
			context:  "selector",
			json:     `{"children":[{"flags":"s","matcher":"=","name":{"name":"a","type":"Identifier"},"type":"AttributeSelector","value":{"name":"b","type":"Identifier"}}],"type":"Selector"}`,
			generate: "[a=b s]",
		},
		{
			source:   `[a="b"I]`,
			context:  "selector",
			json:     `{"children":[{"flags":"I","matcher":"=","name":{"name":"a","type":"Identifier"},"type":"AttributeSelector","value":{"type":"String","value":"b"}}],"type":"Selector"}`,
			generate: `[a="b"I]`,
		},
		{
			source:   "[a=b S]",
			context:  "selector",
			types:    "Selector AttributeSelector Identifier Identifier",
			generate: "[a=b S]",
		},
		{
			source:   "[a=b c]{}",
			types:    "StyleSheet Rule Raw Block",
			generate: "[a=b c]{}",
			errors:   `Attribute selector modifier (i or s) is expected`,
		},
		{
			source:   "[a=b ii]{}",
			types:    "StyleSheet Rule Raw Block",
			generate: "[a=b ii]{}",
			errors:   `Attribute selector modifier (i or s) is expected`,
		},
		{
			// a modifier needs a matcher and a value
			source:   "[a i]{}",
			types:    "StyleSheet Rule Raw Block",
			generate: "[a i]{}",
			errors:   `Attribute selector (=, ~=, ^=, $=, *=, |=) is expected`,
		},
	})
}

// A \ delim is followed by a newline in generated CSS (\ and a newline is
// not an escape), but no newline is added when the next token starts with
// one, so parse and generate reach a fixed point.
// https://drafts.csswg.org/css-syntax-3/#starts-with-a-valid-escape
func TestSpecGenerateReverseSolidusFixedPoint(t *testing.T) {
	for _, source := range []string{
		"a{--x:a\\\n}",
		"a{--x:\\\n}",
		"a{--x:a\\\nb}",
		"a{--x:a\\\r\nb}",
		"a{--x:a\\\fb}",
		"a{--x:a\\\n\\\n\\\n}",
		"a{--x:a\\\n  \n}",
		"a{--x:a\\\n/**/b}",
		"a{b:c\\\n}",
		"@a b\\\n;",
		"a\\\n{}",
	} {
		t.Run(source, func(t *testing.T) {
			ast, err := Parse(source, ParseOptions{})
			if err != nil {
				t.Fatal(err)
			}
			first := mustGenerate(t, ast)
			if first != source {
				t.Errorf("Generate(Parse(%q)) = %q, want the source", source, first)
			}
			ast, err = Parse(first, ParseOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if second := mustGenerate(t, ast); second != first {
				t.Errorf("Generate(Parse(%q)) = %q, want %q", first, second, first)
			}
		})
	}
}

// A \ delim not followed by a newline still gets one.
func TestSpecGenerateReverseSolidusNewline(t *testing.T) {
	for _, tt := range []struct {
		name string
		node Node
		want string
	}{
		{
			name: "before an identifier",
			node: &Value{Children: new(List[Node]).AppendData(&Raw{Value: `\`}).AppendData(&Identifier{Name: "b"})},
			want: "\\\nb",
		},
		{
			name: "before a space",
			node: &Value{Children: new(List[Node]).AppendData(&Raw{Value: `\`}).AppendData(&Raw{Value: " b"})},
			want: "\\\n b",
		},
		{
			name: "at the end",
			node: &Value{Children: new(List[Node]).AppendData(&Raw{Value: `a\`})},
			want: "a\\\n",
		},
		{
			name: "before a newline",
			node: &Value{Children: new(List[Node]).AppendData(&Raw{Value: `\`}).AppendData(&Raw{Value: "\nb"})},
			want: "\\\nb",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := mustGenerate(t, tt.node); got != tt.want {
				t.Errorf("Generate(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// CSS Syntax 3 "consume a declaration": the value is everything up to the
// ; or the end of the block, and only a trailing ! important (last two
// non-whitespace tokens) makes the declaration important.
// https://drafts.csswg.org/css-syntax-3/#consume-declaration
//
// A trailing ! with another identifier keeps upstream's hack support
// (Important is that identifier, e.g. "ie").
func TestSpecDeclarationImportant(t *testing.T) {
	runSpecParseTests(t, []specParseTest{
		{
			source:   "color:red!important!important",
			context:  "declaration",
			json:     `{"important":true,"property":"color","type":"Declaration","value":{"type":"Raw","value":"red!important"}}`,
			generate: "color:red!important!important",
			errors:   "Unexpected input",
		},
		{
			source:   "color:red !important !important",
			context:  "declaration",
			json:     `{"important":true,"property":"color","type":"Declaration","value":{"type":"Raw","value":"red !important"}}`,
			generate: "color:red !important!important",
			errors:   "Unexpected input",
		},
		{
			source:   "color:red ! important",
			context:  "declaration",
			json:     `{"important":true,"property":"color","type":"Declaration","value":{"children":[{"name":"red","type":"Identifier"}],"type":"Value"}}`,
			generate: "color:red!important",
		},
		{
			source:   "color:red !/**/ important /**/",
			context:  "declaration",
			json:     `{"important":true,"property":"color","type":"Declaration","value":{"children":[{"name":"red","type":"Identifier"}],"type":"Value"}}`,
			generate: "color:red!important",
		},
		{
			source:   "color:red!IMPORTANT",
			context:  "declaration",
			json:     `{"important":"IMPORTANT","property":"color","type":"Declaration","value":{"children":[{"name":"red","type":"Identifier"}],"type":"Value"}}`,
			generate: "color:red!IMPORTANT",
		},
		{
			source:   "color:red!ie",
			context:  "declaration",
			json:     `{"important":"ie","property":"color","type":"Declaration","value":{"children":[{"name":"red","type":"Identifier"}],"type":"Value"}}`,
			generate: "color:red!ie",
		},
		{
			source:   "color:red !important !ie",
			context:  "declaration",
			json:     `{"important":"ie","property":"color","type":"Declaration","value":{"type":"Raw","value":"red !important"}}`,
			generate: "color:red !important!ie",
			errors:   "Unexpected input",
		},
		{
			source:   "color:!important",
			context:  "declaration",
			json:     `{"important":true,"property":"color","type":"Declaration","value":{"children":[],"type":"Value"}}`,
			generate: "color:!important",
		},
		{
			// a ! that is not followed by an identifier is part of the value
			source:   "color:red!",
			context:  "declaration",
			json:     `{"important":false,"property":"color","type":"Declaration","value":{"type":"Raw","value":"red!"}}`,
			generate: "color:red!",
			errors:   "Unexpected input",
		},
		{
			source:   "color:a!b c",
			context:  "declaration",
			json:     `{"important":false,"property":"color","type":"Declaration","value":{"type":"Raw","value":"a!b c"}}`,
			generate: "color:a!b c",
			errors:   "Unexpected input",
		},
		{
			source:   "--x:a!important!important",
			context:  "declaration",
			json:     `{"important":true,"property":"--x","type":"Declaration","value":{"type":"Raw","value":"a!important"}}`,
			generate: "--x:a!important!important",
		},
		{
			source:   "--x: a !important",
			context:  "declaration",
			json:     `{"important":true,"property":"--x","type":"Declaration","value":{"type":"Raw","value":" a "}}`,
			generate: "--x: a !important",
		},
		{
			source:   "a{color:red!important!important;b:c!important}",
			types:    "StyleSheet Rule SelectorList Selector TypeSelector Block Declaration Raw Declaration Value Identifier",
			generate: "a{color:red!important!important;b:c!important}",
			errors:   "Unexpected input",
		},
		{
			source:   "a{color:red!;b:c}",
			types:    "StyleSheet Rule SelectorList Selector TypeSelector Block Declaration Raw Declaration Value Identifier",
			generate: "a{color:red!;b:c}",
			errors:   "Unexpected input",
		},
	})
}

// CSS Syntax 3: a declaration value is a list of component values, so a ;
// inside (), [] or a function is part of the value. The ; is a Raw node.
// https://drafts.csswg.org/css-syntax-3/#consume-list-of-components
func TestSpecSemicolonInValueBlock(t *testing.T) {
	runSpecParseTests(t, []specParseTest{
		{
			source:   "x:(;)",
			context:  "declaration",
			json:     `{"important":false,"property":"x","type":"Declaration","value":{"children":[{"children":[{"type":"Raw","value":";"}],"type":"Parentheses"}],"type":"Value"}}`,
			generate: "x:(;)",
		},
		{
			source:   "x:f(;)",
			context:  "declaration",
			json:     `{"important":false,"property":"x","type":"Declaration","value":{"children":[{"children":[{"type":"Raw","value":";"}],"name":"f","type":"Function"}],"type":"Value"}}`,
			generate: "x:f(;)",
		},
		{
			source:   "x:[;]",
			context:  "declaration",
			json:     `{"important":false,"property":"x","type":"Declaration","value":{"children":[{"children":[{"type":"Raw","value":";"}],"type":"Brackets"}],"type":"Value"}}`,
			generate: "x:[;]",
		},
		{
			source:   "x:f(a ; b;;)",
			context:  "declaration",
			json:     `{"important":false,"property":"x","type":"Declaration","value":{"children":[{"children":[{"name":"a","type":"Identifier"},{"type":"Raw","value":";"},{"name":"b","type":"Identifier"},{"type":"Raw","value":";"},{"type":"Raw","value":";"}],"name":"f","type":"Function"}],"type":"Value"}}`,
			generate: "x:f(a;b;;)",
		},
		{
			// the var() fallback is a <declaration-value>, which has no
			// top-level ; (https://drafts.csswg.org/css-variables-2/#using-variables),
			// so the value falls back to Raw as upstream's does
			source:   "x:var(--y,;)",
			context:  "declaration",
			json:     `{"important":false,"property":"x","type":"Declaration","value":{"type":"Raw","value":"var(--y,;)"}}`,
			generate: "x:var(--y,;)",
			errors:   `")" is expected`,
		},
		{
			source:   "a{x:(;);y:[a;b] f(;)}",
			types:    "StyleSheet Rule SelectorList Selector TypeSelector Block Declaration Value Parentheses Raw Declaration Value Brackets Identifier Raw Identifier Function Raw",
			generate: "a{x:(;);y:[a;b]f(;)}",
		},
		{
			source:   "a{--x:(;);--y:f(;)}",
			types:    "StyleSheet Rule SelectorList Selector TypeSelector Block Declaration Raw Declaration Raw",
			generate: "a{--x:(;);--y:f(;)}",
		},
		{
			source:   "@x (;) [a;b];",
			types:    "StyleSheet Atrule AtrulePrelude Parentheses Raw Brackets Identifier Raw Identifier",
			generate: "@x (;)[a;b];",
		},
	})
}

// Media Queries 4 range syntax:
// https://drafts.csswg.org/mediaqueries-4/#mq-range-context
//
//	<mf-range> = <mf-name> <mf-comparison> <mf-value>
//	           | <mf-value> <mf-comparison> <mf-name>
//	           | <mf-value> <mf-lt> <mf-name> <mf-lt> <mf-value>
//	           | <mf-value> <mf-gt> <mf-name> <mf-gt> <mf-value>
//
// A range that does not match is a <general-enclosed>.
func TestSpecFeatureRange(t *testing.T) {
	runSpecParseTests(t, []specParseTest{
		{
			source:   "(width = 600px)",
			context:  "mediaQuery",
			json:     `{"condition":{"children":[{"kind":"media","left":{"name":"width","type":"Identifier"},"leftComparison":"=","middle":{"type":"Dimension","unit":"px","value":"600"},"right":null,"rightComparison":null,"type":"FeatureRange"}],"kind":"media","type":"Condition"},"mediaType":null,"modifier":null,"type":"MediaQuery"}`,
			generate: "(width=600px)",
		},
		{
			source:   "(600px = width)",
			context:  "mediaQuery",
			json:     `{"condition":{"children":[{"kind":"media","left":{"type":"Dimension","unit":"px","value":"600"},"leftComparison":"=","middle":{"name":"width","type":"Identifier"},"right":null,"rightComparison":null,"type":"FeatureRange"}],"kind":"media","type":"Condition"},"mediaType":null,"modifier":null,"type":"MediaQuery"}`,
			generate: "(600px=width)",
		},
		{
			source:   "@container (inline-size=30em){}",
			types:    "StyleSheet Atrule AtrulePrelude Condition FeatureRange Identifier Dimension Block",
			generate: "@container (inline-size=30em){}",
		},
		{
			source:   "(400px <= width < 700px)",
			context:  "mediaQuery",
			types:    "MediaQuery Condition FeatureRange Dimension Identifier Dimension",
			generate: "(400px<=width<700px)",
		},
		{
			source:   "(700px > width >= 400px)",
			context:  "mediaQuery",
			types:    "MediaQuery Condition FeatureRange Dimension Identifier Dimension",
			generate: "(700px>width>=400px)",
		},
		{
			source:   "(400px < width > 700px)",
			context:  "mediaQuery",
			types:    "MediaQuery Condition GeneralEnclosed Raw",
			generate: "(400px < width > 700px)",
			errors:   `Expected "<", "<=" or ")"; Unexpected input; Condition is expected`,
		},
		{
			source:   "(700px >= width <= 400px)",
			context:  "mediaQuery",
			types:    "MediaQuery Condition GeneralEnclosed Raw",
			generate: "(700px >= width <= 400px)",
			errors:   `Expected ">", ">=" or ")"; Unexpected input; Condition is expected`,
		},
		{
			source:   "(400px = width = 700px)",
			context:  "mediaQuery",
			types:    "MediaQuery Condition GeneralEnclosed Raw",
			generate: "(400px = width = 700px)",
			errors:   `")" is expected; Unexpected input; Condition is expected`,
		},
		{
			source:   "(400px < width = 700px)",
			context:  "mediaQuery",
			types:    "MediaQuery Condition GeneralEnclosed Raw",
			generate: "(400px < width = 700px)",
			errors:   `Expected "<", "<=" or ")"; Unexpected input; Condition is expected`,
		},
	})
}

// A condition does not mix and and or at one level, and not applies to a
// single term:
//
//	<media-condition> = <media-not> | <media-in-parens> [ <media-and>* | <media-or>* ]
//	<media-condition-without-or> = <media-not> | <media-in-parens> <media-and>*
//
// https://drafts.csswg.org/mediaqueries-4/#media, and the same grammar for
// https://drafts.csswg.org/css-conditional-3/#at-supports and
// https://drafts.csswg.org/css-contain-3/#container-rule.
func TestSpecConditionGrammar(t *testing.T) {
	runSpecParseTests(t, []specParseTest{
		{
			source:   "@media (a) and (b) or (c){}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@media (a) and (b) or (c){}",
			errors:   `Identifier "and" is expected`,
		},
		{
			source:   "@media (a) or (b) and (c){}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@media (a) or (b) and (c){}",
			errors:   `Identifier "or" is expected`,
		},
		{
			source:   "@media (a) and ((b) or (c)){}",
			types:    "StyleSheet Atrule AtrulePrelude MediaQueryList MediaQuery Condition Feature Identifier Condition Feature Identifier Feature Block",
			generate: "@media (a) and ((b) or (c)){}",
			errors:   "Number, dimension, ratio or identifier is expected",
		},
		{
			source:   "@media (a) AND (b) And (c){}",
			types:    "StyleSheet Atrule AtrulePrelude MediaQueryList MediaQuery Condition Feature Identifier Feature Identifier Feature Block",
			generate: "@media (a) AND (b) And (c){}",
		},
		{
			source:   "@media (a) or (b) OR (c){}",
			types:    "StyleSheet Atrule AtrulePrelude MediaQueryList MediaQuery Condition Feature Identifier Feature Identifier Feature Block",
			generate: "@media (a) or (b) OR (c){}",
		},
		{
			source:   "@media ((a) and (b) or (c)){}",
			types:    "StyleSheet Atrule AtrulePrelude MediaQueryList MediaQuery Condition GeneralEnclosed Parentheses Identifier Identifier Parentheses Identifier Identifier Parentheses Identifier Block",
			generate: "@media ((a) and (b) or (c)){}",
			errors:   `Number, dimension, ratio or identifier is expected; Identifier "and" is expected`,
		},
		{
			source:   "@media not (a){}",
			types:    "StyleSheet Atrule AtrulePrelude MediaQueryList MediaQuery Condition Identifier Feature Block",
			generate: "@media not (a){}",
		},
		{
			source:   "@media not (a) and (b){}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@media not (a) and (b){}",
			errors:   "Unexpected input",
		},
		{
			source:   "@media (a) and not (b){}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@media (a) and not (b){}",
			errors:   "Parenthesis or function is expected",
		},
		{
			source:   "@media (a) (b){}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@media (a) (b){}",
			errors:   `Identifier "and" or "or" is expected`,
		},
		{
			source:   "@media (a) and{}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@media (a) and{}",
			errors:   "Parenthesis or function is expected",
		},
		{
			source:   "@media screen and (a) and not (b){}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@media screen and (a) and not (b){}",
			errors:   "Parenthesis or function is expected",
		},
		{
			source:   "@media screen and not (a){}",
			types:    "StyleSheet Atrule AtrulePrelude MediaQueryList MediaQuery Condition Identifier Feature Block",
			generate: "@media screen and not (a){}",
		},
		{
			source:   "@media screen and (a) and (b){}",
			types:    "StyleSheet Atrule AtrulePrelude MediaQueryList MediaQuery Condition Feature Identifier Feature Block",
			generate: "@media screen and (a) and (b){}",
		},
		{
			// <media-condition-without-or>
			source:   "@media screen and (a) or (b){}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@media screen and (a) or (b){}",
			errors:   `Identifier "and" is expected`,
		},
		{
			source:   "@media (a), (b) or (c), not (d){}",
			types:    "StyleSheet Atrule AtrulePrelude MediaQueryList MediaQuery Condition Feature MediaQuery Condition Feature Identifier Feature MediaQuery Condition Identifier Feature Block",
			generate: "@media (a),(b) or (c),not (d){}",
		},
		{
			source:   "@supports (a:b) and (c:d) or (e:f){}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@supports (a:b) and (c:d) or (e:f){}",
			errors:   `Identifier "and" is expected`,
		},
		{
			source:   "@supports not (a:b) or (c:d){}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@supports not (a:b) or (c:d){}",
			errors:   "Unexpected input",
		},
		{
			source:   "@supports (a:b) or (not (c:d)) or selector(a){}",
			types:    "StyleSheet Atrule AtrulePrelude Condition SupportsDeclaration Declaration Value Identifier Identifier Condition Identifier SupportsDeclaration Declaration Value Identifier Identifier FeatureFunction Selector TypeSelector Block",
			generate: "@supports (a:b) or (not (c:d)) or selector(a){}",
			errors:   "Colon is expected",
		},
		{
			source:   "@container (width>1px) and (height>1px) or (x){}",
			types:    "StyleSheet Atrule Raw Block",
			generate: "@container (width>1px) and (height>1px) or (x){}",
			errors:   `Identifier "and" is expected`,
		},
		{
			source:   "@container card (width>1px) or style(--a:b){}",
			types:    "StyleSheet Atrule AtrulePrelude Identifier Condition FeatureRange Identifier Dimension Identifier FeatureFunction Declaration Raw Block",
			generate: "@container card (width>1px) or style(--a:b){}",
		},
	})
}

// CSS Nesting: a style block holds declarations and nested style rules.
// CSS Syntax 3 "consume a block's contents" tries a declaration first and
// parses a qualified rule when that is not a declaration: an identifier
// (or a property hack), a colon and a value that is not a {}-block mixed
// with anything else; a custom property is always a declaration.
// https://drafts.csswg.org/css-syntax-3/#consume-block-contents
// https://drafts.csswg.org/css-nesting-1/#syntax
func TestSpecNestedRules(t *testing.T) {
	const (
		ruleA       = "StyleSheet Rule SelectorList Selector ClassSelector Block "
		nestedClass = "Rule SelectorList Selector ClassSelector Block"
	)
	runSpecParseTests(t, []specParseTest{
		{
			source:   ".a{.b{}}",
			types:    ruleA + nestedClass,
			generate: ".a{.b{}}",
		},
		{
			source:   ".a{.b &{}}",
			types:    ruleA + "Rule SelectorList Selector ClassSelector Combinator NestingSelector Block",
			generate: ".a{.b &{}}",
		},
		{
			source:   ".a{> .b{}}",
			context:  "rule",
			json:     `{"block":{"children":[{"block":{"children":[],"type":"Block"},"prelude":{"children":[{"children":[{"name":">","type":"Combinator"},{"name":"b","type":"ClassSelector"}],"type":"Selector"}],"type":"SelectorList"},"type":"Rule"}],"type":"Block"},"prelude":{"children":[{"children":[{"name":"a","type":"ClassSelector"}],"type":"Selector"}],"type":"SelectorList"},"type":"Rule"}`,
			generate: ".a{>.b{}}",
		},
		{
			source:   ".a{+ .b{}}",
			types:    ruleA + "Rule SelectorList Selector Combinator ClassSelector Block",
			generate: ".a{+.b{}}",
		},
		{
			source:   ".a{~ .b{}}",
			types:    ruleA + "Rule SelectorList Selector Combinator ClassSelector Block",
			generate: ".a{~.b{}}",
		},
		{
			source:   ".a{>.b,+.c{}}",
			types:    ruleA + "Rule SelectorList Selector Combinator ClassSelector Selector Combinator ClassSelector Block",
			generate: ".a{>.b,+.c{}}",
		},
		{
			source:   ".a{color:red; .b{} color:blue}",
			types:    ruleA + "Declaration Value Identifier " + nestedClass + " Declaration Value Identifier",
			generate: ".a{color:red;.b{}color:blue}",
		},
		{
			source:   ".a{:hover{}}",
			types:    ruleA + "Rule SelectorList Selector PseudoClassSelector Block",
			generate: ".a{:hover{}}",
		},
		{
			source:   ".a{div{}}",
			types:    ruleA + "Rule SelectorList Selector TypeSelector Block",
			generate: ".a{div{}}",
		},
		{
			// a {}-block and anything else is not a declaration value
			source:   ".a{div:hover{color:red}}",
			types:    ruleA + "Rule SelectorList Selector TypeSelector PseudoClassSelector Block Declaration Value Identifier",
			generate: ".a{div:hover{color:red}}",
		},
		{
			source:   ".a{font:bold{}}",
			types:    ruleA + "Rule SelectorList Selector TypeSelector PseudoClassSelector Block",
			generate: ".a{font:bold{}}",
		},
		{
			source:   ".a{div .b{} p{}}",
			types:    ruleA + "Rule SelectorList Selector TypeSelector Combinator ClassSelector Block Rule SelectorList Selector TypeSelector Block",
			generate: ".a{div .b{}p{}}",
		},
		{
			source:   ".a{#b{}}",
			types:    ruleA + "Rule SelectorList Selector IdSelector Block",
			generate: ".a{#b{}}",
		},
		{
			source:   ".a{*{}}",
			types:    ruleA + "Rule SelectorList Selector TypeSelector Block",
			generate: ".a{*{}}",
		},
		{
			source:   ".a{[x]{}}",
			types:    ruleA + "Rule SelectorList Selector AttributeSelector Identifier Block",
			generate: ".a{[x]{}}",
		},
		{
			source:   ".a{.b{.c{color:red}}}",
			types:    ruleA + "Rule SelectorList Selector ClassSelector Block Rule SelectorList Selector ClassSelector Block Declaration Value Identifier",
			generate: ".a{.b{.c{color:red}}}",
		},
		{
			// a custom property is a declaration, whatever its value
			source:   ".a{--x:{a:b}}",
			types:    ruleA + "Declaration Raw",
			generate: ".a{--x:{a:b}}",
		},
		{
			source:   ".a{--x:hover{}}",
			types:    ruleA + "Declaration Raw",
			generate: ".a{--x:hover{}}",
		},
		{
			// a {}-block as the whole value is a declaration
			source:   ".a{a:{}}",
			types:    ruleA + "Declaration Raw",
			generate: ".a{a:{}}",
			errors:   "Unexpected input",
		},
		{
			source:   ".a{a:{} !important}",
			types:    ruleA + "Declaration Raw",
			generate: ".a{a:{}!important}",
			errors:   "Unexpected input",
		},
		{
			// property hacks are still declarations
			source:   ".a{*zoom:1;_b:2;#c:3;+d:4}",
			types:    ruleA + "Declaration Value Number Declaration Value Number Declaration Value Number Declaration Value Number",
			generate: ".a{*zoom:1;_b:2;#c:3;+d:4}",
		},
		{
			source:   ".a{color:red}",
			types:    ruleA + "Declaration Value Identifier",
			generate: ".a{color:red}",
		},
		{
			// neither a declaration nor a rule (no {} before the ;)
			source:   ".a{foo bar;color:red}",
			types:    ruleA + "Raw Declaration Value Identifier",
			generate: ".a{foo bar;color:red}",
			errors:   "Colon is expected",
		},
		{
			source:   ".a{.b;color:red}",
			types:    ruleA + "Raw Declaration Value Identifier",
			generate: ".a{.b;color:red}",
			errors:   "Identifier is expected",
		},
		{
			source:   ".a{@media (x){color:red}}",
			types:    ruleA + "Atrule AtrulePrelude MediaQueryList MediaQuery Condition Feature Block Declaration Value Identifier",
			generate: ".a{@media (x){color:red}}",
		},
		{
			source:   ".a{@media (x){.b{}}}",
			types:    ruleA + "Atrule AtrulePrelude MediaQueryList MediaQuery Condition Feature Block " + nestedClass,
			generate: ".a{@media (x){.b{}}}",
		},
		{
			source:   ".a{&:hover{}}",
			types:    ruleA + "Rule SelectorList Selector NestingSelector PseudoClassSelector Block",
			generate: ".a{&:hover{}}",
		},
		{
			// top-level @media blocks hold rules, as before
			source:   "@media (x){.b{}}",
			types:    "StyleSheet Atrule AtrulePrelude MediaQueryList MediaQuery Condition Feature Block " + nestedClass,
			generate: "@media (x){.b{}}",
		},
		{
			// descriptor blocks hold no style rules: upstream's parse
			source:   "@font-face{a:b {}}",
			types:    "StyleSheet Atrule Block Declaration Raw",
			generate: "@font-face{a:b {}}",
			errors:   "Unexpected input",
		},
		{
			source:   "@page{.b{}color:red}",
			types:    "StyleSheet Atrule Block Raw",
			generate: "@page{.b{}color:red}",
			errors:   "Identifier is expected",
		},
		{
			source:   ".b{} color:red",
			context:  "declarationList",
			types:    "DeclarationList " + nestedClass + " Declaration Value Identifier",
			generate: ".b{}color:red",
		},
		{
			source:   ".bar & { color: green; }; div:hover { color: red; }",
			context:  "declarationList",
			types:    "DeclarationList Rule SelectorList Selector ClassSelector Combinator NestingSelector Block Declaration Value Identifier Rule SelectorList Selector TypeSelector PseudoClassSelector Block Declaration Value Identifier",
			generate: ".bar &{color:green}div:hover{color:red}",
		},
	})
}
