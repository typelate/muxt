package format_test

import (
	"errors"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/format"
)

func TestSource(t *testing.T) {
	for _, tt := range []struct{ name, src, want string }{
		{"no trailing newline", "<p>hi</p>", "<p>hi</p>"},
		{"one trailing newline", "<p>hi</p>\n", "<p>hi</p>\n"},
		{"two trailing newlines", "<p>hi</p>\n\n", "<p>hi</p>\n\n"},
		{"extra trailing spaces", "<p>hi</p>  \n", "<p>hi</p>  \n"},
		{"empty", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := format.Source("a.gohtml", tt.src)
			if err != nil {
				t.Fatalf("Source(%q) error: %v", tt.src, err)
			}
			if got != tt.want {
				t.Errorf("Source(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}

	_, err := format.Source("a.gohtml", "{{if}}")
	if !errors.As(err, new(*template.ParseError)) {
		t.Errorf("Source(%q) error = %v, want *template.ParseError", "{{if}}", err)
	}
}
