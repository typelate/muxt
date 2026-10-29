package selectors

import "testing"

func TestParseAnB(t *testing.T) {
	for _, tt := range []struct {
		in   string
		a, b int
		bad  bool
	}{
		{in: "odd", a: 2, b: 1},
		{in: "even", a: 2, b: 0},
		{in: "3", a: 0, b: 3},
		{in: "n", a: 1, b: 0},
		{in: "2n", a: 2, b: 0},
		{in: "2n+1", a: 2, b: 1},
		{in: "2n - 1", a: 2, b: -1},
		{in: "-n+3", a: -1, b: 3},
		{in: "+3", a: 0, b: 3},
		{in: "", bad: true},
		{in: "n+", bad: true},
	} {
		t.Run(tt.in, func(t *testing.T) {
			a, b, err := parseAnB(tt.in)
			if tt.bad {
				if err == nil {
					t.Fatalf("parseAnB(%q) = (%d,%d), want error", tt.in, a, b)
				}
				return
			}
			if err != nil || a != tt.a || b != tt.b {
				t.Fatalf("parseAnB(%q) = (%d,%d,%v), want (%d,%d,nil)", tt.in, a, b, err, tt.a, tt.b)
			}
		})
	}
}

func TestAnbMatches(t *testing.T) {
	// 2n+1 matches odd positions.
	for idx, want := range map[int]bool{1: true, 2: false, 3: true, 4: false} {
		if got := anbMatches(2, 1, idx); got != want {
			t.Errorf("anbMatches(2,1,%d) = %v, want %v", idx, got, want)
		}
	}
	if !anbMatchesAll(1, 0) { // :nth-child(n)
		t.Error("anbMatchesAll(1,0) = false, want true")
	}
	if !anbMatchesNone(0, 0) { // :nth-child(0) matches nothing
		t.Error("anbMatchesNone(0,0) = false, want true")
	}
}
