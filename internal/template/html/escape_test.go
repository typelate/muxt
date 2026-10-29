package html_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/template/html"
)

func TestJSEscapeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "null byte",
			input: "\x00",
			want:  `\u0000`,
		},
		{
			name:  "line separator U+2028",
			input: "\u2028",
			want:  `\u2028`,
		},
		{
			name:  "paragraph separator U+2029",
			input: "\u2029",
			want:  `\u2029`,
		},
		{
			name:  "backtick",
			input: "`",
			want:  `\u0060`,
		},
		{
			name:  "basic escapes still work",
			input: `\"'<>&`,
			want:  `\\\u0022\u0027\u003c\u003e\u0026`,
		},
		{
			name:  "plain text unchanged",
			input: "hello world",
			want:  "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := html.JSEscapeString(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestJSEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "null byte",
			input: "\x00",
			want:  `\u0000`,
		},
		{
			name:  "line separator U+2028",
			input: "\u2028",
			want:  `\u2028`,
		},
		{
			name:  "backtick",
			input: "`",
			want:  `\u0060`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			html.JSEscape(&buf, []byte(tt.input))
			assert.Equal(t, tt.want, buf.String())
		})
	}
}
