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
// follows, so {{if 1 -}} trims and {{$x-}} does not.
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
		{name: "range", text: `{{range .A}}`, content: `range .A`, keyword: "range"},
		{name: "with", text: `{{with .A}}`, content: `with .A`, keyword: "with"},
		{name: "template", text: `{{template "x" .}}`, content: `template "x" .`, keyword: "template"},
		{name: "block", text: `{{block "x" .}}`, content: `block "x" .`, keyword: "block"},
		{name: "define", text: `{{define "x"}}`, content: `define "x"`, keyword: "define"},
		{name: "else", text: `{{else}}`, content: `else`, keyword: "else"},
		{name: "else if", text: `{{else if .A}}`, content: `else if .A`, keyword: "else"},
		{name: "a trimmed end", text: `{{- end -}}`, content: `end`, keyword: "end"},
		{name: "a function that starts with a keyword", text: `{{endorse .A}}`, content: `endorse .A`},
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

// TestLeadingWordReadsTheWholeIdentifier states that a keyword is only a
// keyword when it is the whole word. A function may be named endX or
// withDefault, and text/template lexes those as one identifier.
func TestLeadingWordReadsTheWholeIdentifier(t *testing.T) {
	for _, text := range []string{`{{endX}}`, `{{end2}}`, `{{end_x}}`, `{{withDefault .A "x"}}`, `{{ifEmpty .A}}`, `{{rangeOf .A}}`} {
		if got := regions(text, "", "")[0].keyword; got != "" {
			t.Errorf("regions(%q) keyword = %q, want none", text, got)
		}
	}
}

// TestMatchEndSkipsAFunctionNamedLikeAKeyword states that such a function
// does not open a block, so the end is matched at the right depth.
func TestMatchEndSkipsAFunctionNamedLikeAKeyword(t *testing.T) {
	const text = `{{range .Items}}{{withDefault . "x"}}{{end}}`
	if end, _, ok := matchEnd(regions(text, "", ""), 0); !ok || end != 2 {
		t.Errorf("matchEnd = %d, %t, want 2, true", end, ok)
	}
}

// TestMatchEnd states which {{end}} and {{else}} belong to a block: the
// ones at its own depth, and of an else chain only the first.
func TestMatchEnd(t *testing.T) {
	for _, tt := range []struct {
		name              string
		text              string
		wantEnd, wantElse int
		ok                bool
	}{
		{name: "no else", text: `{{range .A}}x{{end}}`, wantEnd: 1, wantElse: -1, ok: true},
		{name: "an else", text: `{{range .A}}{{.B}}{{else}}{{.C}}{{end}}`, wantEnd: 4, wantElse: 2, ok: true},
		{name: "a nested else is not ours", text: `{{with .A}}{{if .B}}{{else}}{{end}}{{else}}{{end}}`, wantEnd: 5, wantElse: 4, ok: true},
		{name: "the first else of a chain", text: `{{if .A}}a{{else if .B}}b{{else}}c{{end}}`, wantEnd: 3, wantElse: 1, ok: true},
		{name: "not a block", text: `{{.A}}{{end}}`},
		{name: "never closed", text: `{{range .A}}{{if .B}}{{end}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			end, elseIndex, ok := matchEnd(regions(tt.text, "", ""), 0)
			if end != tt.wantEnd || elseIndex != tt.wantElse || ok != tt.ok {
				t.Errorf("matchEnd = %d, %d, %t, want %d, %d, %t", end, elseIndex, ok, tt.wantEnd, tt.wantElse, tt.ok)
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
