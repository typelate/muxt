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
	"strconv"
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

// knownViolations maps a fixture (its path under testdata without .txtar)
// to the spec §4 violation that makes its rendering check fail today.
// TestFixtures skips only the rendering-equivalence checks (d) and (e) for
// these; golden, idempotency and byte-exact checks still run. Each task
// that fixes a violation removes its entries; Task 20 deletes the map.
var knownViolations = map[string]string{}

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

			// (d) and (e) rendering equivalence
			problems, err := renderingProblems(fx, got)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if id, known := knownViolations[name]; known {
				if len(problems) == 0 {
					t.Errorf("%s: listed in knownViolations as %s but renders equivalently; remove the entry", name, id)
				}
				for _, p := range problems {
					t.Logf("known violation %s: %s", id, p)
				}
				return
			}
			for _, p := range problems {
				t.Errorf("%s: %s", name, p)
			}
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

func TestKnownViolationsNameFixtures(t *testing.T) {
	names := make(map[string]bool)
	for _, filename := range fixtureFiles(t) {
		names[fixtureName(filename)] = true
	}
	for name, id := range knownViolations {
		if !names[name] {
			t.Errorf("knownViolations[%q] = %q names no fixture under testdata", name, id)
		}
	}
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

// renderingProblems runs checks (d) and (e): for each data set, input and
// output render equivalently at the top level and inside each caller of
// each {{define}}. It returns one message per failing data set and caller.
// An error means the fixture itself is broken (its input does not run).
func renderingProblems(fx fixture, output string) ([]string, error) {
	callers, err := defineCallers(fx.input)
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, ds := range fx.data {
		for _, c := range callers {
			want, err := render(fx.input, c.source, ds.value)
			if err != nil {
				return nil, fmt.Errorf("data %s, caller %s: input: %w", ds.name, c.name, err)
			}
			got, err := render(output, c.source, ds.value)
			if err != nil {
				problems = append(problems, fmt.Sprintf("data %s, caller %s: output: %v", ds.name, c.name, err))
				continue
			}
			diff, err := rendering.Diff(want, got, c.context, fx.sheets)
			if err != nil {
				return nil, fmt.Errorf("data %s, caller %s: %w", ds.name, c.name, err)
			}
			if diff != "" {
				problems = append(problems, fmt.Sprintf("data %s, caller %s: rendering changed: %s\ninput rendered:\n%s\noutput rendered:\n%s", ds.name, c.name, diff, want, got))
			}
		}
	}
	return problems, nil
}

type caller struct {
	name    string // "top" or the caller source
	source  string // "" renders the file's top-level template
	context rendering.Context
}

// defineCallers lists the top-level render plus, for each {{define}} in
// src, the wrappers of spec §8.2: block, inline, and table when the
// body's first element is a tr.
func defineCallers(src string) ([]caller, error) {
	tree, err := template.ParseTree("input.gohtml", "input.gohtml", src, nil, lex.Options{
		EmitComment: true,
		ParseHTML:   true,
		BreakOK:     true,
		ContinueOK:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("parse input: %w", err)
	}
	callers := []caller{{name: "top", context: rendering.Body}}
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
		call := `{{template ` + strconv.Quote(name) + ` .}}`
		callers = append(callers,
			caller{name: "<div>" + call + "</div>", source: "<div>" + call + "</div>", context: rendering.Body},
			caller{name: "<p>a" + call + "b</p>", source: "<p>a" + call + "b</p>", context: rendering.Body},
		)
		if firstElement[name] == "tr" {
			s := "<table><tbody>" + call + "</tbody></table>"
			callers = append(callers, caller{name: s, source: s, context: rendering.TableBody})
		}
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
