package format

import (
	"slices"
	"testing"

	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/format/internal/rendering"
	"github.com/typelate/muxt/internal/template/lex"
)

// TestActionFormat pins how the formatter prints action interiors (spec
// §3.10): each action-bearing token is printed from its parse tree with
// canonical spacing, literals and names as written, trim markers and
// delimiters kept, redundant parentheses dropped, and complex or
// author-broken pipelines laid out one stage or argument per line. Each
// row is checked for idempotence, for parse-tree equality between input
// and output (actionShapes), and with the rendering oracle.
func TestActionFormat(t *testing.T) {
	for _, tt := range []struct {
		name        string
		src         string
		want        string // "" means unchanged
		left, right string
	}{
		{name: "spaces inside delimiters", src: "<p>{{ .X }}</p>", want: "<p>{{.X}}</p>"},
		{name: "runs of spaces", src: "{{  if   eq .A   true  }}<p>a</p>{{end}}", want: "{{if eq .A true}}<p>a</p>{{end}}"},
		{name: "trim markers kept", src: "{{- if .A -}}<p>a</p>{{- end -}}"},
		{name: "trim marker spacing", src: "{{-\t if .A\t-}}<p>a</p>{{-  end  -}}", want: "{{- if .A -}}<p>a</p>{{- end -}}"},
		{name: "range declarations", src: "{{range $i,$v:=.L}}<p>{{$i}}{{$v}}</p>{{end}}", want: "{{range $i, $v := .L}}<p>{{$i}}{{$v}}</p>{{end}}"},
		{name: "assignment", src: "{{$x:=1}}{{$x  =  2}}<p>{{$x}}</p>", want: "{{$x := 1}}{{$x = 2}}<p>{{$x}}</p>"},
		{name: "two stages", src: `<p>{{.X|printf "%s"}}</p>`, want: `<p>{{.X | printf "%s"}}</p>`},
		{name: "two stages unchanged", src: `<p>{{.A | printf "%v"}}</p>`},
		{name: "redundant parens around a field", src: "{{if (.X)}}<p>x</p>{{end}}", want: "{{if .X}}<p>x</p>{{end}}"},
		{name: "redundant parens nested", src: "{{if ((.X))}}<p>x</p>{{end}}", want: "{{if .X}}<p>x</p>{{end}}"},
		{name: "redundant parens around dot, variable and constants", src: "{{with $v := (.)}}{{($v.X)}}{{(1)}}{{(\"s\")}}{{(true)}}<p></p>{{end}}", want: "{{with $v := .}}{{$v.X}}{{1}}{{\"s\"}}{{true}}<p></p>{{end}}"},
		{name: "a call in parens is kept", src: "<p>{{eq (len .X) 4}}</p>"},
		{name: "parens of an argument are kept", src: `<p>{{printf "%s" (.X)}}</p>`},
		// Parentheses kept because they change how text/template runs:
		// a bare constant argument takes the parameter's type, (1) is an
		// int; (nil) as an argument is "nil is not a command", bare nil a
		// nil value; (.B) evaluates through evalPipeline, which unwraps an
		// any-typed nil that bare .B passes as is; and in a later stage
		// parentheses refuse the piped value a bare field takes.
		{name: "parens around a constant argument are kept", src: `<p>{{eq .A (1)}}</p>`},
		{name: "parens around nil in an argument are kept", src: `<p>{{printf "%v" (nil)}}</p>`},
		{name: "parens around an argument of a later stage are kept", src: `<p>{{.A | printf "%v" (.B)}}</p>`},
		{name: "parens of a later stage are kept", src: "<p>{{.X | (len)}}</p>"},
		{name: "parens with a declaration are kept", src: "<p>{{($y := .X)}}</p>"},
		{name: "parens before a field chain are kept", src: `<p>{{(index .M "k").Name}}</p>`},
		{name: "parens before a field chain lose their spaces", src: "<p>{{( .M ).k}}</p>", want: "<p>{{(.M).k}}</p>"},
		{name: "template call", src: "{{define \"a\"}}<p>a</p>{{end}}{{template \"a\"  .}}", want: "{{define \"a\"}}<p>a</p>{{end}}{{template \"a\" .}}"},
		{name: "template call without pipeline", src: "{{define \"a\"}}<p>a</p>{{end}}{{ template   \"a\" }}", want: "{{define \"a\"}}<p>a</p>{{end}}{{template \"a\"}}"},
		{name: "block raw name kept", src: "{{block   `a b`   . }}<p>b</p>{{end}}", want: "{{block `a b` .}}<p>b</p>{{end}}"},
		{name: "define unchanged", src: "{{define \"d\"}}<p>d</p>{{end}}"},
		{name: "define spacing", src: "{{ define  \"d\" }}<p>d</p>{{ end }}", want: "{{define \"d\"}}<p>d</p>{{end}}"},
		{name: "end", src: "{{if .A}}<p>a</p>{{end }}", want: "{{if .A}}<p>a</p>{{end}}"},
		{name: "else if", src: "{{if .A}}<p>a</p>{{else   if .B}}<p>b</p>{{  else  }}<p>c</p>{{end}}", want: "{{if .A}}<p>a</p>{{else if .B}}<p>b</p>{{else}}<p>c</p>{{end}}"},
		{name: "else with", src: "{{with .A}}<p>a</p>{{else   with .B}}<p>b</p>{{end}}", want: "{{with .A}}<p>a</p>{{else with .B}}<p>b</p>{{end}}"},
		{name: "break and continue", src: "{{range .L}}{{if .}}{{ break }}{{end}}{{ continue }}{{end}}<p></p>", want: "{{range .L}}{{if .}}{{break}}{{end}}{{continue}}{{end}}<p></p>"},
		{name: "string literal as written", src: `<p>{{"a  b"}}</p>`},
		{name: "raw string literal as written", src: "<p>{{ `a  \"b` }}</p>", want: "<p>{{`a  \"b`}}</p>"},
		{name: "number text kept", src: "<p>{{ 0x1F }}</p>", want: "<p>{{0x1F}}</p>"},
		{name: "char constant", src: "<p>{{ 'a' }}</p>", want: "<p>{{'a'}}</p>"},
		{name: "nil", src: `<p>{{ printf "%v"  nil }}</p>`, want: `<p>{{printf "%v" nil}}</p>`},
		{name: "comment untouched", src: "<p>{{/*  c  */}}{{- /* d */ -}}</p>"},
		{name: "custom delimiters", src: "<p>[[ .X ]]</p>", want: "<p>[[.X]]</p>", left: "[[", right: "]]"},
		{name: "custom delimiters with trim", src: "<p>[[-  .X  -]]</p>", want: "<p>[[- .X -]]</p>", left: "[[", right: "]]"},
		{name: "in an attribute value", src: "<a href=\"{{ .X }}\" {{ if .A }}id=\"a\"{{ end }}>x</a>", want: "<a href=\"{{.X}}\" {{if .A}}id=\"a\"{{end}}>x</a>"},
		{name: "in pre", src: "<pre>  {{ .X }}\n</pre>", want: "<pre>  {{.X}}\n</pre>"},
		{name: "in raw text", src: "<script>var x = {{ .X }};</script><title>{{ .X }}</title>", want: "<script>var x = {{.X}};</script><title>{{.X}}</title>"},
		{name: "in a file without elements", src: "{{define \"t\"}}Hello {{ .X }}{{end}}", want: "{{define \"t\"}}Hello {{.X}}{{end}}"},
		{
			name: "in a muxt:fmt off region untouched",
			src:  "<div>\n{{/* muxt:fmt off */}}\n<p>{{ .X }}</p>\n{{/* muxt:fmt on */}}\n<p>{{ .X }}</p>\n</div>",
			want: "<div>\n\t{{/* muxt:fmt off */}}\n<p>{{ .X }}</p>\n{{/* muxt:fmt on */}}\n\t<p>{{.X}}</p>\n</div>",
		},
		{
			name: "in a muxt:fmt preserve node untouched",
			src:  "<div>\n{{/* muxt:fmt preserve */}}\n<p>{{ .X }}</p>\n<p>{{ .X }}</p>\n</div>",
			want: "<div>\n\t{{/* muxt:fmt preserve */}}\n<p>{{ .X }}</p>\n\t<p>{{.X}}</p>\n</div>",
		},
		{
			name: "a line break between the name and the pipeline collapses",
			src:  "<div>\n    {{template \"a\"\n\t.}}\n  <p>x</p>\n</div>{{define \"a\"}}a{{end}}",
			want: "<div>\n\t{{template \"a\" .}}\n\t<p>x</p>\n</div>{{define \"a\"}}a{{end}}",
		},
		{
			name: "a line break after the keyword collapses",
			src:  "<div>\n  {{if\n  .A}}\n<p>x</p>\n{{end}}\n</div>",
			want: "<div>\n\t{{if .A}}\n\t\t<p>x</p>\n\t{{end}}\n</div>",
		},
		{
			name: "a multi-line end collapses",
			src:  "<div>\n{{if .A}}\n<p>x</p>\n  {{end\n}}\n</div>",
			want: "<div>\n\t{{if .A}}\n\t\t<p>x</p>\n\t{{end}}\n</div>",
		},
		{
			name: "a raw string literal spanning lines leaves the action as written",
			src:  "<div>\n   {{printf `a\nb`  .X}}\n<p>x</p>\n</div>",
			want: "<div>\n   {{printf `a\nb`  .X}}\n\t<p>x</p>\n</div>",
		},
		// Layout across lines. Automatic splitting applies only to an
		// action that starts its line; author breaks are canonicalized
		// wherever the action is.
		{
			name: "three parenthesized arguments split one per line",
			src:  "{{if and (eq .A true) (eq .B true) (not .C)}}<p>x</p>{{end}}",
			want: "{{if and\n\t(eq .A true)\n\t(eq .B true)\n\t(not .C)\n}}<p>x</p>{{end}}",
		},
		{
			name: "three stages stay on one line",
			src:  "<div>\n{{.X | printf \"%s\" | len}}\n</div>",
			want: "<div>\n\t{{.X | printf \"%s\" | len}}\n</div>",
		},
		{
			name: "four stages split one per line",
			src:  "<div>\n{{.X|printf \"%s\"|printf \"%s\"|len}}\n</div>",
			want: "<div>\n\t{{.X\n\t\t| printf \"%s\"\n\t\t| printf \"%s\"\n\t\t| len\n\t}}\n</div>",
		},
		{
			name: "four stages in the middle of a line stay on one line",
			src:  "<p>{{.X|printf \"%s\"|printf \"%s\"|len}}</p>",
			want: "<p>{{.X | printf \"%s\" | printf \"%s\" | len}}</p>",
		},
		{
			name: "three parenthesized arguments inside a start tag stay on one line",
			src:  "<span class='chip'{{if or (eq .A true) (eq .B true) (eq .C true)}} x{{end}}>y</span>",
		},
		{
			name: "the same action on its own line splits",
			src:  "<div>\n{{if or (eq .A true) (eq .B true) (eq .C true)}}\nx\n{{end}}\n</div>",
			want: "<div>\n\t{{if or\n\t\t(eq .A true)\n\t\t(eq .B true)\n\t\t(eq .C true)\n\t}}\n\t\tx\n\t{{end}}\n</div>",
		},
		{
			name: "an author break before a stage splits in the middle of a line",
			src:  "<p>{{.X\n| len}}</p>",
			want: "<p>{{.X\n\t| len\n}}</p>",
		},
		{
			name: "an author break after a pipe splits",
			src:  "<p>{{.X |\n  len}}</p>",
			want: "<p>{{.X\n\t| len\n}}</p>",
		},
		{
			name: "an author break between arguments splits one per line",
			src:  "<p>{{printf \"%s %s\"\n.X .X}}</p>",
			want: "<p>{{printf\n\t\"%s %s\"\n\t.X\n\t.X\n}}</p>",
		},
		{
			name: "an author break inside a start tag splits",
			src:  "<span class='chip'{{if or (eq .A true)\n(eq .B true)}} x{{end}}>y</span>",
			want: "<span class='chip'{{if or\n\t(eq .A true)\n\t(eq .B true)\n}} x{{end}}>y</span>",
		},
		{
			name: "an author break inside parentheses before the pipeline collapses",
			src:  "<p>{{printf \"%s\" (\n.X)}}</p>",
			want: "<p>{{printf \"%s\" (.X)}}</p>",
		},
		{
			name: "a qualifying nested pipeline recurses",
			src:  `{{if and (eq .A true) (eq (.X | printf "%s" | printf "%s" | len) 4) (not .B)}}<p>x</p>{{end}}`,
			want: "{{if and\n\t(eq .A true)\n\t(eq (.X\n\t\t| printf \"%s\"\n\t\t| printf \"%s\"\n\t\t| len\n\t) 4)\n\t(not .B)\n}}<p>x</p>{{end}}",
		},
		{
			name: "a qualifying nested pipeline in an unsplit command",
			src:  "<div>\n{{printf \"%d\" (.X | printf \"%s\" | printf \"%s\" | len)}}\n</div>",
			want: "<div>\n\t{{printf \"%d\" (.X\n\t\t| printf \"%s\"\n\t\t| printf \"%s\"\n\t\t| len\n\t)}}\n</div>",
		},
		{
			name: "split stages and split arguments",
			src:  "<div>\n{{.X | printf \"%s\" | printf \"%s\" | and (eq .A true) (eq .B true) (not .C)}}\n</div>",
			want: "<div>\n\t{{.X\n\t\t| printf \"%s\"\n\t\t| printf \"%s\"\n\t\t| and\n\t\t\t(eq .A true)\n\t\t\t(eq .B true)\n\t\t\t(not .C)\n\t}}\n</div>",
		},
		{
			name: "continuation lines follow the line's depth",
			src:  "<div>\n<section>\n{{.X | printf \"%s\" | printf \"%s\" | len}}\n</section>\n</div>",
			want: "<div>\n\t<section>\n\t\t{{.X\n\t\t\t| printf \"%s\"\n\t\t\t| printf \"%s\"\n\t\t\t| len\n\t\t}}\n\t</section>\n</div>",
		},
		{
			name: "a split action starting a line",
			src:  "<div>\n    {{if and (eq .A true) (eq .B true) (not .C)}}\n<p>x</p>\n{{end}}\n</div>",
			want: "<div>\n\t{{if and\n\t\t(eq .A true)\n\t\t(eq .B true)\n\t\t(not .C)\n\t}}\n\t\t<p>x</p>\n\t{{end}}\n</div>",
		},
		{
			name: "a split action keeps its trim markers",
			src:  "<p>\n{{- .X | printf \"%s\" | printf \"%s\" | len -}}\n</p>",
			want: "<p>\n\t{{- .X\n\t\t| printf \"%s\"\n\t\t| printf \"%s\"\n\t\t| len\n\t-}}\n</p>",
		},
		{
			name: "a split range keeps its declarations on the first line",
			src:  "{{range $i, $v := .L | slice | slice | len}}{{end}}<p></p>",
			want: "{{range $i, $v := .L\n\t| slice\n\t| slice\n\t| len\n}}{{end}}<p></p>",
		},
		{
			name: "a template call with a qualifying pipeline",
			src:  "{{define \"a\"}}a{{end}}\n{{template \"a\" .X | printf \"%s\" | printf \"%s\" | len}}<p></p>",
			want: "{{define \"a\"}}a{{end}}\n{{template \"a\" .X\n\t| printf \"%s\"\n\t| printf \"%s\"\n\t| len\n}}<p></p>",
		},
		{
			name: "an else if on its own line with three parenthesized arguments",
			src:  "{{if .A}}\n<p>a</p>\n{{else if or (eq .B true) (not .C) (not .A)}}\n<p>b</p>\n{{end}}",
			want: "{{if .A}}\n\t<p>a</p>\n{{else if or\n\t(eq .B true)\n\t(not .C)\n\t(not .A)\n}}\n\t<p>b</p>\n{{end}}",
		},
		{
			name: "an else if in the middle of a line stays on one line",
			src:  "{{if .A}}<p>a</p>{{else if or (eq .B true) (not .C) (not .A)}}<p>b</p>{{end}}",
		},
		{
			name: "in an attribute value stays on one line",
			src:  `<a href="{{.X | printf "%s" | printf "%s" | urlquery}}">x</a>`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			if tt.left != "" {
				opts = append(opts, Delimiters(tt.left, tt.right))
			}
			want := tt.want
			if want == "" {
				want = tt.src
			}
			got, err := Source("t.gohtml", tt.src, opts...)
			if err != nil {
				t.Fatalf("Source(%q) error: %v", tt.src, err)
			}
			if got != want {
				t.Errorf("Source(%q)\n got %q\nwant %q", tt.src, got, want)
			}
			again, err := Source("t.gohtml", got, opts...)
			if err != nil {
				t.Fatalf("Source(output) error: %v", err)
			}
			if again != got {
				t.Errorf("not idempotent: Source(%q) = %q", got, again)
			}
			left, right := tt.left, tt.right
			if left == "" {
				left, right = "{{", "}}"
			}
			in, out := parseShapes(t, tt.src, left, right), parseShapes(t, got, left, right)
			if !slices.Equal(in, out) {
				t.Errorf("parse trees differ\n input: %q\noutput: %q", in, out)
			}
			if tt.left != "" {
				return // the oracle renders with the default delimiters
			}
			for _, dot := range actionDots() {
				if p := fuzzRenderingProblem(tt.src, got, "", rendering.Body, dot); p != "" {
					t.Error(p)
				}
			}
		})
	}
}

func parseShapes(t *testing.T, src, left, right string) []string {
	t.Helper()
	tree, err := template.ParseTree("t.gohtml", "t.gohtml", src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
		LeftDelim:   left,
		RightDelim:  right,
	})
	if err != nil {
		t.Fatalf("ParseTree(%q): %v", src, err)
	}
	return actionShapes(tree.Root, CanonicalName)
}

func actionDots() []map[string]any {
	return append(fuzzDots(), map[string]any{
		"A": true, "B": false, "C": false, "X": "\nhi ",
		"L": []any{"a", "b"},
		"M": map[string]any{"k": map[string]any{"Name": "n"}},
	})
}
