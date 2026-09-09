package mutation

import "testing"

// TestRegionsBoundTheActionsContent states where an action's content
// begins and ends, which is what a pipeline substitution is spliced over.
//
// The trim markers are the interesting part. A marker is not part of the
// pipeline, so innerEnd must stop short of it; splicing over one changes
// the whitespace the template renders as well as the value, and the
// mutant then reports on something other than the action. text/template
// only reads "-" as a marker when whitespace separates it from what it
// follows, so {{if 1-}} trims and {{$x-}} does not.
func TestRegionsBoundTheActionsContent(t *testing.T) {
	for _, tt := range []struct {
		name    string
		text    string
		content string
		keyword string
	}{
		{name: "a plain action", text: `{{.X}}`, content: `.X`},
		{name: "a leading trim marker", text: `{{- .X}}`, content: `.X`},
		{name: "a trailing trim marker", text: `{{.X -}}`, content: `.X`},
		{name: "markers on both sides", text: `{{- .X -}}`, content: `.X`},
		{
			name:    "a dash that follows whitespace is a marker",
			text:    `{{if 1 -}}`,
			content: `if 1`,
			keyword: "if",
		},
		{
			name:    "a dash that does not follow whitespace belongs to the pipeline",
			text:    `{{$x-}}`,
			content: `$x-`,
		},
		{
			name:    "a right delimiter inside a string does not end the action",
			text:    `{{printf "}}"}}`,
			content: `printf "}}"`,
		},
		{
			name:    "a comment is taken whole",
			text:    `{{- /* you'd think "this" ends it }} */ -}}`,
			content: `/* you'd think "this" ends it }} */`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			found := regions(tt.text, "", "")
			if len(found) != 1 {
				t.Fatalf("regions = %d, want 1", len(found))
			}
			r := found[0]
			if r.start != 0 || r.end != len(tt.text) {
				t.Errorf("region spans [%d,%d), want [0,%d)", r.start, r.end, len(tt.text))
			}

			// The content runs from after the delimiter and any leading
			// marker, which trimLeft finds, to innerEnd.
			from := trimLeft(tt.text, r.start+2, r.innerEnd)
			if got := tt.text[from:r.innerEnd]; got != tt.content {
				t.Errorf("content = %q, want %q", got, tt.content)
			}
			if r.keyword != tt.keyword {
				t.Errorf("keyword = %q, want %q", r.keyword, tt.keyword)
			}
		})
	}
}

// TestRegionsKeepsScanningPastSomethingItCannotRead states that one
// unreadable action does not cost the rest of the template its mutants.
//
// This is how an apostrophe in a comment once silently removed every
// action after it from a whole file.
func TestRegionsKeepsScanningPastSomethingItCannotRead(t *testing.T) {
	const text = `{{.A}}{{'unterminated}}{{.B}}`
	found := regions(text, "", "")

	var starts []int
	for _, r := range found {
		starts = append(starts, r.start)
	}
	if len(found) < 2 {
		t.Fatalf("regions = %v, want the actions on both sides of the unreadable one", starts)
	}
	last := found[len(found)-1]
	if got := text[last.start:last.end]; got != `{{.B}}` {
		t.Errorf("last region = %q, want %q", got, `{{.B}}`)
	}
}
