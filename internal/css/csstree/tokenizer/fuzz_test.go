package tokenizer

import (
	"testing"
	"unicode/utf8"
)

func FuzzTokenize(f *testing.F) {
	for _, seed := range []string{`url((\0`, `url(\`, `a\`, `"\`, `/*`, `@\`, `#\`, `1e\`, `url( a\)`, "\xef\xbb\xbfa", `u+1?-`, "\xef\xbf\xbe", "a\x00b", "\\\x00", "url(\x00\\\x00)", "\\\xe2\x82", "url(\"\\))", "'\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		prevEnd := 0
		if r, _ := utf8.DecodeRuneInString(source); len(source) > 0 {
			prevEnd = IsBOM(int(r)) // a leading BOM is skipped, as upstream does
		}
		Tokenize(source, func(tokenType TokenType, start, end int) {
			if start != prevEnd || end < start || end > len(source) {
				t.Fatalf("Tokenize(%q) emitted %v [%d, %d) after end %d", source, tokenType, start, end, prevEnd)
			}
			prevEnd = end
		})
		if prevEnd != len(source) {
			t.Fatalf("Tokenize(%q) ended at %d, want %d", source, prevEnd, len(source))
		}

		ts := NewTokenStream(source, Tokenize)
		for !ts.EOF {
			ts.Next()
		}
	})
}
