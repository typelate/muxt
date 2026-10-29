package analysis

import (
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestCheckStructure(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		nDiags   int    // expected number of diagnostics
		contains string // substring expected in first diagnostic (if any)
	}{
		{name: "well-formed", input: "<div><p>hello</p></div>", nDiags: 0, contains: ""},
		{name: "empty element", input: "<div></div>", nDiags: 0, contains: ""},
		{name: "void element", input: "<br>", nDiags: 0, contains: ""},
		{name: "self-closing void", input: "<img />", nDiags: 0, contains: ""},
		{name: "unclosed div", input: "<div><p>hello</p>", nDiags: 1, contains: "<div>"},
		// p's end tag is omissible (WHATWG implied end tags); no longer flagged as unclosed.
		{name: "unclosed p", input: "<p>hello", nDiags: 0, contains: ""},
		{name: "unclosed nested", input: "<div><p>hello", nDiags: 1, contains: "<div>"},
		{name: "sibling unclosed", input: "<div></div><p>hello", nDiags: 0, contains: ""},
		{name: "template only", input: "{{.X}}", nDiags: 0, contains: ""},
		{name: "html with template", input: "<p>{{.X}}</p>", nDiags: 0, contains: ""},
		{name: "closed in if", input: "{{if .X}}<div>hello</div>{{end}}", nDiags: 0, contains: ""},
		{name: "define well-formed", input: `{{define "x"}}<div>hello</div>{{end}}`, nDiags: 0, contains: ""},
		{name: "raw text element closed", input: "<script>var x = 1;</script>", nDiags: 0, contains: ""},
		// Unclosed elements inside control structures.
		{name: "unclosed in if branch", input: "{{if .X}}<div>open{{end}}", nDiags: 1, contains: "<div>"},
		{name: "unclosed in range branch", input: "{{range .X}}<p>text{{end}}", nDiags: 0, contains: ""},
		{name: "unclosed in with branch", input: "{{with .X}}<span>text{{end}}", nDiags: 1, contains: "<span>"},
		{name: "unclosed in else branch", input: "{{if .X}}<div></div>{{else}}<p>open{{end}}", nDiags: 0, contains: ""},
		{name: "unclosed in define", input: `{{define "x"}}<div>text{{end}}`, nDiags: 1, contains: "<div>"},
		// Multiple unclosed elements.
		{name: "deeply nested unclosed", input: "<div><section><article><p>text", nDiags: 3, contains: "<div>"},
		// Self-closing non-void should not produce unclosed diagnostic.
		{name: "self-closing non-void no diag", input: "<div/>", nDiags: 0, contains: ""},
		// Implied end tags (li, p, ...) are spec-blessed authoring, not
		// bugs — but the second <li> still auto-closes the first, which
		// checkAutoClose reports (a different diagnostic than "unclosed").
		{name: "implied end tag li is not unclosed", input: `<ul><li>a<li>b</ul>`, nDiags: 1, contains: "close the open <li>"},
		{name: "implied end tag p is not unclosed", input: `<div><p>a</div>`, nDiags: 0, contains: ""},
		{name: "span is still unclosed", input: `<div><span></div>`, nDiags: 1, contains: "element <span> is not closed"},
		{name: "misnested formatting not double reported", input: `<b><i>x</b>y`, nDiags: 1, contains: "browsers reopen a copy of <i>"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmpl, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			diags := CheckStructure(tmpl)
			if len(diags) != test.nDiags {
				t.Errorf("got %d diagnostics, want %d", len(diags), test.nDiags)
				for _, d := range diags {
					t.Logf("  %s", d)
				}
			}
			if test.contains != "" && len(diags) > 0 {
				found := false
				for _, d := range diags {
					if contains(d.Message, test.contains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no diagnostic contains %q; got %v", test.contains, diags)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
