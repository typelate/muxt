package template

import (
	"testing"

	"github.com/typelate/muxt/internal/template/lex"
)

// FuzzParseText fuzzes the text/template-compatible parser.
// It checks that arbitrary input never causes a panic.
func FuzzParseText(f *testing.F) {
	seeds := []string{
		// basics
		"",
		"hello",
		"{{.}}",
		"{{.Name}}",
		"{{if .}}yes{{end}}",
		"{{if .}}yes{{else}}no{{end}}",
		"{{range .}}{{.}}{{end}}",
		"{{with .}}{{.}}{{end}}",
		"{{$x := .}}{{$x}}",
		`{{template "foo" .}}`,
		`{{define "foo"}}bar{{end}}`,
		`{{block "foo" .}}default{{end}}`,

		// pipelines
		`{{. | printf "%v"}}`,
		`{{.Name | printf "%s"}}`,
		`{{.Items | len}}`,

		// builtins
		`{{eq 1 2}}`,
		`{{lt 1 2}}`,
		`{{and true false}}`,
		`{{or false true}}`,
		`{{not true}}`,
		`{{index . 0}}`,
		`{{slice . 1 3}}`,
		`{{call .Fn "arg"}}`,
		`{{len .}}`,
		`{{print "hello"}}`,
		`{{printf "%d" 42}}`,
		`{{println "x"}}`,

		// numbers
		`{{-3}}`,
		`{{0x1F}}`,
		`{{0o77}}`,
		`{{0b1010}}`,
		`{{1.5e10}}`,
		`{{1+2i}}`,
		`{{'a'}}`,

		// trim markers
		`{{- .Name}}`,
		`{{.Name -}}`,
		`{{- .Name -}}`,

		// comments
		`{{/* comment */}}`,

		// variables with assignment
		`{{$x := 1}}{{$x = 2}}{{$x}}`,

		// range with index
		`{{range $i, $v := .}}{{$i}}:{{$v}}{{end}}`,

		// break/continue
		`{{range .}}{{if .}}{{break}}{{end}}{{end}}`,
		`{{range .}}{{if .}}{{continue}}{{end}}{{end}}`,

		// else if / else with
		`{{if false}}a{{else if true}}b{{end}}`,
		`{{with 0}}x{{else with "hi"}}{{.}}{{end}}`,

		// chaining
		`{{(printf "%s" "hi") | printf "%s"}}`,

		// nested parens
		`{{((.))}}`,
		`{{((((.)))) | print}}`,

		// tricky edge cases
		"{{",
		"}}",
		"{{}}",
		`{{""}}`,
		"{{..",
		"{{(){}",
		`{{if`,
		`{{if .}}`,
		`{{if .}}{{else}}`,
		`{{end}}`,
		`{{range}}`,
		"{{range .}}{{else}}{{end}}",
		`{{"unclosed`,
		"{{`raw",
		`{{"unterm\}}`,
		`{{define}}`,
		`{{define "x"}}`,
		`{{template}}`,
		`{{.Nested.Deeply.Chained.Field}}`,
		`{{$.Root.Access}}`,
		`{{$}}`,
		`{{nil}}`,
		`{{true}}`,
		`{{false}}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic. Errors are fine.
		_, _ = ParseTree("fuzz", "fuzz", input, nil, lex.Options{
			LeftDelim:  "{{",
			RightDelim: "}}",
			BreakOK:    true,
			ContinueOK: true,
		})
	})
}

// FuzzParseHTML fuzzes the HTML-aware parser.
// It checks that arbitrary input never causes a panic, and that
// successfully parsed trees can be re-serialized via String().
func FuzzParseHTML(f *testing.F) {
	seeds := []string{
		// basic HTML
		"",
		"hello",
		"<p>hello</p>",
		"<div><p>inner</p></div>",
		"<br>",
		"<hr>",
		"<img src=\"test.png\">",
		`<!DOCTYPE html><html><body>hello</body></html>`,
		"<!-- comment -->",
		"<p><!-- {{.}} --></p>",

		// attributes
		`<div class="x">y</div>`,
		`<div class='x'>y</div>`,
		`<div class=x>y</div>`,
		`<input disabled>`,
		`<input type="text" value="">`,
		`<a href="https://example.com">link</a>`,

		// dynamic content
		"<p>{{.}}</p>",
		"<p>{{.Name}}</p>",
		`<a href="{{.URL}}">link</a>`,
		`<div class="{{.Class}}">x</div>`,
		`<div class={{.Class}}>x</div>`,
		`<img src="{{.}}">`,

		// control structures in HTML
		"{{if .}}<p>yes</p>{{end}}",
		"{{if .}}<p>yes</p>{{else}}<p>no</p>{{end}}",
		"{{range .}}<li>{{.}}</li>{{end}}",
		"<ul>{{range .}}<li>{{.}}</li>{{end}}</ul>",
		"{{with .Name}}<span>{{.}}</span>{{end}}",

		// event handlers
		`<button onclick="doStuff()">click</button>`,
		`<button onclick="{{.}}">click</button>`,
		`<div onmouseover="{{.}}">x</div>`,

		// raw text elements
		"<script>var x = 1;</script>",
		"<style>body { color: red; }</style>",
		"<textarea>{{.}}</textarea>",
		"<title>{{.}}</title>",
		`<script>var x = "{{.}}";</script>`,

		// void elements with actions
		`<img src="{{.}}">`,
		`<meta charset="utf-8">`,
		`<link rel="stylesheet" href="{{.}}">`,

		// nested control + HTML
		`<div>{{if .}}<span>{{.}}</span>{{end}}</div>`,
		`<table>{{range .}}<tr><td>{{.}}</td></tr>{{end}}</table>`,

		// define inside HTML
		`{{define "item"}}<li>{{.}}</li>{{end}}`,

		// edge cases: unclosed/malformed
		"<p>",
		"<p>hello",
		"</p>",
		"<>",
		"</>",
		"<div><span></div></span>",
		"<p",
		`<p class="x`,
		`<p class=`,
		"<p>{{.}",
		"{{range .}}<li>{{.}}</li>",

		// bare angle brackets
		"a < b",
		"a > b",
		"1 < 2 > 0",
		"<script>if (a < b) {}</script>",

		// multiple roots
		"<p>one</p><p>two</p>",
		"<div>a</div>text<div>b</div>",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		tree, err := ParseTree("fuzz", "fuzz", input, nil, lex.Options{
			LeftDelim:  "{{",
			RightDelim: "}}",
			ParseHTML:  true,
			BreakOK:    true,
			ContinueOK: true,
		})
		if err != nil {
			return
		}
		// If parsing succeeded, String() and AnnotateHTMLContexts must not panic.
		_ = tree.Root.String()
		AnnotateHTMLContexts(tree.Root)
	})
}

// FuzzParseTextRoundTrip fuzzes the text parser and checks that
// re-parsing a successfully parsed template's String() output
// does not produce an error (parse round-trip stability).
func FuzzParseTextRoundTrip(f *testing.F) {
	seeds := []string{
		"{{.}}",
		"{{.Name}}",
		`{{printf "%s" .}}`,
		`{{if .}}yes{{end}}`,
		`{{if .}}yes{{else}}no{{end}}`,
		`{{range .}}{{.}}{{end}}`,
		`{{range $i, $v := .}}{{$i}}:{{$v}}{{end}}`,
		`{{with .}}{{.}}{{end}}`,
		`{{$x := .}}{{$x}}`,
		`{{eq 1 2}}`,
		`{{"hello"}}`,
		"{{`raw`}}",
		`{{/* comment */}}`,
		`{{.Field1 | printf "%v"}}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	opts := lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		BreakOK:    true,
		ContinueOK: true,
	}
	f.Fuzz(func(t *testing.T, input string) {
		tree1, err := ParseTree("fuzz", "fuzz", input, nil, opts)
		if err != nil {
			return
		}
		serialized := tree1.Root.String()
		_, err = ParseTree("fuzz", "fuzz", serialized, nil, opts)
		if err != nil {
			t.Errorf("round-trip parse failed:\n  input:      %q\n  serialized: %q\n  error:      %v", input, serialized, err)
		}
	})
}
