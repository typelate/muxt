package template_test

import (
	"bytes"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/net/html"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// parseText parses a plain text template (non-HTML).
func parseText(t *testing.T, name, text string) *template.Tree {
	t.Helper()
	tree, err := template.ParseTree(name, name, text, nil, lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		BreakOK:    true,
		ContinueOK: true,
	})
	require.NoError(t, err, "parse %q", text)
	return tree
}

// parseHTML parses an HTML template and annotates HTML contexts for escaping.
func parseHTML(t *testing.T, name, text string) *template.Tree {
	t.Helper()
	tree, err := template.ParseTree(name, name, text, nil, lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		ParseHTML:  true,
		BreakOK:    true,
		ContinueOK: true,
	})
	require.NoError(t, err, "parse %q", text)
	template.AnnotateHTMLContexts(tree.Root)
	return tree
}

// execText executes a text template and returns the output.
func execText(t *testing.T, name, tmpl string, data any, opts ...template.Option) string {
	t.Helper()
	tree := parseText(t, name, tmpl)
	var buf bytes.Buffer
	require.NoError(t, template.ExecuteTemplate(&buf, tree, data, opts...), "execute %q", tmpl)
	return buf.String()
}

// execHTML executes an HTML template and returns the output.
func execHTML(t *testing.T, name, tmpl string, data any, opts ...template.Option) string {
	t.Helper()
	tree := parseHTML(t, name, tmpl)
	var buf bytes.Buffer
	require.NoError(t, template.ExecuteTemplate(&buf, tree, data, opts...), "execute %q", tmpl)
	return buf.String()
}

// execTextErr executes a text template and returns the error (nil if none).
func execTextErr(t *testing.T, name, tmpl string, data any, opts ...template.Option) error {
	t.Helper()
	tree := parseText(t, name, tmpl)
	var buf bytes.Buffer
	return template.ExecuteTemplate(&buf, tree, data, opts...)
}

// execHTMLErr executes an HTML template and returns the error (nil if none).
func execHTMLErr(t *testing.T, name, tmpl string, data any, opts ...template.Option) error {
	t.Helper()
	tree := parseHTML(t, name, tmpl)
	var buf bytes.Buffer
	return template.ExecuteTemplate(&buf, tree, data, opts...)
}

// textExecCase is the common shape for text-template tests that render a
// template with a single data value and compare rendered output against a
// literal string. Tests whose assertions need to be relaxed (map iteration
// order, html.Render quirks) use Contains instead — see TestTextTemplateFuzzy.
type textExecCase struct {
	name string
	tmpl string
	data any
	want string
}

// TestTextTemplate drives a large batch of single-assertion text template
// cases in one place. Each case parses the template, executes it against
// `data`, and requires the output to equal `want`. Grouping them into one
// table keeps the set trivially scannable; a missing case is a one-line
// addition rather than a whole new Test function.
func TestTextTemplate(t *testing.T) {
	cases := []textExecCase{
		// --- basics: literals, dot, field, method ---
		{name: "plain text", tmpl: "hello", data: nil, want: "hello"},
		{name: "dot", tmpl: "{{.}}", data: "hello", want: "hello"},
		{name: "field access", tmpl: "Hello, {{.Name}}!", data: struct{ Name string }{"World"}, want: "Hello, World!"},
		{name: "method call", tmpl: "{{.Error}}", data: errors.New("boom"), want: "boom"},

		// --- control structures ---
		{name: "if true", tmpl: "{{if .}}yes{{end}}", data: true, want: "yes"},
		{name: "if else false", tmpl: "{{if .}}yes{{else}}no{{end}}", data: false, want: "no"},
		{name: "range slice", tmpl: "{{range .}}[{{.}}]{{end}}", data: []string{"a", "b"}, want: "[a][b]"},
		{name: "range with index", tmpl: "{{range $i, $v := .}}{{$i}}:{{$v}} {{end}}", data: []string{"a", "b"}, want: "0:a 1:b "},
		{name: "range empty hits else", tmpl: "{{range .}}item{{else}}empty{{end}}", data: []string{}, want: "empty"},
		{name: "with sets dot", tmpl: "{{with .Name}}{{.}}{{end}}", data: struct{ Name string }{"World"}, want: "World"},
		{name: "with empty string hits else", tmpl: `{{with ""}}yes{{else}}no{{end}}`, data: nil, want: "no"},
		{name: "nested if", tmpl: `{{if .A}}{{if .B}}both{{end}}{{end}}`, data: struct{ A, B bool }{true, true}, want: "both"},

		// --- pipelines and variables ---
		{name: "pipeline", tmpl: `{{.Name | printf "%s!"}}`, data: struct{ Name string }{"World"}, want: "World!"},
		{name: "variable", tmpl: `{{$x := "hi"}}{{$x}}`, data: nil, want: "hi"},
		{name: "parenthesized pipeline", tmpl: `{{(printf "%s" "hello")}}`, data: nil, want: "hello"},

		// --- builtin functions ---
		{name: "eq equal strings", tmpl: `{{if eq .A .B}}equal{{end}}`, data: struct{ A, B string }{"x", "x"}, want: "equal"},
		{name: "len of slice", tmpl: `{{len .}}`, data: []int{1, 2, 3}, want: "3"},
		{name: "not of true", tmpl: `{{not true}}`, data: nil, want: "false"},
		{name: "printf with int", tmpl: `{{printf "%d items" 5}}`, data: nil, want: "5 items"},
		{name: "index into slice", tmpl: `{{index . 1}}`, data: []string{"a", "b", "c"}, want: "b"},
		{name: "print", tmpl: `{{print "hello"}}`, data: nil, want: "hello"},
		{name: "println adds newline", tmpl: `{{println "hello"}}`, data: nil, want: "hello\n"},
		{name: "printf two args", tmpl: `{{printf "%s %s" "hello" "world"}}`, data: nil, want: "hello world"},

		// --- literals ---
		{name: "number literal", tmpl: `{{42}}`, data: nil, want: "42"},
		{name: "string literal", tmpl: `{{"hello"}}`, data: nil, want: "hello"},
		{name: "bool literal", tmpl: `{{true}}`, data: nil, want: "true"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, execText(t, "test", tc.tmpl, tc.data))
		})
	}
}

func TestUserFunc(t *testing.T) {
	funcs := template.FuncMap{
		"greet": func(name string) string {
			return "Hello, " + name + "!"
		},
	}
	got := execText(t, "test", `{{greet .}}`, "World", template.WithFuncs(funcs))
	assert.Equal(t, "Hello, World!", got)
}

// --- Phase 4: {{template}} calls ---

func TestTemplateCall(t *testing.T) {
	mainTree := parseText(t, "main", `before {{template "sub" .}} after`)
	subTree := parseText(t, "sub", `[{{.}}]`)
	finder := template.FindTreeFunc(func(name string) (*template.Tree, bool) {
		if name == "sub" {
			return subTree, true
		}
		return nil, false
	})
	var buf bytes.Buffer
	require.NoError(t, template.ExecuteTemplate(&buf, mainTree, "data", template.WithFinder(finder)))
	assert.Equal(t, "before [data] after", buf.String())
}

func TestTemplateNotFound(t *testing.T) {
	mainTree := parseText(t, "main", `{{template "missing" .}}`)
	var buf bytes.Buffer
	err := template.ExecuteTemplate(&buf, mainTree, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "not defined")
}

// --- Phase 5: ExecuteElement function (HTML → html.Node) ---

func TestElementSimple(t *testing.T) {
	tree := parseHTML(t, "test", `<div>hello</div>`)
	frag := tree.Root.Nodes[0].(*template.HTMLDocumentFragmentNode)
	elem := frag.Children[0].(*template.HTMLElementNode)
	node, err := template.ExecuteElement(tree, elem, nil)
	require.NoError(t, err)
	require.Equal(t, html.ElementNode, node.Type)
	assert.Equal(t, "div", node.Data)
	require.NotNil(t, node.FirstChild)
	assert.Equal(t, html.TextNode, node.FirstChild.Type)
	assert.Equal(t, "hello", node.FirstChild.Data)
}

func TestElementWithAttribute(t *testing.T) {
	tree := parseHTML(t, "test", `<a href="https://example.com">link</a>`)
	frag := tree.Root.Nodes[0].(*template.HTMLDocumentFragmentNode)
	elem := frag.Children[0].(*template.HTMLElementNode)
	node, err := template.ExecuteElement(tree, elem, nil)
	require.NoError(t, err)
	require.NotEmpty(t, node.Attr)
	assert.Equal(t, "href", node.Attr[0].Key)
	assert.Equal(t, "https://example.com", node.Attr[0].Val)
}

func TestElementDynamicAttribute(t *testing.T) {
	tree := parseHTML(t, "test", `<a href="{{.URL}}">{{.Label}}</a>`)
	frag := tree.Root.Nodes[0].(*template.HTMLDocumentFragmentNode)
	elem := frag.Children[0].(*template.HTMLElementNode)
	data := struct {
		URL   string
		Label string
	}{URL: "https://example.com", Label: "click"}
	node, err := template.ExecuteElement(tree, elem, data)
	require.NoError(t, err)
	require.NotEmpty(t, node.Attr)
	assert.Equal(t, "https://example.com", node.Attr[0].Val)
	require.NotNil(t, node.FirstChild)
	assert.Equal(t, "click", node.FirstChild.Data)
}

func TestElementConditionalAttribute(t *testing.T) {
	tree := parseHTML(t, "test", `<div {{if .Active}}class="active"{{end}}></div>`)
	frag := tree.Root.Nodes[0].(*template.HTMLDocumentFragmentNode)
	elem := frag.Children[0].(*template.HTMLElementNode)

	// Active = true
	data := struct{ Active bool }{Active: true}
	node, err := template.ExecuteElement(tree, elem, data)
	require.NoError(t, err)
	found := false
	for _, a := range node.Attr {
		if a.Key == "class" && a.Val == "active" {
			found = true
		}
	}
	assert.True(t, found, "expected class=active, got attrs %v", node.Attr)

	// Active = false
	data.Active = false
	node, err = template.ExecuteElement(tree, elem, data)
	require.NoError(t, err)
	for _, a := range node.Attr {
		if a.Key == "class" {
			t.Errorf("expected no class attr when inactive, got %+v", a)
		}
	}
}

func TestElementRangeChildren(t *testing.T) {
	tree := parseHTML(t, "test", `<ul>{{range .}}<li>{{.}}</li>{{end}}</ul>`)
	frag := tree.Root.Nodes[0].(*template.HTMLDocumentFragmentNode)
	elem := frag.Children[0].(*template.HTMLElementNode)
	node, err := template.ExecuteElement(tree, elem, []string{"a", "b"})
	require.NoError(t, err)
	// Count li children
	count := 0
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "li" {
			count++
		}
	}
	assert.Equal(t, 2, count, "expected 2 <li> children")
}

// --- Phase 6: ExecuteTemplate for HTML trees ---

func TestHTMLTreeTemplate(t *testing.T) {
	got := execHTML(t, "test", `<div><p>{{.}}</p></div>`, "hi")
	assert.Contains(t, got, "<div><p>hi</p></div>")
}

func TestHTMLDoctype(t *testing.T) {
	got := execHTML(t, "test", `<!DOCTYPE html><html><body>hello</body></html>`, nil)
	assert.Contains(t, got, "<!DOCTYPE html>")
	assert.Contains(t, got, "hello")
}

func TestHTMLComment(t *testing.T) {
	// Comments are stripped to match html/template behavior.
	got := execHTML(t, "test", `<!-- comment -->`, nil)
	assert.Empty(t, got, "comments should be stripped")
}

// --- Phase 7: Coverage ---

// --- Phase 8: Error messages ---

func TestMissingFieldError(t *testing.T) {
	tree := parseText(t, "test", "{{.Missing}}")
	var buf bytes.Buffer
	err := template.ExecuteTemplate(&buf, tree, struct{}{})
	require.Error(t, err)
	var execErr template.ExecError
	require.ErrorAs(t, err, &execErr)
}

// TestTextTemplateFuzzyMatch covers cases where output is partially
// nondeterministic (map iteration order) and only a substring can be asserted.
// Kept separate from TestTextTemplate because a Contains assertion is a weaker
// contract that deserves its own test.
func TestTextTemplateFuzzyMatch(t *testing.T) {
	got := execText(t, "test", `{{range $k, $v := .}}{{$k}}={{$v}} {{end}}`, map[string]int{"a": 1})
	assert.Contains(t, got, "a=1")
}

func TestHTMLVoidElement(t *testing.T) {
	got := execHTML(t, "test", `<br>`, nil)
	// html.Render renders <br> as <br>
	assert.Contains(t, got, "<br")
}

func TestHTMLNestedElements(t *testing.T) {
	got := execHTML(t, "test", `<div><span>text</span></div>`, nil)
	assert.Contains(t, got, "<div><span>text</span></div>")
}

func TestHTMLWithTemplate(t *testing.T) {
	mainTree := parseHTML(t, "main", `<div>{{template "inner" .}}</div>`)
	innerTree := parseHTML(t, "inner", `<span>{{.}}</span>`)
	finder := template.FindTreeFunc(func(name string) (*template.Tree, bool) {
		if name == "inner" {
			return innerTree, true
		}
		return nil, false
	})
	var buf bytes.Buffer
	require.NoError(t, template.ExecuteTemplate(&buf, mainTree, "hello", template.WithFinder(finder)))
	assert.Contains(t, buf.String(), "<span>hello</span>")
}

func TestHTMLWithExtractedDefine(t *testing.T) {
	// Parse a template with an inline {{define}} containing HTML nodes.
	// Extract produces a tree whose Root is a ListNode containing HTML nodes
	// directly (no HTMLDocumentFragmentNode wrapper).
	mainTree := parseHTML(t, "main", `<div>{{template "greeting" .}}</div>{{define "greeting"}}<span>Hi {{.}}</span>{{end}}`)
	trees := make(map[string]*template.Tree)
	trees["main"] = mainTree
	for name, tr := range template.Extract(mainTree) {
		template.AnnotateHTMLContexts(tr.Root)
		trees[name] = tr
	}
	finder := template.FindTreeFunc(func(name string) (*template.Tree, bool) {
		tr, ok := trees[name]
		return tr, ok
	})
	var buf bytes.Buffer
	require.NoError(t, template.ExecuteTemplate(&buf, mainTree, "Alice", template.WithFinder(finder)))
	assert.Contains(t, buf.String(), "<span>Hi Alice</span>")
}

func TestHTMLWithExtractedBlock(t *testing.T) {
	trees := make(map[string]*template.Tree)
	mainTree, err := template.ParseTree("main", "main", `<div>{{block "content" .}}<p>default {{.}}</p>{{end}}</div>`, trees, lex.NewOptions(nil))
	require.NoError(t, err)
	template.AnnotateHTMLContexts(mainTree.Root)
	for _, tr := range trees {
		template.AnnotateHTMLContexts(tr.Root)
	}
	finder := template.FindTreeFunc(func(name string) (*template.Tree, bool) {
		tr, ok := trees[name]
		return tr, ok
	})
	var buf bytes.Buffer
	require.NoError(t, template.ExecuteTemplate(&buf, mainTree, "world", template.WithFinder(finder)))
	assert.Contains(t, buf.String(), "default world")
}

func TestExecErrorUnwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	execErr := template.ExecError{Name: "test", Err: inner}
	assert.ErrorIs(t, execErr, inner)
}

func TestHTMLDynamicComment(t *testing.T) {
	// Comments are stripped entirely to match html/template behavior.
	got := execHTML(t, "test", `<!-- version: {{.}} -->`, "1.0")
	assert.Empty(t, got, "comments should be stripped")
}

func TestHTMLMultipleRootElements(t *testing.T) {
	got := execHTML(t, "test", `<p>one</p><p>two</p>`, nil)
	assert.Contains(t, got, "<p>one</p><p>two</p>")
}

func TestHTMLRangeWithElements(t *testing.T) {
	got := execHTML(t, "test", `<ul>{{range .}}<li>{{.}}</li>{{end}}</ul>`, []string{"a", "b"})
	assert.Contains(t, got, "<li>a</li><li>b</li>")
}

func TestHTMLIfInBody(t *testing.T) {
	got := execHTML(t, "test", `<div>{{if .}}yes{{else}}no{{end}}</div>`, true)
	assert.Contains(t, got, "<div>yes</div>")
}

// --- Batch 1: Data Type Coverage for Dot ---

func TestDotTypes(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data any
		want string
	}{
		{"int", "{{.}}", 13, "13"},
		{"uint", "{{.}}", uint(14), "14"},
		{"float64", "{{.}}", 15.1, "15.1"},
		{"bool true", "{{.}}", true, "true"},
		{"complex", "{{.}}", complex(16.2, -17), "(16.2-17i)"},
		{"int slice", "{{.}}", []int{-1, -2, -3}, "[-1 -2 -3]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Batch 2: Truthiness Edge Cases ---

func TestTruthiness(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data any
		want string
	}{
		{"int zero", "{{if .}}NON-ZERO{{else}}ZERO{{end}}", 0, "ZERO"},
		{"int one", "{{if .}}NON-ZERO{{else}}ZERO{{end}}", 1, "NON-ZERO"},
		{"float nonzero", "{{if .}}NON-ZERO{{else}}ZERO{{end}}", 1.5, "NON-ZERO"},
		{"float zero", "{{if .}}NON-ZERO{{else}}ZERO{{end}}", 0.0, "ZERO"},
		{"empty slice", "{{if .}}NON-EMPTY{{else}}EMPTY{{end}}", []int{}, "EMPTY"},
		{"empty array", "{{if .}}NON-EMPTY{{else}}EMPTY{{end}}", [0]int{}, "EMPTY"},
		{"struct always true", "{{if .}}NON-EMPTY{{else}}EMPTY{{end}}", struct{}{}, "NON-EMPTY"},
		{"nil", "{{if .}}NON-NIL{{else}}NIL{{end}}", nil, "NIL"},
		{"empty string", `{{if .}}NON-EMPTY{{else}}EMPTY{{end}}`, "", "EMPTY"},
		{"nonempty string", `{{if .}}NON-EMPTY{{else}}EMPTY{{end}}`, "hi", "NON-EMPTY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Batch 3: Field/Method Access Edge Cases ---

func TestNestedStructField(t *testing.T) {
	type Inner struct{ V string }
	type Outer struct{ U Inner }
	data := Outer{U: Inner{V: "v"}}
	got := execText(t, "test", "{{.U.V}}", data)
	assert.Equal(t, "v", got)
}

func TestPointerField(t *testing.T) {
	type S struct{ PI *int }
	n := 42
	data := S{PI: &n}
	got := execText(t, "test", "{{.PI}}", data)
	assert.Equal(t, "42", got)
}

func TestMapFieldSyntax(t *testing.T) {
	data := struct{ MSI map[string]int }{MSI: map[string]int{"one": 1}}
	got := execText(t, "test", "{{.MSI.one}}", data)
	assert.Equal(t, "1", got)
}

// --- Batch 4: Variable Scope & Assignment ---

func TestDollarRoot(t *testing.T) {
	got := execText(t, "test", "{{$}}", 123)
	assert.Equal(t, "123", got)
}

func TestDollarFieldAccess(t *testing.T) {
	data := struct{ Name string }{Name: "Bob"}
	got := execText(t, "test", "{{$.Name}}", data)
	assert.Equal(t, "Bob", got)
}

func TestVariableReassignment(t *testing.T) {
	got := execText(t, "test", `{{$x := 2}}{{$x = 3}}{{$x}}`, nil)
	assert.Equal(t, "3", got)
}

func TestNestedVariableScope(t *testing.T) {
	data := struct{ I int }{I: 7}
	got := execText(t, "test", `{{if $x := true}}{{with $y := .I}}{{$x}},{{$y}}{{end}}{{end}}`, data)
	assert.Equal(t, "true,7", got)
}

func TestRangeVariableDecl(t *testing.T) {
	got := execText(t, "test", `{{range $x := .}}{{$x}}{{end}}`, []int{1, 2, 3})
	assert.Equal(t, "123", got)
}

// --- Batch 5: Range Over Integer ---

func TestRangeInt(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data any
		want string
	}{
		{"basic int", "{{range $v := .}}{{$v}}{{end}}", 5, "01234"},
		{"int index and value", "{{range $i, $v := .}}{{$i}}:{{$v}} {{end}}", 3, "0:0 1:1 2:2 "},
		{"int zero else", "{{range .}}x{{else}}empty{{end}}", 0, "empty"},
		{"uint zero else", "{{range .}}x{{else}}empty{{end}}", uint(0), "empty"},
		{"uint nonzero", "{{range $v := .}}{{$v}}{{end}}", uint(3), "012"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRangeIntHTML(t *testing.T) {
	got := execHTML(t, "test", `<ul>{{range $v := .}}<li>{{$v}}</li>{{end}}</ul>`, 3)
	assert.Contains(t, got, "<li>0</li><li>1</li><li>2</li>")
}

// --- Batch 6: Comparison Functions ---

func TestComparisons(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data any
		want string
	}{
		{"ne 1 2", "{{ne 1 2}}", nil, "true"},
		{"ne 1 1", "{{ne 1 1}}", nil, "false"},
		{"lt 1 2", "{{lt 1 2}}", nil, "true"},
		{"lt 2 1", "{{lt 2 1}}", nil, "false"},
		{"le 1 1", "{{le 1 1}}", nil, "true"},
		{"le 1 2", "{{le 1 2}}", nil, "true"},
		{"le 2 1", "{{le 2 1}}", nil, "false"},
		{"gt 2 1", "{{gt 2 1}}", nil, "true"},
		{"gt 1 2", "{{gt 1 2}}", nil, "false"},
		{"ge 1 1", "{{ge 1 1}}", nil, "true"},
		{"ge 2 1", "{{ge 2 1}}", nil, "true"},
		{"ge 1 2", "{{ge 1 2}}", nil, "false"},
		{"eq float", "{{eq 1.5 1.5}}", nil, "true"},
		{"lt string", `{{lt "ab" "abc"}}`, nil, "true"},
		{"eq int-uint cross type", "{{eq .Three .Uthree}}", struct {
			Three  int
			Uthree uint
		}{3, 3}, "true"},
		{"lt int-uint cross type", "{{lt .NegOne .Uthree}}", struct {
			NegOne int
			Uthree uint
		}{-1, 3}, "true"},
		{"multi-arg eq", "{{eq 3 4 5 6 3}}", nil, "true"},
		{"multi-arg eq no match", "{{eq 3 4 5 6 7}}", nil, "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Batch 7: call Builtin ---

func TestCallBuiltin(t *testing.T) {
	binaryFn := func(a, b string) string { return a + "+" + b }
	variadicFn := func(args ...string) string { return strings.Join(args, ",") }
	data := struct {
		Fn  func(string, string) string
		VFn func(...string) string
	}{Fn: binaryFn, VFn: variadicFn}

	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{"binary", `{{call .Fn "a" "b"}}`, "a+b"},
		{"variadic 0 args", `{{call .VFn}}`, ""},
		{"variadic 2 args", `{{call .VFn "a" "b"}}`, "a,b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, data)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Batch 8: index & slice Edge Cases ---

func TestIndexEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data any
		want string
	}{
		{"slice first", "{{index . 0}}", []string{"a", "b"}, "a"},
		{"map string key", `{{index . "key"}}`, map[string]string{"key": "val"}, "val"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSliceBuiltin(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data any
		want string
	}{
		{"full slice", "{{slice .}}", []int{1, 2, 3}, "[1 2 3]"},
		{"from index", "{{slice . 1}}", []int{1, 2, 3}, "[2 3]"},
		{"from to", "{{slice . 1 2}}", []int{1, 2, 3}, "[2]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Batch 9: break / continue ---

func TestRangeBreak(t *testing.T) {
	got := execText(t, "test", `{{range .}}-{{.}}-{{break}}NOTREACHED{{end}}`, []int{3, 4, 5})
	assert.Equal(t, "-3-", got)
}

func TestRangeContinue(t *testing.T) {
	got := execText(t, "test", `{{range .}}-{{.}}-{{continue}}NOTREACHED{{end}}`, []int{3, 4, 5})
	assert.Equal(t, "-3--4--5-", got)
}

// --- Batch 10: Error Handling ---

func TestErrorNilPointerAccess(t *testing.T) {
	type S struct{ X *int }
	err := execTextErr(t, "test", "{{.X.Y}}", S{X: nil})
	require.Error(t, err)
	// Error should mention the field access failure.
	var execErr template.ExecError
	require.ErrorAs(t, err, &execErr)
}

func TestErrorMapMissingKeyReturnsZero(t *testing.T) {
	// Maps return zero value for missing keys, not an error.
	got := execText(t, "test", `{{index . "missing"}}`, map[string]int{"a": 1})
	assert.Equal(t, "0", got)
}

func TestMissingKeyInvalid(t *testing.T) {
	// Default mode: missing struct field is an error, missing map key prints <no value>.
	t.Run("struct field error", func(t *testing.T) {
		type S struct{ A string }
		err := execTextErr(t, "test", `{{.B}}`, S{A: "a"})
		require.Error(t, err)
	})
	t.Run("map key no value", func(t *testing.T) {
		got := execText(t, "test", `[{{.missing}}]`, map[string]int{"a": 1})
		assert.Equal(t, "[<no value>]", got)
	})
}

func TestMissingKeyZero(t *testing.T) {
	// missingkey=zero only affects map lookups. Struct fields always error.
	t.Run("struct field error", func(t *testing.T) {
		type S struct{ A string }
		err := execTextErr(t, "test", `{{.B}}`, S{A: "a"},
			template.WithMissingKey(template.MissingKeyZero))
		require.Error(t, err)
	})
	t.Run("map key zero int", func(t *testing.T) {
		got := execText(t, "test", `[{{.missing}}]`, map[string]int{"a": 1},
			template.WithMissingKey(template.MissingKeyZero))
		assert.Equal(t, "[0]", got)
	})
	t.Run("map key zero string", func(t *testing.T) {
		got := execText(t, "test", `[{{.missing}}]`, map[string]string{"a": "b"},
			template.WithMissingKey(template.MissingKeyZero))
		assert.Equal(t, "[]", got)
	})
}

func TestMissingKeyError(t *testing.T) {
	t.Run("struct field error", func(t *testing.T) {
		type S struct{ A string }
		err := execTextErr(t, "test", `{{.B}}`, S{A: "a"},
			template.WithMissingKey(template.MissingKeyError))
		require.Error(t, err)
	})
	t.Run("map key error", func(t *testing.T) {
		err := execTextErr(t, "test", `{{.missing}}`, map[string]int{"a": 1},
			template.WithMissingKey(template.MissingKeyError))
		require.Error(t, err)
		assert.ErrorContains(t, err, "missing")
	})
}

func TestErrorIndexOutOfRange(t *testing.T) {
	err := execTextErr(t, "test", `{{index . 10}}`, []int{1, 2, 3})
	require.Error(t, err)
	assert.ErrorContains(t, err, "out of range")
}

func TestErrorCallNonFunction(t *testing.T) {
	err := execTextErr(t, "test", `{{call .}}`, 42)
	require.Error(t, err)
	assert.ErrorContains(t, err, "non-function")
}

func TestErrorWrongArgCount(t *testing.T) {
	fn := func(a string) string { return a }
	data := struct{ Fn func(string) string }{Fn: fn}
	err := execTextErr(t, "test", `{{call .Fn "a" "b"}}`, data)
	require.Error(t, err)
	assert.ErrorContains(t, err, "wrong number of args")
}

func TestErrorMaxRecursionDepth(t *testing.T) {
	mainTree := parseText(t, "main", `{{template "main" .}}`)
	finder := template.FindTreeFunc(func(name string) (*template.Tree, bool) {
		if name == "main" {
			return mainTree, true
		}
		return nil, false
	})
	var buf bytes.Buffer
	err := template.ExecuteTemplate(&buf, mainTree, nil, template.WithFinder(finder))
	require.Error(t, err)
	assert.ErrorContains(t, err, "exceeded maximum template depth")
}

// --- Batch 11: and / or Builtins ---

func TestAndOr(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{"and false 0", "{{and false 0}}", "false"},
		{"and 1 0", "{{and 1 0}}", "0"},
		{"and 1 1", "{{and 1 1}}", "1"},
		{"or 0 0", "{{or 0 0}}", "0"},
		{"or 1 0", "{{or 1 0}}", "1"},
		{"or 0 true", "{{or 0 true}}", "true"},
		{"piped and", "{{1 | and 1}}", "1"},
		{"piped or", "{{0 | or 1}}", "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Batch 12: print Edge Cases ---

func TestPrintEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{"print nil", "{{print nil}}", "<nil>"},
		{"print multi", "{{print 1 2 3}}", "1 2 3"},
		{"printf hex", `{{printf "%04x" 127}}`, "007f"},
		{"printf float", `{{printf "%g" 3.5}}`, "3.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Batch 13: else if / else with ---

func TestElseIf(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data any
		want string
	}{
		{"else if true", "{{if false}}FALSE{{else if true}}TRUE{{end}}", nil, "TRUE"},
		{"chained else if", "{{if eq 1 3}}1{{else if eq 2 3}}2{{else if eq 3 3}}3{{end}}", nil, "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestElseWith(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		data any
		want string
	}{
		{"else with true", "{{with 0}}{{.}}{{else with true}}{{.}}{{end}}", nil, "true"},
		{"chained else with", `{{with 0}}{{.}}{{else with false}}{{.}}{{else with "hi"}}{{.}}{{end}}`, nil, "hi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execText(t, "test", tt.tmpl, tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- Batch 14: Range Over Channel ---

func TestRangeChannel(t *testing.T) {
	ch := make(chan string, 2)
	ch <- "a"
	ch <- "b"
	close(ch)
	got := execText(t, "test", `{{range $i, $v := .}}[{{$i}}]{{$v}}{{end}}`, ch)
	assert.Equal(t, "[0]a[1]b", got)
}

func TestRangeChannelEmpty(t *testing.T) {
	ch := make(chan string)
	close(ch)
	got := execText(t, "test", `{{range .}}x{{else}}empty{{end}}`, ch)
	assert.Equal(t, "empty", got)
}

// --- Batch 15: HTML-Specific Gaps ---

func TestHTMLMultipleAttributes(t *testing.T) {
	got := execHTML(t, "test", `<div class="a" id="b">text</div>`, nil)
	assert.Contains(t, got, `class="a"`)
	assert.Contains(t, got, `id="b"`)
}

func TestHTMLBooleanAttribute(t *testing.T) {
	got := execHTML(t, "test", `<input type="text" disabled>`, nil)
	assert.Contains(t, got, "disabled")
}

func TestHTMLRangeElseBranch(t *testing.T) {
	got := execHTML(t, "test", `{{range .}}<div>{{.}}</div>{{else}}<p>empty</p>{{end}}`, []string{})
	assert.Contains(t, got, "<p>empty</p>")
}

// --- Byte-for-byte comparison with html/template ---

// stdlibHTML executes a template using html/template and returns the output.
func stdlibHTML(t *testing.T, tmpl string, data any, funcs ...htmltemplate.FuncMap) string {
	t.Helper()
	tp := htmltemplate.New("test")
	for _, fm := range funcs {
		tp = tp.Funcs(fm)
	}
	tp = htmltemplate.Must(tp.Parse(tmpl))
	var buf bytes.Buffer
	require.NoError(t, tp.Execute(&buf, data), "html/template execute %q", tmpl)
	return buf.String()
}

// TestStdlibCompat runs template+data pairs through both html/template and
// execute.ExecuteTemplate, then asserts byte-for-byte identical output.
func TestStdlibCompat(t *testing.T) {
	type inner struct {
		V string
	}
	type testData struct {
		A      string
		B      string
		Name   string
		I      int
		U      inner
		Items  []string
		Nums   []int
		M      map[string]string
		MI     map[string]int
		Active bool
		PI     *int
		Fn     func(string) string
	}
	n := 42
	td := testData{
		A:      "alpha",
		B:      "beta",
		Name:   "World",
		I:      7,
		U:      inner{V: "nested"},
		Items:  []string{"a", "b", "c"},
		Nums:   []int{1, 2, 3},
		M:      map[string]string{"key": "val", "foo": "bar"},
		MI:     map[string]int{"one": 1, "two": 2},
		Active: true,
		PI:     &n,
		Fn:     func(s string) string { return "fn:" + s },
	}

	tests := []struct {
		name string
		tmpl string
		data any
	}{
		// Basic text and interpolation
		{"static text", `<p>hello</p>`, nil},
		{"dot string", `<p>{{.}}</p>`, "hello"},
		{"dot int", `<p>{{.}}</p>`, 42},
		{"dot float", `<p>{{.}}</p>`, 3.14},
		{"dot bool", `<p>{{.}}</p>`, true},
		{"field access", `<p>{{.Name}}</p>`, td},
		{"nested field", `<p>{{.U.V}}</p>`, td},
		{"pointer deref", `<p>{{.PI}}</p>`, td},

		// HTML escaping in text context
		{"escape lt gt", `<p>{{.}}</p>`, "<script>alert(1)</script>"},
		{"escape amp", `<p>{{.}}</p>`, "a&b"},
		{"escape quotes", `<p>{{.}}</p>`, `he said "hi"`},
		{"escape single quotes", `<p>{{.}}</p>`, `it's`},

		// Attribute escaping
		{"attr static", `<a href="https://example.com">link</a>`, nil},
		{"attr dynamic safe url", `<a href="{{.}}">link</a>`, "https://example.com"},
		{"attr dynamic js url", `<a href="{{.}}">link</a>`, "javascript:alert(1)"},
		{"attr dynamic data url", `<a href="{{.}}">link</a>`, "data:text/html,<script>"},
		{"attr dynamic relative", `<a href="{{.}}">link</a>`, "/foo/bar"},
		{"attr class", `<div class="{{.}}">x</div>`, "my-class"},
		{"attr id", `<div id="{{.}}">x</div>`, "my-id"},
		{"attr with special chars", `<div title="{{.}}">x</div>`, `a "quoted" & <value>`},

		// URL attribute filtering
		{"href http", `<a href="{{.}}">x</a>`, "http://example.com"},
		{"href https", `<a href="{{.}}">x</a>`, "https://example.com"},
		{"href mailto", `<a href="{{.}}">x</a>`, "mailto:user@example.com"},
		{"href javascript", `<a href="{{.}}">x</a>`, "javascript:void(0)"},
		{"href vbscript", `<a href="{{.}}">x</a>`, "vbscript:MsgBox"},
		{"action safe", `<form action="{{.}}">x</form>`, "/submit"},
		{"action dangerous", `<form action="{{.}}">x</form>`, "javascript:steal()"},

		// Control structures
		{"if true", `<p>{{if .}}yes{{else}}no{{end}}</p>`, true},
		{"if false", `<p>{{if .}}yes{{else}}no{{end}}</p>`, false},
		{"if zero int", `<p>{{if .}}yes{{else}}no{{end}}</p>`, 0},
		{"if empty string", `<p>{{if .}}yes{{else}}no{{end}}</p>`, ""},
		{"if nil", `<p>{{if .}}yes{{else}}no{{end}}</p>`, nil},
		{"with value", `<p>{{with .Name}}{{.}}{{end}}</p>`, td},
		{"with empty", `<p>{{with ""}}x{{else}}empty{{end}}</p>`, nil},

		// Range
		{"range slice", `<ul>{{range .}}<li>{{.}}</li>{{end}}</ul>`, []string{"a", "b"}},
		{"range empty else", `<ul>{{range .}}<li>{{.}}</li>{{else}}<li>none</li>{{end}}</ul>`, []string{}},
		{"range index", `{{range $i, $v := .}}{{$i}}:{{$v}} {{end}}`, []string{"x", "y"}},

		// Variables
		{"dollar", `<p>{{$}}</p>`, "root"},
		{"dollar field", `<p>{{$.Name}}</p>`, td},
		{"var declare", `<p>{{$x := "hi"}}{{$x}}</p>`, nil},
		{"var reassign", `<p>{{$x := 1}}{{$x = 2}}{{$x}}</p>`, nil},

		// Builtins
		{"len", `<p>{{len .}}</p>`, []int{1, 2, 3}},
		{"not", `<p>{{not true}}</p>`, nil},
		{"print", `<p>{{print "a" "b"}}</p>`, nil},
		{"printf", `<p>{{printf "%d" 42}}</p>`, nil},
		{"println", `<p>{{println "hi"}}</p>`, nil},

		// Comparisons
		{"eq true", `<p>{{eq 1 1}}</p>`, nil},
		{"eq false", `<p>{{eq 1 2}}</p>`, nil},
		{"ne", `<p>{{ne 1 2}}</p>`, nil},
		{"lt", `<p>{{lt 1 2}}</p>`, nil},
		{"le", `<p>{{le 1 1}}</p>`, nil},
		{"gt", `<p>{{gt 2 1}}</p>`, nil},
		{"ge", `<p>{{ge 1 1}}</p>`, nil},

		// and/or
		{"and true", `<p>{{and 1 2}}</p>`, nil},
		{"and false", `<p>{{and 0 1}}</p>`, nil},
		{"or true", `<p>{{or 0 1}}</p>`, nil},
		{"or false", `<p>{{or 0 0}}</p>`, nil},

		// index/slice
		{"index slice", `<p>{{index . 1}}</p>`, []string{"a", "b", "c"}},
		{"index map", `<p>{{index . "key"}}</p>`, map[string]string{"key": "val"}},

		// else if / else with
		{"else if", `<p>{{if false}}a{{else if true}}b{{end}}</p>`, nil},
		{"else with", `<p>{{with 0}}{{.}}{{else with "hi"}}{{.}}{{end}}</p>`, nil},

		// Chained else if
		{"chained else if", `<p>{{if eq 1 2}}a{{else if eq 2 3}}b{{else if eq 3 3}}c{{end}}</p>`, nil},

		// Multiple attributes
		{"multi attr", `<div class="a" id="b">x</div>`, nil},

		// Boolean attribute
		{"bool attr", `<input type="text" disabled>`, nil},

		// Void elements
		{"br", `<br>`, nil},
		{"hr", `<hr>`, nil},
		{"img", `<img src="test.png">`, nil},

		// Doctype
		{"doctype", `<!DOCTYPE html><html><body>hello</body></html>`, nil},

		// Mixed static and dynamic attributes
		{"mixed attrs", `<a class="link" href="{{.}}">click</a>`, "https://example.com"},

		// Nested elements with actions
		{"nested actions", `<div><span>{{.A}}</span><span>{{.B}}</span></div>`, td},

		// Self-closing and void
		{"meta", `<meta charset="utf-8">`, nil},
		{"link", `<link rel="stylesheet" href="style.css">`, nil},

		// call function
		{"call", `<p>{{call .Fn "x"}}</p>`, td},

		// Complex escaping: ampersand in URL
		{"url amp", `<a href="{{.}}">x</a>`, "https://example.com?a=1&b=2"},

		// JS event handler escaping
		{"onclick dynamic", `<button onclick="{{.}}">click</button>`, "doStuff()"},
		{"onmouseover dynamic", `<div onmouseover="{{.}}">x</div>`, "highlight(this)"},
		{"onclick static", `<button onclick="alert('hi')">click</button>`, nil},

		// src with javascript URL (void element + URL filtering)
		{"src javascript", `<img src="{{.}}">`, "javascript:alert(1)"},

		// Comments are stripped (matching html/template)
		{"comment static", `<p>hello</p><!-- comment --><p>world</p>`, nil},
		{"comment dynamic", `<!-- {{.}} --><p>hi</p>`, "secret"},

		// Control structures in attribute values
		{"attr val dq if true", `<input value="{{if .Active}}yes{{end}}">`, td},
		{"attr val dq if-else true", `<input value="{{if .Active}}yes{{else}}no{{end}}">`, td},
		{"attr val dq if-else false", `<input value="{{if .Active}}yes{{else}}no{{end}}">`, struct{ Active bool }{}},
		{"attr val dq with hit", `<input value="{{with .Name}}{{.}}{{else}}fallback{{end}}">`, td},
		{"attr val dq with miss", `<input value="{{with .Name}}{{.}}{{else}}fallback{{end}}">`, struct{ Name string }{}},
		{"attr val dq range", `<input value="{{range .Items}}{{.}} {{end}}">`, td},
		{"attr val dq range empty", `<input value="{{range .Items}}{{.}} {{else}}none{{end}}">`, struct{ Items []string }{}},
		{"attr val dq prefix if", `<input value="prefix-{{if .Active}}yes{{end}}">`, td},
		{"attr val dq if suffix", `<input value="{{if .Active}}yes{{end}}-suffix">`, td},
		{"attr val dq prefix if-else suffix", `<input value="pre-{{if .Active}}yes{{else}}no{{end}}-suf">`, td},
		{"attr val dq nested if-if", `<input value="{{if .Active}}[{{if .Name}}{{.Name}}{{end}}]{{end}}">`, td},
		{"attr val dq nested if-with", `<input value="{{if .Active}}{{with .Name}}{{.}}{{end}}{{end}}">`, td},
		{"attr val then cond attr on sibling", `<input value="{{if .Active}}yes{{end}}"><span {{if .Active}}hidden{{end}}>x</span>`, td},
		{"attr val if-else on two elements", `<div class="{{if .Active}}on{{else}}off{{end}}"><span {{if .Active}}hidden{{end}}>x</span></div>`, td},

		// Complicated embedded control flow in attr values
		{"attr val triple nested if-with-range", `<div class="{{if .Active}}{{with .Items}}{{range .}}{{.}}{{end}}{{end}}{{end}}">x</div>`, td},
		{"attr val chained else-if hit", `<div class="{{if eq .I 1}}one{{else if eq .I 7}}seven{{else}}other{{end}}">x</div>`, td},
		{"attr val chained else-if fallthrough", `<div class="{{if eq .I 1}}one{{else if eq .I 2}}two{{else}}other{{end}}">x</div>`, td},
		{"attr val range index var", `<div data-x="{{range $i, $v := .Items}}{{$i}}:{{$v}} {{end}}">x</div>`, td},
		// NB: range over map omitted due to nondeterministic iteration order
		{"attr val var decl in if", `<div class="{{if .Active}}{{$x := "on"}}{{$x}}{{end}}">x</div>`, td},
		{"attr val with var capture", `<div data-x="{{with .Name}}hello-{{.}}{{end}}">x</div>`, td},
		{"attr val call fn in if", `<div class="{{if .Active}}{{call .Fn "x"}}{{end}}">x</div>`, td},
		{"attr val nested range", `<div data-x="{{range .Items}}[{{range $.Nums}}{{.}}{{end}}]{{end}}">x</div>`, td},
		{"attr val multiple controls concat", `<div class="{{if .Active}}a{{end}}-{{with .Name}}{{.}}{{end}}-{{range .Items}}{{.}}{{end}}">x</div>`, td},
		{"attr val if eq", `<input value="{{if eq .Name "World"}}match{{else}}no{{end}}">`, td},
		{"attr val if ne", `<input value="{{if ne .Name "World"}}diff{{else}}same{{end}}">`, td},
		{"attr val if lt", `<input value="{{if lt .I 10}}small{{else}}big{{end}}">`, td},
		{"attr val if and", `<input value="{{if and .Active .Name}}both{{else}}nope{{end}}">`, td},
		{"attr val if or true", `<input value="{{if or .Active .Name}}either{{else}}neither{{end}}">`, td},
		{"attr val if len", `<input value="{{if gt (len .Items) 0}}has-items{{else}}empty{{end}}">`, td},
		{"attr val escape in if", `<div title="{{if .Active}}{{.A}}{{end}}">x</div>`, struct {
			Active bool
			A      string
		}{true, `he said "hi" & <bye>`}},
		{"attr val 4-deep nesting", `<div class="{{if .Active}}{{if .Name}}{{if gt .I 0}}{{with .U}}{{.V}}{{end}}{{end}}{{end}}{{end}}">x</div>`, td},
		{"attr val kitchen sink", `<div class="{{with .Name}}n:{{.}}{{else}}anon{{end}} {{range .Items}}{{if ne . "b"}}{{.}}{{end}}{{end}}">x</div>`, td},
		{"attr val printf in range", `<div data-x="{{range .Nums}}{{printf "%03d" .}} {{end}}">x</div>`, td},
		{"attr val with dollar", `<div class="{{with .U}}{{.V}}-{{$.Name}}{{end}}">x</div>`, td},
		{"attr val range dollar", `<div data-x="{{range .Items}}{{.}}-{{$.Name}} {{end}}">x</div>`, td},
		{"attr val range else hit", `<input value="{{range .Items}}{{.}},{{else}}empty{{end}}">`, td},
		{"attr val range else miss", `<input value="{{range .Items}}{{.}},{{else}}empty{{end}}">`, struct{ Items []string }{}},
		// Control flow wrapping entire key=value in attr position
		{"attr pos if wrapping key=val true", `<input {{if .Active}}name="x"{{end}}>`, td},
		{"attr pos if-else wrapping attrs", `<input {{if .Active}}class="on"{{else}}class="off"{{end}}>`, td},
		{"attr pos with wrapping key=val hit", `<input {{with .Name}}value="{{.}}"{{end}}>`, td},
		{"attr pos chained else-if", `<div {{if eq .I 1}}class="one"{{else if eq .I 7}}class="seven"{{else}}class="other"{{end}}>x</div>`, td},
		{"attr pos nested if with val", `<div {{if .Active}}class="{{if eq .I 7}}lucky{{else}}unlucky{{end}}"{{end}}>x</div>`, td},
		{"attr pos multiple ifs", `<input {{if .Active}}disabled{{end}} {{if .Name}}title="{{.Name}}"{{end}}>`, td},
		{"attr pos if wrapping href", `<a {{if .Active}}href="{{.A}}"{{end}}>link</a>`, td},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := stdlibHTML(t, tt.tmpl, tt.data)
			got := execHTML(t, "test", tt.tmpl, tt.data)
			assert.Equal(t, want, got, "template: %s", tt.tmpl)
		})
	}
}

// --- ExecuteDocument function tests ---

func TestDocumentSimple(t *testing.T) {
	tree := parseHTML(t, "test", `<div><p>hello</p></div>`)
	doc, err := template.ExecuteDocument(tree, nil)
	require.NoError(t, err)
	require.Equal(t, html.DocumentNode, doc.Type)
	// First child should be a div element.
	div := doc.FirstChild
	require.NotNil(t, div)
	require.Equal(t, html.ElementNode, div.Type)
	assert.Equal(t, "div", div.Data)
	p := div.FirstChild
	require.NotNil(t, p)
	require.Equal(t, html.ElementNode, p.Type)
	assert.Equal(t, "p", p.Data)
	text := p.FirstChild
	require.NotNil(t, text)
	assert.Equal(t, html.TextNode, text.Type)
	assert.Equal(t, "hello", text.Data)
}

func TestDocumentWithData(t *testing.T) {
	tree := parseHTML(t, "test", `<span>{{.}}</span>`)
	doc, err := template.ExecuteDocument(tree, "world")
	require.NoError(t, err)
	span := doc.FirstChild
	require.NotNil(t, span)
	assert.Equal(t, "span", span.Data)
	text := span.FirstChild
	require.NotNil(t, text)
	assert.Equal(t, "world", text.Data)
}

func TestDocumentMultipleRoots(t *testing.T) {
	tree := parseHTML(t, "test", `<p>one</p><p>two</p>`)
	doc, err := template.ExecuteDocument(tree, nil)
	require.NoError(t, err)
	count := 0
	for c := doc.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "p" {
			count++
		}
	}
	assert.Equal(t, 2, count, "expected 2 <p> children")
}

func TestDocumentRange(t *testing.T) {
	tree := parseHTML(t, "test", `<ul>{{range .}}<li>{{.}}</li>{{end}}</ul>`)
	doc, err := template.ExecuteDocument(tree, []string{"a", "b", "c"})
	require.NoError(t, err)
	ul := doc.FirstChild
	require.NotNil(t, ul)
	assert.Equal(t, "ul", ul.Data)
	count := 0
	for c := ul.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "li" {
			count++
		}
	}
	assert.Equal(t, 3, count, "expected 3 <li> children")
}

func TestDocumentAttributes(t *testing.T) {
	tree := parseHTML(t, "test", `<a href="{{.}}">link</a>`)
	doc, err := template.ExecuteDocument(tree, "https://example.com")
	require.NoError(t, err)
	a := doc.FirstChild
	require.NotNil(t, a)
	assert.Equal(t, "a", a.Data)
	require.NotEmpty(t, a.Attr)
	assert.Equal(t, "href", a.Attr[0].Key)
	assert.Equal(t, "https://example.com", a.Attr[0].Val)
}

func TestDocumentNilTree(t *testing.T) {
	tree := parseHTML(t, "test", ``)
	doc, err := template.ExecuteDocument(tree, nil)
	require.NoError(t, err)
	assert.Equal(t, html.DocumentNode, doc.Type)
}

func TestDocumentNonHTMLTreeErrors(t *testing.T) {
	tree := parseText(t, "test", `hello {{.}}`)
	_, err := template.ExecuteDocument(tree, "world")
	require.Error(t, err)
}

// --- Issue C1: Panic on missing map keys in HTML mode ---

func TestHTMLMissingMapKeyDoesNotPanic(t *testing.T) {
	// Missing map key with default MissingKeyInvalid mode should produce
	// "<no value>" or similar, NOT panic on printableValue(val).
	require.NotPanics(t, func() {
		_ = execHTMLErr(t, "test", `<p>{{.Missing}}</p>`, map[string]int{"a": 1})
	})
}

func TestHTMLMissingMapKeyInAttrPosition(t *testing.T) {
	// Missing map key in attribute action position should not panic.
	// Template syntax: action produces the entire attribute, not part of a key.
	require.NotPanics(t, func() {
		_ = execHTMLErr(t, "test", `<div {{.Missing}}>x</div>`, map[string]string{"a": "b"})
	})
}

func TestHTMLMissingMapKeyInAttrValue(t *testing.T) {
	// Missing map key in attribute value should not panic.
	require.NotPanics(t, func() {
		_ = execHTMLErr(t, "test", `<div class="{{.Missing}}"></div>`, map[string]string{"a": "b"})
	})
}

func TestHTMLMissingMapKeyInAttrListAction(t *testing.T) {
	// Missing map key in walkHTMLAttrList action path should not panic.
	require.NotPanics(t, func() {
		tree := parseHTML(t, "test", `<ul>{{range .Items}}<li {{.Cls}}>{{.Name}}</li>{{end}}</ul>`)
		var buf bytes.Buffer
		_ = template.ExecuteTemplate(&buf, tree, map[string]any{
			"Items": []map[string]string{{"Name": "x"}},
		})
	})
}

// --- Issue H2: No JS/CSS escaping inside <script>/<style> ---

func TestHTMLScriptElementEscaping(t *testing.T) {
	result := execHTML(t, "test", `<script>var x = "{{.}}";</script>`, `</script><script>alert(1)`)
	// Inside <script>, content should be JS-escaped, not HTML-escaped.
	// HTML entities like &lt; are not interpreted inside <script> by browsers,
	// so they produce wrong output. JS escaping (\u003c) is correct.
	assert.NotContains(t, result, `&lt;`, "should use JS escaping, not HTML escaping, inside <script>")
	assert.Contains(t, result, `\u003c`, "should JS-escape < inside <script>")
}

func TestHTMLScriptElementSimpleText(t *testing.T) {
	result := execHTML(t, "test", `<script>var x = "{{.}}";</script>`, `hello`)
	assert.Contains(t, result, `hello`)
}

func TestHTMLStyleElementEscaping(t *testing.T) {
	result := execHTML(t, "test", `<style>body { background: "{{.}}" }</style>`, `</style><script>alert(1)</script>`)
	// Inside <style>, CSS escaping should prevent breaking out of the style element.
	assert.NotContains(t, result, `</style><script>`)
}

// --- Issue H3: Channel iteration unsupported in HTML range loops ---

func TestHTMLRangeChannel(t *testing.T) {
	ch := make(chan int, 3)
	ch <- 1
	ch <- 2
	ch <- 3
	close(ch)
	result := execHTML(t, "test", `{{range $v := .}}<li>{{$v}}</li>{{end}}`, ch)
	assert.Equal(t, "<li>1</li><li>2</li><li>3</li>", result)
}

func TestHTMLRangeChannelEmpty(t *testing.T) {
	ch := make(chan int)
	close(ch)
	result := execHTML(t, "test", `{{range $v := .}}<li>{{$v}}</li>{{else}}<li>none</li>{{end}}`, ch)
	assert.Equal(t, "<li>none</li>", result)
}

// --- Issue M2: Attribute key validation ---
// Note: Dynamic attribute keys via template actions in attribute-key position
// are not currently reachable from parsed templates (the parser treats {{action}}
// in attribute position as a separate attribute action, not part of a key).
// The validation is added defensively in evalHTMLAttribute.

// --- Issue M3: break/continue discards partially-built HTML element ---

func TestHTMLBreakPreservesPartialElement(t *testing.T) {
	result := execHTML(t, "test", `{{range .}}<ul><li>{{.}}</li>{{break}}</ul>{{end}}`, []string{"first", "second"})
	assert.Contains(t, result, "<ul><li>first</li></ul>")
}

// --- Issue M4: evalHTMLAttrValueNodes extracts tag name from element nodes ---
// The parser does not produce HTMLElementNode inside attribute values, so this
// code path (lines 992-993 in state.go) is not reachable from parsed templates.
// Added defensive fix to extract text content from element nodes if reached.

// --- Issue M5: "<no value>" magic string for attribute suppression ---

func TestHTMLNoValueMagicStringInAttrValue(t *testing.T) {
	// User data literally "<no value>" in an attribute value should not be suppressed.
	result := execHTML(t, "test", `<div class="{{.}}"></div>`, "<no value>")
	assert.Contains(t, result, "class=")
}

func TestHTMLNoValueMagicStringInAttrAction(t *testing.T) {
	// An action in attribute position that evaluates to literally "<no value>"
	// should produce output, not be silently suppressed by matching a magic string.
	// The value will be HTML-escaped in the output.
	result := execHTML(t, "test", `<div {{.}}>x</div>`, "<no value>")
	assert.Contains(t, result, "no value", "literal '<no value>' string should not be dropped")
	assert.NotEqual(t, "<div>x</div>", result, "attribute should not be silently dropped")
}

// --- Issue L4: isJSEventAttr overly broad ---

func TestHTMLNonEventOnPrefixAttr(t *testing.T) {
	// "only" is not a JS event handler; its dynamic value should not be JS-escaped.
	result := execHTML(t, "test", `<div only="{{.}}"></div>`, "some text")
	assert.Equal(t, `<div only="some text"></div>`, result)
	// A quote in a non-event attribute should be HTML-escaped, not JS-escaped.
	result = execHTML(t, "test", `<div only="{{.}}"></div>`, `he said "hi"`)
	assert.NotContains(t, result, `\u0022`, `"only" attr should not be JS-escaped`)
	assert.Contains(t, result, `&#34;`, `"only" attr should be HTML-escaped`)
}

func TestHTMLActualEventHandlerAttr(t *testing.T) {
	// "onclick" IS a JS event handler; its dynamic value should be JS-escaped.
	result := execHTML(t, "test", `<div onclick="{{.}}">x</div>`, `he said "hi"`)
	assert.Contains(t, result, `\u0022`, `"onclick" attr should be JS-escaped`)
}

// TestAndOrShortCircuit pins htt's runtime behavior that the LSP hover/error
// note (check.BuiltinDoc) describes: and/or short-circuit argument evaluation
// left to right, matching text/template since Go 1.18. If htt ever stops
// short-circuiting, this test fails as a reminder to update that note.
func TestAndOrShortCircuit(t *testing.T) {
	var fCalls, gCalls, hCalls int
	funcs := template.FuncMap{
		"f": func() string { fCalls++; return "" },  // empty (falsy)
		"g": func() string { gCalls++; return "x" }, // non-empty (truthy)
		"h": func() string { hCalls++; return "y" }, // non-empty (truthy)
	}
	// and stops at the first empty argument: f is empty, so g is never called.
	execText(t, "and", `{{and (f) (g)}}`, nil, template.WithFuncs(funcs))
	assert.Equal(t, 1, fCalls, "and evaluates the first argument")
	assert.Equal(t, 0, gCalls, "and short-circuits: the second argument is not evaluated")

	// or stops at the first non-empty argument: g is non-empty, so h is not called.
	gCalls = 0
	execText(t, "or", `{{or (g) (h)}}`, nil, template.WithFuncs(funcs))
	assert.Equal(t, 1, gCalls, "or evaluates the first argument")
	assert.Equal(t, 0, hCalls, "or short-circuits: the second argument is not evaluated")

	// and keeps going while arguments are truthy: g is non-empty, so h runs too.
	gCalls, hCalls = 0, 0
	execText(t, "and2", `{{and (g) (h)}}`, nil, template.WithFuncs(funcs))
	assert.Equal(t, 1, gCalls)
	assert.Equal(t, 1, hCalls, "and keeps evaluating while arguments are truthy")
}

// TestAndOrUserOverrideNotShortCircuited confirms the short-circuit applies
// only to the built-in: a user-supplied and/or is a normal (eager) call.
func TestAndOrUserOverrideNotShortCircuited(t *testing.T) {
	var fCalls, gCalls int
	funcs := template.FuncMap{
		"f":   func() string { fCalls++; return "" }, // empty (falsy)
		"g":   func() string { gCalls++; return "x" },
		"and": func(a, b string) string { return a + b }, // shadows the built-in
	}
	// f is empty; the built-in and would stop and never call g. The user's
	// and takes both arguments, so g must still be evaluated.
	execText(t, "override", `{{and (f) (g)}}`, nil, template.WithFuncs(funcs))
	assert.Equal(t, 1, fCalls)
	assert.Equal(t, 1, gCalls, "a user-supplied and is a normal (eager) call, not short-circuited")
}

// TestAndOrValueSelection confirms short-circuiting does not change which
// value and/or return: the first empty (and) / first non-empty (or), else the
// last operand — including the single-operand and piped forms.
func TestAndOrValueSelection(t *testing.T) {
	for _, tc := range []struct{ tmpl, want string }{
		{`{{and 1 2}}`, "2"},     // both truthy -> last
		{`{{and 1 ""}}`, ""},     // second empty -> second
		{`{{and "" 2}}`, ""},     // first empty -> first
		{`{{or "" 3}}`, "3"},     // first empty -> second
		{`{{or 1 2}}`, "1"},      // first non-empty -> first
		{`{{or "" ""}}`, ""},     // all empty -> last
		{`{{and 5}}`, "5"},       // single operand -> itself
		{`{{"x" | and 1}}`, "x"}, // piped: and(1, "x"), both truthy -> last
	} {
		assert.Equal(t, tc.want, execText(t, "v", tc.tmpl, nil), tc.tmpl)
	}
}
