package mutation

import (
	"strings"
	"testing"
	"text/template/parse"
)

// TestWalkActionsOrder states the order every identifier rests on.
//
// An action's place in this walk is part of what identifies it, so the
// order is a contract, not an implementation detail: reordering a body
// and its else, or reporting a construct after the body it encloses,
// renumbers every action and quietly stops a recorded verdict matching
// the action it was reached for. Nothing else in the suite would notice,
// because the symptom is a slower run rather than a failing one.
//
// The rule: the enclosing action first, then its body, then its else.
func TestWalkActionsOrder(t *testing.T) {
	const text = `{{define "t"}}` +
		`{{if .A}}{{.B}}{{else}}{{.C}}{{end}}` +
		`{{range .D}}{{.E}}{{else}}{{.F}}{{end}}` +
		`{{with .G}}{{.H}}{{else}}{{.I}}{{end}}` +
		`{{template "p" .J}}` +
		`{{end}}`

	trees, err := parse.Parse("t", text+`{{define "p"}}x{{end}}`, "", "")
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	seen := 0
	walkActions(text, regions(text, "", ""), nil, nil, trees["t"].Root, func(a action) {
		seen++
		if a.index != seen {
			t.Errorf("action %q has index %d, want %d: indexes must count the walk", a.text, a.index, seen)
		}
		got = append(got, a.text)
	})

	want := []string{
		"{{if .A}}", "{{.B}}", "{{.C}}",
		"{{range .D}}", "{{.E}}", "{{.F}}",
		"{{with .G}}", "{{.H}}", "{{.I}}",
		`{{template "p" .J}}`,
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("walk order:\n got %v\nwant %v", got, want)
	}
}

// TestWalkActionsNarrowsDot states that a body is walked with the dot its
// construct selects, which is what makes an action's types depend on
// where it is written rather than only on what it says.
func TestWalkActionsNarrowsDot(t *testing.T) {
	const text = `{{define "t"}}{{range .Items}}{{.}}{{end}}{{end}}`
	trees, err := parse.Parse("t", text, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// With no type information the narrowing resolves to nothing, which
	// is still a narrowing: the body must not inherit the outer dot.
	var bodyDot []string
	walkActions(text, regions(text, "", ""), nil, nil, trees["t"].Root, func(a action) {
		if a.text == "{{.}}" {
			bodyDot = append(bodyDot, typeKey(a.dot))
		}
	})
	if len(bodyDot) != 1 {
		t.Fatalf("body actions = %d, want 1", len(bodyDot))
	}
}
