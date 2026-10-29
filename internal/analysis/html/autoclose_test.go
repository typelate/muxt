package analysis

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestCheckAutoClose(t *testing.T) {
	for _, test := range []struct {
		name, input string
		nDiags      int
		contains    string
	}{
		{"div closes p", `<p>text<div>block</div></p>`, 1, "close the open <p>"},
		{"div closes p through span", `<p><span><div>x</div></span></p>`, 1, "close the open <p>"},
		{"p in td not closed by outer div rule", `<div><table><tr><td><p>x</p></td></tr></table></div>`, 0, ""},
		{"li in li", `<ul><li>a<li>b</li></ul>`, 1, "close the open <li>"},
		{"li in nested ul ok", `<ul><li>a<ul><li>b</li></ul></li></ul>`, 0, ""},
		{"h2 in h1", `<h1>a<h2>b</h2></h1>`, 1, "close the open <h1>"},
		{"td after open td", `<table><tr><td>a<td>b</td></tr></table>`, 1, "close the open <td>"},
		{"b in p ok", `<p>a<b>c</b></p>`, 0, ""},
		{"conditional div in p", `<p>{{if .X}}<div>b</div>{{end}}</p>`, 1, "in some branches"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("ParseTree error: %v", err)
			}
			var diags Diagnostics
			checkAutoClose(tree, &diags)
			if len(diags) != test.nDiags {
				t.Fatalf("checkAutoClose(%q) = %d diagnostics %v, want %d", test.input, len(diags), diags, test.nDiags)
			}
			if test.contains != "" && !strings.Contains(diags[0].Message, test.contains) {
				t.Errorf("message %q does not contain %q", diags[0].Message, test.contains)
			}
		})
	}
}
