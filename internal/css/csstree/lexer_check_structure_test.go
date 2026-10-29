// Ported from lib/__tests/lexer-check-structure.js.

package csstree

import (
	"reflect"
	"regexp"
	"testing"
)

func assertStructureWarnings(t *testing.T, node map[string]any, got []StructureWarning, messages ...string) {
	t.Helper()
	var want []StructureWarning
	for _, message := range messages {
		want = append(want, StructureWarning{Node: node, Message: message})
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CheckPlainObjectStructure(%v) = %v, want %v", node, got, want)
	}
}

func TestLexerCheckStructure(t *testing.T) {
	lexer := DefaultLexer()

	t.Run("structure in AST node definition", func(t *testing.T) {
		t.Run("should fail when no structure field in node definition", func(t *testing.T) {
			_, err := Fork(ExtendFunc(func(prev *Config) *Config {
				prev.Node["Test"] = NodeConfig{}
				return prev
			}))

			pattern := regexp.MustCompile("Missed `structure` field in `Test` node type definition")
			if err == nil || !pattern.MatchString(err.Error()) {
				t.Errorf("Fork() error = %v, want %v", err, pattern)
			}
		})

		t.Run("should fail on bad value in structure", func(t *testing.T) {
			_, err := Fork(ExtendFunc(func(prev *Config) *Config {
				prev.Node["Test"] = NodeConfig{
					Structure: []StructureField{
						{Name: "foo", Types: []FieldType{{Kind: 123}}},
					},
				}
				return prev
			}))

			pattern := regexp.MustCompile("Wrong value `123` in `Test\\.foo` structure definition")
			if err == nil || !pattern.MatchString(err.Error()) {
				t.Errorf("Fork() error = %v, want %v", err, pattern)
			}
		})
	})

	t.Run("should pass correct structure", func(t *testing.T) {
		ast := mustParse(t, ".foo { color: red }", ParseOptions{Positions: true})
		warns, err := lexer.CheckStructure(ast)
		if err != nil {
			t.Fatal(err)
		}

		if warns != nil {
			t.Errorf("CheckStructure() = %v, want nil", warns)
		}
	})

	t.Run("should ignore properties from prototype", func(t *testing.T) {
		t.Skip("not ported: Go maps have no prototype to extend")
	})

	t.Run("all parse test fixtures must have correct structure", func(t *testing.T) {
		forEachASTTest(t, false, func(t *testing.T, test astFixtureTest) {
			if test.Skip {
				t.Skip("skipped by fixture")
			}

			ast, err := Parse(test.Source, test.ParseOptions(false))
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", test.Source, err)
			}

			// structure should be correct
			warns, err := lexer.CheckStructure(ast)
			if err != nil {
				t.Fatal(err)
			}
			if warns != nil {
				t.Errorf("CheckStructure(Parse(%q)) = %v, want nil", test.Source, warns)
			}
		})
	})

	t.Run("errors", func(t *testing.T) {
		t.Run("node should be an object", func(t *testing.T) {
			t.Skip("not ported: a plain object node is a map; an array with a type property cannot be built in Go")
		})

		t.Run("missed fields", func(t *testing.T) {
			node := map[string]any{
				"type": "Foo",
			}

			assertStructureWarnings(t, node, lexer.CheckPlainObjectStructure(node),
				"Unknown node type `Foo`",
			)
		})

		t.Run("missed field", func(t *testing.T) {
			node := map[string]any{
				"type":  "Dimension",
				"value": "123",
			}

			assertStructureWarnings(t, node, lexer.CheckPlainObjectStructure(node),
				"Field `Dimension.loc` is missed",
				"Field `Dimension.unit` is missed",
			)
		})

		t.Run("unknown field", func(t *testing.T) {
			node := map[string]any{
				"type":  "Number",
				"loc":   nil,
				"value": "123",
				"foo":   1,
			}

			assertStructureWarnings(t, node, lexer.CheckPlainObjectStructure(node),
				"Unknown field `foo` for Number node type",
			)
		})

		t.Run("bad value", func(t *testing.T) {
			t.Run("bad data type", func(t *testing.T) {
				node := map[string]any{
					"type":  "Number",
					"loc":   nil,
					"value": 123,
				}

				assertStructureWarnings(t, node, lexer.CheckPlainObjectStructure(node),
					"Bad value for `Number.value`",
				)
			})

			t.Run("bad loc", func(t *testing.T) {
				node := map[string]any{
					"type":  "Number",
					"loc":   map[string]any{},
					"value": "123",
				}

				assertStructureWarnings(t, node, lexer.CheckPlainObjectStructure(node),
					"Bad value for `Number.loc.source`",
				)
			})

			t.Run("bad loc #2", func(t *testing.T) {
				node := map[string]any{
					"type": "Number",
					"loc": map[string]any{
						"source": "-",
						"start":  map[string]any{"line": 1, "column": 1},
						"end":    map[string]any{"line": 1, "column": 1, "offset": 0},
					},
					"value": "123",
				}

				assertStructureWarnings(t, node, lexer.CheckPlainObjectStructure(node),
					"Bad value for `Number.loc.start`",
				)
			})

			t.Run("bad loc #3", func(t *testing.T) {
				node := map[string]any{
					"type": "Number",
					"loc": map[string]any{
						"source": "-",
						"start":  map[string]any{"line": 1, "column": 1, "offset": 0},
						"end":    map[string]any{"line": 1, "column": 1},
					},
					"value": "123",
				}

				assertStructureWarnings(t, node, lexer.CheckPlainObjectStructure(node),
					"Bad value for `Number.loc.end`",
				)
			})
		})
	})
}
