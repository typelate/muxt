package format

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCountSensitiveOpens(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{
			name: "actual pre tag",
			line: "<pre>",
			want: 1,
		},
		{
			name: "pre with attributes",
			line: `<pre class="code">`,
			want: 1,
		},
		{
			name: "pre inside attribute value",
			line: `<div title="<pre>">`,
			want: 0,
		},
		{
			name: "textarea inside attribute value",
			line: `<div data-content="<textarea>">`,
			want: 0,
		},
		{
			name: "no sensitive tags",
			line: `<div class="normal">`,
			want: 0,
		},
		{
			name: "pre tag after attribute with pre",
			line: `<div title="<pre>"><pre>`,
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countSensitiveOpens(tt.line)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCountSensitiveCloses(t *testing.T) {
	tests := []struct {
		name string
		line string
		want int
	}{
		{
			name: "actual pre close tag",
			line: "</pre>",
			want: 1,
		},
		{
			name: "close pre inside attribute value",
			line: `<div title="</pre>">`,
			want: 0,
		},
		{
			name: "no sensitive close tags",
			line: `</div>`,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countSensitiveCloses(tt.line)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCleanWhitespace_PreInAttribute(t *testing.T) {
	// When <pre> appears inside an attribute value, cleanWhitespace should
	// NOT enter preserve mode. Trailing whitespace should still be trimmed
	// and consecutive blank lines should still be collapsed.
	input := "<div title=\"<pre>\">\n\t<p>a</p>  \n\n\n\t<p>b</p>\n</div>"
	want := "<div title=\"<pre>\">\n\t<p>a</p>\n\n\t<p>b</p>\n</div>"
	got := cleanWhitespace(input)
	assert.Equal(t, want, got)
}
