package rendering

import (
	"strings"
	"testing"
)

func TestCallersPreserveDefineBytes(t *testing.T) {
	// A call-site caller must make the oracle see the body's line breaks
	// at least: two bodies that differ in indentation and LF count render
	// differently there, and alike in the collapse callers.
	const call = `{{template "x" .}}`
	const a, b = "\n\n\thi\n", "\nhi\n"
	for _, c := range Callers("x", "") {
		ra := strings.Replace(c.Source, call, a, 1)
		rb := strings.Replace(c.Source, call, b, 1)
		diff, err := Diff(ra, rb, c.Context, nil)
		if err != nil {
			t.Fatalf("%s: Diff error: %v", c.Name, err)
		}
		if got := diff != ""; got != c.CallSite {
			t.Errorf("%s: Diff(%q, %q) differs = %v, want %v", c.Name, ra, rb, got, c.CallSite)
		}
	}
}

func TestCallersTable(t *testing.T) {
	for _, tt := range []struct {
		first string
		table bool
	}{{"tr", true}, {"td", false}, {"", false}} {
		found := false
		for _, c := range Callers("x", tt.first) {
			if c.Name == "table" {
				found = c.Context == TableBody
			}
		}
		if found != tt.table {
			t.Errorf("Callers(%q, %q) has a table caller = %v, want %v", "x", tt.first, found, tt.table)
		}
	}
}
