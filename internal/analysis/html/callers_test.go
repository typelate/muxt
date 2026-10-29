package analysis_test

import (
	"testing"

	"github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

func TestCallers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string][]template.HTMLContext // template name -> expected contexts
	}{
		{
			name:  "single template call in text",
			input: `<p>{{template "widget" .}}</p>`,
			expected: map[string][]template.HTMLContext{
				"widget": {{State: "text", Element: "p"}},
			},
		},
		{
			name:  "single template call in attr",
			input: `<a href="{{template "url" .}}">link</a>`,
			expected: map[string][]template.HTMLContext{
				"url": {{State: "attr", Element: "a", Attr: "href"}},
			},
		},
		{
			name:  "same template called from two contexts",
			input: `<a href="{{template "x" .}}">{{template "x" .}}</a>`,
			expected: map[string][]template.HTMLContext{
				"x": {
					{State: "attr", Element: "a", Attr: "href"},
					{State: "text", Element: "a"},
				},
			},
		},
		{
			name:  "multiple templates",
			input: `<p>{{template "a" .}}</p><div class="{{template "b" .}}">text</div>`,
			expected: map[string][]template.HTMLContext{
				"a": {{State: "text", Element: "p"}},
				"b": {{State: "attr", Element: "div", Attr: "class"}},
			},
		},
		{
			name:  "template in comment",
			input: `<!-- {{template "info" .}} -->`,
			expected: map[string][]template.HTMLContext{
				"info": {{State: "comment"}},
			},
		},
		{
			name:     "no template calls",
			input:    `<p>{{.Text}}</p>`,
			expected: map[string][]template.HTMLContext{},
		},
		{
			name:  "template in range",
			input: `<ul>{{range .Items}}<li>{{template "item" .}}</li>{{end}}</ul>`,
			expected: map[string][]template.HTMLContext{
				"item": {{State: "text", Element: "li"}},
			},
		},
		{
			name:  "template in if branches",
			input: `{{if .Show}}<p>{{template "x" .}}</p>{{else}}<div>{{template "x" .}}</div>{{end}}`,
			expected: map[string][]template.HTMLContext{
				"x": {
					{State: "text", Element: "p"},
					{State: "text", Element: "div"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			template.AnnotateHTMLContexts(tree.Root)

			callers := analysis.Callers(tree)

			// Check expected templates are found.
			for name, wantContexts := range test.expected {
				gotNodes, ok := callers[name]
				if !ok {
					t.Errorf("template %q: not found in callers", name)
					continue
				}
				if len(gotNodes) != len(wantContexts) {
					t.Errorf("template %q: got %d call sites, want %d", name, len(gotNodes), len(wantContexts))
					continue
				}
				for i, want := range wantContexts {
					got := gotNodes[i].HTMLContext
					if got == nil {
						t.Errorf("template %q call site %d: HTMLContext is nil", name, i)
						continue
					}
					if got.State != want.State {
						t.Errorf("template %q call site %d: state = %q, want %q", name, i, got.State, want.State)
					}
					if got.Element != want.Element {
						t.Errorf("template %q call site %d: element = %q, want %q", name, i, got.Element, want.Element)
					}
					if got.Attr != want.Attr {
						t.Errorf("template %q call site %d: attr = %q, want %q", name, i, got.Attr, want.Attr)
					}
				}
			}

			// Check no unexpected templates.
			for name := range callers {
				if _, ok := test.expected[name]; !ok {
					t.Errorf("unexpected template %q in callers", name)
				}
			}
		})
	}
}

func TestContextConflicts(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantConflicts []string // template names expected to have conflicts
	}{
		{
			name:          "no conflicts - single call",
			input:         `<p>{{template "x" .}}</p>`,
			wantConflicts: nil,
		},
		{
			name:          "no conflicts - same state",
			input:         `<p>{{template "x" .}}</p><div>{{template "x" .}}</div>`,
			wantConflicts: nil,
		},
		{
			name:          "conflict - text vs attr",
			input:         `<a href="{{template "x" .}}">{{template "x" .}}</a>`,
			wantConflicts: []string{"x"},
		},
		{
			name:          "conflict - text vs comment",
			input:         `<p>{{template "x" .}}</p><!-- {{template "x" .}} -->`,
			wantConflicts: []string{"x"},
		},
		{
			name:          "conflict - attr vs comment",
			input:         `<a href="{{template "x" .}}">link</a><!-- {{template "x" .}} -->`,
			wantConflicts: []string{"x"},
		},
		{
			name:          "no conflicts - no template calls",
			input:         `<p>{{.Text}}</p>`,
			wantConflicts: nil,
		},
		{
			name:          "mixed - one conflict one clean",
			input:         `<p>{{template "ok" .}}</p><div>{{template "ok" .}}</div><a href="{{template "bad" .}}">{{template "bad" .}}</a>`,
			wantConflicts: []string{"bad"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree, err := template.ParseTree("test.gohtml", test.name, test.input, nil, lex.NewOptions())
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			template.AnnotateHTMLContexts(tree.Root)

			callers := analysis.Callers(tree)
			conflicts := analysis.ContextConflicts(callers)

			gotNames := make(map[string]bool)
			for _, c := range conflicts {
				gotNames[c.Name] = true
				// Verify Error() returns something useful.
				if c.Error() == "" {
					t.Errorf("conflict for %q: Error() returned empty string", c.Name)
				}
			}

			for _, want := range test.wantConflicts {
				if !gotNames[want] {
					t.Errorf("expected conflict for template %q, but not found", want)
				}
				delete(gotNames, want)
			}
			for extra := range gotNames {
				t.Errorf("unexpected conflict for template %q", extra)
			}
		})
	}
}
