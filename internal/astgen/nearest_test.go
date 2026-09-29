package astgen_test

import (
	"testing"

	"github.com/typelate/muxt/internal/astgen"
)

func TestNearestStringExactAndTiedMatches(t *testing.T) {
	for _, tt := range []struct {
		name       string
		target     string
		candidates []string
		want       string
		found      bool
	}{
		{name: "an exact match is not a suggestion", target: "abc", candidates: []string{"abc"}},
		{name: "an exact match is not a suggestion beside a near one", target: "abc", candidates: []string{"abc", "abd"}},
		{name: "an exact match found later is not a suggestion", target: "abc", candidates: []string{"abd", "abc"}},
		{name: "ties keep the first candidate", target: "ab3", candidates: []string{"ab1", "ab2"}, want: "ab1", found: true},
		{name: "a wrong case exact match is a suggestion", target: "ABC", candidates: []string{"abc"}, want: "abc", found: true},
		{name: "distance at the limit is a suggestion", target: "abcdef", candidates: []string{"abcdxx"}, want: "abcdxx", found: true},
		{name: "distance past the limit is not", target: "abcdef", candidates: []string{"abcxxx"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, found := astgen.NearestString(tt.target, tt.candidates)
			if found != tt.found || got != tt.want {
				t.Errorf("NearestString(%q, %q) = %q, %v; want %q, %v", tt.target, tt.candidates, got, found, tt.want, tt.found)
			}
		})
	}
}
