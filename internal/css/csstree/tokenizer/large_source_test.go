package tokenizer

import (
	"strings"
	"testing"
)

// Upstream packs token end offsets into 24 bits; a source of 16 MiB or more
// would corrupt offsets and token types. The port uses 58 offset bits.
func TestTokenStreamLargeSource(t *testing.T) {
	const size = 1<<24 + 10
	source := strings.Repeat(" ", size-1) + "a"

	ts := NewTokenStream(source, Tokenize)
	ts.Next() // past the whitespace token

	if ts.TokenType != Ident || ts.TokenStart != size-1 || ts.TokenEnd != size {
		t.Errorf("last token = %v [%d, %d), want %v [%d, %d)", ts.TokenType, ts.TokenStart, ts.TokenEnd, Ident, size-1, size)
	}
}

// A negative offset reads as no character, as charCodeAt does upstream, so
// these scan forward instead of panicking.
func TestConsumeNegativeOffset(t *testing.T) {
	if got := ConsumeName("abc", -1); got != -1 {
		t.Errorf(`ConsumeName("abc", -1) = %d, want -1`, got)
	}
	if got := ConsumeBadURLRemnants("abc)", -1); got != 4 {
		t.Errorf(`ConsumeBadURLRemnants("abc)", -1) = %d, want 4`, got)
	}
}
