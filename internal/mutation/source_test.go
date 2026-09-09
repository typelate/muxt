package mutation

import (
	"slices"
	"strconv"
	"testing"
)

// TestLiteralOffsetsMapsEveryByteBack states where each byte of a
// template written in Go source sits in the literal that carries it.
//
// A mutation is found in the decoded template and written back into the
// .go file, so every offset in this table is where an edit lands. Get one
// wrong and the tool rewrites the wrong bytes of somebody's source: the
// overlay either stops compiling, which reads as a skipped mutant, or
// compiles into something the mutation never meant to say.
//
// The expectations are written out rather than derived, because deriving
// them would use the same arithmetic under test.
func TestLiteralOffsetsMapsEveryByteBack(t *testing.T) {
	for _, tt := range []struct {
		name    string
		literal string
		value   string
		want    []int
	}{
		{
			name:    "a raw literal is its own value",
			literal: "`abc`",
			value:   "abc",
			want:    []int{1, 2, 3, 4},
		},
		{
			name: "a raw literal drops carriage returns from its value",
			// Go discards \r inside a raw literal, so the value is one
			// byte shorter than the text and every byte after it sits
			// one further along than its index suggests.
			literal: "`a\r\nb`",
			value:   "a\nb",
			want:    []int{1, 3, 4, 5},
		},
		{
			name:    "an escape is one byte of value and two of literal",
			literal: `"a\nb"`,
			value:   "a\nb",
			want:    []int{1, 2, 4, 5},
		},
		{
			name: "a multibyte rune written directly repeats its offset",
			// Both bytes of the rune come from the same place in the
			// literal, so both map back to it.
			literal: `"é"`,
			value:   "é",
			want:    []int{1, 1, 3},
		},
		{
			name:    "a multibyte rune written as an escape does too",
			literal: `"\u00e9"`,
			value:   "é",
			want:    []int{1, 1, 7},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// The pair has to be a real one, or the table is asserting
			// against arithmetic nobody will ever run.
			decoded, err := strconv.Unquote(tt.literal)
			if err != nil {
				t.Fatalf("Unquote(%s) = %v", tt.literal, err)
			}
			if decoded != tt.value {
				t.Fatalf("%s decodes to %q, but the table says %q", tt.literal, decoded, tt.value)
			}

			got, err := literalOffsets(tt.literal, tt.value)
			if err != nil {
				t.Fatalf("literalOffsets(%s) = %v", tt.literal, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("literalOffsets(%s) = %v, want %v", tt.literal, got, tt.want)
			}
			if len(got) != len(tt.value)+1 {
				t.Errorf("offsets = %d, want one per byte of the value plus an end", len(got))
			}
		})
	}
}

// TestLiteralOffsetsRefusesAValueItCannotAccountFor states that a literal
// and value that do not correspond are refused.
//
// Offsets that do not cover the value would place edits by an arithmetic
// that has already lost track of the text, and a wrong offset writes over
// the wrong bytes of a real file.
func TestLiteralOffsetsRefusesAValueItCannotAccountFor(t *testing.T) {
	for _, tt := range []struct {
		name    string
		literal string
		value   string
	}{
		{name: "a raw literal shorter than its value", literal: "`ab`", value: "abc"},
		{name: "an interpreted literal shorter than its value", literal: `"ab"`, value: "abc"},
		{name: "an escape that is not one", literal: `"\q"`, value: "q"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := literalOffsets(tt.literal, tt.value); err == nil {
				t.Errorf("literalOffsets(%s, %q) = %v, want an error", tt.literal, tt.value, got)
			}
		})
	}
}
