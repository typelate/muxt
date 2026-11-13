package composite

import "testing"

func TestEditDistance(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{
		{a: "", b: "", want: 0},
		{a: "", b: "abc", want: 3},
		{a: "abc", b: "", want: 3},
		{a: "abc", b: "abc", want: 0},
		{a: "abc", b: "abd", want: 1},  // only the last character differs
		{a: "abc", b: "abcd", want: 1}, // one appended
		{a: "abcd", b: "abc", want: 1}, // one removed
		{a: "kitten", b: "sitting", want: 3},
	} {
		if got := editDistance(tt.a, tt.b); got != tt.want {
			t.Errorf("editDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// TestClosestKey states which known key a hint names: the nearest within a
// fifth of the typed key's length (at least one edit), the first of equally
// near ones, and nothing when the nearest is farther.
func TestClosestKey(t *testing.T) {
	for _, tt := range []struct {
		name  string
		key   string
		known []string
		want  string
	}{
		{name: "a tie names the first", key: "ab", known: []string{"ac", "ad"}, want: "ac"},
		{name: "the nearer of two", key: "abcd", known: []string{"abxx", "abcx"}, want: "abcx"},
		{name: "a short key allows one edit", key: "abcd", known: []string{"abcx"}, want: "abcx"},
		{name: "a short key refuses two edits", key: "abcd", known: []string{"abxx"}},
		{name: "ten letters allow two edits", key: "abcdefghij", known: []string{"abcdefghxx"}, want: "abcdefghxx"},
		{name: "ten letters refuse three edits", key: "abcdefghij", known: []string{"abcdefgxxx"}},
		{name: "a difference in the last character", key: "output_htmy", known: []string{"output_htmx"}, want: "output_htmx"},
		{name: "nothing known", key: "abc"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := closestKey(tt.key, tt.known); got != tt.want {
				t.Errorf("closestKey(%q, %q) = %q, want %q", tt.key, tt.known, got, tt.want)
			}
		})
	}
}
