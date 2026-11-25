package format_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/htt"
	"github.com/typelate/htt/lex"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template/format"
)

// rest is content the formatter reindents, so a file left as written
// shows: restFormatted is what it becomes.
const (
	rest          = "\n<div>\n<p>b</p>\n</div>"
	restFormatted = "\n<div>\n\t<p>b</p>\n</div>"
)

// TestSelfClosedWhereTheFileCallsIt: a self-closed element that is not
// void closes in SVG and MathML content, so a define or block holding one
// is no obstacle when every place the file renders it is such content.
func TestSelfClosedWhereTheFileCallsIt(t *testing.T) {
	for _, tt := range []struct {
		name, input, want string
	}{
		{
			name:  "define called only inside svg",
			input: `{{define "icon"}}<path d="M0"/>{{end}}<svg>{{template "icon"}}</svg>` + rest,
			want:  `{{define "icon"}}<path d="M0"/>{{end}}<svg>{{template "icon"}}</svg>` + restFormatted,
		},
		{
			name:  "define called only inside math",
			input: `{{define "m"}}<mspace/>{{end}}<math>{{template "m"}}</math>` + rest,
			want:  `{{define "m"}}<mspace/>{{end}}<math>{{template "m"}}</math>` + restFormatted,
		},
		{
			name:  "define called twice inside svg, once in a branch",
			input: `{{define "icon"}}<path/>{{end}}<svg>{{if .A}}{{template "icon"}}{{end}}<g>{{template "icon"}}</g></svg>` + rest,
			want:  `{{define "icon"}}<path/>{{end}}<svg>{{if .A}}{{template "icon"}}{{end}}<g>{{template "icon"}}</g></svg>` + restFormatted,
		},
		{
			name:  "define called by a define called only inside svg",
			input: `{{define "group"}}<g>{{template "icon"}}</g>{{end}}{{define "icon"}}<path/>{{end}}<svg>{{template "group"}}</svg>` + rest,
			want:  `{{define "group"}}<g>{{template "icon"}}</g>{{end}}{{define "icon"}}<path/>{{end}}<svg>{{template "group"}}</svg>` + restFormatted,
		},
		{
			name:  "block inside svg",
			input: `<svg>{{block "icon" .}}<path/>{{end}}</svg>` + rest,
			want:  `<svg>{{block "icon" .}}<path/>{{end}}</svg>` + restFormatted,
		},
		{
			name:  "define called inside svg that calls itself",
			input: `{{define "tree"}}<path/>{{range .Kids}}{{template "tree" .}}{{end}}{{end}}<svg>{{template "tree" .}}</svg>` + rest,
			want:  `{{define "tree"}}<path/>{{range .Kids}}{{template "tree" .}}{{end}}{{end}}<svg>{{template "tree" .}}</svg>` + restFormatted,
		},
		{
			name:  "define called inside svg in a define the file does not call",
			input: `{{define "icon"}}<path/>{{end}}{{define "page"}}<svg>{{template "icon"}}</svg>` + rest + `{{end}}`,
			want:  `{{define "icon"}}<path/>{{end}}{{define "page"}}<svg>{{template "icon"}}</svg>` + restFormatted + `{{end}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := format.Source("t.gohtml", tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got, "Source(%q)", tt.input)
		})
	}
}

// TestSelfClosedWhereACallIsHTML: where one place the file renders the
// define is HTML content, the browser ignores the slash there, and the
// file is left as written.
func TestSelfClosedWhereACallIsHTML(t *testing.T) {
	for _, tt := range []struct {
		name, input string
	}{
		{name: "called inside svg and inside a div", input: `{{define "icon"}}<path/>{{end}}<svg>{{template "icon"}}</svg><div>{{template "icon"}}  a  b</div>`},
		{name: "called inside svg after a breakout tag", input: `{{define "icon"}}<path/>{{end}}<svg><b></b>{{template "icon"}}  a  b</svg>`},
		{name: "called inside foreignObject", input: `{{define "icon"}}<path/>{{end}}<svg><foreignObject>{{template "icon"}}  a  b</foreignObject></svg>`},
		{name: "called by a define called inside a div", input: `{{define "group"}}{{template "icon"}}  a  b{{end}}{{define "icon"}}<path/>{{end}}<svg>{{template "group"}}</svg><div>{{template "group"}}</div>`},
		{name: "a self-closed pre in a define called inside svg breaks out", input: `{{define "x"}}<pre/>  a  b{{end}}<svg>{{template "x"}}</svg>`},
		{name: "a define that breaks out of svg, then a self-closed path", input: `{{define "d"}}<div></div>{{end}}<svg>{{template "d"}}<path/>  a  b</svg>`},
		{name: "a block that breaks out of svg, then a self-closed path", input: `<svg>{{block "d" .}}<div></div>{{end}}<path/>  a  b</svg>`},
		{name: "block inside svg also called inside a div", input: `<svg>{{block "icon" .}}<path/>{{end}}</svg><div>{{template "icon"}}  a  b</div>`},
		{name: "called inside svg and in an attribute value", input: `{{define "icon"}}<path/>{{end}}<svg>{{template "icon"}}</svg><a title="{{template "icon"}}">  a  b</a>`},
		{name: "called inside svg by a define also called inside a div", input: `{{define "group"}}<path/>{{end}}{{define "a"}}{{template "group"}}  a  b{{end}}<svg>{{template "a"}}</svg><div>{{template "a"}}</div>`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := format.Source("t.gohtml", tt.input+rest)
			require.NoError(t, err)
			assert.Equal(t, tt.input+rest, got, "Source(%q)", tt.input+rest)
		})
	}
}

// TestSelfClosedWhereThePackageCallsIt: the package's other templates
// render a define too. One of them calling it inside svg makes its body
// SVG content; one calling it in HTML content makes it HTML.
func TestSelfClosedWhereThePackageCallsIt(t *testing.T) {
	const icon = "{{define \"icon\"}}\n<g>\n<path/>\n</g>\n{{end}}"
	for _, tt := range []struct {
		name, input, context, want string
	}{
		{
			// The formatter leaves a body called inside svg as it is,
			// closed elements or not, so the body shows no change.
			name:    "called inside svg only by another template",
			input:   icon + rest,
			context: `<svg>{{template "icon"}}</svg>`,
			want:    icon + restFormatted,
		},
		{
			name:    "called inside a div only by another template: the body is as written",
			input:   icon + rest,
			context: `<div>{{template "icon"}}</div>`,
			want:    icon + restFormatted,
		},
		{
			name:    "called by no template: the body is as written",
			input:   icon + rest,
			context: `<div></div>`,
			want:    icon + restFormatted,
		},
		{
			name:    "called inside svg in the file and inside a div by another template",
			input:   icon + `<svg>{{template "icon"}}</svg>` + rest,
			context: `<div>{{template "icon"}}</div>`,
			want:    icon + `<svg>{{template "icon"}}</svg>` + rest,
		},
		{
			name:    "called inside svg by a define of another template that a div calls",
			input:   icon + rest,
			context: `{{define "group"}}{{template "icon"}}{{end}}<svg>{{template "group"}}</svg><div>{{template "group"}}</div>`,
			want:    icon + restFormatted,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			context, err := htt.ParseTree("other.gohtml", "other.gohtml", tt.context, nil, lex.Options{
				EmitComment: true, ParseHTML: true, BreakOK: true, ContinueOK: true, LeftDelim: "{{", RightDelim: "}}",
			})
			require.NoError(t, err)
			got, err := format.Source("t.gohtml", tt.input, format.Styles(analysis.WhitespaceOptions{Context: []*htt.Tree{context}}))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got, "Source(%q) beside %q", tt.input, tt.context)
		})
	}
}

// A define Go executes as a route is HTML content, whatever calls it.
func TestSelfClosedInARouteCalledInsideSVG(t *testing.T) {
	const input = `{{define "GET /icon"}}<path/>{{end}}<svg>{{template "GET /icon"}}</svg>` + rest
	got, err := format.Source("t.gohtml", input)
	require.NoError(t, err)
	assert.Equal(t, input, got, "Source(%q)", input)
}
