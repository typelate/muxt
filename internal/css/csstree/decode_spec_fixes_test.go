package csstree

// The decoders read their input as CSS Syntax §3.3 preprocessing leaves it:
// U+0000 NULL and invalid UTF-8 are U+FFFD REPLACEMENT CHARACTER, escaped or
// not. Upstream keeps a NUL as is, and does not treat "\" before a NUL as an
// escape.

import "testing"

func TestDecodeSpecFixes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		decode func(string) string
		str    string
		want   string
	}{
		{"DecodeIdent", DecodeIdent, "a\x00b", "a\uFFFDb"},   // upstream "a\x00b"
		{"DecodeIdent", DecodeIdent, "a\\\x00b", "a\uFFFDb"}, // upstream "ab"
		{"DecodeIdent", DecodeIdent, "\\\x00", "\uFFFD"},     // upstream ""
		{"DecodeIdent", DecodeIdent, "a\xffb", "a\uFFFDb"},
		{"DecodeIdent", DecodeIdent, "a\xe2\x82b", "a\uFFFDb"},
		{"DecodeIdent", DecodeIdent, "\xed\xa0\x80", "\uFFFD\uFFFD\uFFFD"},
		{"DecodeIdent", DecodeIdent, "\\\xff", "\uFFFD"},
		{"DecodeIdent", DecodeIdent, "\\\xe2\x82a", "\uFFFDa"},
		{"DecodeIdent", DecodeIdent, "\\é\\😀x", "é😀x"},
		{"DecodeIdent", DecodeIdent, "é😀", "é😀"},
		{"DecodeString", DecodeString, "'a\x00b'", "a\uFFFDb"},
		{"DecodeString", DecodeString, "'\\\x00'", "\uFFFD"},
		{"DecodeString", DecodeString, "\"\xffx\"", "\uFFFDx"},
		{"DecodeString", DecodeString, "\"\\é\"", "é"},
		{"DecodeURL", DecodeURL, "url(a\x00b)", "a\uFFFDb"},
		{"DecodeURL", DecodeURL, "url(\\\x00)", "\uFFFD"},
		{"DecodeURL", DecodeURL, "url( \xe2\x82 )", "\uFFFD"},
		{"DecodeURL", DecodeURL, "url(\\é)", "é"},
	} {
		if got := tt.decode(tt.str); got != tt.want {
			t.Errorf("%s(%q) = %q, want %q", tt.name, tt.str, got, tt.want)
		}
	}
}

// A NUL in a url or string reaches the AST value as U+FFFD.
func TestParseNULSpecFixes(t *testing.T) {
	ast, err := Parse("url(a\x00b) 'a\x00b'", ParseOptions{Context: "value"})
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	value, ok := ast.(*Value)
	if !ok || value.Children.Size() != 2 {
		t.Fatalf("Parse = %#v, want a Value with two children", ast)
	}
	if url, ok := value.Children.First().(*URL); !ok || url.Value != "a\uFFFDb" {
		t.Errorf("first child = %#v, want URL a\uFFFDb", value.Children.First())
	}
	if str, ok := value.Children.Last().(*String); !ok || str.Value != "a\uFFFDb" {
		t.Errorf("last child = %#v, want String a\uFFFDb", value.Children.Last())
	}
}
