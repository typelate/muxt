package lex_test

import (
	"testing"

	"github.com/typelate/muxt/internal/template/lex"
)

// FuzzScanText fuzzes the text-mode lexer, ensuring no panics on arbitrary input.
func FuzzScanText(f *testing.F) {
	seeds := []string{
		"",
		"hello",
		"{{.}}",
		"{{.Name}}",
		"{{if .}}yes{{end}}",
		"{{range .}}{{.}}{{end}}",
		`{{$x := .}}`,
		`{{$x = .}}`,

		// numbers
		"{{42}}", "{{-3}}", "{{3.14}}", "{{1e10}}", "{{0x1F}}",
		"{{0o77}}", "{{0b101}}", "{{1+2i}}", "{{'a'}}",
		"{{1_000}}", "{{0xff_ff}}",

		// strings
		`{{"hello"}}`, "{{`raw`}}", `{{"escape\n"}}`,
		`{{"unclosed`, "{{`unclosed",

		// trim markers
		"{{- .}}", "{{. -}}", "{{- . -}}",
		"{{-3}}", // NOT a trim marker

		// comments
		"{{/* comment */}}", "{{/**/}}",
		"{{/* unclosed",

		// delimiters
		"{{", "}}", "{{}}", "{{ }}", "{{|}}", "{{|.|}}",

		// pipes and chains
		"{{. | print}}", "{{.X.Y.Z}}", "{{$.Var}}", "{{(.)}}",

		// keywords
		"{{if .}}", "{{else}}", "{{end}}", "{{range .}}",
		"{{with .}}", "{{define \"x\"}}", "{{template \"x\"}}",
		"{{block \"x\" .}}", "{{break}}", "{{continue}}",
		"{{nil}}", "{{true}}", "{{false}}",

		// adjacent actions
		"{{.}}{{.}}", "text{{.}}text{{.}}text",

		// unicode
		"{{.Übergrößenträger}}",
		"你好{{.世界}}",

		// pathological
		"{{{{", "}}}}", "{{{}}}",
		"{{.....}}", "{{$$$$}}",
		"{{ ( ( ( . ) ) ) }}",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		s := lex.New(input, lex.Options{
			LeftDelim:  "{{",
			RightDelim: "}}",
			BreakOK:    true,
			ContinueOK: true,
		})
		for {
			tok := s.Next()
			if tok.Type == lex.TokenTypeEOF || tok.Type == lex.TokenTypeError {
				break
			}
		}
	})
}

// FuzzScanHTML fuzzes the HTML-mode lexer, ensuring no panics on arbitrary input.
func FuzzScanHTML(f *testing.F) {
	seeds := []string{
		"",
		"hello",
		"<p>hello</p>",
		"<div><span>x</span></div>",
		"<br>", "<hr>", "<img src=\"x\">",
		"<!-- comment -->",
		"<!DOCTYPE html>",

		// attributes
		`<div class="x">`, `<div class='x'>`, `<div class=x>`,
		`<input disabled>`, `<input type="text" value="">`,

		// dynamic
		"<p>{{.}}</p>",
		`<a href="{{.}}">`,
		`<div class="{{.}}">`,
		`<div class={{.}}>`,

		// raw text elements
		"<script>var x = 1;</script>",
		"<style>body{}</style>",
		"<textarea>text</textarea>",
		"<title>page</title>",
		"<script>{{.}}</script>",
		"<script>if (a < b) { x = 1; }</script>",

		// bare angle brackets
		"a < b", "a > b",
		"<script>a < b > c</script>",

		// event handlers
		`<button onclick="alert('hi')">`,
		`<div onmouseover="{{.}}">`,

		// unclosed/malformed
		"<p", "<p>", "</p>", "<>", "</>",
		`<p class="`, `<p class=`, "<div><span></div>",
		"<p>{{.}", "{{range .}}<li>",

		// deeply nested
		"<div><div><div><div><p>deep</p></div></div></div></div>",

		// multiple tags
		"<p>a</p><p>b</p><p>c</p>",

		// mixed content
		"text<b>bold</b>text",
		"<p>before{{if .}}<b>bold</b>{{end}}after</p>",

		// pathological
		"<<<<<", ">>>>>", "<<<>>>",
		"</", "<!", "<!-", "<!--", "<!--->", "<!-->",
		"<script><script>", "</script></script>",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		s := lex.New(input, lex.Options{
			LeftDelim:  "{{",
			RightDelim: "}}",
			ParseHTML:  true,
			BreakOK:    true,
			ContinueOK: true,
		})
		for {
			tok := s.Next()
			if tok.Type == lex.TokenTypeEOF || tok.Type == lex.TokenTypeError {
				break
			}
		}
	})
}
