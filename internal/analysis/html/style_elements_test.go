package analysis

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestCheckStyleElements(t *testing.T) {
	type want struct {
		code     template.DiagnosticCode
		at       string // source text starting at the diagnostic position
		contains string // message substring
	}
	for _, test := range []struct {
		name, input string
		want        []want
	}{
		{name: "valid rule", input: `<style>.a { color: red !important; --x: anything }</style>`},
		{
			name:  "unknown property",
			input: `<style>.a { colr: red }</style>`,
			want:  []want{{template.DiagUnknownCSSProperty, `colr: red`, `unknown CSS property "colr"`}},
		},
		{
			name:  "invalid value",
			input: `<style>.a { margin: 10px foo }</style>`,
			want:  []want{{template.DiagInvalidCSSValue, `foo }`, `"10px foo" is not a valid value for CSS property "margin"`}},
		},
		{
			name:  "rule in @media",
			input: `<style>@media (min-width: 1px) { .a { colr: red } }</style>`,
			want:  []want{{template.DiagUnknownCSSProperty, `colr: red`, `"colr"`}},
		},
		{
			name:  "nested rule",
			input: `<style>.a { color: red; .b { colr: red } }</style>`,
			want:  []want{{template.DiagUnknownCSSProperty, `colr: red`, `"colr"`}},
		},
		{
			name:  "declaration in @media nested in a rule",
			input: `<style>.a { @media (min-width: 1px) { colr: red } }</style>`,
			want:  []want{{template.DiagUnknownCSSProperty, `colr: red`, `"colr"`}},
		},
		{name: "@font-face descriptors are not properties", input: `<style>@font-face { font-family: X; src: url(x.woff2) }</style>`},
		{name: "@page descriptors are not properties", input: `<style>@page { margin: 1cm; size: A4 }</style>`},
		{
			name:  "unknown at-rule",
			input: `<style>@medai screen { .a { color: red } }</style>`,
			want:  []want{{template.DiagUnknownCSSAtRule, `@medai`, `unknown CSS at-rule "@medai"`}},
		},
		{name: "vendor-prefixed at-rule", input: `<style>@-moz-document url-prefix() { .a { color: red } }</style>`},
		{name: "declaration with a template action", input: `<style>.a { color: {{.C}} }</style>`},
		{
			name:  "rule in a conditional",
			input: `<style>{{if .X}}.a { colr: red }{{end}}</style>`,
			want:  []want{{template.DiagUnknownCSSProperty, `colr: red`, `"colr"`}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("ParseTree error: %v", err)
			}

			diags := CheckStyleElements(tree)

			if len(diags) != len(test.want) {
				t.Fatalf("CheckStyleElements(%q) = %d diagnostics %v, want %d", test.input, len(diags), diags, len(test.want))
			}
			for i, w := range test.want {
				d := diags[i]
				if d.Code != w.code {
					t.Errorf("diagnostic %d code = %q, want %q", i, d.Code, w.code)
				}
				if d.Severity != SeverityWarning {
					t.Errorf("diagnostic %d severity = %v, want %v", i, d.Severity, SeverityWarning)
				}
				if got := test.input[d.Pos:]; !strings.HasPrefix(got, w.at) {
					t.Errorf("diagnostic %d is at %q, want at %q", i, got, w.at)
				}
				if !strings.Contains(d.Message, w.contains) {
					t.Errorf("diagnostic %d message = %q, want it to contain %q", i, d.Message, w.contains)
				}
			}
		})
	}
}

func TestCheckStylesheet(t *testing.T) {
	sheet := Stylesheet{Filename: "app.css", Text: ".a {\n  colr: red;\n}\n"}

	diags := CheckStylesheet(sheet)

	if len(diags) != 1 {
		t.Fatalf("CheckStylesheet = %d diagnostics %v, want 1", len(diags), diags)
	}
	d := diags[0]
	if d.Code != template.DiagUnknownCSSProperty {
		t.Errorf("code = %q, want %q", d.Code, template.DiagUnknownCSSProperty)
	}
	if got := d.Position; got.Filename != "app.css" || got.Line != 2 || got.Column != 3 {
		t.Errorf("position = %s, want app.css:2:3", got)
	}
}
