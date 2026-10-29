package html

import (
	"fmt"
	"html"
	"io"
	"net/url"

	"github.com/typelate/muxt/internal/template"
)

// HTMLEscape writes to w the escaped HTML equivalent of the plain text data b.
func HTMLEscape(w io.Writer, b []byte) {
	_, _ = io.WriteString(w, html.EscapeString(string(b)))
}

// HTMLEscapeString returns the escaped HTML equivalent of the plain text data s.
func HTMLEscapeString(s string) string {
	return html.EscapeString(s)
}

// HTMLEscaper returns the escaped HTML equivalent of the textual
// representation of its arguments.
func HTMLEscaper(args ...any) string {
	return html.EscapeString(fmt.Sprint(args...))
}

// JSEscape writes to w the escaped JavaScript equivalent of the plain text data b.
func JSEscape(w io.Writer, b []byte) {
	_, _ = io.WriteString(w, JSEscapeString(string(b)))
}

// JSEscapeString returns the escaped JavaScript equivalent of the plain text data s.
func JSEscapeString(s string) string {
	return template.EscapeJS(s)
}

// JSEscaper returns the escaped JavaScript equivalent of the textual
// representation of its arguments.
func JSEscaper(args ...any) string {
	return JSEscapeString(fmt.Sprint(args...))
}

// URLQueryEscaper returns the escaped value of the textual representation
// of its arguments in a form suitable for embedding in a URL query.
func URLQueryEscaper(args ...any) string {
	return url.QueryEscape(fmt.Sprint(args...))
}

// IsTrue reports whether the value is 'true', in the sense of not the zero
// of its type, and whether the value is valid. This is the definition of
// truth used by if and other such actions.
func IsTrue(val any) (truth, ok bool) {
	return template.IsTrue(val)
}
