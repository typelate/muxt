package format

import (
	"slices"
	"testing"
)

func TestSplitRuns(t *testing.T) {
	for _, tt := range []struct {
		text string
		want []segment
	}{
		{text: "a \t\n\r b", want: []segment{{0, 1, false}, {1, 6, true}, {6, 7, false}}},
		{text: "a b", want: []segment{{0, 1, false}, {1, 2, true}, {2, 3, false}}},
		{text: "a\fb", want: []segment{{0, 3, false}}},
		{text: "a\u00a0b", want: []segment{{0, 4, false}}},
		{text: "a　b", want: []segment{{0, len("a　b"), false}}},
		{text: "  ", want: []segment{{0, 2, true}}},
		{text: ""},
	} {
		if got := splitRuns([]byte(tt.text)); !slices.Equal(got, tt.want) {
			t.Errorf("splitRuns(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}
