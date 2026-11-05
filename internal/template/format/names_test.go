package format

import (
	"bytes"
	htmltemplate "html/template"
	"slices"
	"strconv"
	"strings"
	"testing"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template/format/internal/rendering"
	"github.com/typelate/muxt/template"
	"github.com/typelate/muxt/template/lex"
)

// TestDefineNames pins how the formatter writes the name literals of
// {{define}}, {{block}} and {{template}}: a name with the
// shape of a muxt route definition is written METHOD SP pattern
// [SP status] [SP call], the call printed as gofmt prints it, in the
// literal's own quotes; any other name is as written.
func TestDefineNames(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		want string // "" means unchanged
		keep func(string) bool
		// context is another template of the package (Styles), whose
		// defines a call may resolve to.
		context string
	}{
		{name: "method pattern and call", src: `{{define "GET  /x/{id}   M( ctx , id )"}}<p>a</p>{{end}}`, want: `{{define "GET /x/{id} M(ctx, id)"}}<p>a</p>{{end}}`},
		{name: "call arguments", src: `{{define "POST /x M(ctx,form)"}}<p>a</p>{{end}}`, want: `{{define "POST /x M(ctx, form)"}}<p>a</p>{{end}}`},
		{name: "backticks kept", src: "{{define `GET  /x M( ctx)` }}<p>a</p>{{end}}", want: "{{define `GET /x M(ctx)`}}<p>a</p>{{end}}"},
		{name: "canonical backticks unchanged", src: "{{define `GET /x M(ctx)`}}<p>a</p>{{end}}"},
		{name: "no call", src: `{{define "GET   /x"}}<p>a</p>{{end}}`, want: `{{define "GET /x"}}<p>a</p>{{end}}`},
		{name: "no method", src: `{{define "/x   M(ctx)"}}<p>a</p>{{end}}`, want: `{{define "/x M(ctx)"}}<p>a</p>{{end}}`},
		{name: "host", src: `{{define "GET  example.com/x"}}<p>a</p>{{end}}`, want: `{{define "GET example.com/x"}}<p>a</p>{{end}}`},
		{name: "status code", src: `{{define "POST  /x  201   M(ctx)"}}<p>a</p>{{end}}`, want: `{{define "POST /x 201 M(ctx)"}}<p>a</p>{{end}}`},
		{name: "status constant", src: `{{define "POST /x  http.StatusCreated"}}<p>a</p>{{end}}`, want: `{{define "POST /x http.StatusCreated"}}<p>a</p>{{end}}`},
		{name: "partial name", src: `{{define "partial"}}<p>a</p>{{end}}`},
		{name: "spaced partial name", src: `{{define "with  spaces"}}<p>a</p>{{end}}`},
		{name: "host with spaces", src: `{{define " GET /x"}}<p>a</p>{{end}}`},
		{name: "lowercase method", src: `{{define "get  /x"}}<p>a</p>{{end}}`},
		{name: "template call with its define", src: `{{define "GET  /x  M(ctx)"}}<p>a</p>{{end}}{{template "GET  /x  M(ctx)" .}}`, want: `{{define "GET /x M(ctx)"}}<p>a</p>{{end}}{{template "GET /x M(ctx)" .}}`},
		// A call no define of the file or package spells exactly is
		// unresolved (an execution error, or a template registered
		// from Go): its canonical spelling may name another template.
		{name: "unresolved call", src: `<div>{{template "GET  /x  M( ctx )"}}</div>`},
		{name: "unresolved call beside the define of its canonical form", src: `{{define "T / Show(ctx,id)"}}{{end}}{{template "T / Show(ctx,id )"}}`, want: `{{define "T / Show(ctx, id)"}}{{end}}{{template "T / Show(ctx,id )"}}`},
		{name: "call resolved in the context", src: `<div>{{template "GET  /x  M(ctx)" .}}</div>`, want: `<div>{{template "GET /x M(ctx)" .}}</div>`, context: `{{define "GET  /x  M(ctx)"}}<p>a</p>{{end}}`},
		{name: "block", src: `{{block "GET  /x M( ctx )" .}}<p>b</p>{{end}}`, want: `{{block "GET /x M(ctx)" .}}<p>b</p>{{end}}`},
		{name: "call that does not parse", src: `{{define "GET  /x M(ctx,"}}<p>a</p>{{end}}`},
		{name: "handler that is not a call", src: `{{define "GET  /x M"}}<p>a</p>{{end}}`},
		{name: "string argument", src: `{{define "GET  /x M(ctx, \"a,  b\")"}}<p>a</p>{{end}}`, want: `{{define "GET /x M(ctx, \"a,  b\")"}}<p>a</p>{{end}}`},
		{name: "raw string argument kept", src: "{{define \"GET  /x M(ctx, `a  b`)\"}}<p>a</p>{{end}}", want: "{{define \"GET /x M(ctx, `a  b`)\"}}<p>a</p>{{end}}"},
		{name: "nested call", src: `{{define "GET /x M(ctx, F( id ,form))"}}<p>a</p>{{end}}`, want: `{{define "GET /x M(ctx, F(id, form))"}}<p>a</p>{{end}}`},
		{name: "escapes kept when canonical", src: `{{define "GET /x M(ctx, \x22a\x22)"}}<p>a</p>{{end}}`},
		{name: "comment in the call", src: `{{define "GET  /x M(ctx /* c */)"}}<p>a</p>{{end}}`},
		{name: "line break in the name", src: "{{define `GET\n/x M(ctx)`}}<p>a</p>{{end}}"},
		{name: "function literal", src: `{{define "GET  /x M(func() {  return })"}}<p>a</p>{{end}}`, want: `{{define "GET /x M(func() { return })"}}<p>a</p>{{end}}`},
		{name: "two defines that would collide", src: `{{define "GET  /x"}}<p>a</p>{{end}}{{define "GET /x"}}<p>b</p>{{end}}`},
		{name: "a reference left as written pins the name", src: "{{define \"GET  /x\"}}<p>a</p>{{end}}\n<div>\n{{/* muxt:fmt off */}}\n{{template \"GET  /x\" .}}\n</div>\n", want: "{{define \"GET  /x\"}}<p>a</p>{{end}}\n<div>\n\t{{/* muxt:fmt off */}}\n{{template \"GET  /x\" .}}\n</div>\n"},
		{name: "a kept name", src: `{{define "GET  /x"}}<p>a</p>{{end}}{{define "GET  /y"}}<p>b</p>{{end}}`, want: `{{define "GET  /x"}}<p>a</p>{{end}}{{define "GET /y"}}<p>b</p>{{end}}`, keep: func(name string) bool { return name == "GET /x" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == "" {
				want = tt.src
			}
			var opts []Option
			if tt.keep != nil {
				opts = append(opts, KeepNames(tt.keep))
			}
			if tt.context != "" {
				opts = append(opts, Styles(analysis.WhitespaceOptions{Context: parseTrees(t, []string{tt.context})}))
			}
			got, err := Source("t.gohtml", tt.src, opts...)
			if err != nil {
				t.Fatalf("Source(%q) error: %v", tt.src, err)
			}
			if got != want {
				t.Errorf("Source(%q)\n got: %q\nwant: %q", tt.src, got, want)
			}
			again, err := Source("t.gohtml", got, opts...)
			if err != nil || again != got {
				t.Errorf("not idempotent: Source(%q) = %q, %v", got, again, err)
			}
			for _, dot := range actionDots() {
				if p := fuzzRenderingProblem(tt.src, got, "", rendering.Body, dot); p != "" {
					t.Error(p)
				}
			}
		})
	}
}

func TestCanonicalName(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{in: "GET  /x/{id}   M( ctx , id )", want: "GET /x/{id} M(ctx, id)"},
		{in: "GET /x M(ctx)", want: "GET /x M(ctx)"},
		{in: "partial", want: "partial"},
		{in: "GET  /x M(ctx,", want: "GET  /x M(ctx,"},
		{in: "GET /x  ", want: "GET /x"},
		{in: "GET  /x M(func() { a(); b() })", want: "GET /x M(func() { a(); b() })"},
	} {
		if got := CanonicalName(tt.in); got != tt.want {
			t.Errorf("CanonicalName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestKeptNames pins the consistency rule: a name keeps its spellings
// when a template outside the run spells it otherwise than canonically,
// or when two distinct define or block names of the package share its
// canonical form.
func TestKeptNames(t *testing.T) {
	for _, tt := range []struct {
		name         string
		run, outside []string
		want         []string
	}{
		{name: "nothing outside", run: []string{`{{define "GET  /x"}}a{{end}}`}},
		{name: "outside spelled the old way", run: []string{`{{define "GET  /x"}}a{{end}}`}, outside: []string{`{{template "GET  /x" .}}`}, want: []string{"GET /x"}},
		{name: "outside already canonical", run: []string{`{{define "GET  /x"}}a{{end}}`}, outside: []string{`{{template "GET /x" .}}`}},
		{name: "outside a third spelling", run: []string{`{{define "GET  /x"}}a{{end}}`}, outside: []string{`{{template "GET   /x" .}}`}, want: []string{"GET /x"}},
		{name: "outside partials", run: []string{`{{define "GET  /x"}}a{{end}}`}, outside: []string{`{{template "with  spaces" .}}`}},
		{name: "two defines across files", run: []string{`{{define "GET  /x"}}a{{end}}`, `{{define "GET /x"}}b{{end}}`}, want: []string{"GET /x"}},
		{name: "define and block across run and outside", run: []string{`{{define "GET  /x"}}a{{end}}`}, outside: []string{`{{block "GET /x" .}}b{{end}}`}, want: []string{"GET /x"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := KeptNames(parseTrees(t, tt.run), parseTrees(t, tt.outside))
			var names []string
			for name := range got {
				names = append(names, name)
			}
			slices.Sort(names)
			if !slices.Equal(names, tt.want) {
				t.Errorf("KeptNames = %q, want %q", names, tt.want)
			}
		})
	}
}

// TestDefineNamesAcrossFiles is the oracle of the consistency rule: a
// two-file package renders the same once each file is formatted with
// the names KeptNames keeps, whether both files are in the run or only
// the one that defines the route.
func TestDefineNamesAcrossFiles(t *testing.T) {
	const (
		defines = `{{define "GET  /x/{id}  M( ctx , id )"}}<p>{{.}}</p>{{end}}` + "\n"
		calls   = `<div>{{template "GET  /x/{id}  M( ctx , id )" .}}</div>` + "\n"
	)
	for _, tt := range []struct {
		name                   string
		outside                string // the call file's text outside the run; "" when it is in the run
		wantDefines, wantCalls string
	}{
		{
			name:        "both files in the run",
			wantDefines: `{{define "GET /x/{id} M(ctx, id)"}}<p>{{.}}</p>{{end}}` + "\n",
			wantCalls:   `<div>{{template "GET /x/{id} M(ctx, id)" .}}</div>` + "\n",
		},
		{
			name:        "the call outside the run, spelled the old way",
			outside:     calls,
			wantDefines: defines,
			wantCalls:   calls,
		},
		{
			name:        "the call outside the run, already canonical",
			outside:     `<div>{{template "GET /x/{id} M(ctx, id)" .}}</div>` + "\n",
			wantDefines: `{{define "GET /x/{id} M(ctx, id)"}}<p>{{.}}</p>{{end}}` + "\n",
			wantCalls:   `<div>{{template "GET /x/{id} M(ctx, id)" .}}</div>` + "\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			run := []string{defines}
			var outside []string
			callsIn := calls
			if tt.outside != "" {
				outside = append(outside, tt.outside)
				callsIn = tt.outside
			} else {
				run = append(run, calls)
			}
			kept := KeptNames(parseTrees(t, run), parseTrees(t, outside))
			keep := KeepNames(func(name string) bool { return kept[name] })
			gotDefines, err := Source("defines.gohtml", defines, keep)
			if err != nil {
				t.Fatalf("Source(defines) error: %v", err)
			}
			gotCalls := callsIn
			if tt.outside == "" {
				// As muxt fmt does, the package's other templates are
				// the context, where the call resolves.
				context := Styles(analysis.WhitespaceOptions{Context: parseTrees(t, []string{defines})})
				if gotCalls, err = Source("calls.gohtml", calls, keep, context); err != nil {
					t.Fatalf("Source(calls) error: %v", err)
				}
			}
			if gotDefines != tt.wantDefines || gotCalls != tt.wantCalls {
				t.Errorf("formatted\n got: %q %q\nwant: %q %q", gotDefines, gotCalls, tt.wantDefines, tt.wantCalls)
			}
			got, err := executePair(gotDefines, gotCalls)
			if err != nil {
				t.Fatalf("executing the formatted pair: %v", err)
			}
			// A call already canonical outside the run named no template
			// before: formatting the define completes the rename.
			if want, err := executePair(defines, callsIn); err == nil && got != want {
				t.Errorf("rendering changed: got %q, want %q", got, want)
			}
		})
	}
}

// executePair parses the two files into one template set, as a package
// embeds them, and executes the second.
func executePair(defines, calls string) (string, error) {
	ts, err := htmltemplate.New("calls").Parse(calls)
	if err != nil {
		return "", err
	}
	if _, err := ts.New("defines").Parse(defines); err != nil {
		return "", err
	}
	var b bytes.Buffer
	err = ts.ExecuteTemplate(&b, "calls", "v")
	return b.String(), err
}

func parseTrees(t *testing.T, sources []string) []*template.Tree {
	t.Helper()
	var trees []*template.Tree
	for i, src := range sources {
		name := "t" + strconv.Itoa(i) + ".gohtml"
		tree, err := template.ParseTree(name, name, src, nil, lex.Options{EmitComment: true, ParseHTML: true, BreakOK: true, ContinueOK: true})
		if err != nil {
			t.Fatalf("ParseTree(%q): %v", src, err)
		}
		trees = append(trees, tree)
	}
	return trees
}

// renamedCaller returns the synthetic caller callerSrc (rendering.Callers)
// calling, in place of a name output no longer defines, the canonical
// form output defines instead: a caller outside the file names the
// template as the file does after formatting, as muxt fmt's consistency
// rule makes every reference in the package do.
func renamedCaller(callerSrc, output string) string {
	if callerSrc == "" {
		return callerSrc
	}
	defined := make(map[string]bool)
	tree, err := template.ParseTree("output.gohtml", "output.gohtml", output, nil, lex.Options{EmitComment: true, ParseHTML: true, BreakOK: true, ContinueOK: true})
	if err != nil {
		return callerSrc
	}
	template.Walk(tree, func(_ *template.Tree, n template.Node) {
		switch n := n.(type) {
		case *template.DefineNode:
			defined[n.Name] = true
		case *template.BlockNode:
			defined[n.Name] = true
		}
	})
	caller, err := template.ParseTree("caller.gohtml", "caller.gohtml", callerSrc, nil, lex.Options{EmitComment: true, ParseHTML: true, BreakOK: true, ContinueOK: true})
	if err != nil {
		return callerSrc
	}
	template.Walk(caller, func(_ *template.Tree, n template.Node) {
		if call, ok := n.(*template.TemplateNode); ok {
			if c := CanonicalName(call.Name); !defined[call.Name] && defined[c] {
				callerSrc = strings.ReplaceAll(callerSrc, strconv.Quote(call.Name), strconv.Quote(c))
			}
		}
	})
	return callerSrc
}
