package csstree

import (
	"math"
	"testing"
)

// TestJSNumber checks jsNumber against JavaScript's Number() (Go only:
// upstream uses Number() directly). The expected values were produced by
// Node.js.
func TestJSNumber(t *testing.T) {
	nan := math.NaN()
	for _, test := range []struct {
		input string
		want  float64
	}{
		{"", 0},
		{" ", 0},
		{"0", 0},
		{"+0", 0},
		{"1", 1},
		{"1.", 1},
		{".5", 0.5},
		{"+.5", 0.5},
		{"-.5e3", -500},
		{"1e", nan},
		{"1e+", nan},
		{"1E-5", 1e-5},
		{"0x1F", 31},
		{"0b101", 5},
		{"0o17", 15},
		{"0x", nan},
		{"-0x10", nan},
		{"Infinity", math.Inf(1)},
		{"-Infinity", math.Inf(-1)},
		{"infinity", nan},
		{"inf", nan},
		{"NaN", nan},
		{"1_000", nan},
		{" 12 ", 12},
		{"\t3\n", 3},
		{" 1", 1},
		{"1e400", math.Inf(1)},
		{"-1e400", math.Inf(-1)},
		{"1e-400", 0},
		{"+", nan},
		{"-", nan},
		{".", nan},
		{"1.2.3", nan},
		{"00012", 12},
		{"12px", nan},
		{"0x1p3", nan},
		{"0b2", nan},
	} {
		got := jsNumber(test.input)
		if got != test.want && !(math.IsNaN(got) && math.IsNaN(test.want)) {
			t.Errorf("jsNumber(%q) = %v, want %v", test.input, got, test.want)
		}
	}

	if got := jsNumber("-0"); got != 0 || !math.Signbit(got) {
		t.Errorf(`jsNumber("-0") = %v, want -0`, got)
	}
}
