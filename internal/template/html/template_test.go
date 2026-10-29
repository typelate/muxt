package html

import (
	"bytes"
	stdtemplate "html/template"
	"os"
	"path/filepath"
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
		t.Fatalf("stdlib output for %q = %q, expected it to differ from htt's normalized output (fixture is stale)", src, want)
	}
	if got := executeShim(t, src, nil); got != wantShim {
		t.Errorf("shim output for %q = %q, want %q", src, got, wantShim)
	}
}
