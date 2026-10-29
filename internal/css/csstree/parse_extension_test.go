// Ported from lib/__tests/parse-extension.js.

package csstree

import (
	"reflect"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// percentageSign is U+0025 PERCENTAGE SIGN (%). Upstream's DollarSign is
// dollarSign of node_attribute_selector.go.
const percentageSign = 0x0025

// variableTestNode is upstream's { type: 'Variable', loc, name }.
type variableTestNode struct {
	Loc  *Location
	Name string
}

func (n *variableTestNode) Type() string        { return "Variable" }
func (n *variableTestNode) Location() *Location { return n.Loc }

func (n *variableTestNode) MarshalJSON() ([]byte, error) {
	return marshalNode(n, jsonField{"name", n.Name})
}

// testTestNode is upstream's { type: 'Test', loc }.
type testTestNode struct {
	Loc *Location
}

func (n *testTestNode) Type() string        { return "Test" }
func (n *testTestNode) Location() *Location { return n.Loc }

func (n *testTestNode) MarshalJSON() ([]byte, error) {
	return marshalNode(n)
}

func assertParseError(t *testing.T, syntax *Syntax, source string, options ParseOptions, want string) {
	t.Helper()
	parse := Parse
	if syntax != nil {
		parse = syntax.Parse
	}
	if ast, err := parse(source, options); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Parse(%q) = %v, %v, want error %q", source, ast, err, want)
	}
}

func assertPlainObject(t *testing.T, syntax *Syntax, source string, options ParseOptions, expected map[string]any) {
	t.Helper()
	ast, err := syntax.Parse(source, options)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", source, err)
	}
	actual, err := ToPlainObject(ast)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("ToPlainObject(Parse(%q)) = %v, want %v", source, actual, expected)
	}
}

func TestParseExtension(t *testing.T) {
	t.Run("extension", func(t *testing.T) {
		t.Run("value", func(t *testing.T) {
			extended, err := Fork(ExtendFunc(func(syntaxConfig *Config) *Config {
				defaultGetNode := syntaxConfig.Scope["Value"].GetNode

				syntaxConfig.Scope["Value"].GetNode = func(p *Parser, context *SequenceContext) Node {
					if p.IsDelim(dollarSign, 0) {
						start := p.TokenStart
						p.Next()

						return &variableTestNode{
							Loc:  p.GetLocation(start, p.TokenEnd),
							Name: p.Consume(tokenizer.Ident),
						}
					}

					return defaultGetNode(p, context)
				}

				return syntaxConfig
			}))
			if err != nil {
				t.Fatal(err)
			}

			t.Run("should not affect base syntax", func(t *testing.T) {
				assertParseError(t, nil, "$a", ParseOptions{Context: "value"}, "Unexpected input")
			})

			t.Run("should parse according new rules", func(t *testing.T) {
				assertPlainObject(t, extended, "$a", ParseOptions{Context: "value"}, map[string]any{
					"type": "Value",
					"loc":  nil,
					"children": []any{
						map[string]any{
							"type": "Variable",
							"loc":  nil,
							"name": "a",
						},
					},
				})
			})

			t.Run("should fail on unknown", func(t *testing.T) {
				assertParseError(t, extended, "@a", ParseOptions{Context: "value"}, "Unexpected input")
			})
		})

		t.Run("selector", func(t *testing.T) {
			extended, err := Fork(ExtendFunc(func(syntaxConfig *Config) *Config {
				defaultGetNode := syntaxConfig.Scope["Selector"].GetNode

				syntaxConfig.Scope["Selector"].GetNode = func(p *Parser, context *SequenceContext) Node {
					if p.IsDelim(percentageSign, 0) {
						start := p.TokenStart
						p.Next()

						return &testTestNode{
							Loc: p.GetLocation(start, p.TokenEnd),
						}
					}

					return defaultGetNode(p, context)
				}

				return syntaxConfig
			}))
			if err != nil {
				t.Fatal(err)
			}

			t.Run("should not affect base syntax", func(t *testing.T) {
				assertParseError(t, nil, "a %", ParseOptions{Context: "selector"}, "Unexpected input")
			})

			t.Run("should parse according new rules", func(t *testing.T) {
				assertPlainObject(t, extended, "a %", ParseOptions{Context: "selector"}, map[string]any{
					"type": "Selector",
					"loc":  nil,
					"children": []any{
						map[string]any{
							"type": "TypeSelector",
							"loc":  nil,
							"name": "a",
						},
						map[string]any{
							"type": "Combinator",
							"loc":  nil,
							"name": " ",
						},
						map[string]any{
							"type": "Test",
							"loc":  nil,
						},
					},
				})
			})

			t.Run("should fail on unknown", func(t *testing.T) {
				assertParseError(t, extended, "@a", ParseOptions{Context: "selector"}, "Selector is expected")
			})
		})
	})
}
