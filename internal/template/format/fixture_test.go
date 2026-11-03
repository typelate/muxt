package format

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	htmltemplate "html/template"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/template/parse"

	"golang.org/x/tools/txtar"

	analysis "github.com/typelate/muxt/internal/analysis/html"
	"github.com/typelate/muxt/internal/template"
	"github.com/typelate/muxt/internal/template/format/internal/rendering"
	"github.com/typelate/muxt/internal/template/lex"
)

var update = flag.Bool("update", false, "rewrite format.gohtml in testdata fixtures with the formatter's output")

// opaqueRegions returns the byte spans of src that the formatter must
// copy unchanged: the content of the subtrees classify marks opaque (raw
// text, RCDATA and preformatted elements, and elements whose white-space
// is preserved or unknown under sheets). Spans are in source order and do
// not overlap.
func opaqueRegions(src string, sheets []analysis.Stylesheet) [][2]int {
	tree, err := template.ParseTree("opaque", "opaque", src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	if err != nil {
		return nil
	}
	styles := analysis.ComputeWhitespace([]*template.Tree{tree}, analysis.WhitespaceOptions{
		Sheets:         sheets,
		AuthorCSSKnown: true,
	})
	var regions [][2]int
	for _, n := range classify(tree.Root, styles).opaque {
		el, ok := n.(*template.HTMLElementNode)
		if !ok {
			continue
		}
		start, end := int(el.AttrEnd), int(el.EndTagStart)
		if !el.Closed {
			// An unclosed element's End stops at its start tag; its
			// content runs to the end of its last child.
			end = start
			if len(el.Children) > 0 {
				end = int(el.Children[len(el.Children)-1].End())
			}
		}
		regions = append(regions, [2]int{start, end})
	}
	return regions
}

func TestOpaqueRegions(t *testing.T) {
	for _, tt := range []struct {
		src, css string
		want     []string
	}{
		{src: `<p>a</p>`},
		{src: `<pre>  a </pre>`, want: []string{"  a "}},
		{src: "<PRE>\nx</PRE><textarea>\n{{.X}}</textarea>", want: []string{"\nx", "\n{{.X}}"}},
		{src: `<pre><b> a </b><script>x</script></pre>`, want: []string{"<b> a </b><script>x</script>"}},
		{src: `<title>{{.T}}</title><style>p{}</style><script>1</script>`, want: []string{"{{.T}}", "p{}", "1"}},
		{src: `<iframe>i</iframe><noembed>e</noembed><noframes>f</noframes><noscript>s</noscript>`, want: []string{"i", "e", "f", "s"}},
		{src: `<listing>l</listing><xmp>x</xmp>`, want: []string{"l", "x"}},
		{src: `<plaintext>p`, want: []string{"p"}},
		{src: `<p class="k"> a </p><p> b </p>`, css: `.k{white-space:pre-wrap}`, want: []string{" a "}},
		{src: `<p style="white-space:pre"> a <span> b </span></p>`, want: []string{" a <span> b </span>"}},
		{src: `{{define "x"}}<pre> d </pre>{{end}}`, want: []string{" d "}},
	} {
		var sheets []analysis.Stylesheet
		if tt.css != "" {
			sheets = []analysis.Stylesheet{{Filename: "opaque.css", Text: tt.css}}
		}
		var got []string
		for _, r := range opaqueRegions(tt.src, sheets) {
			got = append(got, tt.src[r[0]:r[1]])
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("opaqueRegions(%q) = %q, want %q", tt.src, got, tt.want)
		}
	}
}

type fixture struct {
	archive *txtar.Archive
	input   string
	format  string
	data    []dataSet
	sheets  []analysis.Stylesheet
}

type dataSet struct {
	name  string
	value map[string]any
}

func readFixture(t *testing.T, filename string) fixture {
	t.Helper()
	ar, err := txtar.ParseFile(filename)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	fx := fixture{archive: ar, format: "\x00missing"}
	hasInput := false
	for _, f := range ar.Files {
		switch {
		case f.Name == "input.gohtml":
			fx.input, hasInput = string(f.Data), true
		case f.Name == "format.gohtml":
			fx.format = string(f.Data)
		case strings.HasPrefix(f.Name, "data/") && strings.HasSuffix(f.Name, ".json"):
			var v map[string]any
			if err := json.Unmarshal(f.Data, &v); err != nil {
				t.Fatalf("%s: %v", f.Name, err)
			}
			fx.data = append(fx.data, dataSet{name: strings.TrimSuffix(path.Base(f.Name), ".json"), value: v})
		case strings.HasPrefix(f.Name, "css/") && strings.HasSuffix(f.Name, ".css"):
			fx.sheets = append(fx.sheets, analysis.Stylesheet{Filename: f.Name, Text: string(f.Data)})
		default:
			t.Fatalf("unknown fixture section %q", f.Name)
		}
	}
	if !hasInput {
		t.Fatal("missing input.gohtml section")
	}
	if len(fx.data) == 0 {
		fx.data = []dataSet{{name: "{}", value: map[string]any{}}}
	}
	return fx
}

func fixtureFiles(t testing.TB) []string {
	t.Helper()
	var files []string
	for _, pattern := range []string{"testdata/*.txtar", "testdata/violations/*.txtar"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	if len(files) == 0 {
		t.Fatal("no fixtures found")
	}
	return files
}

func fixtureName(filename string) string {
	rel := strings.TrimPrefix(filepath.ToSlash(filename), "testdata/")
	return strings.TrimSuffix(rel, ".txtar")
}

func TestFixtures(t *testing.T) {
	skipped, pending := 0, 0
	defer func() {
		t.Logf("synthetic callers skipped: %d; call-site callers pending Task 7: %d", skipped, pending)
	}()
	for _, filename := range fixtureFiles(t) {
		name := fixtureName(filename)
		t.Run(name, func(t *testing.T) {
			fx := readFixture(t, filename)
			if hasBranch(fx.input) && len(fx.data) < 2 {
				t.Errorf("%s: has {{if}}, {{with}} or {{range}} but %d data set(s); add data/truthy.json and data/empty.json so both arms render", name, len(fx.data))
			}

			// (a) golden
			styles := Styles(analysis.WhitespaceOptions{Sheets: fx.sheets, AuthorCSSKnown: true})
			got, err := Source("input.gohtml", fx.input, styles)
			if err != nil {
				t.Fatalf("%s: Source(input) error: %v", name, err)
			}
			if *update {
				writeGolden(t, filename, fx.archive, got)
			} else if got != fx.format {
				t.Errorf("%s: Source(input) does not match format.gohtml\ngot:\n%s\nwant:\n%s", name, got, fx.format)
			}

			// (b) idempotency
			again, err := Source("format.gohtml", got, styles)
			if err != nil {
				t.Errorf("%s: Source(output) error: %v\noutput:\n%s", name, err, got)
			} else if again != got {
				t.Errorf("%s: not idempotent\nfirst:\n%s\nsecond:\n%s", name, got, again)
			}

			// (c) opaque regions byte for byte, in order
			checkOpaqueRegions(t, name, fx.input, got, fx.sheets)
			checkDirectiveRegions(t, name, fx.input, got)

			// (d) and (e) rendering equivalence
			oracle, err := renderingProblems(fx, got)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			for _, p := range oracle.problems {
				t.Errorf("%s: %s", name, p)
			}
			logOracleSkips(t, oracle)
			skipped += len(oracle.skipped)
			pending += len(oracle.pending)
		})
	}
}

// TestWPTFormatting formats each vendored web-platform-tests white-space
// file (see testdata/wpt/README.md) with no stylesheet files and author
// CSS known, and checks that the output is idempotent, keeps the opaque
// regions byte for byte, and renders like the original.
func TestWPTFormatting(t *testing.T) {
	files, err := filepath.Glob("testdata/wpt/*.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no WPT files found")
	}
	styles := Styles(analysis.WhitespaceOptions{AuthorCSSKnown: true})
	for _, filename := range files {
		name := filepath.Base(filename)
		if name == "white-space-empty-text-sibling.html" {
			continue // scripted; not a static page
		}
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			src := string(b)
			got, err := Source(name, src, styles)
			if err != nil {
				t.Fatalf("Source(%s) error: %v", name, err)
			}
			again, err := Source(name, got, styles)
			if err != nil {
				t.Fatalf("Source(output) error: %v\noutput:\n%s", err, got)
			}
			if again != got {
				t.Errorf("%s: not idempotent\nfirst:\n%s\nsecond:\n%s", name, got, again)
			}
			checkOpaqueRegions(t, name, src, got, nil)
			want, err := render(src, "", map[string]any{})
			if err != nil {
				t.Fatalf("render input: %v", err)
			}
			rendered, err := render(got, "", map[string]any{})
			if err != nil {
				t.Fatalf("render output: %v\noutput:\n%s", err, got)
			}
			diff, err := rendering.Diff(want, rendered, rendering.Body, nil)
			if err != nil {
				t.Fatal(err)
			}
			if diff != "" {
				t.Errorf("%s: rendering changed: %s\noutput:\n%s", name, diff, got)
			}
			if name == "white-space-normal-011.html" {
				const open = `<div id="reference">`
				start := strings.Index(src, open)
				if start < 0 {
					t.Fatalf("no #reference div in %s", name)
				}
				end := strings.Index(src[start:], "</div>")
				if end < 0 {
					t.Fatalf("#reference div in %s is not closed", name)
				}
				reference := src[start : start+end+len("</div>")]
				if !strings.Contains(got, reference) {
					t.Errorf("%s: #reference content changed; want %q in output:\n%s", name, reference, got)
				}
			}
		})
	}
}

// hasBranch reports whether src contains an {{if}}, {{with}} or {{range}},
// whose arms render only under data that selects them.
func hasBranch(src string) bool {
	tree, err := template.ParseTree("input.gohtml", "input.gohtml", src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	if err != nil {
		return false
	}
	found := false
	template.Walk(tree, func(_ *template.Tree, n template.Node) {
		switch n.(type) {
		case *template.IfNode, *template.WithNode, *template.RangeNode:
			found = true
		}
	})
	return found
}

func writeGolden(t *testing.T, filename string, ar *txtar.Archive, got string) {
	t.Helper()
	i := slices.IndexFunc(ar.Files, func(f txtar.File) bool { return f.Name == "format.gohtml" })
	if i < 0 {
		ar.Files = slices.Insert(ar.Files, 1, txtar.File{Name: "format.gohtml"})
		i = 1
	}
	ar.Files[i].Data = []byte(got)
	if err := os.WriteFile(filename, txtar.Format(ar), 0o644); err != nil {
		t.Fatal(err)
	}
}

// logOracleSkips logs the callers the oracle skipped and the call-site
// failures pending Task 7.
func logOracleSkips(t *testing.T, oracle oracleResult) {
	t.Helper()
	for _, s := range oracle.skipped {
		t.Logf("skipped %s", s)
	}
	for _, p := range oracle.pending {
		t.Logf("pending Task 7: %s", p)
	}
}

func checkOpaqueRegions(t *testing.T, name, input, output string, sheets []analysis.Stylesheet) {
	t.Helper()
	cursor := 0
	for _, r := range opaqueRegions(input, sheets) {
		region := input[r[0]:r[1]]
		i := strings.Index(output[cursor:], region)
		if i < 0 {
			t.Errorf("%s: opaque region at input bytes %d:%d not found in output after byte %d: %q", name, r[0], r[1], cursor, region)
			continue
		}
		cursor += i + len(region)
	}
}

// defineCallSiteModes reports whether the formatter keeps a define's
// bytes when it sees a preserving call site (plan Task 7). Until it does,
// call-site caller failures are reported as pending, not as problems.
const defineCallSiteModes = false

// oracleResult is what renderingProblems found.
type oracleResult struct {
	problems []string // rendering changed, or the output no longer executes
	pending  []string // call-site caller failures while !defineCallSiteModes
	skipped  []string // synthetic callers the input does not execute in
}

// renderingProblems runs checks (d) and (e): for each data set, input and
// output render equivalently at the top level and inside each synthetic
// caller of each {{define}} (rendering.Callers). A call-site caller is
// appended to the input as a {{define}} before formatting, so the
// formatter sees the call site; the others render output, formatted
// without them. A caller the input does not execute in (an html/template
// context error, say) or that the oracle cannot read is skipped. An
// error means the fixture itself is broken (its top level does not run).
func renderingProblems(fx fixture, output string) (oracleResult, error) {
	callers, err := defineCallers(fx.input)
	if err != nil {
		return oracleResult{}, err
	}
	styles := Styles(analysis.WhitespaceOptions{Sheets: fx.sheets, AuthorCSSKnown: true})
	var r oracleResult
	for _, ds := range fx.data {
		for _, c := range callers {
			label := fmt.Sprintf("data %s, caller %s", ds.name, c.Name)
			input, out := fx.input, output
			if c.CallSite {
				input = fx.input + oracleCallerDefine(c)
				out, err = Source("input.gohtml", input, styles)
				if err != nil {
					r.problems = append(r.problems, fmt.Sprintf("%s: Source(input with caller) error: %v", label, err))
					continue
				}
			}
			want, err := render(input, c.Source, ds.value)
			if err != nil {
				if c.Source == "" {
					return oracleResult{}, fmt.Errorf("%s: input: %w", label, err)
				}
				r.skipped = append(r.skipped, fmt.Sprintf("%s: input: %v", label, err))
				continue
			}
			if c.Source != "" {
				if _, err := rendering.Tokens(want, c.Context, fx.sheets); err != nil {
					r.skipped = append(r.skipped, fmt.Sprintf("%s: oracle cannot read the input's page: %v", label, err))
					continue
				}
			}
			var problem string
			got, err := render(out, c.Source, ds.value)
			if err != nil {
				problem = fmt.Sprintf("%s: output: %v", label, err)
			} else {
				diff, err := rendering.Diff(want, got, c.Context, fx.sheets)
				if err != nil {
					return oracleResult{}, fmt.Errorf("%s: %w", label, err)
				}
				if diff != "" {
					problem = fmt.Sprintf("%s: rendering changed: %s\ninput rendered:\n%s\noutput rendered:\n%s", label, diff, want, got)
				}
			}
			switch {
			case problem == "":
			case c.CallSite && !defineCallSiteModes:
				r.pending = append(r.pending, problem)
			default:
				r.problems = append(r.problems, problem)
			}
		}
	}
	return r, nil
}

// oracleCallerDefine wraps a call-site caller in a {{define}} the
// formatter sees beside the input.
func oracleCallerDefine(c rendering.Caller) string {
	return `{{define "muxt-oracle-caller"}}` + c.Source + `{{end}}`
}

// defineCallers lists the top-level render (an empty Source) plus the
// synthetic callers of each {{define}} in src.
func defineCallers(src string) ([]rendering.Caller, error) {
	tree, err := template.ParseTree("input.gohtml", "input.gohtml", src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	callers := []rendering.Caller{{Name: "top", Context: rendering.Body}}
	var defines []string
	firstElement := make(map[string]string)
	template.WalkPath(tree, func(_ *template.Tree, path []template.Node, n template.Node) {
		if d, ok := n.(*template.DefineNode); ok {
			defines = append(defines, d.Name)
			return
		}
		el, ok := n.(*template.HTMLElementNode)
		if !ok {
			return
		}
		for _, p := range path {
			if d, ok := p.(*template.DefineNode); ok {
				if _, seen := firstElement[d.Name]; !seen {
					firstElement[d.Name] = strings.ToLower(el.TagName)
				}
				return
			}
		}
	})
	for _, name := range defines {
		callers = append(callers, rendering.Callers(name, firstElement[name])...)
	}
	return callers, nil
}

// render executes src with stdlib html/template, the runtime muxt uses.
// When callerSrc is empty it renders the top-level template (an empty
// top level, such as a file of only defines, renders as ""); otherwise
// it renders callerSrc with src's templates in scope.
func render(src, callerSrc string, data map[string]any) (string, error) {
	tmpl, err := htmltemplate.New("input.gohtml").Parse(src)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	if callerSrc != "" {
		tmpl, err = tmpl.New("caller").Parse(callerSrc)
		if err != nil {
			return "", fmt.Errorf("parse caller: %w", err)
		}
	} else if tmpl.Tree == nil || parse.IsEmptyTree(tmpl.Tree.Root) {
		return "", nil
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute: %w", err)
	}
	return buf.String(), nil
}

// directiveRegions returns the byte spans of src that muxt:fmt off and
// preserve ask the formatter to copy unchanged, in source order, without
// those inside another.
func directiveRegions(src string) [][2]int {
	tree, err := template.ParseTree("directives", "directives", src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	if err != nil {
		return nil
	}
	d, err := scanDirectives("directives", src, tree.Root, "{{", "}}")
	if err != nil {
		return nil
	}
	var regions [][2]int
	for c, r := range d.regions {
		regions = append(regions, [2]int{d.commentEnd(c), r.end})
	}
	slices.SortFunc(regions, func(a, b [2]int) int { return a[0] - b[0] })
	var outer [][2]int
	for _, r := range regions {
		if len(outer) == 0 || r[0] >= outer[len(outer)-1][1] {
			outer = append(outer, r)
		}
	}
	return outer
}

func checkDirectiveRegions(t *testing.T, name, input, output string) {
	t.Helper()
	cursor := 0
	for _, r := range directiveRegions(input) {
		region := input[r[0]:r[1]]
		i := strings.Index(output[cursor:], region)
		if i < 0 {
			t.Errorf("%s: directive region at input bytes %d:%d not found in output after byte %d: %q", name, r[0], r[1], cursor, region)
			continue
		}
		cursor += i + len(region)
	}
}

func TestRenderingProblemsCallers(t *testing.T) {
	fx := fixture{
		input: "{{define \"m\"}}\n\thi\n{{end}}\n",
		data:  []dataSet{{name: "{}", value: map[string]any{}}},
	}
	got, err := Source("input.gohtml", fx.input)
	if err != nil {
		t.Fatal(err)
	}
	r, err := renderingProblems(fx, got)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.problems) != 0 || len(r.skipped) != 0 {
		t.Errorf("renderingProblems(%q) problems %q, skipped %q, want none", fx.input, r.problems, r.skipped)
	}
	// Formatted without its call site, the body loses its tab, which
	// pre-wrap, the attribute and the textarea all keep.
	if wantPending := 3; !defineCallSiteModes && len(r.pending) != wantPending {
		t.Errorf("renderingProblems(%q) has %d pending, want %d: %q", fx.input, len(r.pending), wantPending, r.pending)
	}

	// A joined line changes the inline caller's rendering (SB -> empty).
	r, err = renderingProblems(fx, "{{define \"m\"}}hi{{end}}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(r.problems, func(p string) bool { return strings.Contains(p, "caller inline:") }) {
		t.Errorf("renderingProblems with a joined body: problems %q, want one for the inline caller", r.problems)
	}

	// A body that ends inside an attribute value cannot be called from
	// one: html/template rejects that caller, and only that caller.
	fx.input = `{{define "a"}}<a href="{{.}}">x</a>{{end}}`
	r, err = renderingProblems(fx, fx.input)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.skipped) != 1 || !strings.Contains(r.skipped[0], "caller attribute:") {
		t.Errorf("renderingProblems(%q) skipped %q, want the attribute caller only", fx.input, r.skipped)
	}
}

// whatwgRow is one regression snippet from the WHATWG rulings
// (docs/superpowers/specs/panel/whatwg-rulings.md §4) with the exact
// output the rulings and the style rules (spec §3) allow.
type whatwgRow struct {
	snippet int    // the snippet's number in §4
	input   string // the file's whole source
	want    string // Source's exact output
	css     string // a stylesheet file, if any
	data    map[string]any
	task    string // for whatwgPending: the plan task the row waits for
}

// whatwgRegressions must pass.
var whatwgRegressions = []whatwgRow{
	// 1: the kind never changes (a segment break between East Asian
	// characters is removed; a space is kept).
	{snippet: 1, input: "<p>日本\n語</p>\n", want: "<p>日本\n\t語</p>\n"},
	{snippet: 1, input: "<p>日本 語</p>\n", want: "<p>日本 語</p>\n"},
	// 2: pre-line keeps the LF count; b may be reindented.
	{snippet: 2, input: "<div style=\"white-space:pre-line\">a\n\n\t\tb</div>\n", want: "<div style=\"white-space:pre-line\">a\n\n\tb</div>\n"},
	// 3: no LF is inserted after <pre>, and its leading LF stays.
	{snippet: 3, input: "<pre>{{.X}}</pre>\n", want: "<pre>{{.X}}</pre>\n", data: map[string]any{"X": "\nhi"}},
	{snippet: 3, input: "<pre>\n\nx</pre>\n", want: "<pre>\n\nx</pre>\n"},
	// 4: adjacent inline elements are never split.
	{snippet: 4, input: "<button><span>Un</span><span>read</span></button>\n", want: "<button><span>Un</span><span>read</span></button>\n"},
	// 5: a comment between text is never split off, and one on its own
	// line is reindented but never joined.
	{snippet: 5, input: "a<!--x-->b\n", want: "a<!--x-->b\n"},
	{snippet: 5, input: "<div>\na\n\t\t<!--x-->\nb\n</div>\n", want: "<div>\n\ta\n\t<!--x-->\n\tb\n</div>\n"},
	// 6: with body pre-wrap, a call that writes text in <head> moves the
	// following run into <body>, which preserves it.
	{snippet: 6, css: "body{white-space:pre-wrap}", input: "{{define \"h\"}}t{{end}}<html><head>{{template \"h\"}}\n\t\t<link></head></html>\n", want: "{{define \"h\"}}t{{end}}<html><head>{{template \"h\"}}\n\t\t<link></head></html>\n"},
	// 9: no LF is added before {{end}} of a define used inline.
	{snippet: 9, input: "{{define \"n\"}}<b>x</b>{{end}}\n{{template \"n\" .}},\n", want: "{{define \"n\"}}<b>x</b>{{end}}\n{{template \"n\" .}},\n"},
	// 10: no final LF is added.
	{snippet: 10, input: "<b>{{.}}</b>", want: "<b>{{.}}</b>"},
	// 11: the run before {{.B -}} keeps its kind (trailing tab stripped,
	// LF kept); the trimmed run after it is reindented.
	{snippet: 11, input: "<div>\n{{.A}}\t\n{{.B -}}\n\tx\n</div>\n", want: "<div>\n\t{{.A}}\n\t{{.B -}}\n\tx\n</div>\n", data: map[string]any{"A": "a", "B": "b"}},
	// 12: attribute separators survive; continuation lines are reindented.
	{snippet: 12, input: "<a href=x\nclass=y>z</a>\n", want: "<a href=x\n\tclass=y>z</a>\n"},
	{snippet: 12, input: "<a href=x />\n", want: "<a href=x />\n"},
	// 13: SVG text content and custom elements are left verbatim, the
	// latter whole: opener line, body and closer (spec §3 item 4).
	{snippet: 13, input: "<svg><text>a\nb</text></svg>\n", want: "<svg><text>a\nb</text></svg>\n"},
	{snippet: 13, input: "<my-chip>a\n\tb</my-chip>\n", want: "<my-chip>a\n\tb</my-chip>\n"},
	{snippet: 13, input: "<div>\n<my-chip>a\n\t\t\tb</my-chip>\n</div>\n", want: "<div>\n<my-chip>a\n\t\t\tb</my-chip>\n</div>\n"},
	// 14: an empty element never gains an LF (:empty).
	{snippet: 14, input: "<div class=x></div>\n", want: "<div class=x></div>\n"},
	{snippet: 14, input: "<div>\n<div class=x></div>\n</div>\n", want: "<div>\n\t<div class=x></div>\n</div>\n"},
	// 15: CR is a segment break (the parser turns it into LF), so the
	// line after it is reindented and the CR kept; FF is content.
	{snippet: 15, input: "<p>a\rb</p>\n", want: "<p>a\r\tb</p>\n"},
	{snippet: 15, input: "<p>a\fb</p>\n", want: "<p>a\fb</p>\n"},
}

// whatwgPending wait for a later plan task; each row's task says which.
// Move a row to whatwgRegressions when TestWHATWGRegressions reports
// that it passes.
var whatwgPending = []whatwgRow{
	// 7: a define called from a pre-wrap element keeps its bytes.
	{snippet: 7, task: "Task 7", css: ".bubble{white-space:pre-wrap}", input: "{{define \"m\"}}\n\thi\n{{end}}\n<div class=bubble>{{template \"m\"}}</div>\n", want: "{{define \"m\"}}\n\thi\n{{end}}\n<div class=bubble>{{template \"m\"}}</div>\n"},
	// 8: a define called from an attribute value keeps its bytes.
	{snippet: 8, task: "Task 7", input: "{{define \"t\"}}a\n\t\tb\n{{end}}\n<p title=\"{{template \"t\"}}\"></p>\n", want: "{{define \"t\"}}a\n\t\tb\n{{end}}\n<p title=\"{{template \"t\"}}\"></p>\n"},
}

// TestWHATWGRegressions checks each snippet's exact output, idempotency
// and rendering under the oracle (with its synthetic define callers).
func TestWHATWGRegressions(t *testing.T) {
	for i, row := range whatwgRegressions {
		t.Run(fmt.Sprintf("%d/%d", row.snippet, i), func(t *testing.T) {
			for _, f := range whatwgFailures(row) {
				t.Errorf("snippet %d %q: %s", row.snippet, row.input, f)
			}
		})
	}
	for i, row := range whatwgPending {
		t.Run(fmt.Sprintf("pending/%d/%d", row.snippet, i), func(t *testing.T) {
			failures := whatwgFailures(row)
			if len(failures) == 0 {
				t.Skipf("pending %s, but snippet %d %q passes now: move it to whatwgRegressions", row.task, row.snippet, row.input)
			}
			t.Skipf("pending %s: snippet %d %q:\n%s", row.task, row.snippet, row.input, strings.Join(failures, "\n"))
		})
	}
}

func whatwgFailures(row whatwgRow) []string {
	fx := fixture{input: row.input, data: []dataSet{{name: "row", value: row.data}}}
	if row.data == nil {
		fx.data[0].value = map[string]any{}
	}
	if row.css != "" {
		fx.sheets = []analysis.Stylesheet{{Filename: "css/row.css", Text: row.css}}
	}
	styles := Styles(analysis.WhitespaceOptions{Sheets: fx.sheets, AuthorCSSKnown: true})
	got, err := Source("input.gohtml", row.input, styles)
	if err != nil {
		return []string{fmt.Sprintf("Source error: %v", err)}
	}
	var failures []string
	if got != row.want {
		failures = append(failures, fmt.Sprintf("Source = %q, want %q", got, row.want))
	}
	if again, err := Source("format.gohtml", got, styles); err != nil || again != got {
		failures = append(failures, fmt.Sprintf("not idempotent: Source(%q) = %q, %v", got, again, err))
	}
	oracle, err := renderingProblems(fx, got)
	if err != nil {
		return append(failures, err.Error())
	}
	failures = append(failures, oracle.problems...)
	for _, p := range oracle.pending {
		failures = append(failures, "Task 7: "+p)
	}
	return failures
}
