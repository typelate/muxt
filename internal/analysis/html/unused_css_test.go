package analysis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestCheckUnusedCSS(t *testing.T) {
	type want struct {
		code     template.DiagnosticCode
		file     string
		at       string // source text starting at the diagnostic position
		contains string // message substring
	}
	for _, test := range []struct {
		name      string
		templates []string // parsed as t0.gohtml, t1.gohtml, ...
		sheets    []Stylesheet
		want      []want
	}{
		{
			name:      "rule matches an element",
			templates: []string{`<style>.a { color: red }</style><p class="a"></p>`},
		},
		{
			name:      "rule matches nothing",
			templates: []string{`<style>.missing { color: red }</style><p class="a"></p>`},
			want:      []want{{template.DiagUnusedCSSRule, "t0.gohtml", `.missing {`, `no element in the templates matches ".missing"; the rule is unused`}},
		},
		{
			name:      "one selector of a list matches nothing",
			templates: []string{`<style>.a, .missing {}</style><p class="a"></p>`},
			want:      []want{{template.DiagUnusedCSSSelector, "t0.gohtml", `.missing {`, `no element in the templates matches ".missing"`}},
		},
		{
			name:      "dynamic class may match",
			templates: []string{`<style>.b {}</style><p class="{{.C}}"></p>`},
		},
		{
			name:      "selector built with a template action is not checked",
			templates: []string{`<style>.item-{{.N}} {}</style>`},
		},
		{
			name:      "rule in a conditional is checked",
			templates: []string{`<style>{{if .X}}.missing {}{{end}}</style>`},
			want:      []want{{template.DiagUnusedCSSRule, "t0.gohtml", `.missing {`, `".missing"`}},
		},
		{
			name:      "pseudo-elements and pseudo-classes match their element",
			templates: []string{`<style>.a::before {} .a:hover {}</style><p class="a"></p>`},
		},
		{
			name:      "rule in @media is checked",
			templates: []string{`<style>@media (min-width: 1px) { .missing {} }</style>`},
			want:      []want{{template.DiagUnusedCSSRule, "t0.gohtml", `.missing {`, `".missing"`}},
		},
		{
			name:      "keyframe selectors are not checked",
			templates: []string{`<style>@keyframes spin { from {} to {} }</style>`},
		},
		{
			name:      "descendant combinator in a full document",
			templates: []string{`<html><body><style>.a .b {}</style><p class="b"></p><div class="a"></div></body></html>`},
			want:      []want{{template.DiagUnusedCSSRule, "t0.gohtml", `.a .b {`, `".a .b"`}},
		},
		{
			name: "descendant combinator matching a partial template",
			templates: []string{
				`<html><body><style>.a .b {}</style><div class="a">{{template "p"}}</div></body></html>` +
					`{{define "p"}}<p class="b"></p>{{end}}`,
			},
		},
		{
			name:      "sibling combinator whose subject exists",
			templates: []string{`<html><body><style>.x + .b {}</style><p class="b"></p></body></html>`},
		},
		{
			name:      "nested rules",
			templates: []string{`<style>.a { & .missing {} .b {} }</style><div class="a"><p class="b"></p></div>`},
			want:      []want{{template.DiagUnusedCSSRule, "t0.gohtml", `& .missing {`, `"& .missing"`}},
		},
		{
			name:      "unsupported selector is not checked",
			templates: []string{`<style>.a:has(.b .c) {}</style>`},
		},
		{
			name:      "rule matched by another template",
			templates: []string{`<style>.b {}</style>`, `<p class="b"></p>`},
		},
		{
			name:      "stylesheet file",
			templates: []string{`<p class="a"></p>`},
			sheets:    []Stylesheet{{Filename: "app.css", Text: ".a {}\n.missing {}\n"}},
			want:      []want{{template.DiagUnusedCSSRule, "app.css", `.missing {`, `".missing"`}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sources := make(map[string]string)
			var trees []*template.Tree
			for i, text := range test.templates {
				name := fmt.Sprintf("t%d.gohtml", i)
				tree, err := template.ParseTree(name, name, text, nil, lex.NewOptions())
				if err != nil {
					t.Fatalf("ParseTree(%q) error: %v", text, err)
				}
				trees = append(trees, tree)
				sources[name] = text
			}
			for _, sheet := range test.sheets {
				sources[sheet.Filename] = sheet.Text
			}

			diags := CheckUnusedCSS(trees, test.sheets)

			if len(diags) != len(test.want) {
				t.Fatalf("CheckUnusedCSS = %d diagnostics %v, want %d", len(diags), diags, len(test.want))
			}
			for i, w := range test.want {
				d := diags[i]
				if d.Code != w.code {
					t.Errorf("diagnostic %d code = %q, want %q", i, d.Code, w.code)
				}
				if d.Severity != SeverityWarning {
					t.Errorf("diagnostic %d severity = %v, want %v", i, d.Severity, SeverityWarning)
				}
				if d.Position.Filename != w.file {
					t.Errorf("diagnostic %d file = %q, want %q", i, d.Position.Filename, w.file)
				}
				if got := sources[d.Position.Filename][d.Position.Offset:]; !strings.HasPrefix(got, w.at) {
					t.Errorf("diagnostic %d is at %q, want at %q", i, got, w.at)
				}
				if !strings.Contains(d.Message, w.contains) {
					t.Errorf("diagnostic %d message = %q, want it to contain %q", i, d.Message, w.contains)
				}
			}
		})
	}

	t.Run("stylesheet position is 1-based line and column", func(t *testing.T) {
		sheets := []Stylesheet{{Filename: "app.css", Text: ".a {}\n  .missing {}\n"}}

		diags := CheckUnusedCSS(nil, sheets)

		if len(diags) != 2 {
			t.Fatalf("CheckUnusedCSS = %d diagnostics %v, want 2", len(diags), diags)
		}
		if got := diags[1].Position; got.Line != 2 || got.Column != 3 {
			t.Errorf("position = %d:%d, want 2:3", got.Line, got.Column)
		}
	})
}
