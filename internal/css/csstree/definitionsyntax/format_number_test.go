package definitionsyntax

import (
	"math"
	"testing"
)

// formatNumber must agree with JavaScript's String(number), which upstream
// uses when it generates range bounds. Expected values come from node.
func TestFormatNumber(t *testing.T) {
	for _, tt := range []struct {
		value float64
		want  string
	}{
		{1e-7, "1e-7"},
		{0.000001, "0.000001"},
		{1.5, "1.5"},
		{-0.25, "-0.25"},
		{123456789012345680000, "123456789012345680000"},
		{1e21, "1e+21"},
		{1.5e21, "1.5e+21"},
		{-1e-7, "-1e-7"},
		{1<<53 + 2, "9007199254740994"},
		{1e300, "1e+300"},
		{5e-324, "5e-324"},
		{100, "100"},
		{-3, "-3"},
		{math.Copysign(0, -1), "0"},
		{math.Inf(-1), "-Infinity"},
		{math.NaN(), "NaN"},
	} {
		if got := formatNumber(tt.value); got != tt.want {
			t.Errorf("formatNumber(%v) = %q, want %q", tt.value, got, tt.want)
		}
	}
}
