package fixture

import (
	"html/template"
	"reflect"
	"strings"
	"testing"
)

func TestExecute(t *testing.T) {
	ts := template.Must(template.New("page").Parse(`<p title="{{.Title}}">{{.Body}}</p>`))
	var out strings.Builder
	if err := ts.Execute(&out, struct{ Title, Body string }{`a"b`, "<i>"}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), `<p title="a&#34;b">&lt;i&gt;</p>`; got != want {
		t.Errorf("Execute wrote %s, want %s", got, want)
	}
	if got, want := reflect.TypeOf(ts.Tree).Elem().PkgPath(), "html/template/internal/engine"; got != want {
		t.Errorf("Template.Tree is from %q, want %q: html/template is not the overlaid engine", got, want)
	}
}
