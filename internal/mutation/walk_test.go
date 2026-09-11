package mutation

import (
	"strings"
	"testing"
	"text/template/parse"
)

// TestWalkActionsOrder states the order actions are reported in, which is
// the order a template's mutants are enumerated.
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
	walkActions(text, regions(text, "", ""), nil, nil, trees["t"].Root, func(a action) {
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
