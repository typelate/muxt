package analysis

import (
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestStyleDeclarationAt(t *testing.T) {
	for _, test := range []struct {
		name, input, cursor string
		want                StyleDeclaration // compared field by field below; zero Property means none
		problem             string
	}{
		{
			name:   "longhand property",
			input:  `<p style="margin: 0; color: red">x</p>`,
			cursor: "red",
			want:   StyleDeclaration{Property: "color", Value: "red", Known: true, Syntax: "<color>", Initial: "canvastext", Inherited: true},
		},
		{
			name:   "shorthand property",
			input:  `<p style="margin: 0; color: red">x</p>`,
			cursor: "margin",
			want:   StyleDeclaration{Property: "margin", Value: "0", Known: true, Shorthand: true, Initial: "margin-bottom, margin-left, margin-right, margin-top"},
		},
		{
			name:    "unknown property",
			input:   `<p style="colr: red">x</p>`,
			cursor:  "colr",
			want:    StyleDeclaration{Property: "colr", Value: "red"},
			problem: `unknown CSS property "colr"`,
		},
		{name: "outside a style attribute", input: `<p title="color: red">x</p>`, cursor: "red"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("ParseTree error: %v", err)
			}
			pos := lex.Pos(strings.Index(test.input, test.cursor))

			got, ok := StyleDeclarationAt(tree, pos)

			if want := test.want.Property != ""; ok != want {
				t.Fatalf("StyleDeclarationAt(%q, %d) found = %t, want %t", test.input, pos, ok, want)
			}
			if !ok {
				return
			}
			if got.Property != test.want.Property || got.Value != test.want.Value || got.Known != test.want.Known ||
				got.Shorthand != test.want.Shorthand || got.Inherited != test.want.Inherited || got.Initial != test.want.Initial {
				t.Errorf("StyleDeclarationAt(%q, %d) = %+v, want %+v", test.input, pos, got, test.want)
			}
			if test.want.Syntax != "" && got.Syntax != test.want.Syntax {
				t.Errorf("Syntax = %q, want %q", got.Syntax, test.want.Syntax)
			}
			if got.Pos > pos || got.End <= pos {
				t.Errorf("declaration span [%d, %d) does not contain the cursor %d", got.Pos, got.End, pos)
			}
			switch {
			case test.problem == "" && len(got.Problems) > 0:
				t.Errorf("Problems = %v, want none", got.Problems)
			case test.problem != "" && (len(got.Problems) != 1 || !strings.Contains(got.Problems[0].Message, test.problem)):
				t.Errorf("Problems = %v, want one containing %q", got.Problems, test.problem)
			}
		})
	}
}

func TestCheckStyleAttributes(t *testing.T) {
	type want struct {
		code     template.DiagnosticCode
		at       string // source text starting at the diagnostic position
		contains string // message substring
	}
	for _, test := range []struct {
		name, input string
		want        []want
	}{
		{name: "valid declarations", input: `<p style="color: red; margin: 0 auto"></p>`},
		{
			name:  "unknown property",
			input: `<p style="colr: red"></p>`,
			want:  []want{{template.DiagUnknownCSSProperty, `colr: red`, `unknown CSS property "colr"`}},
		},
		{
			name:  "value does not match the property syntax",
			input: `<p style="margin: 10px foo"></p>`,
			want:  []want{{template.DiagInvalidCSSValue, `foo"`, `"10px foo" is not a valid value for CSS property "margin"; want <'margin-top'>{1,4}`}},
		},
		{name: "declaration with a template action is not checked", input: `<p style="color: {{.C}}; margin: 0"></p>`},
		{
			name:  "static declaration next to a dynamic one is checked",
			input: `<p style="color: {{.C}}; colr: red"></p>`,
			want:  []want{{template.DiagUnknownCSSProperty, `colr: red`, `unknown CSS property "colr"`}},
		},
		{name: "whole value is dynamic", input: `<p style="{{.S}}"></p>`},
		{name: "custom property and var()", input: `<p style="--gap: 1rem; gap: var(--gap)"></p>`},
		{name: "unknown vendor-prefixed property", input: `<p style="-moz-made-up: 1"></p>`},
		{name: "other attribute", input: `<p title="colr: red"></p>`},
		{
			name:  "conditional style attribute",
			input: `<p {{if .X}}style="colr: red"{{end}}></p>`,
			want:  []want{{template.DiagUnknownCSSProperty, `colr: red`, `unknown CSS property "colr"`}},
		},
		{
			name:  "style attribute name is case-insensitive",
			input: `<p STYLE="colr: red"></p>`,
			want:  []want{{template.DiagUnknownCSSProperty, `colr: red`, `unknown CSS property "colr"`}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("ParseTree error: %v", err)
			}

			diags := CheckStyleAttributes(tree)

			if len(diags) != len(test.want) {
				t.Fatalf("CheckStyleAttributes(%q) = %d diagnostics %v, want %d", test.input, len(diags), diags, len(test.want))
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
