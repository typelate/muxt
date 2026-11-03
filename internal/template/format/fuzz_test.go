package format

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/format/internal/rendering"
	"github.com/typelate/muxt/internal/template/lex"
)

// FuzzFormatEquivalent checks that formatting is idempotent and keeps the
// rendered page (spec §8.3) for every input that parses, under each dot
// from fuzzDots that the input executes with and renders to a page the
// oracle can read: at the top level and through the synthetic callers of
// each {{define}} (rendering.Callers). A call-site caller that does not
// parse beside the input is skipped and counted. Until
// defineCallSiteModes, a call-site caller's failure is logged and
// counted instead of failing; both counts are logged when the run ends. Its seeds are the
// fixture inputs and the WHATWG regression snippets.
func FuzzFormatEquivalent(f *testing.F) {
	addFixtureSeeds(f)
	var pending, unparsable atomic.Int64
	f.Cleanup(func() {
		f.Logf("call-site caller failures pending Task 7: %d; callers skipped because input plus caller does not parse: %d", pending.Load(), unparsable.Load())
	})
	f.Fuzz(func(t *testing.T, input string) {
		out, err := Source("fuzz.gohtml", input)
		if formatterBug(err) {
			t.Fatalf("Source(input) error: %v\ninput: %q", err, input)
		}
		if err != nil {
			return
		}
		again, err := Source("fuzz.gohtml", out)
		if err != nil {
			t.Fatalf("Source(output) error: %v\ninput:  %q\noutput: %q", err, input, out)
		}
		checkActionShapes(t, input, out)
		if again != out {
			t.Errorf("not idempotent\ninput:      %q\nformat(1):  %q\nformat(2):  %q", input, out, again)
		}
		callers, err := defineCallers(input)
		if err != nil {
			callers = []rendering.Caller{{Name: "top", Context: rendering.Body}}
		}
		for _, c := range callers {
			in, formatted := input, out
			if c.CallSite {
				in = input + oracleCallerDefine(c)
				formatted, err = Source("fuzz.gohtml", in)
				if formatterBug(err) {
					t.Fatalf("Source(input with caller %s) error: %v\ninput: %q", c.Name, err, in)
				}
				if err != nil {
					// The input ends so that the appended define does not
					// parse (an open delimiter, say): not a formatter bug.
					unparsable.Add(1)
					continue
				}
			}
			for _, dot := range fuzzDots() {
				p := fuzzRenderingProblem(in, formatted, c.Source, c.Context, dot)
				switch {
				case p == "":
				case c.CallSite && !defineCallSiteModes:
					pending.Add(1)
					t.Logf("pending Task 7: caller %s: %s", c.Name, p)
				default:
					t.Errorf("caller %s: %s", c.Name, p)
				}
			}
		}
	})
}

// formatterBug reports whether err is one Source returns for a
// formatter bug rather than for its input: overlapping edits, an action
// it could not read, or printed actions that do not parse as the
// source's.
func formatterBug(err error) bool {
	return errors.Is(err, errOverlappingEdits) || errors.Is(err, errActionChanged) || errors.Is(err, errActionUnread)
}

// checkActionShapes fails t unless output parses to the same actions as
// input (actionShapes), positions and spacing aside.
func checkActionShapes(t *testing.T, input, output string) {
	t.Helper()
	parse := func(src string) []string {
		tree, err := template.ParseTree("fuzz.gohtml", "fuzz.gohtml", src, nil, lex.Options{
			EmitComment: true,
			ParseHTML:   true,
			BreakOK:     true,
			ContinueOK:  true,
		})
		if err != nil {
			t.Fatalf("ParseTree: %v\nsource: %q", err, src)
		}
		return actionShapes(tree.Root, CanonicalName)
	}
	if in, out := parse(input), parse(output); !slices.Equal(in, out) {
		t.Errorf("actions parse differently\ninput:  %q\noutput: %q\ninput actions:  %q\noutput actions: %q", input, output, in, out)
	}
}

// FuzzFormatUnresolvedCalls formats inputs that call templates they do not
// define, as the formatter sees a partial without its package, then
// renders input and output with a small define set appended under every
// unresolved name. The formatter assumes such a call writes what belongs
// where it stands (spec §2); each define set must render alike.
func FuzzFormatUnresolvedCalls(f *testing.F) {
	for _, s := range []string{
		`<p>a{{template "x" .}}b</p>`,
		"<div>\n\t{{template \"x\" .}}\n</div>",
		"<p>\n\ta\n\t{{template \"x\" .}}\n\tb\n</p>",
		"<table>\n\t{{template \"row\" .}}\n</table>",
		"<table>\n<tbody>\n{{range .Items}}\n{{template \"row\" .}}\n{{end}}\n</tbody>\n</table>",
		"<html>\n<head>\n{{template \"head\" .}}\n\t<link>\n</head>\n<body>\n{{template \"body\" .}}\n</body>\n</html>",
		"<ul>\n{{range .Items}}\n\t<li>{{template \"x\" .}}</li>\n{{end}}\n</ul>",
		"{{template \"x\" .}}\n\t{{template \"y\" .}}",
		"<span>{{template \"x\" .}}</span> <span>{{block \"b\" .}}{{template \"y\"}}{{end}}</span>",
		"{{if .A}}\n\t{{template \"x\" .}}\n{{else}}\n\t<p>{{template \"x\" .}}</p>\n{{end}}",
	} {
		f.Add(s)
	}
	addFixtureSeeds(f)
	f.Fuzz(func(t *testing.T, input string) {
		names := unresolvedCalls(input)
		if len(names) == 0 {
			return
		}
		out, err := Source("fuzz.gohtml", input)
		if formatterBug(err) {
			t.Fatalf("Source(input) error: %v\ninput: %q", err, input)
		}
		if err != nil {
			return
		}
		checkActionShapes(t, input, out)
		// The output calls a route name as the formatter rewrote it
		// (CanonicalName); the package defines it so too.
		outNames := unresolvedCalls(out)
		for _, body := range []string{"t", "<tr><td>t</td></tr>", "<span>t</span>", " \n\t"} {
			var defines, outDefines strings.Builder
			for _, name := range names {
				defines.WriteString(`{{define ` + strconv.Quote(name) + `}}` + body + `{{end}}`)
			}
			for _, name := range outNames {
				outDefines.WriteString(`{{define ` + strconv.Quote(name) + `}}` + body + `{{end}}`)
			}
			for _, dot := range fuzzDots() {
				if p := fuzzRenderingProblem(input+defines.String(), out+outDefines.String(), "", rendering.Body, dot); p != "" {
					t.Errorf("with every unresolved name defined as %q: %s", body, p)
				}
			}
		}
	})
}

// addFixtureSeeds adds the fixture inputs and the WHATWG regression
// snippets to f's corpus, leaving out those with documented pending
// problems: they fail until their task lands.
func addFixtureSeeds(f *testing.F) {
	for _, filename := range fixtureFiles(f) {
		ar, err := txtar.ParseFile(filename)
		if err != nil {
			f.Fatal(err)
		}
		if pendingLine.Match(ar.Comment) {
			continue
		}
		for _, file := range ar.Files {
			if file.Name == "input.gohtml" {
				f.Add(string(file.Data))
			}
		}
	}
	for _, row := range whatwgRegressions {
		f.Add(row.input) // not whatwgPending: those fail until their task lands
	}
}

// fuzzRenderingProblem describes how output renders unlike input
// through callerSrc ("" for the top level) with dot, or returns "".
// Inputs that do not execute, or whose page the oracle cannot read, are
// not checked.
func fuzzRenderingProblem(input, output, callerSrc string, ctx rendering.Context, dot map[string]any) string {
	want, err := render(input, callerSrc, dot)
	if err != nil {
		return ""
	}
	if _, err := rendering.Tokens(want, ctx, nil); err != nil {
		return "" // the oracle cannot read the input's page (e.g. parsers disagree on a tag)
	}
	got, err := render(output, renamedCaller(callerSrc, output), dot)
	if err != nil {
		return fmt.Sprintf("output does not execute with %v: %v\ninput:  %q\noutput: %q", dot, err, input, output)
	}
	diff, err := rendering.Diff(want, got, ctx, nil)
	if err != nil {
		return fmt.Sprintf("rendering.Diff with %v: %v\ninput:  %q\noutput: %q", dot, err, input, output)
	}
	if diff != "" {
		return fmt.Sprintf("rendering changed with %v: %s\ninput:  %q\noutput: %q\ninput rendered:  %q\noutput rendered: %q", dot, diff, input, output, want, got)
	}
	return ""
}

// unresolvedCalls returns the names src calls with {{template}} that no
// {{define}} or {{block}} in src defines, in order of first call.
func unresolvedCalls(src string) []string {
	tree, err := template.ParseTree("fuzz.gohtml", "fuzz.gohtml", src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	if err != nil {
		return nil
	}
	defined := map[string]bool{"fuzz.gohtml": true}
	var called []string
	template.Walk(tree, func(_ *template.Tree, n template.Node) {
		switch n := n.(type) {
		case *template.DefineNode:
			defined[n.Name] = true
		case *template.BlockNode:
			defined[n.Name] = true
		case *template.TemplateNode:
			if !slices.Contains(called, n.Name) {
				called = append(called, n.Name)
			}
		}
	})
	return slices.DeleteFunc(called, func(name string) bool { return defined[name] })
}

// fuzzDots returns the data the fuzz targets execute each input with:
// empty, so falsy arms render, and each mix of true and false A and B,
// with Show true, a one-element Items, and an X that starts with an LF
// (as <pre>{{.X}} must keep) and ends with a space.
func fuzzDots() []map[string]any {
	dots := []map[string]any{{}}
	for _, a := range []bool{true, false} {
		for _, b := range []bool{true, false} {
			dots = append(dots, map[string]any{"Show": true, "A": a, "B": b, "X": "\nhi ", "Items": []any{"a"}})
		}
	}
	return dots
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
		"<p>{{ if and (eq .A 1) (eq .B 2) (not .C) }}x{{ end }}</p>",
		"<div>\n<p>{{.X | printf \"%s\" | len}}</p>\n</div>",
		"<div>\n  {{.X | printf \"%s\" | printf \"%s\" | len}}\n</div>",
		"<span class='chip'{{if or (eq .A 1) (eq .B 2) (eq .C 3)}} x{{end}}>y</span>",
		"<p>{{.X\n| len}}{{printf \"%s %s\"\n.X .X}}</p>",
		"<a href=\"{{ (.X) }}\">{{ ( .M ).k }}</a>",
		"{{- with $v := (.) -}}<p>{{- $v | printf \"%v\" | len -}}</p>{{- end -}}",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		formatted1, err := Source("fuzz.gohtml", input)
		if formatterBug(err) {
			t.Fatalf("Source(input) error: %v\ninput: %q", err, input)
		}
		if err != nil {
			return
		}
		formatted2, err := Source("fuzz.gohtml", formatted1)
		if err != nil {
			t.Fatalf("re-parse of formatted output failed:\n  input:     %q\n  formatted: %q\n  error:     %v", input, formatted1, err)
		}
		checkActionShapes(t, input, formatted1)
		if formatted1 != formatted2 {
			t.Errorf("format not idempotent:\n  input:      %q\n  format(1):  %q\n  format(2):  %q", input, formatted1, formatted2)
		}
	})
}
