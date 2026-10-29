package tokenizer

// Places where the port follows CSS Syntax Module Level 3 instead of
// upstream csstree 3.2.1. Each case name gives the section and, after
// "upstream:", what csstree produces for the same input.

import (
	"slices"
	"testing"
)

func TestTokenizeSpecFixes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		source string
		want   []tokenChunk
	}{
		// §4.3.5 consume a string token: on a newline, "Reconsume the
		// current input code point, create a <bad-string-token>, and return it."
		{
			`§4.3.5 a bad-string ends before LF (upstream: "a\n bad-string)`,
			"\"a\nb\"",
			[]tokenChunk{{"bad-string-token", "\"a"}, {"whitespace-token", "\n"}, {"ident-token", "b"}, {"string-token", "\""}},
		},
		{
			`§4.3.5 a bad-string ends before CR (upstream: "a\r bad-string)`,
			"\"a\rb\"",
			[]tokenChunk{{"bad-string-token", "\"a"}, {"whitespace-token", "\r"}, {"ident-token", "b"}, {"string-token", "\""}},
		},
		{
			`§4.3.5 a bad-string ends before CRLF (upstream: "a\r\n bad-string)`,
			"\"a\r\nb\"",
			[]tokenChunk{{"bad-string-token", "\"a"}, {"whitespace-token", "\r\n"}, {"ident-token", "b"}, {"string-token", "\""}},
		},
		{
			`§4.3.5 a bad-string ends before FF (upstream: 'a\f bad-string)`,
			"'a\fb",
			[]tokenChunk{{"bad-string-token", "'a"}, {"whitespace-token", "\f"}, {"ident-token", "b"}},
		},
		{
			`§4.3.5 a newline right after the quote (upstream: "\n bad-string)`,
			"\"\n",
			[]tokenChunk{{"bad-string-token", "\""}, {"whitespace-token", "\n"}},
		},
		// §4.3.14 consume the remnants of a bad url: after an escape the
		// loop goes on with the code point right after it.
		{
			`§4.3.14 the ) after an escape ends the remnants (upstream: bad-url url(a"\a)b), ident c)`,
			"url(a\"\\a)b)c",
			[]tokenChunk{{"bad-url-token", "url(a\"\\a)"}, {"ident-token", "b"}, {")-token", ")"}, {"ident-token", "c"}},
		},
		{
			`§4.3.14 an escaped ) then ) (upstream: bad-url runs to EOF)`,
			"url(x\"\\))d",
			[]tokenChunk{{"bad-url-token", "url(x\"\\))"}, {"ident-token", "d"}},
		},
		{
			`§4.3.14 a hex escape and its whitespace (upstream: bad-url url(x"\31 )), ident d)`,
			"url(x\"\\31 ))d",
			[]tokenChunk{{"bad-url-token", "url(x\"\\31 )"}, {")-token", ")"}, {"ident-token", "d"}},
		},
		{
			`§4.3.14 an escape at EOF`,
			"url(x\"\\a",
			[]tokenChunk{{"bad-url-token", "url(x\"\\a"}},
		},
		{
			`§4.3.14 a backslash at EOF`,
			"url(x\"\\",
			[]tokenChunk{{"bad-url-token", "url(x\"\\"}},
		},
		// §3.3 preprocessing: U+0000 NULL reads as U+FFFD REPLACEMENT
		// CHARACTER, a name code point. Token offsets still index the
		// original bytes, so the NUL stays in the chunk.
		{
			`§3.3 NUL is a name code point (upstream: ident a, delim NUL, ident b)`,
			"a\x00b",
			[]tokenChunk{{"ident-token", "a\x00b"}},
		},
		{
			`§3.3 NUL starts an ident (upstream: delim)`,
			"\x00 ",
			[]tokenChunk{{"ident-token", "\x00"}, {"whitespace-token", " "}},
		},
		{
			`§3.3 a backslash before NUL is an escape (upstream: delim, delim)`,
			"\\\x00",
			[]tokenChunk{{"ident-token", "\\\x00"}},
		},
		{
			`§3.3 NUL in a url (upstream: bad-url)`,
			"url(a\x00b)",
			[]tokenChunk{{"url-token", "url(a\x00b)"}},
		},
		{
			`§3.3 an escaped NUL in a url (upstream: bad-url)`,
			"url(\\\x00)",
			[]tokenChunk{{"url-token", "url(\\\x00)"}},
		},
		{
			`§3.3 NUL after # makes a hash (upstream: delim, delim)`,
			"#\x00",
			[]tokenChunk{{"hash-token", "#\x00"}},
		},
		{
			`§3.3 NUL after @ makes an at-keyword (upstream: delim, delim)`,
			"@\x00",
			[]tokenChunk{{"at-keyword-token", "@\x00"}},
		},
		{
			`§3.3 NUL after - makes an ident (upstream: delim, delim)`,
			"-\x00",
			[]tokenChunk{{"ident-token", "-\x00"}},
		},
		{
			`§3.3 NUL after a number makes a dimension (upstream: number, delim)`,
			"1\x00",
			[]tokenChunk{{"dimension-token", "1\x00"}},
		},
		{
			`§3.3 an escaped NUL in a string`,
			"'\\\x00'",
			[]tokenChunk{{"string-token", "'\\\x00'"}},
		},
		{
			`§3.3 an escaped NUL at the end of a string (upstream: the NUL is still consumed)`,
			"'\\\x00",
			[]tokenChunk{{"string-token", "'\\\x00"}},
		},
		// Invalid UTF-8 decodes to U+FFFD, a name code point like every
		// byte >= 0x80 already is; an escape consumes the whole sequence.
		{
			`§3.2 invalid UTF-8 after a backslash`,
			"\\\xe2\x82 a",
			[]tokenChunk{{"ident-token", "\\\xe2\x82"}, {"whitespace-token", " "}, {"ident-token", "a"}},
		},
		// §3.2: only U+FEFF is a byte order mark in a decoded string.
		{
			`§3.2 U+FFFE at the start is not a BOM (upstream: skipped)`,
			"\ufffea",
			[]tokenChunk{{"ident-token", "\ufffea"}},
		},
		{
			`§3.2 U+FFFE alone (upstream: no tokens)`,
			"\ufffe",
			[]tokenChunk{{"ident-token", "\ufffe"}},
		},
		{
			`§3.2 U+FEFF at the start is a BOM`,
			"\ufeff\ufffe",
			[]tokenChunk{{"ident-token", "\ufffe"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tokenizeChunks(tt.source); !slices.Equal(got, tt.want) {
				t.Errorf("Tokenize(%q) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}

func TestIsBOMSpecFixes(t *testing.T) {
	for _, tt := range []struct{ code, want int }{
		{0xfeff, 3},
		{0xfffe, 0}, // upstream 1: it takes U+FFFE for a little-endian BOM
	} {
		if got := IsBOM(tt.code); got != tt.want {
			t.Errorf("IsBOM(%#x) = %d, want %d", tt.code, got, tt.want)
		}
	}
}

// OffsetToLocation counts U+FFFE at the start as a character, not a BOM.
func TestOffsetToLocationFFFE(t *testing.T) {
	const source = "\ufffea"
	got := NewOffsetToLocation(source, 0, 1, 1).GetLocation(len(source), "")
	if want := (Position{Offset: 4, Line: 1, Column: 5}); got.Position != want {
		t.Errorf("GetLocation(%d) = %+v, want %+v", len(source), got.Position, want)
	}
}

func TestCharCodeAtSpecFixes(t *testing.T) {
	for _, tt := range []struct {
		source string
		offset int
		want   int
	}{
		{"\x00", 0, 0xFFFD}, // upstream 0, the same as out of range
		{"a", 0, 'a'},
		{"a", 1, 0},
		{"a", -1, 0},
		{"\xff", 0, 0xff},
	} {
		if got := CharCodeAt(tt.source, tt.offset); got != tt.want {
			t.Errorf("CharCodeAt(%q, %d) = %#x, want %#x", tt.source, tt.offset, got, tt.want)
		}
	}
}

func TestCodePointAt(t *testing.T) {
	for _, tt := range []struct {
		source string
		offset int
		want   rune
		size   int
	}{
		{"", 0, 0, 0},
		{"a", 1, 0, 0},
		{"a", -1, 0, 0},
		{"a", 0, 'a', 1},
		{"\x00", 0, 0xFFFD, 1},
		{"é", 0, 'é', 2},
		{"x😀", 1, '😀', 4},
		{"\uFFFD", 0, 0xFFFD, 3},
		// invalid UTF-8: each maximal subpart of a valid sequence is one
		// U+FFFD, as the WHATWG Encoding Standard's UTF-8 decoder reads it
		{"\xff", 0, 0xFFFD, 1},
		{"\x80", 0, 0xFFFD, 1},
		{"\xc0\x80", 0, 0xFFFD, 1},
		{"\xc3", 0, 0xFFFD, 1},
		{"\xe2\x82", 0, 0xFFFD, 2},
		{"\xe2\x82a", 0, 0xFFFD, 2},
		{"\xe0\x80", 0, 0xFFFD, 1},
		{"\xed\xa0\x80", 0, 0xFFFD, 1}, // a surrogate
		{"\xf0\x9f\x98", 0, 0xFFFD, 3},
		{"\xf0\x8f", 0, 0xFFFD, 1},
		{"\xf4\x90\x80\x80", 0, 0xFFFD, 1},
		{"\xf5", 0, 0xFFFD, 1},
	} {
		got, size := CodePointAt(tt.source, tt.offset)
		if got != tt.want || size != tt.size {
			t.Errorf("CodePointAt(%q, %d) = %U, %d, want %U, %d", tt.source, tt.offset, got, size, tt.want, tt.size)
		}
	}
}

func TestConsumeEscapedSpecFixes(t *testing.T) {
	for _, tt := range []struct {
		source string
		want   int
	}{
		{"\\\x00", 2},
		{"\\é", 3},         // upstream 2: one UTF-16 code unit
		{"\\😀", 5},         // upstream 2: the high surrogate only
		{"\\\xe2\x82a", 3}, // one maximal subpart
		{"\\\xffa", 2},
		{"\\a", 2},
		{"\\", 2},
	} {
		if got := ConsumeEscaped(tt.source, 0); got != tt.want {
			t.Errorf("ConsumeEscaped(%q, 0) = %d, want %d", tt.source, got, tt.want)
		}
	}
}

func TestDecodeEscapedSpecFixes(t *testing.T) {
	for _, tt := range []struct {
		escaped, want string
	}{
		{"\x00", "\uFFFD"}, // upstream "\x00"
		{"\xff", "\uFFFD"}, // upstream gets U+FFFD from its decoder
		{"\xe2\x82", "\uFFFD"},
		{"é", "é"},
		{"😀", "😀"},
		{"\uFFFD", "\uFFFD"},
	} {
		if got := DecodeEscaped(tt.escaped); got != tt.want {
			t.Errorf("DecodeEscaped(%q) = %q, want %q", tt.escaped, got, tt.want)
		}
	}
}
