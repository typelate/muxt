package mutation

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		{name: "an empty comment", text: `{{/**/}}`, content: `/**/`},
		{name: "a right delimiter just before the comment closes", text: `{{/*}}*/}}`, content: `/*}}*/`},
		{name: "a comment whose body opens with a slash", text: `{{/*/}}*/}}`, content: `/*/}}*/`},
		{name: "a raw string holding the right delimiter", text: "{{printf `}}`}}", content: "printf `}}`"},
		{name: "a raw string may span lines", text: "{{printf `a\nb`}}", content: "printf `a\nb`"},
		{name: "a character constant", text: `{{printf "%c" '}'}}`, content: `printf "%c" '}'`},
		{name: "an escaped quote", text: `{{printf "\"}}"}}`, content: `printf "\"}}"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			found := regions(tt.text, "", "")
			require.Len(t, found, 1, "regions")
			r := found[0]
			assert.Equal(t, 0, r.start, "region start")
			assert.Equal(t, len(tt.text), r.end, "region end")

			// The content runs from after the delimiter and any leading
			// marker, which trimLeft finds, to innerEnd.
			from := trimLeft(tt.text, r.start+2, r.innerEnd)
			assert.Equal(t, tt.content, tt.text[from:r.innerEnd], "content")
			assert.Equal(t, tt.keyword, r.keyword, "keyword")
		})
	}
}

// TestLeadingWordReadsTheWholeIdentifier states that a keyword is only a
// keyword when it is the whole word. A function may be named endX or
// withDefault, and text/template lexes those as one identifier.
func TestLeadingWordReadsTheWholeIdentifier(t *testing.T) {
	for _, text := range []string{`{{endX}}`, `{{end2}}`, `{{end_x}}`, `{{withDefault .A "x"}}`, `{{ifEmpty .A}}`, `{{rangeOf .A}}`} {
		t.Run(text, func(t *testing.T) {
			assert.Empty(t, regions(text, "", "")[0].keyword, "regions(%q) keyword", text)
		})
	}
}

// TestMatchEndSkipsAFunctionNamedLikeAKeyword states that such a function
// does not open a block, so the end is matched at the right depth.
func TestMatchEndSkipsAFunctionNamedLikeAKeyword(t *testing.T) {
	const text = `{{range .Items}}{{withDefault . "x"}}{{end}}`
	end, _, ok := matchEnd(regions(text, "", ""), 0)
	assert.True(t, ok, "matchEnd ok")
	assert.Equal(t, 2, end, "matchEnd end")
}

// TestRegionsIgnoresAnUnfinishedTail states that an action cut off by the
// end of the text is not reported, and costs the actions before it nothing.
func TestRegionsIgnoresAnUnfinishedTail(t *testing.T) {
	for _, text := range []string{`{{.A}}{{`, `{{.A}}{{-`, `{{.A}}{{.B`, `{{.A}}{{/* x`, `{{.A}}{{/* x */`} {
		t.Run(text, func(t *testing.T) {
			found := regions(text, "", "")
			require.Len(t, found, 1, "regions(%q)", text)
			assert.Equal(t, "{{.A}}", text[found[0].start:found[0].end], "regions(%q) only", text)
		})
	}
}

// TestRegionsEndAStringAtALineEnd states that an interpreted string running
// into the end of a line is taken as unterminated, so the rest of the
// template is still scanned rather than swallowed.
func TestRegionsEndAStringAtALineEnd(t *testing.T) {
	const text = "{{printf \"a\nb\"}}{{.B}}"
	found := regions(text, "", "")
	require.Len(t, found, 1, "regions(%q)", text)
	assert.Equal(t, "{{.B}}", text[found[0].start:found[0].end], "regions(%q) only", text)
}

// TestRegionAt states that a position belongs to the action whose
// delimiters enclose it, from the first byte of the left one to the last
// of the right one.
func TestRegionAt(t *testing.T) {
	const text = `a{{.A}}{{.B}}b`
	found := regions(text, "", "")
	for _, tt := range []struct {
		pos, index int
		ok         bool
	}{
		{pos: 0},
		{pos: 1, index: 0, ok: true},
		{pos: 6, index: 0, ok: true},
		{pos: 7, index: 1, ok: true},
		{pos: 12, index: 1, ok: true},
		{pos: 13},
	} {
		t.Run(strconv.Itoa(tt.pos), func(t *testing.T) {
			index, _, ok := regionAt(found, tt.pos)
			assert.Equal(t, tt.index, index, "regionAt(%d) index", tt.pos)
			assert.Equal(t, tt.ok, ok, "regionAt(%d) ok", tt.pos)
		})
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
			assert.Equal(t, tt.wantEnd, end, "matchEnd end")
			assert.Equal(t, tt.wantElse, elseIndex, "matchEnd else")
			assert.Equal(t, tt.ok, ok, "matchEnd ok")
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
	require.GreaterOrEqual(t, len(found), 2, "regions starting at %v: want the actions on both sides of the unreadable one", starts)
	last := found[len(found)-1]
	assert.Equal(t, `{{.B}}`, text[last.start:last.end], "last region")
}
