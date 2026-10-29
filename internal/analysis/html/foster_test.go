package analysis

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestCheckFosterParenting(t *testing.T) {
	for _, test := range []struct {
		name, input string
		nDiags      int
		contains    string
	}{
		{"div in table", `<table><div>x</div><tr><td>a</td></tr></table>`, 1, "before the <table>"},
		{"div in tbody", `<table><tbody><div>x</div></tbody></table>`, 1, "before the <table>"},
		{"div in tr", `<table><tr><div>x</div></tr></table>`, 1, "before the <table>"},
		{"text in table", `<table>words<tr><td>a</td></tr></table>`, 1, "text in <table>"},
		{"whitespace text ok", "<table>\n  <tr><td>a</td></tr>\n</table>", 0, ""},
		{"tr td ok", `<table><tbody><tr><td>a</td></tr></tbody></table>`, 0, ""},
		{"caption colgroup ok", `<table><caption>c</caption><colgroup><col></colgroup><tr><td>a</td></tr></table>`, 0, ""},
		{"script style template form ok", `<table><script>s</script><style>c</style><form></form><tr><td>a</td></tr></table>`, 0, ""},
		{"hidden input ok", `<table><input type="hidden" name="n"><tr><td>a</td></tr></table>`, 0, ""},
		{"text input fostered", `<table><input type="text"><tr><td>a</td></tr></table>`, 1, "before the <table>"},
		{"template action ok", `<table>{{template "rows" .}}</table>{{define "rows"}}<tr><td>a</td></tr>{{end}}`, 0, ""},
		{"conditional div", `<table>{{if .X}}<div>x</div>{{end}}</table>`, 1, "in some branches"},
		{"repeated div in table", `<table>{{range .Rows}}<div>x</div>{{end}}</table>`, 1, "(in a loop body)"},
		{"range rows ok", `<table>{{range .Rows}}<tr><td>{{.}}</td></tr>{{end}}</table>`, 0, ""},
		{"div in td ok", `<table><tr><td><div>x</div></td></tr></table>`, 0, ""},
		{"nested table isolates", `<table><tr><td><table><div>x</div></table></td></tr></table>`, 1, "before the <table>"},
		{"comment in table ok", "<table><!-- note --><tr><td>a</td></tr></table>", 0, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("ParseTree error: %v", err)
			}
			var diags Diagnostics
			checkFosterParenting(tree, &diags)
			if len(diags) != test.nDiags {
				t.Fatalf("checkFosterParenting(%q) = %d diagnostics %v, want %d", test.input, len(diags), diags, test.nDiags)
			}
			if test.contains != "" && !strings.Contains(diags[0].Message, test.contains) {
				t.Errorf("message %q does not contain %q", diags[0].Message, test.contains)
			}
		})
	}
}
