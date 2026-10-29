package template_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/template"
)

// parseMode selects which parser execCoverage uses for a case. Using a typed
// enum avoids the classic boolean-flag trap ("html bool — but what does
// true mean here?").
type parseMode int

const (
	parseModeText parseMode = iota
	parseModeHTML
)

// coverCase describes a single coverage scenario: a template, the data to
// execute it with, the parser to use, the expected output substring, and the
// cover-profile text the template should produce.
type coverCase struct {
	name        string
	tmpl        string
	data        any
	mode        parseMode
	wantOut     string // substring that must appear in rendered output
	wantProfile string // exact cover-profile line set (whitespace-trimmed)
}

func TestCoverage(t *testing.T) {
	cases := []coverCase{
		// --- text templates: control flow ---
		{
			name:    "if true",
			tmpl:    "{{if .}}yes{{end}}",
			data:    true,
			wantOut: "yes",
			wantProfile: `mode: set
test:1.1,1.19 3 1`,
		},
		{
			name:    "if false",
			tmpl:    "{{if .}}yes{{end}}END",
			data:    false,
			wantOut: "END",
			wantProfile: `mode: set
test:1.1,1.9 1 1
test:1.9,1.12 1 0
test:1.12,1.22 2 1`,
		},
		{
			name:    "if false with else",
			tmpl:    "{{if .}}yes{{else}}no{{end}}",
			data:    false,
			wantOut: "no",
			wantProfile: `mode: set
test:1.1,1.9 1 1
test:1.9,1.12 1 0
test:1.12,1.29 3 1`,
		},
		{
			name:    "range over slice",
			tmpl:    "{{range .}}[{{.}}]{{end}}",
			data:    []string{"a", "b"},
			wantOut: "[a][b]",
			wantProfile: `mode: set
test:1.1,1.12 1 1
test:1.12,1.19 3 2
test:1.19,1.26 1 1`,
		},
		{
			name:    "range over empty slice executes else",
			tmpl:    "{{range .}}[{{.}}]{{else}}empty{{end}}",
			data:    []string{},
			wantOut: "empty",
			wantProfile: `mode: set
test:1.1,1.12 1 1
test:1.12,1.19 3 0
test:1.19,1.39 3 1`,
		},
		{
			name:    "range with unused else branch",
			tmpl:    "{{range .}}{{.}}{{else}}empty{{end}}",
			data:    []string{"a"},
			wantOut: "a",
			wantProfile: `mode: set
test:1.1,1.17 2 1
test:1.17,1.30 2 0
test:1.30,1.37 1 1`,
		},
		{
			name:    "range with break exits after first iteration",
			tmpl:    "{{range .}}.{{break}}y{{end}}END",
			data:    1,
			wantOut: ".END",
			wantProfile: `mode: set
test:1.1,1.22 3 1
test:1.22,1.23 1 0
test:1.23,1.33 2 1`,
		},
		{
			name:    "range with break skips tail in body",
			tmpl:    "{{range .}}-{{.}}-{{break}}after{{end}}",
			data:    []int{1, 2, 3},
			wantOut: "-1-",
			wantProfile: `mode: set
test:1.1,1.28 5 1
test:1.28,1.33 1 0
test:1.33,1.40 1 1`,
		},
		{
			name:    "range with continue skips tail each iteration",
			tmpl:    "{{range .}}.{{continue}}y{{end}}END",
			data:    3,
			wantOut: "...END",
			wantProfile: `mode: set
test:1.1,1.12 1 1
test:1.12,1.25 2 3
test:1.25,1.26 1 0
test:1.26,1.36 2 1`,
		},
		{
			name:    "range with continue and multi-line body",
			tmpl:    "{{range .}}-{{.}}-{{continue}}after{{end}}",
			data:    []int{1, 2, 3},
			wantOut: "-1--2--3-",
			wantProfile: `mode: set
test:1.1,1.12 1 1
test:1.12,1.31 4 3
test:1.31,1.36 1 0
test:1.36,1.43 1 1`,
		},
		{
			name:    "with sets dot",
			tmpl:    `{{with .}}val={{.}}{{end}}`,
			data:    "hi",
			wantOut: "val=hi",
			wantProfile: `mode: set
test:1.1,1.27 4 1`,
		},

		// --- text templates: data plumbing ---
		{
			name:    "pipeline",
			tmpl:    `{{. | printf "%s!"}}`,
			data:    "go",
			wantOut: "go!",
			wantProfile: `mode: set
test:1.1,1.21 1 1`,
		},
		{
			name:    "variable assignment",
			tmpl:    `{{$x := "v"}}{{$x}}`,
			data:    nil,
			wantOut: "v",
			wantProfile: `mode: set
test:1.1,1.20 2 1`,
		},

		// --- HTML templates ---
		{
			name:    "html plain element",
			tmpl:    "<p>hello</p>",
			mode:    parseModeHTML,
			data:    nil,
			wantOut: "<p>hello</p>",
			wantProfile: `mode: set
test:1.1,1.13 4 1`,
		},
		{
			name:    "html action inside element",
			tmpl:    "<p>{{.}}</p>",
			mode:    parseModeHTML,
			data:    "world",
			wantOut: "<p>world</p>",
			wantProfile: `mode: set
test:1.1,1.13 4 1`,
		},
		{
			name:    "html static attribute",
			tmpl:    `<div class="x">y</div>`,
			mode:    parseModeHTML,
			data:    nil,
			wantOut: `<div class="x">y</div>`,
			wantProfile: `mode: set
test:1.1,1.23 5 1`,
		},
		{
			name:    "html range over list items",
			tmpl:    "<ul>{{range .}}<li>{{.}}</li>{{end}}</ul>",
			mode:    parseModeHTML,
			data:    []string{"a", "b"},
			wantOut: "<li>a</li>",
			wantProfile: `mode: set
test:1.1,1.16 3 1
test:1.16,1.30 4 2
test:1.30,1.42 2 1`,
		},
		{
			name:    "html range with break collapses to single block",
			tmpl:    "<ul>{{range .}}<li>{{.}}</li>{{break}}{{end}}</ul>",
			mode:    parseModeHTML,
			data:    []string{"a", "b"},
			wantOut: "<li>a</li>",
			wantProfile: `mode: set
test:1.1,1.51 10 1`,
		},
		{
			name:    "html comment is stripped",
			tmpl:    "<!-- secret --><p>hi</p>",
			mode:    parseModeHTML,
			data:    nil,
			wantOut: "<p>hi</p>",
			wantProfile: `mode: set
test:1.1,1.25 5 1`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, profile := execCoverage(t, tc)
			assert.Contains(t, out, tc.wantOut, "rendered output")
			assert.Equal(t,
				strings.TrimSpace(tc.wantProfile),
				strings.TrimSpace(profile),
				"cover profile")
		})
	}

	t.Run("nil *CoverProfile.WriteTo is a no-op", func(t *testing.T) {
		tree := parseText(t, "test", "{{.}}")
		var buf bytes.Buffer
		require.NoError(t, template.ExecuteTemplate(&buf, tree, "ok"))
		assert.Equal(t, "ok", buf.String())

		var cp *template.CoverProfile
		n, err := cp.WriteTo(&buf)
		require.NoError(t, err, "WriteTo on nil")
		assert.Equal(t, int64(0), n, "bytes written to WriteTo on nil")
	})
}

// execCoverage parses the template, executes it with coverage enabled, and
// returns both the rendered output and the serialized cover profile. This
// keeps the driving loop in TestCoverage focused on case-level assertions.
func execCoverage(t *testing.T, tc coverCase) (out, profile string) {
	t.Helper()

	var tree *template.Tree
	switch tc.mode {
	case parseModeHTML:
		tree = parseHTML(t, "test", tc.tmpl)
	case parseModeText:
		tree = parseText(t, "test", tc.tmpl)
	}

	var cp template.CoverProfile
	var outBuf bytes.Buffer
	require.NoError(t,
		template.ExecuteTemplate(&outBuf, tree, tc.data, template.WithCoverProfile(&cp)),
		"execute %q", tc.tmpl,
	)

	var profBuf bytes.Buffer
	_, err := cp.WriteTo(&profBuf)
	require.NoError(t, err, "CoverProfile.WriteTo")

	return outBuf.String(), profBuf.String()
}
