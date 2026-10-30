package format

import (
	"testing"

	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/template/format/internal/rendering"
)

// FuzzFormatEquivalent checks that formatting is idempotent and keeps the
// rendered page (spec §8.3) for every input that parses, under each dot
// from fuzzDots that the input executes with and renders to a page the
// oracle can read. Its seeds are the
// fixture inputs, less knownViolations.
func FuzzFormatEquivalent(f *testing.F) {
	for _, filename := range fixtureFiles(f) {
		if _, known := knownViolations[fixtureName(filename)]; known {
			continue
		}
		ar, err := txtar.ParseFile(filename)
		if err != nil {
			f.Fatal(err)
		}
		for _, file := range ar.Files {
			if file.Name == "input.gohtml" {
				f.Add(string(file.Data))
			}
		}
	}
	f.Fuzz(func(t *testing.T, input string) {
		out, err := Source("fuzz.gohtml", input)
		if err != nil {
			return
		}
		again, err := Source("fuzz.gohtml", out)
		if err != nil {
			t.Fatalf("Source(output) error: %v\ninput:  %q\noutput: %q", err, input, out)
		}
		if again != out {
			t.Errorf("not idempotent\ninput:      %q\nformat(1):  %q\nformat(2):  %q", input, out, again)
		}
		for _, dot := range fuzzDots() {
			want, err := render(input, "", dot)
			if err != nil {
				continue
			}
			if _, err := rendering.Tokens(want, rendering.Body, nil); err != nil {
				continue // the oracle cannot read the input's page (e.g. parsers disagree on a tag)
			}
			got, err := render(out, "", dot)
			if err != nil {
				t.Fatalf("output does not execute with %v: %v\ninput:  %q\noutput: %q", dot, err, input, out)
			}
			diff, err := rendering.Diff(want, got, rendering.Body, nil)
			if err != nil {
				t.Fatalf("rendering.Diff with %v: %v\ninput:  %q\noutput: %q", dot, err, input, out)
			}
			if diff != "" {
				t.Errorf("rendering changed with %v: %s\ninput:  %q\noutput: %q\ninput rendered:  %q\noutput rendered: %q", dot, diff, input, out, want, got)
			}
		}
	})
}

// fuzzDots returns the data FuzzFormatEquivalent executes each input with:
// empty, so falsy arms render, and truthy, so if/with bodies and a
// non-empty range render.
func fuzzDots() []map[string]any {
	return []map[string]any{
		{},
		{"Show": true, "X": "x", "Items": []any{"a", "b"}},
	}
}

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
	f.Fuzz(func(t *testing.T, input string) {
		formatted1, err := Source("fuzz.gohtml", input)
		if err != nil {
			return
		}
		formatted2, err := Source("fuzz.gohtml", formatted1)
		if err != nil {
			t.Fatalf("re-parse of formatted output failed:\n  input:     %q\n  formatted: %q\n  error:     %v", input, formatted1, err)
		}
		if formatted1 != formatted2 {
			t.Errorf("format not idempotent:\n  input:      %q\n  format(1):  %q\n  format(2):  %q", input, formatted1, formatted2)
		}
	})
}
