// Ported from lib/__tests/generate.js.

package csstree

import (
	"reflect"
	"strings"
	"testing"
)

// unknownTestNode is upstream's { type: 'xxx' }.
type unknownTestNode struct{}

func (unknownTestNode) Type() string        { return "xxx" }
func (unknownTestNode) Location() *Location { return nil }

func createGenerateTests(t *testing.T, test astFixtureTest) {
	expected := test.Source
	if test.Generate != nil {
		expected = *test.Generate
	}

	t.Run("generate", func(t *testing.T) {
		if test.Skip {
			t.Skip("skipped by fixture")
		}

		ast, err := Parse(test.Source, test.ParseOptions(false))
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", test.Source, err)
		}
		actual, err := Generate(ast, GenerateOptions{})
		if err != nil {
			t.Fatalf("Generate(Parse(%q)) error = %v", test.Source, err)
		}

		// strings should be equal
		if actual != expected {
			t.Errorf("Generate(Parse(%q)) = %q, want %q", test.Source, actual, expected)
		}
	})

	t.Run("(plain object)", func(t *testing.T) {
		if test.Skip {
			t.Skip("skipped by fixture")
		}

		ast, err := Parse(test.Source, test.ParseOptions(false))
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", test.Source, err)
		}
		// Go's Generate takes a typed AST, so the plain object is
		// converted back with FromPlainObject
		plainObject, err := ToPlainObject(ast)
		if err != nil {
			t.Fatalf("ToPlainObject(Parse(%q)) error = %v", test.Source, err)
		}
		converted, err := FromPlainObject(plainObject)
		if err != nil {
			t.Fatalf("FromPlainObject(ToPlainObject(Parse(%q))) error = %v", test.Source, err)
		}
		actual, err := Generate(converted, GenerateOptions{})
		if err != nil {
			t.Fatalf("Generate(FromPlainObject(ToPlainObject(Parse(%q)))) error = %v", test.Source, err)
		}

		// strings should be equal
		if actual != expected {
			t.Errorf("Generate(FromPlainObject(ToPlainObject(Parse(%q)))) = %q, want %q", test.Source, actual, expected)
		}
	})

	t.Run("(round-trip)", func(t *testing.T) {
		if test.Skip || test.SkipRoundtrip {
			t.Skip("skipped by fixture")
		}

		expectedAST, err := Parse(test.Source, test.ParseOptions(false))
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", test.Source, err)
		}
		generated, err := Generate(expectedAST, GenerateOptions{})
		if err != nil {
			t.Fatalf("Generate(Parse(%q)) error = %v", test.Source, err)
		}
		actualAST, err := Parse(generated, test.ParseOptions(false))
		if err != nil {
			t.Fatalf("Parse(%q) error = %v", generated, err)
		}

		// https://drafts.csswg.org/css-syntax/#serialization
		// The only requirement for serialization is that it must "round-trip" with parsing,
		// that is, parsing the stylesheet must produce the same data structures as parsing,
		// serializing, and parsing again, except for consecutive <whitespace-token>s,
		// which may be collapsed into a single token.
		got := toPlainJSON(t, actualAST)
		want := toPlainJSON(t, expectedAST)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(Generate(Parse(%q))) = %v, want %v", test.Source, got, want)
		}
	})
}

func createGenerateWithSourceMapTest(t *testing.T, test astFixtureTest) {
	if test.Skip {
		t.Skip("skipped by fixture")
	}

	options := test.ParseOptions(false)
	options.Positions = true
	ast, err := Parse(test.Source, options)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v", test.Source, err)
	}

	actual, err := Generate(ast, GenerateOptions{SourceMap: new(SourceMapGenerator)})
	if err != nil {
		t.Fatalf("Generate(Parse(%q), {sourceMap: true}) error = %v", test.Source, err)
	}

	// strings should be equal
	expected := test.Source
	if test.Generate != nil {
		expected = *test.Generate
	}
	if actual != expected {
		t.Errorf("Generate(Parse(%q), {sourceMap: true}) = %q, want %q", test.Source, actual, expected)
	}
}

func TestGenerate(t *testing.T) {
	forEachASTTest(t, false, createGenerateTests)

	t.Run("should throws on unknown node type", func(t *testing.T) {
		_, err := Generate(unknownTestNode{}, GenerateOptions{})
		if err == nil || !strings.Contains(err.Error(), "Unknown node type") {
			t.Errorf("Generate({type: 'xxx'}) error = %v, want Unknown node type", err)
		}
	})

	t.Run("sourceMap", func(t *testing.T) {
		forEachASTTest(t, false, createGenerateWithSourceMapTest)

		t.Run("should generate a map", func(t *testing.T) {
			ast, err := Parse(".a {\n  color: red;\n}\n", ParseOptions{
				Filename:  "test.css",
				Positions: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			sourceMap := new(SourceMapGenerator)
			css, err := Generate(ast, GenerateOptions{SourceMap: sourceMap})
			if err != nil {
				t.Fatal(err)
			}

			if want := ".a{color:red}"; css != want {
				t.Errorf("Generate() css = %q, want %q", css, want)
			}
			if want := `{"version":3,"sources":["test.css"],"names":[],"mappings":"AAAA,E,CACE,S"}`; sourceMap.String() != want {
				t.Errorf("Generate() map = %s, want %s", sourceMap, want)
			}
		})

		t.Run("complex CSS", func(t *testing.T) {
			ast, err := Parse(".a { color: #ff0000; } .b { display: block; float: left; } @media foo { .c { color: red } }", ParseOptions{
				Filename:  "test.css",
				Positions: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			sourceMap := new(SourceMapGenerator)
			css, err := Generate(ast, GenerateOptions{SourceMap: sourceMap})
			if err != nil {
				t.Fatal(err)
			}

			if want := ".a{color:#ff0000}.b{display:block;float:left}@media foo{.c{color:red}}"; css != want {
				t.Errorf("Generate() css = %q, want %q", css, want)
			}
			if want := `{"version":3,"sources":["test.css"],"names":[],"mappings":"AAAA,E,CAAK,a,CAAkB,E,CAAK,a,CAAgB,U,CAAe,WAAa,E,CAAK,W"}`; sourceMap.String() != want {
				t.Errorf("Generate() map = %s, want %s", sourceMap, want)
			}
		})
	})

	t.Run("spec mode", func(t *testing.T) {
		for _, fixture := range generateAutoEmitWSFixture() {
			leftValue := fixture.Left.Value
			rightValue := fixture.Right.Value
			expectedNoWs := leftValue + rightValue
			expectedWs := leftValue + " " + rightValue

			t.Run(fixture.ID, func(t *testing.T) {
				children := new(List[Node])
				children.AppendData(&Raw{Value: leftValue})
				children.AppendData(&Raw{Value: rightValue})
				actual, err := Generate(&Value{Children: children}, GenerateOptions{Mode: "spec"})
				if err != nil {
					t.Fatal(err)
				}

				if fixture.WS {
					if actual != expectedWs {
						t.Errorf("Generate(%q %q) = %q, want %q", leftValue, rightValue, actual, expectedWs)
					}
				} else {
					if actual != expectedNoWs && actual != expectedWs {
						t.Errorf("Generate(%q %q) = %q, want %q", leftValue, rightValue, actual, expectedWs)
					}
				}
			})
		}
	})

	t.Run("auto-emit whitespace", func(t *testing.T) {
		t.Run("should auto insert whitespaces where necessary", func(t *testing.T) {
			const source = "span#foo { border: 1%var(--a)#ff0000; }"
			ast, err := Parse(source, ParseOptions{})
			if err != nil {
				t.Fatal(err)
			}

			actual, err := Generate(ast, GenerateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if want := "span#foo{border:1% var(--a) #ff0000}"; actual != want {
				t.Errorf("Generate(Parse(%q)) = %q, want %q", source, actual, want)
			}
		})

		t.Run("should not insert whitespaces for values passed to tokenize() by default", func(t *testing.T) {
			const source = "span#foo { border: 1%var(--a)#ff0000; }"
			ast, err := Parse(source, ParseOptions{
				NoParseRulePrelude: true,
				NoParseValue:       true,
			})
			if err != nil {
				t.Fatal(err)
			}

			actual, err := Generate(ast, GenerateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if want := "span#foo{border:1%var(--a)#ff0000}"; actual != want {
				t.Errorf("Generate(Parse(%q)) = %q, want %q", source, actual, want)
			}
		})
	})
}
