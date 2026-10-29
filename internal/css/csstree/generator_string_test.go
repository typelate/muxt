package csstree

import (
	"reflect"
	"testing"
)

func TestIsClosedString(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  bool
	}{
		{`"a"`, true},
		{`'a'`, true},
		{`""`, true},
		{`"`, false},
		{`"a`, false},
		{`"a'`, false},
		{`"a\"`, false},
		{`"a\\"`, true},
		{`"a\\\"`, false},
	} {
		if got := isClosedString(tt.value); got != tt.want {
			t.Errorf("isClosedString(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

// A Raw value that ends inside a string must not swallow what the generator
// writes after it (upstream issue #93 after CSS Syntax 4.3.5 bad-strings).
func TestGenerateOpenStringRoundTrip(t *testing.T) {
	for _, source := range []string{
		".a{color:red\"\n}.b{color:blue}",
		".a{color:red'\n}.b{color:blue}",
		".a{color:red\"\\\n}.b{color:blue}", // \ newline continues the string to EOF
	} {
		ast, err := Parse(source, ParseOptions{})
		if err != nil {
			t.Fatal(err)
		}
		css, err := Generate(ast, GenerateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		again, err := Parse(css, ParseOptions{})
		if err != nil {
			t.Fatal(err)
		}
		want, err := ToPlainObject(ast)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ToPlainObject(again)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Parse(Generate(Parse(%q))) = %v, want %v (generated %q)", source, got, want, css)
		}
	}
}
