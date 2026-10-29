// Go only: upstream has no tests for lib/syntax/config/mix.js beyond the
// fork tests of common.js.

package csstree

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestMix(t *testing.T) {
	t.Run("generic is mixed only when set", func(t *testing.T) {
		base := &Config{Generic: new(true)}

		if result := mix(base, &Config{}); result.Generic == nil || !*result.Generic {
			t.Errorf("mix(generic: true, {}).Generic = %v, want true", result.Generic)
		}
		if result := mix(base, &Config{Generic: new(false)}); result.Generic == nil || *result.Generic {
			t.Errorf("mix(generic: true, {generic: false}).Generic = %v, want false", result.Generic)
		}
	})

	t.Run("appendOrSet", func(t *testing.T) {
		for _, test := range []struct {
			a, b, want string
		}{
			{"", "a", "a"},
			{"a", "b", "b"},
			{"a", "| b", "a| b"},
			{"", "  |  b", "b"},
			{"a", "", ""},
		} {
			if got := appendOrSet(test.a, test.b); got != test.want {
				t.Errorf("appendOrSet(%q, %q) = %q, want %q", test.a, test.b, got, test.want)
			}
		}
	})

	t.Run("lexer keys", func(t *testing.T) {
		dest := &Config{
			CSSWideKeywords: []string{"inherit"},
			Units:           map[string][]string{"length": {"px"}},
			Types:           map[string]string{"a": "<b>"},
			Properties:      map[string]string{"color": "<color>"},
			Atrules: map[string]AtruleDefinition{
				"media": {Prelude: "<media-query-list>", Descriptors: map[string]string{"x": "<y>"}},
			},
		}
		result := mix(dest, &Config{
			CSSWideKeywords: []string{"unset"},
			Units:           map[string][]string{"angle": nil},
			Types:           map[string]string{"a": "| <c>", "d": "<e>"},
			Properties:      map[string]string{"color": "red"},
			Atrules: map[string]AtruleDefinition{
				"media": {Prelude: "| <x>", Descriptors: map[string]string{"x": "| <z>", "w": ""}},
				"page":  {},
			},
		})

		if want := []string{"inherit", "unset"}; !slices.Equal(result.CSSWideKeywords, want) {
			t.Errorf("CSSWideKeywords = %v, want %v", result.CSSWideKeywords, want)
		}
		if want := map[string][]string{"length": {"px"}, "angle": {}}; !reflect.DeepEqual(result.Units, want) {
			t.Errorf("Units = %v, want %v", result.Units, want)
		}
		if want := map[string]string{"a": "<b>| <c>", "d": "<e>"}; !maps.Equal(result.Types, want) {
			t.Errorf("Types = %v, want %v", result.Types, want)
		}
		if want := map[string]string{"color": "red"}; !maps.Equal(result.Properties, want) {
			t.Errorf("Properties = %v, want %v", result.Properties, want)
		}
		wantAtrules := map[string]AtruleDefinition{
			"media": {Prelude: "<media-query-list>| <x>", Descriptors: map[string]string{"x": "<y>| <z>", "w": ""}},
			"page":  {},
		}
		if !reflect.DeepEqual(result.Atrules, wantAtrules) {
			t.Errorf("Atrules = %v, want %v", result.Atrules, wantAtrules)
		}
		if want := map[string]string{"a": "<b>"}; !maps.Equal(dest.Types, want) {
			t.Errorf("dest.Types = %v, want unchanged %v", dest.Types, want)
		}
	})

	t.Run("node fields are merged", func(t *testing.T) {
		base := nodeConfigs()["Dimension"]
		generate := func(*Generator, Node) {}
		result := mix(&Config{Node: map[string]NodeConfig{"Dimension": base}}, &Config{
			Node: map[string]NodeConfig{"Dimension": {Generate: generate}},
		})

		merged := result.Node["Dimension"]
		if merged.Name != "Dimension" || merged.Structure == nil || merged.Clone == nil || merged.FromPlainObject == nil || merged.Generate == nil {
			t.Errorf("mix(Dimension, {generate}).Node[Dimension] = %+v, want base fields kept", merged)
		}
	})

	t.Run("fork copies scopes", func(t *testing.T) {
		base := DefaultSyntax()
		getNode := base.config.Scope["Value"].GetNode
		if _, err := base.Fork(ExtendFunc(func(config *Config) *Config {
			config.Scope["Value"].GetNode = nil
			config.Scope["Value"].Functions["var"] = nil
			return config
		})); err != nil {
			t.Fatal(err)
		}
		if base.config.Scope["Value"].GetNode == nil || getNode == nil {
			t.Error("Fork() changed the scope of the base syntax")
		}
		if base.config.Scope["Value"].Functions["var"] == nil {
			t.Error("Fork() changed the scope functions of the base syntax")
		}
	})

	t.Run("fork without structure fails", func(t *testing.T) {
		_, err := Fork(&Config{Node: map[string]NodeConfig{"Foo": {}}})
		if want := "Missed `structure` field in `Foo` node type definition"; err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Fork({node: {Foo: {}}}) error = %v, want %q", err, want)
		}
	})

	t.Run("fork with nil extension", func(t *testing.T) {
		syntax, err := Fork(nil)
		if err != nil {
			t.Fatal(err)
		}
		ast, err := syntax.Parse("a{b:c}", ParseOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if css, err := syntax.Generate(ast, GenerateOptions{}); err != nil || css != "a{b:c}" {
			t.Errorf("Fork(nil).Generate(Parse(a{b:c})) = %q, %v, want %q", css, err, "a{b:c}")
		}
	})
}
