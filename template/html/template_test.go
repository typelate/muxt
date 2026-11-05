package html

import (
	"bytes"
	stdtemplate "html/template"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// executeStdlib parses and executes src with the standard library's
// html/template engine, for stdlib-parity comparisons.
func executeStdlib(t *testing.T, src string, data any) string {
	t.Helper()
	tmpl := stdtemplate.Must(stdtemplate.New("test").Parse(src))
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// executeShim parses and executes src with this package's html/template
// shim, for stdlib-parity comparisons.
func executeShim(t *testing.T, src string, data any) string {
	t.Helper()
	tmpl := Must(New("test").Parse(src))
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func TestNewAndParse(t *testing.T) {
	tmpl, err := New("test").Parse(`<p>Hello, {{.}}!</p>`)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.Name() != "test" {
		t.Errorf("Name() = %q, want %q", tmpl.Name(), "test")
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, "World"); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "Hello, World!") {
		t.Errorf("Execute = %q, want to contain %q", got, "Hello, World!")
	}
}

func TestExecuteTemplate(t *testing.T) {
	tmpl, err := New("main").Parse(`<div>{{template "greeting" .}}</div>{{define "greeting"}}<span>Hi {{.}}</span>{{end}}`)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "greeting", "Alice"); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "Hi Alice") {
		t.Errorf("ExecuteTemplate = %q, want to contain %q", got, "Hi Alice")
	}
}

func TestFuncs(t *testing.T) {
	tmpl, err := New("test").Funcs(FuncMap{
		"greet": func(name string) string {
			return "Hello, " + name + "!"
		},
	}).Parse(`<p>{{greet .}}</p>`)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, "World"); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "Hello, World!") {
		t.Errorf("Funcs = %q, want to contain %q", got, "Hello, World!")
	}
}

func TestDelims(t *testing.T) {
	tmpl, err := New("test").Delims("[[", "]]").Parse(`<p>[[.]]</p>`)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, "custom"); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "custom") {
		t.Errorf("Delims = %q, want to contain %q", got, "custom")
	}
}

func TestLookup(t *testing.T) {
	tmpl, err := New("main").Parse(`{{define "sub"}}<em>sub</em>{{end}}<p>main</p>`)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.Lookup("sub") == nil {
		t.Error("Lookup(\"sub\") = nil, want non-nil")
	}
	if tmpl.Lookup("nonexistent") != nil {
		t.Error("Lookup(\"nonexistent\") = non-nil, want nil")
	}
}

func TestTemplates(t *testing.T) {
	tmpl, err := New("main").Parse(`{{define "a"}}<a/>{{end}}{{define "b"}}<b/>{{end}}<main/>`)
	if err != nil {
		t.Fatal(err)
	}
	templates := tmpl.Templates()
	if len(templates) < 3 {
		t.Errorf("Templates() returned %d templates, want at least 3", len(templates))
	}
}

func TestDefinedTemplates(t *testing.T) {
	tmpl, err := New("main").Parse(`{{define "a"}}<div/>{{end}}{{define "b"}}<div/>{{end}}<main/>`)
	if err != nil {
		t.Fatal(err)
	}
	got := tmpl.DefinedTemplates()
	if !strings.Contains(got, `"a"`) || !strings.Contains(got, `"b"`) || !strings.Contains(got, `"main"`) {
		t.Errorf("DefinedTemplates() = %q, want to contain \"a\", \"b\", \"main\"", got)
	}
}

func TestClone(t *testing.T) {
	tmpl, err := New("main").Parse(`<p>{{.}}</p>`)
	if err != nil {
		t.Fatal(err)
	}
	clone, err := tmpl.Clone()
	if err != nil {
		t.Fatal(err)
	}
	// Parsing into clone should not affect original.
	_, err = clone.New("extra").Parse(`<span>extra</span>`)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.Lookup("extra") != nil {
		t.Error("Clone: parsing into clone affected original")
	}
	if clone.Lookup("extra") == nil {
		t.Error("Clone: parsed template not found in clone")
	}
	// Clone should still execute the original.
	var buf bytes.Buffer
	if err := clone.Execute(&buf, "cloned"); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "cloned") {
		t.Errorf("Clone Execute = %q, want to contain %q", got, "cloned")
	}
}

func TestParseFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.gohtml")
	if err := os.WriteFile(file, []byte(`<p>{{.}}</p>`), 0o644); err != nil {
		t.Fatal(err)
	}
	tmpl, err := ParseFiles(file)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, "file"); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "file") {
		t.Errorf("ParseFiles Execute = %q, want to contain %q", got, "file")
	}
}

func TestParseGlob(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.gohtml", "b.gohtml"} {
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, []byte(`<p>`+name+`</p>`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tmpl, err := ParseGlob(filepath.Join(dir, "*.gohtml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tmpl.Templates()) < 2 {
		t.Errorf("ParseGlob: got %d templates, want at least 2", len(tmpl.Templates()))
	}
}

func TestParseFS(t *testing.T) {
	fsys := fstest.MapFS{
		"a.gohtml": &fstest.MapFile{Data: []byte(`<p>a</p>`)},
		"b.gohtml": &fstest.MapFile{Data: []byte(`<p>b</p>`)},
	}
	tmpl, err := ParseFS(fsys, "*.gohtml")
	if err != nil {
		t.Fatal(err)
	}
	if len(tmpl.Templates()) < 2 {
		t.Errorf("ParseFS: got %d templates, want at least 2", len(tmpl.Templates()))
	}
}

func TestOption(t *testing.T) {
	tmpl := New("test").Option("missingkey=error")
	if tmpl.common.missingKey != 2 { // MissingKeyError
		t.Errorf("Option missingkey=error: got %d, want 2", tmpl.common.missingKey)
	}
}

func TestMust(t *testing.T) {
	tmpl := Must(New("test").Parse(`<p>ok</p>`))
	if tmpl == nil {
		t.Fatal("Must returned nil")
	}
	defer func() {
		if r := recover(); r == nil {
			t.Error("Must did not panic on error")
		}
	}()
	Must(New("bad").Parse(`<p>{{.`))
}

func TestExecuteTemplateNotFound(t *testing.T) {
	tmpl, err := New("main").Parse(`<p>main</p>`)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err = tmpl.ExecuteTemplate(&buf, "nonexistent", nil)
	if err == nil {
		t.Error("ExecuteTemplate with missing template: want error, got nil")
	}
}

func TestIsTrue(t *testing.T) {
	truth, ok := IsTrue(42)
	if !truth || !ok {
		t.Errorf("IsTrue(42) = %v, %v; want true, true", truth, ok)
	}
	truth, ok = IsTrue(0)
	if truth || !ok {
		t.Errorf("IsTrue(0) = %v, %v; want false, true", truth, ok)
	}
	truth, ok = IsTrue("")
	if truth || !ok {
		t.Errorf("IsTrue(\"\") = %v, %v; want false, true", truth, ok)
	}
}

func TestHTMLEscapeString(t *testing.T) {
	got := HTMLEscapeString(`<script>alert("xss")</script>`)
	if strings.Contains(got, "<script>") {
		t.Errorf("HTMLEscapeString did not escape: %s", got)
	}
}

func TestJSEscapeString(t *testing.T) {
	got := JSEscapeString(`"hello" & 'world'`)
	if strings.Contains(got, `"hello"`) {
		t.Errorf("JSEscapeString did not escape: %s", got)
	}
}

func TestURLQueryEscaper(t *testing.T) {
	got := URLQueryEscaper("hello world&foo=bar")
	if strings.Contains(got, " ") || strings.Contains(got, "&") {
		t.Errorf("URLQueryEscaper did not escape: %s", got)
	}
}

func TestBlock(t *testing.T) {
	tmpl, err := New("main").Parse(`<div>{{block "content" .}}<p>default {{.}}</p>{{end}}</div>`)
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.Lookup("content") == nil {
		t.Error("Block did not register template \"content\"")
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, "world"); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "default world") {
		t.Errorf("Block execute = %q, want to contain %q", got, "default world")
	}
}

func TestMethodParseFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sub.gohtml")
	if err := os.WriteFile(file, []byte(`{{define "sub"}}<em>sub content</em>{{end}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tmpl, err := New("main").Parse(`<div>{{template "sub"}}</div>`)
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err = tmpl.ParseFiles(file)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "sub content") {
		t.Errorf("method ParseFiles = %q, want to contain %q", got, "sub content")
	}
}

func TestOrphanEndTagMatchesStdlib(t *testing.T) {
	for _, src := range []string{
		`orphan</b>end`,
		`<div>x</b></div>`,
	} {
		want := executeStdlib(t, src, nil)
		got := executeShim(t, src, nil)
		if got != want {
			t.Errorf("shim output for %q = %q, want stdlib's %q", src, got, want)
		}
	}
}

// TestMisnestedFormattingDiverges documents an intentional, permanent
// divergence from stdlib html/template for misnested formatting tags —
// not a bug to fix in a later task.
//
// Browsers restructure input like `<b><i>bold</b> tail</i>` using the
// HTML5 adoption agency algorithm (reopening <i> after the misplaced
// </b>). stdlib html/template does no tree construction at all — it is
// a token-stream escaper, so it passes the bytes through completely
// unchanged. htt, by contrast, builds a structural AST and renders that
// structure back out, so it normalizes the misnested markup into
// balanced tags (`</i></b>`) rather than reproducing the source
// verbatim. The trailing `</i>` is a genuine orphan (no ancestor <i> is
// open after the implicit close), so it round-trips verbatim per this
// task's fix — that's the one part of the output this task owns.
// htt's own misnested-tag diagnostic (plan Task 4, DiagMisnestedTag) is
// what alerts template authors to the divergence; matching stdlib's
// byte-for-byte passthrough for this case is out of scope and will
// never be pursued.
func TestMisnestedFormattingDiverges(t *testing.T) {
	const src = `<b><i>bold</b> tail</i>`
	const wantShim = `<b><i>bold</i></b> tail</i>`

	if want := executeStdlib(t, src, nil); want == wantShim {
		t.Fatalf("stdlib output for %q = %q, expected it to differ from the normalized output (fixture is stale)", src, want)
	}
	if got := executeShim(t, src, nil); got != wantShim {
		t.Errorf("shim output for %q = %q, want %q", src, got, wantShim)
	}
}

// TestTemplatesWithoutATree states html/template's treatment of a template
// declared with New and never parsed: Lookup and Templates still return
// it, with a nil Tree, and DefinedTemplates leaves it out.
func TestTemplatesWithoutATree(t *testing.T) {
	ts := Must(New("root").Parse(`{{define "a"}}A{{end}}`))
	ts.New("declared")

	declared := ts.Lookup("declared")
	if declared == nil {
		t.Fatal(`Lookup("declared") = nil, want the declared template`)
	}
	if declared.Tree != nil {
		t.Errorf(`Lookup("declared").Tree = %v, want nil`, declared.Tree)
	}
	var names []string
	for _, tmpl := range ts.Templates() {
		names = append(names, tmpl.Name())
	}
	slices.Sort(names)
	if want := []string{"a", "declared", "root"}; !slices.Equal(names, want) {
		t.Errorf("Templates() names = %v, want %v", names, want)
	}
	if got := ts.DefinedTemplates(); strings.Contains(got, "declared") {
		t.Errorf("DefinedTemplates() = %q, want no declared", got)
	}
	if err := ts.ExecuteTemplate(io.Discard, "declared", nil); err == nil {
		t.Error(`ExecuteTemplate("declared") = nil, want an incomplete or empty template error`)
	}
}

// TestParseRecordsTheNameAsGiven states html/template's ParseName for a
// template parsed directly: the name exactly as given, even when it is
// empty or holds a slash, and no file path.
func TestParseRecordsTheNameAsGiven(t *testing.T) {
	for _, name := range []string{"", "page", "GET /x F()"} {
		ts := Must(New(name).Parse(`{{define "inner"}}x{{end}}{{block "blk" .}}y{{end}}`))
		for _, tmpl := range ts.Templates() {
			if tmpl.Tree == nil {
				continue
			}
			if tmpl.Tree.ParseName != name || tmpl.Tree.Filepath != "" {
				t.Errorf("New(%q).Parse: %q has ParseName %q and Filepath %q, want %q and none", name, tmpl.Name(), tmpl.Tree.ParseName, tmpl.Tree.Filepath, name)
			}
		}
	}
}

// TestParseRedefinition states html/template's rules for a name defined
// twice: within one Parse, two definitions that are not empty are an
// error, and an empty one gives way to the other; across Parse calls, a
// later definition replaces an earlier one unless it is empty.
func TestParseRedefinition(t *testing.T) {
	for _, tt := range []struct {
		name, text, wantErr, want string
	}{
		{name: "a block and a define", text: `{{block "a" .}}x{{end}}{{define "a"}}y{{end}}`, wantErr: `template: t:1: template: multiple definition of template "a"`},
		{name: "two defines", text: `{{define "a"}}x{{end}}{{define "a"}}y{{end}}`, wantErr: `template: t:1: template: multiple definition of template "a"`},
		{name: "a later empty define", text: `{{define "a"}}x{{end}}{{define "a"}}{{end}}`, want: "x"},
		{name: "an earlier empty define", text: `{{define "a"}} {{end}}{{define "a"}}y{{end}}`, want: "y"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts, err := New("t").Parse(tt.text)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("Parse(%q) error = %v, want %q", tt.text, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.text, err)
			}
			if got := ts.Lookup("a").Tree.Root.String(); got != tt.want {
				t.Errorf(`Parse(%q): "a" = %q, want %q`, tt.text, got, tt.want)
			}
		})
	}
	t.Run("across Parse calls", func(t *testing.T) {
		ts := Must(New("t").Parse(`{{define "a"}}x{{end}}`))
		Must(ts.New("u").Parse(`{{define "a"}}{{end}}`))
		if got := ts.Lookup("a").Tree.Root.String(); got != "x" {
			t.Errorf(`after a later empty define, "a" = %q, want "x"`, got)
		}
		Must(ts.New("v").Parse(`{{define "a"}}z{{end}}`))
		if got := ts.Lookup("a").Tree.Root.String(); got != "z" {
			t.Errorf(`after a later define, "a" = %q, want "z"`, got)
		}
	})
}

// TestControlStructureAgainstANameMatchesStdlib states that an
// {{if}}, {{with}} or {{range}} written against a tag or attribute name,
// its body starting with a space, adds attributes, as in html/template;
// the tag is not read as text.
func TestControlStructureAgainstANameMatchesStdlib(t *testing.T) {
	for _, src := range []string{
		`<li{{if .}} data-a{{end}}>x</li>`,
		`<li{{if .}} data-a{{else}} data-b{{end}}{{if not .}} c{{end}}>x</li>`,
		`<input formnovalidate{{if .}} disabled{{end}} aria-label="x">`,
		`<input a{{with .}} b="{{.}}"{{end}}>`,
	} {
		for _, data := range []bool{true, false} {
			want := executeStdlib(t, src, data)
			if got := executeShim(t, src, data); got != want {
				t.Errorf("shim output for %q with %v = %q, want stdlib's %q", src, data, got, want)
			}
		}
	}
}

// TestActionAfterAClosingQuoteMatchesStdlib states that an action written
// right after an attribute's closing quote is in attribute position, not
// part of the attribute's value.
func TestActionAfterAClosingQuoteMatchesStdlib(t *testing.T) {
	for _, src := range []string{
		`<ul id="i-{{.}}"{{if .}} hidden{{end}} role="alert">x</ul>`,
		`<ul id="x"{{if not .}} hidden{{end}}>x</ul>`,
		`<ul id="i"{{with .}} title="{{.}}"{{end}}>x</ul>`,
	} {
		for _, data := range []string{"e", ""} {
			want := executeStdlib(t, src, data)
			if got := executeShim(t, src, data); got != want {
				t.Errorf("shim output for %q with %q = %q, want stdlib's %q", src, data, got, want)
			}
		}
	}
}

// TestTrimMarkersInElementsMatchStdlib states that a trim marker trims
// the text beside it wherever the text is, inside an element too.
func TestTrimMarkersInElementsMatchStdlib(t *testing.T) {
	for _, src := range []string{
		"x\n{{- .}}",
		"<p>a\n{{- .}}</p>",
		"<p>{{.}}\n {{- .}} \n</p>",
		"<p>{{. -}}\n x</p>",
		"<ol>\n{{- range .}}\n<li>{{.}}</li>\n{{- end}}\n</ol>",
		`{{define "x"}}<ul>` + "\n\t{{- range .}}\n\t<li>{{.}}</li>\n\t{{- end}}</ul>{{end}}{{template \"x\" .}}",
	} {
		data := []int{1, 2}
		want := executeStdlib(t, src, data)
		if got := executeShim(t, src, data); got != want {
			t.Errorf("shim output for %q = %q, want stdlib's %q", src, got, want)
		}
	}
}

// TestElementsLeftOpenMatchStdlib states that the engine writes no end
// tag the source does not: an element left open, as when one template
// opens <main> and another closes it, or closed by its parent's end tag.
func TestElementsLeftOpenMatchStdlib(t *testing.T) {
	for _, src := range []string{
		`<main>`,
		`{{define "head"}}<div class="page"><main>{{end}}{{define "foot"}}</main></div>{{end}}{{template "head"}}<h1>{{.}}</h1>{{template "foot"}}`,
		`<ul><li>a<li>{{.}}</ul>`,
		`<p>a<p>{{.}}`,
		`{{if .}}<div class="a">{{else}}<div class="b">{{end}}x</div>`,
	} {
		want := executeStdlib(t, src, "x")
		if got := executeShim(t, src, "x"); got != want {
			t.Errorf("shim output for %q = %q, want stdlib's %q", src, got, want)
		}
	}
}

// TestAttributeSyntaxMatchesStdlib states that the engine writes an
// attribute as its source does: in the source's quotes, or none, with an
// empty value kept, the source's own text as it is, and each action's
// output escaped as html/template escapes it there.
func TestAttributeSyntaxMatchesStdlib(t *testing.T) {
	for _, src := range []string{
		`<script src='/static/lib/htmx.js' defer></script>`,
		`<ul id='brokers'>x</ul>`,
		`<meta name="htmx-config" content='{"code":"204","swap":false}'>`,
		`<select hx-vals='{"@action": "refresh"}'></select>`,
		`<input value="">`,
		`<input value=''>`,
		`<input value>`,
		`<a title=x>x</a>`,
		`<a title="a&amp;b {{.}}">x</a>`,
		`<a title="{{.}}">x</a>`,
		`<a title='{{.}}'>x</a>`,
		`<a title={{.}}>x</a>`,
		`<a title='p {{.}} q'>x</a>`,
		`<a title="it's {{.}}">x</a>`,
		`<a title='{"a": 1} {{.}}'>x</a>`,
		`<a aria-label="{{if .}}Show{{else}}Hide{{end}} {{.}}">x</a>`,
		`<a aria-label='{{range $i := 3}}{{if $i}}, {{end}}{{$}}{{end}}'>x</a>`,
		`<a title="{{.}}" class='c' data-x=y hidden>x</a>`,
	} {
		for _, data := range []string{`a&b'c"d e<f>+g`, ""} {
			want := executeStdlib(t, src, data)
			if got := executeShim(t, src, data); got != want {
				t.Errorf("shim output for %q with %q = %q, want stdlib's %q", src, data, got, want)
			}
		}
	}
}

// TestElementsThatChangeTheTextContextAreClosed states that the engine
// writes the end tag of an element whose content a browser does not read
// as HTML text, even where the source leaves it out, so that output
// escaped as HTML text after it cannot land in a script, style or raw
// text context. html/template refuses the templates whose end context is
// script, style or RCDATA; for the other raw text elements it reads what
// follows as HTML text, so the engine's output differs from html/template's
// only by the end tag.
func TestElementsThatChangeTheTextContextAreClosed(t *testing.T) {
	const data = `alert(1)</x><b>'`
	for _, tt := range []struct {
		element      string
		stdlibRefuse bool
	}{
		{"script", true},
		{"style", true},
		{"textarea", true},
		{"title", true},
		{"xmp", false},
		{"iframe", false},
		{"noembed", false},
		{"noframes", false},
		{"noscript", false},
		{"plaintext", false},
	} {
		for _, src := range []string{
			`{{define "a"}}<` + tt.element + `>{{end}}{{template "a"}}{{.}}`,
			`{{define "a"}}<` + tt.element + `>{{end}}{{define "b"}}<p>{{.}}</p>{{end}}{{template "a"}}{{template "b" .}}`,
		} {
			var stdlib strings.Builder
			err := stdtemplate.Must(stdtemplate.New("t").Parse(src)).Execute(&stdlib, data)
			if tt.stdlibRefuse != (err != nil) {
				t.Errorf("stdlib Execute(%q) error = %v, want refused %v", src, err, tt.stdlibRefuse)
			}
			got := executeShim(t, src, data)
			end := "</" + tt.element + ">"
			escaped := HTMLEscapeString(data)
			if !strings.HasPrefix(got, "<"+tt.element+">"+end) || !strings.Contains(got, escaped) {
				t.Errorf("shim output for %q = %q, want the end tag %s, then the data escaped as HTML text %q", src, got, end, escaped)
			}
			if !tt.stdlibRefuse {
				if want := strings.Replace(stdlib.String(), "<"+tt.element+">", "<"+tt.element+">"+end, 1); got != want {
					t.Errorf("shim output for %q = %q, want stdlib's with the end tag, %q", src, got, want)
				}
			}
		}
	}
}

// TestActionsInAttributeControlStructuresAreEscaped states that output
// in a quoted attribute value is escaped for the value under if, with and
// range as it is outside them, whatever its type, and that a template
// called there cannot end the value: html/template escapes it or refuses
// the template.
func TestActionsInAttributeControlStructuresAreEscaped(t *testing.T) {
	const breakOut = `" onmouseover="alert(1)`
	type data struct {
		X, Y any
	}
	values := []any{
		breakOut,
		stdtemplate.HTML(breakOut),
		stdtemplate.HTMLAttr(breakOut),
		stdtemplate.URL(breakOut),
		HTML(breakOut),
		HTMLAttr(breakOut),
	}
	for _, src := range []string{
		`<a title="{{if .X}}{{.Y}}{{end}}">x</a>`,
		`<a title='{{if .X}}{{.Y}}{{end}}'>x</a>`,
		`<a title="{{with .Y}}{{.}}{{end}}">x</a>`,
		`<a title="{{range $i := 1}}{{$.Y}}{{end}}">x</a>`,
		`<a title="p {{if .X}}q {{.Y}} r{{else}}s{{end}} t">x</a>`,
		`<a title="{{if .X}}{{template "s" .Y}}{{end}}">x</a>{{define "s"}}{{.}}{{end}}`,
	} {
		for _, y := range values {
			d := data{X: true, Y: y}
			var stdlib strings.Builder
			stdErr := stdtemplate.Must(stdtemplate.New("test").Parse(src)).Execute(&stdlib, d)
			got := executeShim(t, src, d)
			if strings.Contains(got, `onmouseover="`) || strings.Contains(got, `onmouseover='`) {
				t.Errorf("shim output for %q with %T = %q: the value ends the attribute", src, y, got)
			}
			if stdErr == nil && isStdlibType(y) && got != stdlib.String() {
				t.Errorf("shim output for %q with %T = %q, want stdlib's %q", src, y, got, stdlib.String())
			}
		}
	}
	for _, src := range []string{
		`<a title="{{if .X}}{{template "s"}}{{end}}">x</a>{{define "s"}}" onmouseover="alert(1){{end}}`,
		`<a title="{{with .X}}{{template "s"}}{{end}}">x</a>{{define "s"}}<b title="y">{{end}}`,
	} {
		got := executeShim(t, src, data{X: true})
		if strings.Contains(got, `onmouseover="`) || strings.Contains(got, `title="y"`) {
			t.Errorf("shim output for %q = %q: the called template ends the attribute", src, got)
		}
	}
}

// isStdlibType reports whether v is a plain string or one of
// html/template's content types, whose output the shim must match.
func isStdlibType(v any) bool {
	switch v.(type) {
	case string, stdtemplate.HTML, stdtemplate.HTMLAttr, stdtemplate.URL:
		return true
	}
	return false
}

// TestFunctionsReturningAReflectValueMatchStdlib states that the value a
// function returns in a reflect.Value, as index and slice do, is the
// pipeline's value, so a field of it can be taken.
func TestFunctionsReturningAReflectValueMatchStdlib(t *testing.T) {
	type tag struct{ Name string }
	for _, src := range []string{
		`<title>{{(index . 0).Name}}</title>`,
		`<p>{{with .}}{{(index . 0).Name}}{{end}}</p>`,
		`<p>{{range slice . 1}}{{.Name}}{{end}}</p>`,
		`<p title="{{(index . 1).Name}}">{{index . 0}}</p>`,
	} {
		data := []tag{{"v1"}, {"v2"}}
		want := executeStdlib(t, src, data)
		if got := executeShim(t, src, data); got != want {
			t.Errorf("shim output for %q = %q, want stdlib's %q", src, got, want)
		}
	}
}

// TestTemplateCallInATagIsRefused states that the engine refuses a
// {{template}} call in a tag, whose attributes html/template would add,
// naming the template, rather than dropping it or failing obscurely.
func TestTemplateCallInATagIsRefused(t *testing.T) {
	for _, src := range []string{
		`{{define "attrs"}} hidden{{end}}<input {{template "attrs"}}>`,
		`{{define "attrs"}} hidden{{end}}<input value='x'{{template "attrs"}}>`,
		`{{define "attrs"}} hidden{{end}}<input {{if .}}{{template "attrs"}}{{end}}>`,
	} {
		want := executeStdlib(t, src, true)
		tmpl := Must(New("test").Parse(src))
		var sb strings.Builder
		err := tmpl.Execute(&sb, true)
		if err == nil || !strings.Contains(err.Error(), `{{template "attrs"}} in a tag is not supported`) {
			t.Errorf("Execute(%q) error = %v, want the call refused (html/template writes %q)", src, err, want)
		}
	}
}
