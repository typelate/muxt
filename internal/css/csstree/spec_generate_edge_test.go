package csstree

import (
	"errors"
	"reflect"
	"testing"

	"github.com/typelate/muxt/internal/css/csstree/tokenizer"
)

// Expected values in this file are upstream's (csstree 3.2.1 with
// source-map-js).

func TestSourceMapSourcesFileAndContent(t *testing.T) {
	a := mustParse(t, "a{b:c}\n@media x{d{e:f}}", ParseOptions{Positions: true, Filename: "a.css"}).(*StyleSheet)
	b := mustParse(t, "g{h:i}", ParseOptions{Positions: true, Filename: "b\"\\\n\t\u0001\u001f\u007fé.css"}).(*StyleSheet)
	c := mustParse(t, "j{k:l}", ParseOptions{Positions: true, Filename: "a.css"}).(*StyleSheet)
	a.Children.AppendList(b.Children)
	a.Children.AppendList(c.Children)

	sourceMap := &SourceMapGenerator{File: "out\n.css"}
	css, err := Generate(a, GenerateOptions{SourceMap: sourceMap})
	if err != nil {
		t.Fatal(err)
	}
	sourceMap.SetSourceContent("a.css", "a{b:c}\n\"x\"")

	if want := "a{b:c}@media x{d{e:f}}g{h:i}j{k:l}"; css != want {
		t.Errorf("css = %q, want %q", css, want)
	}
	want := `{"version":3,"sources":["a.css","b\"\\\n\t\u0001\u001f` + "\u007f" + `é.css"],"names":[],"mappings":"AAAA,C,CAAE,G,CACF,SAAS,C,CAAE,KCDX,C,CAAE,G,CDAF,C,CAAE,G","file":"out\n.css","sourcesContent":["a{b:c}\n\"x\"",null]}`
	if got := sourceMap.String(); got != want {
		t.Errorf("map =\n%s\nwant\n%s", got, want)
	}
	data, err := sourceMap.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("MarshalJSON() = %s, want %s", data, want)
	}
}

func TestSourceMapGeneratorAddMapping(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		for _, tt := range []struct {
			mapping SourceMapping
			want    string
		}{
			{
				mapping: SourceMapping{Generated: SourceMapPosition{Line: 0, Column: 0}},
				want:    `Invalid mapping: {"generated":{"line":0,"column":0},"source":null,"original":null,"name":null}`,
			},
			{
				mapping: SourceMapping{Generated: SourceMapPosition{Line: 1, Column: -1}},
				want:    `Invalid mapping: {"generated":{"line":1,"column":-1},"source":null,"original":null,"name":null}`,
			},
			{
				mapping: SourceMapping{Generated: SourceMapPosition{Line: 1}, Source: "x"},
				want:    `Invalid mapping: {"generated":{"line":1,"column":0},"source":"x","original":null,"name":null}`,
			},
			{
				mapping: SourceMapping{Generated: SourceMapPosition{Line: 1}, Original: &SourceMapPosition{Line: 1}},
				want:    `Invalid mapping: {"generated":{"line":1,"column":0},"source":null,"original":{"line":1,"column":0},"name":null}`,
			},
			{
				mapping: SourceMapping{Generated: SourceMapPosition{Line: 1}, Original: &SourceMapPosition{Line: 0}, Source: "x"},
				want:    `Invalid mapping: {"generated":{"line":1,"column":0},"source":"x","original":{"line":0,"column":0},"name":null}`,
			},
			{
				mapping: SourceMapping{Generated: SourceMapPosition{Line: 1}, Original: &SourceMapPosition{Line: 1, Column: -1}, Source: "x", Name: `n"`},
				want:    `Invalid mapping: {"generated":{"line":1,"column":0},"source":"x","original":{"line":1,"column":-1},"name":"n\""}`,
			},
			{
				mapping: SourceMapping{Generated: SourceMapPosition{Line: 1}, Name: "n"},
				want:    `Invalid mapping: {"generated":{"line":1,"column":0},"source":null,"original":null,"name":"n"}`,
			},
		} {
			var g SourceMapGenerator
			err := g.AddMapping(tt.mapping)
			if err == nil || err.Error() != tt.want {
				t.Errorf("AddMapping(%+v) = %v, want %s", tt.mapping, err, tt.want)
			}
		}
	})

	t.Run("names, order and duplicates", func(t *testing.T) {
		var g SourceMapGenerator
		for _, m := range []SourceMapping{
			{Generated: SourceMapPosition{Line: 2, Column: 5}, Original: &SourceMapPosition{Line: 1, Column: 0}, Source: "z", Name: "n1"},
			{Generated: SourceMapPosition{Line: 1, Column: 3}, Original: &SourceMapPosition{Line: 3, Column: 7}, Source: "y", Name: "n2"},
			{Generated: SourceMapPosition{Line: 1, Column: 3}, Original: &SourceMapPosition{Line: 3, Column: 7}, Source: "y", Name: "n2"},
			{Generated: SourceMapPosition{Line: 1, Column: 3}, Original: &SourceMapPosition{Line: 3, Column: 7}, Source: "x"},
			{Generated: SourceMapPosition{Line: 1, Column: 3}},
			{Generated: SourceMapPosition{Line: 1, Column: 3}, Original: &SourceMapPosition{Line: 2, Column: 7}, Source: "y", Name: "n1"},
			{Generated: SourceMapPosition{Line: 1, Column: 3}, Original: &SourceMapPosition{Line: 3, Column: 6}, Source: "y"},
			{Generated: SourceMapPosition{Line: 1, Column: 1000}, Original: &SourceMapPosition{Line: 300, Column: 0}, Source: "y"},
		} {
			if err := g.AddMapping(m); err != nil {
				t.Fatal(err)
			}
		}
		g.SetSourceContent("y", "yy")

		want := `{"version":3,"sources":["z","y","x"],"names":["n1","n2"],"mappings":"GEEO,ADDAA,AACD,AAACC,A,q+BAySP;KD3SAD","sourcesContent":[null,"yy",null]}`
		if got := g.String(); got != want {
			t.Errorf("map =\n%s\nwant\n%s", got, want)
		}
	})
}

func TestGenerateTokens(t *testing.T) {
	value := func(children ...Node) Node {
		return &Value{Children: new(List[Node]).FromArray(children)}
	}
	for _, tt := range []struct {
		name string
		node Node
		mode string
		want string
	}{
		// a lone \ is followed by a newline so it cannot escape what comes next
		{name: "reverse solidus", node: value(&Raw{Value: `\`}, &Identifier{Name: "a"}), want: "\\\na"},
		{name: "escape", node: value(&Identifier{Name: "a"}, &Raw{Value: `\b`}), want: `a \b`},
		{name: "ident hash safe", node: value(&Identifier{Name: "a"}, &Hash{Value: "b"}), mode: "safe", want: "a #b"},
		{name: "ident hash spec", node: value(&Identifier{Name: "a"}, &Hash{Value: "b"}), mode: "spec", want: "a#b"},
		{name: "ident hash unknown mode", node: value(&Identifier{Name: "a"}, &Hash{Value: "b"}), mode: "bogus", want: "a #b"},
		{name: "number percentage safe", node: value(&Number{Value: "1"}, &Percentage{Value: "2"}), mode: "safe", want: "1 2%"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			css, err := Generate(tt.node, GenerateOptions{Mode: tt.mode})
			if err != nil {
				t.Fatal(err)
			}
			if css != tt.want {
				t.Errorf("Generate() = %q, want %q", css, tt.want)
			}
		})
	}
}

// A panic in a handler ends the generation with an error, as an exception
// thrown by an upstream handler reaches the caller of generate.
func TestGenerateHandlerPanics(t *testing.T) {
	css, err := Generate(&Identifier{Name: "a"}, GenerateOptions{Decorator: func(handlers GenerateHandlers) GenerateHandlers {
		handlers.Emit = func(*GenerateHandlers, string, tokenizer.TokenType, bool) { panic("boom") }
		return handlers
	}})
	if css != "" || err == nil || err.Error() != "boom" {
		t.Errorf("Generate() = %q, %v; want boom", css, err)
	}

	// upstream: reading the type of a null child throws
	_, err = Generate(&Value{Children: new(List[Node]).AppendData(nil)}, GenerateOptions{})
	var unknown *UnknownNodeTypeError
	if !errors.As(err, &unknown) || unknown.Type != "undefined" {
		t.Errorf("Generate(nil child) = %v, want *UnknownNodeTypeError", err)
	}
}

func TestEncodeStringAndURL(t *testing.T) {
	for _, tt := range []struct {
		in, quoted, apostrophe, url string
	}{
		{in: "\x00", quoted: `"` + "�" + `"`, apostrophe: `'` + "�" + `'`, url: "url(�)"},
		{in: `a'b"c\d`, quoted: `"a'b\"c\\d"`, apostrophe: `'a\'b"c\\d'`, url: `url(a\'b\"c\\d)`},
		{in: "\x01a", quoted: `"\1 a"`, apostrophe: `'\1 a'`, url: `url(\1 a)`},
		{in: "\x1f ", quoted: `"\1f  "`, apostrophe: `'\1f  '`, url: `url(\1f\ )`},
		{in: "\x7fF", quoted: `"\7f F"`, apostrophe: `'\7f F'`, url: `url(\7f F)`},
		{in: "é\x00😀", quoted: "\"é�😀\"", apostrophe: "'é�😀'", url: "url(é�😀)"},
	} {
		if got := EncodeString(tt.in, false); got != tt.quoted {
			t.Errorf("EncodeString(%q, false) = %q, want %q", tt.in, got, tt.quoted)
		}
		if got := EncodeString(tt.in, true); got != tt.apostrophe {
			t.Errorf("EncodeString(%q, true) = %q, want %q", tt.in, got, tt.apostrophe)
		}
		if got := EncodeURL(tt.in); got != tt.url {
			t.Errorf("EncodeURL(%q) = %q, want %q", tt.in, got, tt.url)
		}
	}
}

func TestAtruleDefinitionMarshalJSON(t *testing.T) {
	for _, tt := range []struct {
		definition AtruleDefinition
		want       string
	}{
		{definition: AtruleDefinition{}, want: `{"prelude":null,"descriptors":null}`},
		{
			definition: AtruleDefinition{Prelude: "<a> & <b>", Descriptors: map[string]string{"x": "<c>"}},
			want:       `{"prelude":"<a> & <b>","descriptors":{"x":"<c>"}}`,
		},
	} {
		data, err := tt.definition.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != tt.want {
			t.Errorf("MarshalJSON(%+v) = %s, want %s", tt.definition, data, tt.want)
		}
	}
}

// Clone copies every node, list and location of every AST fixture.
func TestCloneASTFixtures(t *testing.T) {
	forEachASTTest(t, false, func(t *testing.T, test astFixtureTest) {
		options := test.ParseOptions(false)
		options.Positions = true
		ast, err := Parse(test.Source, options)
		if err != nil {
			t.Fatal(err)
		}
		astCopy := Clone(ast)

		if got, want := toPlainJSON(t, astCopy), toPlainJSON(t, ast); !reflect.DeepEqual(got, want) {
			t.Fatalf("Clone() = %v, want %v", got, want)
		}

		originals := map[any]bool{}
		mustWalk(t, ast, enterOnly(func(node Node) {
			originals[node] = true
			if loc := node.Location(); loc != nil {
				originals[loc] = true
			}
		}))
		mustWalk(t, astCopy, enterOnly(func(node Node) {
			if originals[node] {
				t.Errorf("Clone() shares %s node %v", node.Type(), node)
			}
			if loc := node.Location(); loc != nil && originals[loc] {
				t.Errorf("Clone() shares the location of %s node", node.Type())
			}
		}))
	})
}
