package mutation

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{name: "an empty raw literal", literal: "``", value: "", want: []int{1}},
		{name: "an empty interpreted literal", literal: `""`, value: "", want: []int{1}},
		{
			// U+0080 is the first rune UTF-8 needs two bytes for.
			name:    "the smallest escaped rune of two bytes",
			literal: `"\` + `u0080"`,
			value:   string(rune(0x80)),
			want:    []int{1, 1, 7},
		},
		{
			name:    "an escaped rune of one byte",
			literal: `"\` + `u0041"`,
			value:   "A",
			want:    []int{1, 7},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// The pair has to be a real one, or the table is asserting
			// against arithmetic nobody will ever run.
			decoded, err := strconv.Unquote(tt.literal)
			require.NoError(t, err, "Unquote(%s)", tt.literal)
			require.Equal(t, tt.value, decoded, "%s decodes to a value other than the table's", tt.literal)

			got, err := literalOffsets(tt.literal, tt.value)
			require.NoError(t, err, "literalOffsets(%s)", tt.literal)
			require.Equal(t, tt.want, got, "literalOffsets(%s)", tt.literal)
			assert.Len(t, got, len(tt.value)+1, "offsets: one per byte of the value plus an end")
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
			got, err := literalOffsets(tt.literal, tt.value)
			assert.Error(t, err, "literalOffsets(%s, %q) = %v", tt.literal, tt.value, got)
		})
	}
}

// TestLiteralEncoder states how mutated text is written back into Go
// source: a raw literal stays raw while it can, which keeps a mutant's diff
// readable, and anything else is quoted.
func TestLiteralEncoder(t *testing.T) {
	for _, tt := range []struct {
		name, literal, mutated, want string
	}{
		{name: "raw stays raw", literal: "`{{.A}}`", mutated: `{{""}}`, want: "`{{\"\"}}`"},
		{name: "raw cannot hold a back quote", literal: "`{{.A}}`", mutated: "{{`x`}}", want: "\"{{`x`}}\""},
		{name: "raw cannot hold a carriage return", literal: "`{{.A}}`", mutated: "a\rb", want: `"a\rb"`},
		{name: "interpreted is quoted", literal: `"{{.A}}\n"`, mutated: "{{0}}\n", want: `"{{0}}\n"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, literalEncoder(tt.literal)(tt.mutated), "literalEncoder(%s)(%q)", tt.literal, tt.mutated)
		})
	}
}
