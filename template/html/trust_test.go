package html

import (
	stdtemplate "html/template"
	"reflect"
	"strings"
	"testing"

	"github.com/typelate/muxt/template"
)

// TestTrustOfStdlibContent states that the standard library's
// html/template content types are trusted like this package's own: code
// that runs this engine in process may hand it either.
func TestTrustOfStdlibContent(t *testing.T) {
	stdHTML := stdtemplate.HTML("<b>")
	for _, tt := range []struct {
		name  string
		value any
		want  template.Trust
	}{
		{name: "html/template.HTML", value: stdtemplate.HTML("<b>"), want: template.TrustHTML},
		{name: "html/template.URL", value: stdtemplate.URL("javascript:x"), want: template.TrustURL},
		{name: "html/template.JS", value: stdtemplate.JS("x()"), want: template.TrustJS},
		{name: "html/template.JSStr", value: stdtemplate.JSStr(`a\"b`), want: template.TrustJSStr},
		{name: "html/template.CSS", value: stdtemplate.CSS("color: red"), want: template.TrustCSS},
		{name: "html/template.HTMLAttr", value: stdtemplate.HTMLAttr(`a="b"`), want: template.TrustHTMLAttr},
		{name: "html/template.Srcset", value: stdtemplate.Srcset("a.png 2x"), want: template.TrustSrcset},
		{name: "*html/template.HTML", value: &stdHTML, want: template.TrustHTML},
		{name: "HTML", value: HTML("<b>"), want: template.TrustHTML},
		{name: "URL", value: URL("javascript:x"), want: template.TrustURL},
		{name: "JS", value: JS("x()"), want: template.TrustJS},
		{name: "JSStr", value: JSStr(`a\"b`), want: template.TrustJSStr},
		{name: "CSS", value: CSS("color: red"), want: template.TrustCSS},
		{name: "HTMLAttr", value: HTMLAttr(`a="b"`), want: template.TrustHTMLAttr},
		{name: "Srcset", value: Srcset("a.png 2x"), want: template.TrustSrcset},
		{name: "string", value: "<b>", want: template.TrustNone},
		{name: "html/template.ErrorCode", value: stdtemplate.ErrBadHTML, want: template.TrustNone},
		{name: "nil", value: nil, want: template.TrustNone},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := trustOf(reflect.ValueOf(tt.value)); got != tt.want {
				t.Errorf("trustOf(%T(%v)) = %v, want %v", tt.value, tt.value, got, tt.want)
			}
		})
	}
}

// TestExecuteTrustsStdlibHTML states the same through Execute.
func TestExecuteTrustsStdlibHTML(t *testing.T) {
	ts := Must(New("page").Parse(`<p>{{.}}</p>`))
	var out strings.Builder
	if err := ts.Execute(&out, stdtemplate.HTML("<b>bold</b>")); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), `<p><b>bold</b></p>`; got != want {
		t.Errorf("Execute wrote %s, want %s", got, want)
	}
}
