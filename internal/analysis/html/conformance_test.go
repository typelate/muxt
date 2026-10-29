package analysis

import (
	"sort"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// netHTMLNormalize parses s as a body fragment with the spec-complete
// x/net/html parser (adoption agency, foster parenting, implied tags)
// and re-serializes it — the "what a browser builds" canonical form.
func netHTMLNormalize(t *testing.T, s string) string {
	t.Helper()
	body := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(s), body)
	if err != nil {
		t.Fatalf("ParseFragment(%q): %v", s, err)
	}
	var sb strings.Builder
	for _, n := range nodes {
		if err := html.Render(&sb, n); err != nil {
			t.Fatalf("Render: %v", err)
		}
	}
	return sb.String()
}

// assertCodeMultiset compares got and want as multisets of diagnostic
// codes (order-insensitive). On mismatch it reports the full diagnostics
// for debugging.
func assertCodeMultiset(t *testing.T, got template.Diagnostics, want []template.DiagnosticCode) {
	t.Helper()
	gotCodes := make([]template.DiagnosticCode, len(got))
	for i, d := range got {
		gotCodes[i] = d.Code
	}
	sort.Slice(gotCodes, func(i, j int) bool { return gotCodes[i] < gotCodes[j] })
	wantCodes := append([]template.DiagnosticCode(nil), want...)
	sort.Slice(wantCodes, func(i, j int) bool { return wantCodes[i] < wantCodes[j] })
	if len(gotCodes) != len(wantCodes) {
		t.Fatalf("diagnostic codes = %v, want %v\nfull diagnostics: %v", gotCodes, wantCodes, got)
	}
	for i := range gotCodes {
		if gotCodes[i] != wantCodes[i] {
			t.Fatalf("diagnostic codes = %v, want %v\nfull diagnostics: %v", gotCodes, wantCodes, got)
		}
	}
}

// executeHTT renders tree with nil data via htt.ExecuteTemplate directly
// (no import cycle: package analysis already imports the root htt
// package). AnnotateHTMLContexts must run before execution so contextual
// escaping is applied the same way application code would see it.
func executeHTT(t *testing.T, tree *template.Tree) string {
	t.Helper()
	template.AnnotateHTMLContexts(tree.Root)
	var sb strings.Builder
	if err := template.ExecuteTemplate(&sb, tree, nil); err != nil {
		t.Fatalf("ExecuteTemplate: %v", err)
	}
	return sb.String()
}

// TestSpecConformance validates the structural checkers against the
// spec-complete x/net/html parser instead of trusting hand-derived rules.
//
// Negative cases (those with wantCodes set) do not run the oracle
// assertion — divergence from the browser view is exactly what was
// diagnosed. Oracle-asserted (clean) fixtures must be written
// oracle-safe: explicit <tbody>, no omitted optional tags. A browser
// silently inserts implied tbody/head/body elements, which is
// authoring-legal but would break a naive byte comparison between htt's
// output and the source; every oracle:true case below spells out any
// tag a browser would otherwise imply.
func TestSpecConformance(t *testing.T) {
	for _, test := range []struct {
		name, input string
		wantCodes   []template.DiagnosticCode // multiset, order-insensitive
		oracle      bool                      // assert htt output == browser view (only meaningful with no wantCodes)
	}{
		// adoption agency (§13.2.6.4.7)
		{"misnested b i", `<b><i>x</b>y`, []template.DiagnosticCode{template.DiagMisnestedTag}, false},
		{"clean formatting", `<p><b><i>x</i></b>y</p>`, nil, true},
		// foster parenting (§13.2.6.4.9)
		{"div in table", `<table><tbody><tr><td>a</td></tr></tbody><div>x</div></table>`, []template.DiagnosticCode{template.DiagFosteredContent}, false},
		{"hidden input in table", `<table><input type="hidden" name="n"><tbody><tr><td>a</td></tr></tbody></table>`, nil, true},
		{"whitespace in table", "<table> <tbody><tr><td>a</td></tr></tbody> </table>", nil, true},
		{"script in table", `<table><script>let x=1</script><tbody><tr><td>a</td></tr></tbody></table>`, nil, true},
		// orphan end tags
		{"orphan close", `a</b>c`, []template.DiagnosticCode{template.DiagOrphanEndTag}, false},
		// nested containers (§13.2.6.4.7)
		{"nested form", `<form><fieldset><form>x</form></fieldset></form>`, []template.DiagnosticCode{template.DiagNestedForm}, false},
		{"nested a", `<a href="/o">o<a href="/i">i</a></a>`, []template.DiagnosticCode{template.DiagNestedLink}, false},
		// auto-close (§13.2.6.3, §13.2.6.4.7)
		{"p div", `<p>a<div>b</div></p>`, []template.DiagnosticCode{template.DiagAutoClosed}, false},
		{"li li", `<ul><li>a<li>b</li></ul>`, []template.DiagnosticCode{template.DiagAutoClosed}, false},
		{"clean list", `<ul><li>a</li><li>b</li></ul>`, nil, true},
		// clean kitchen sink
		{"clean page fragment", `<div class="card"><h2>t</h2><p>body</p><a href="/x">more</a></div>`, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("ParseTree: %v", err)
			}
			got := CheckStructure(tree)
			assertCodeMultiset(t, got, test.wantCodes)
			if test.oracle && len(test.wantCodes) == 0 {
				httOut := executeHTT(t, tree) // helper below
				if norm, in := netHTMLNormalize(t, httOut), netHTMLNormalize(t, test.input); norm != in {
					t.Errorf("browser view of htt output differs from browser view of source\nhtt:     %s\nbrowser: %s", norm, in)
				}
			}
		})
	}
}

// FuzzCheckStructure is a no-panic bridge: CheckStructure must not panic
// on any input the parser accepts, regardless of how malformed the HTML
// structure is.
func FuzzCheckStructure(f *testing.F) {
	f.Add(`<table><div>x</div></table>`)
	f.Add(`<b><i>x</b>y`)
	f.Add(`<form><form></form></form>`)
	f.Add(`<p><span><div>x`)
	f.Fuzz(func(t *testing.T, input string) {
		tree, err := template.ParseTree("fuzz.gohtml", "fuzz", input, nil, lex.NewOptions())
		if err != nil || tree == nil {
			return
		}
		_ = CheckStructure(tree) // must not panic on any parseable input
	})
}
