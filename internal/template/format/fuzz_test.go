package format_test

import (
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/format"
	"github.com/typelate/muxt/internal/template/lex"
)

// FuzzFormatIdempotent fuzzes the formatter and checks that formatting is
// idempotent: format(format(x)) == format(x) for all successfully parsed inputs.
func FuzzFormatIdempotent(f *testing.F) {
	seeds := []string{
		"<p>hello</p>",
		"<p>{{.}}</p>",
		"<p>{{.Name}}</p>",
		`<a href="{{.URL}}">link</a>`,
		"{{if .}}<p>yes</p>{{end}}",
		"{{if .}}<p>yes</p>{{else}}<p>no</p>{{end}}",
		"{{range .}}<li>{{.}}</li>{{end}}",
		"<ul>{{range .}}<li>{{.}}</li>{{end}}</ul>",
		"{{with .Name}}<span>{{.}}</span>{{end}}",
		`<div class="{{.Class}}">x</div>`,
		`<input type="text" disabled>`,
		"<br>",
		"<!DOCTYPE html><html><body>hello</body></html>",
		"<!-- comment -->",
		`{{define "item"}}<li>{{.}}</li>{{end}}`,
		`{{template "header" .}}`,
		`<div>{{if .Active}}<span class="on">{{.Name}}</span>{{else}}<span class="off">none</span>{{end}}</div>`,
		`{{range $i, $v := .Items}}<div>{{$i}}: {{$v}}</div>{{end}}`,
		`{{$x := "hello"}}<p>{{$x}}</p>`,
		`<p>{{. | printf "%v"}}</p>`,
		`{{else if .Other}}<p>other</p>`,
		`<script>var x = {{.}};</script>`,
		`<button onclick="{{.Handler}}">click</button>`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	opts := lex.Options{
		LeftDelim:  "{{",
		RightDelim: "}}",
		ParseHTML:  true,
		BreakOK:    true,
		ContinueOK: true,
	}
	f.Fuzz(func(t *testing.T, input string) {
		tree, err := template.ParseTree("fuzz", "fuzz", input, nil, opts)
		if err != nil {
			return
		}
		formatted1 := format.Node(tree.Root)

		tree2, err := template.ParseTree("fuzz", "fuzz", formatted1, nil, opts)
		if err != nil {
			t.Fatalf("re-parse of formatted output failed:\n  input:     %q\n  formatted: %q\n  error:     %v", input, formatted1, err)
		}
		formatted2 := format.Node(tree2.Root)

		if formatted1 != formatted2 {
			t.Errorf("format not idempotent:\n  input:      %q\n  format(1):  %q\n  format(2):  %q", input, formatted1, formatted2)
		}
	})
}
