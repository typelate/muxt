package template_test

import (
	"bytes"
	htmltemplate "html/template"
	"io"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/lex"
)

// FuzzExecuteHTMLStdlibCompat fuzzes with string data, comparing htt output
// against html/template for the same template+data pair.
// Templates that fail to parse or execute in either engine are skipped.
func FuzzExecuteHTMLStdlibCompat(f *testing.F) {
	seeds := []struct {
		tmpl string
		data string
	}{
		// static
		{"<p>hello</p>", ""},
		{"<br>", ""},
		{"<div><span>x</span></div>", ""},
		{"<!-- comment -->", ""},

		// text context escaping
		{"<p>{{.}}</p>", "hello"},
		{"<p>{{.}}</p>", "<script>alert(1)</script>"},
		{"<p>{{.}}</p>", "a&b"},
		{"<p>{{.}}</p>", `he said "hi"`},
		{"<p>{{.}}</p>", "it's"},
		{"<p>{{.}}</p>", ""},
		{"<p>{{.}}</p>", "a\u2028b\u2029c"},

		// attribute escaping
		{`<div class="{{.}}">x</div>`, "my-class"},
		{`<div class="{{.}}">x</div>`, `"><script>alert(1)</script>`},
		{`<div title="{{.}}">x</div>`, `a "quoted" & <value>`},
		{`<div id="{{.}}">x</div>`, "abc"},

		// URL attributes
		{`<a href="{{.}}">x</a>`, "https://example.com"},
		{`<a href="{{.}}">x</a>`, "http://example.com"},
		{`<a href="{{.}}">x</a>`, "mailto:user@example.com"},
		{`<a href="{{.}}">x</a>`, "javascript:alert(1)"},
		{`<a href="{{.}}">x</a>`, "data:text/html,<script>"},
		{`<a href="{{.}}">x</a>`, "vbscript:MsgBox"},
		{`<a href="{{.}}">x</a>`, "/relative/path"},
		{`<a href="{{.}}">x</a>`, "?query=1&b=2"},
		{`<a href="{{.}}">x</a>`, "#fragment"},
		{`<img src="{{.}}">`, "javascript:alert(1)"},
		{`<a href="{{.}}">x</a>`, "https://example.com?a=1&b=2"},

		// event handlers
		{`<button onclick="{{.}}">x</button>`, "doStuff()"},
		{`<button onclick="{{.}}">x</button>`, ""},

		// comments stripped
		{`<!-- {{.}} --><p>hi</p>`, "secret"},

		// various elements
		{"<p>{{.}}</p>", "normal text"},
		{"<textarea>{{.}}</textarea>", "<script>alert(1)</script>"},
		{"<title>{{.}}</title>", "<script>alert(1)</script>"},
	}
	for _, s := range seeds {
		f.Add(s.tmpl, s.data)
	}
	f.Fuzz(func(t *testing.T, tmpl, data string) {
		// Try html/template first. If it fails to parse or execute, skip.
		stdTmpl, err := htmltemplate.New("test").Parse(tmpl)
		if err != nil {
			return
		}
		var stdBuf bytes.Buffer
		if err := stdTmpl.Execute(&stdBuf, data); err != nil {
			return
		}
		want := stdBuf.String()

		// Parse with htt.
		tree, err := template.ParseTree("test", "test", tmpl, nil, lex.Options{
			LeftDelim:  "{{",
			RightDelim: "}}",
			ParseHTML:  true,
			BreakOK:    true,
			ContinueOK: true,
		})
		if err != nil {
			// htt may reject some templates that html/template accepts (or vice versa).
			// That's not a crash bug, so skip.
			return
		}
		template.AnnotateHTMLContexts(tree.Root)

		var httBuf bytes.Buffer
		if err := template.ExecuteTemplate(&httBuf, tree, data); err != nil {
			// Execution error is not a panic; skip.
			return
		}
		got := httBuf.String()

		if got != want {
			t.Errorf("output mismatch:\n  template: %q\n  data:     %q\n  want:     %q\n  got:      %q", tmpl, data, want, got)
		}
	})
}

// FuzzExecuteTextNoPanic fuzzes the text-mode execution engine to ensure
// no panics occur on arbitrary templates with nil data.
func FuzzExecuteTextNoPanic(f *testing.F) {
	seeds := []string{
		"hello",
		"{{.}}",
		`{{"hello"}}`,
		"{{42}}",
		"{{true}}",
		`{{print "hi"}}`,
		`{{printf "%d" 42}}`,
		"{{if true}}yes{{end}}",
		"{{if false}}yes{{else}}no{{end}}",
		`{{$x := "val"}}{{$x}}`,
		"{{$x := 1}}{{$x = 2}}{{$x}}",
		"{{eq 1 1}}",
		"{{lt 1 2}}",
		"{{and 1 0}}",
		"{{or 0 1}}",
		"{{not true}}",
		`{{len "hello"}}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, tmpl string) {
		tree, err := template.ParseTree("fuzz", "fuzz", tmpl, nil, lex.Options{
			LeftDelim:  "{{",
			RightDelim: "}}",
			BreakOK:    true,
			ContinueOK: true,
		})
		if err != nil {
			return
		}
		// Execute with nil data — errors are fine, panics are not.
		template.ExecuteTemplate(io.Discard, tree, nil)
	})
}
