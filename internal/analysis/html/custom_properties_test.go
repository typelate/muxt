package analysis

import (
	"fmt"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestCheckCustomProperties(t *testing.T) {
	type want struct {
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
			name:      "declared and used in a style element",
			templates: []string{`<style>:root { --brand: red } .a { color: var(--brand) }</style>`},
		},
		{
			name:      "used but never declared",
			templates: []string{`<style>.a { color: var(--brnad) }</style>`},
			want:      []want{{"t0.gohtml", `--brnad)`, `custom property "--brnad" is never declared`}},
		},
		{
			name:      "fallback value",
			templates: []string{`<style>.a { color: var(--x, red) }</style>`},
		},
		{
			name:      "declared in a style attribute",
			templates: []string{`<p style="--gap: 1px"></p><style>.a { margin: var(--gap) }</style>`},
		},
		{
			name:      "used in a style attribute",
			templates: []string{`<p style="margin: var(--gap)"></p>`},
			want:      []want{{"t0.gohtml", `--gap)`, `"--gap"`}},
		},
		{
			name:      "declared in a stylesheet file",
			templates: []string{`<p style="margin: var(--gap)"></p>`},
			sheets:    []Stylesheet{{Filename: "app.css", Text: ":root { --gap: 1rem }"}},
		},
		{
			name:   "used in a stylesheet file",
			sheets: []Stylesheet{{Filename: "app.css", Text: ".a {\n  margin: var(--gap);\n}"}},
			want:   []want{{"app.css", `--gap)`, `"--gap"`}},
		},
		{
			name:      "declared by @property",
			templates: []string{`<style>@property --angle { syntax: '<angle>'; inherits: false; initial-value: 0deg } .a { rotate: var(--angle) }</style>`},
		},
		{
			name:      "used in another custom property",
			templates: []string{`<style>:root { --a: var(--b) }</style>`},
			want:      []want{{"t0.gohtml", `--b)`, `"--b"`}},
		},
		{
			name: "a template may declare any custom property",
			templates: []string{
				`<p style="{{.Name}}: 1px"></p>`,
				`<style>.a { margin: var(--gap) }</style>`,
			},
		},
		{
			name:      "reference built with a template action",
			templates: []string{`<style>.a { margin: var(--{{.N}}) }</style>`},
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

			diags := CheckCustomProperties(trees, test.sheets)

			if len(diags) != len(test.want) {
				t.Fatalf("CheckCustomProperties = %d diagnostics %v, want %d", len(diags), diags, len(test.want))
			}
			for i, w := range test.want {
				d := diags[i]
				if d.Code != template.DiagUndefinedCSSCustomProperty {
					t.Errorf("diagnostic %d code = %q, want %q", i, d.Code, template.DiagUndefinedCSSCustomProperty)
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
}
