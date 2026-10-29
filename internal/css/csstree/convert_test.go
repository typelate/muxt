// Ported from lib/__tests/convert.js.

package csstree

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestConvert(t *testing.T) {
	const css = ".test{a:123}"

	t.Run("convert", func(t *testing.T) {
		t.Run("fromPlainObject", func(t *testing.T) {
			ast := mustParse(t, css, ParseOptions{})
			data, err := json.Marshal(ast)
			if err != nil {
				t.Fatal(err)
			}
			var plainObject map[string]any
			if err := json.Unmarshal(data, &plainObject); err != nil {
				t.Fatal(err)
			}

			converted, err := FromPlainObject(plainObject)
			if err != nil {
				t.Fatalf("FromPlainObject(%s) error = %v", data, err)
			}
			actual, err := Generate(converted, GenerateOptions{})
			if err != nil {
				t.Fatal(err)
			}

			if actual != css {
				t.Errorf("Generate(FromPlainObject(%s)) = %q, want %q", data, actual, css)
			}
		})

		t.Run("toPlainObject", func(t *testing.T) {
			ast := mustParse(t, css, ParseOptions{})
			expected := toPlainJSON(t, ast)
			actual, err := ToPlainObject(ast)
			if err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(any(actual), expected) {
				t.Errorf("ToPlainObject(Parse(%q)) = %v, want %v", css, actual, expected)
			}
		})

		// Go only: FromPlainObject keeps locations and rejects what it
		// cannot build.
		t.Run("fromPlainObject keeps locations", func(t *testing.T) {
			ast := mustParse(t, css, ParseOptions{Positions: true, Filename: "test.css"})
			plainObject, err := ToPlainObject(ast)
			if err != nil {
				t.Fatal(err)
			}
			converted, err := FromPlainObject(plainObject)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := toPlainJSON(t, converted), toPlainJSON(t, ast); !reflect.DeepEqual(got, want) {
				t.Errorf("FromPlainObject(ToPlainObject(ast)) = %v, want %v", got, want)
			}
		})

		t.Run("fromPlainObject errors", func(t *testing.T) {
			for _, test := range []struct {
				name   string
				object string
			}{
				{"unknown type", `{"type":"xxx"}`},
				{"no type", `{"children":[]}`},
				{"string field", `{"type":"Identifier","name":1}`},
				{"list field", `{"type":"Value","children":{}}`},
				{"list item", `{"type":"Value","children":[1]}`},
				{"node field", `{"type":"Rule","prelude":"a","block":null}`},
				{"node type", `{"type":"Rule","prelude":null,"block":{"type":"Raw","value":""}}`},
				{"location", `{"type":"Raw","loc":{"source":"a","start":{"offset":0.5,"line":1,"column":1},"end":{"offset":0,"line":1,"column":1}},"value":""}`},
				{"important", `{"type":"Declaration","important":1,"property":"a","value":null}`},
			} {
				t.Run(test.name, func(t *testing.T) {
					var plainObject map[string]any
					if err := json.Unmarshal([]byte(test.object), &plainObject); err != nil {
						t.Fatal(err)
					}
					if node, err := FromPlainObject(plainObject); err == nil {
						t.Errorf("FromPlainObject(%s) = %v, want an error", test.object, node)
					}
				})
			}
		})
	})
}
