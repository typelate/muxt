package mutation

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/typelate/muxt/internal/asteval"
)

// TestRevisionChanged states which scopes a --diff run mutates: one whose
// template reads differently, or one reached with a type of dot the
// revision did not reach it with.
func TestRevisionChanged(t *testing.T) {
	scopeOf := func(t *testing.T, text string, dot types.Type) scope {
		t.Helper()
		trees, err := asteval.ParseTrees("t", text, "", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		return scope{template: "t", dataType: dot, treeLocation: treeLocation{tree: trees["t"]}}
	}
	str := types.Typ[types.String]
	before := revision{executionKey("t", str): scopeOf(t, `<b>{{.}}</b>`, str).tree.Root.String()}

	for _, tt := range []struct {
		name string
		text string
		dot  types.Type
		want bool
	}{
		{name: "the same text and dot", text: `<b>{{.}}</b>`, dot: str, want: false},
		{name: "reformatted inside an action", text: `<b>{{ . }}</b>`, dot: str, want: false},
		{name: "different text", text: `<i>{{.}}</i>`, dot: str, want: true},
		{name: "a type of dot not reached with before", text: `<b>{{.}}</b>`, dot: types.Typ[types.Int], want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := before.changed(scopeOf(t, tt.text, tt.dot)); got != tt.want {
				t.Errorf("changed = %t, want %t", got, tt.want)
			}
		})
	}
}

// tarOf builds an archive of the given entries, in order.
func tarOf(t *testing.T, entries ...tarEntry) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	for _, e := range entries {
		e.header.Size = int64(len(e.body))
		if err := w.WriteHeader(&e.header); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf
}

type tarEntry struct {
	header tar.Header
	body   string
}

// TestExtractWritesTheTree states that the files of an archive land under
// the directory, and that git's global header, which names the commit
// rather than a file, is passed over.
func TestExtractWritesTheTree(t *testing.T) {
	archive := tarOf(t,
		tarEntry{header: tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": "0123abcd"}}},
		tarEntry{header: tar.Header{Typeflag: tar.TypeDir, Name: "sub/", Mode: 0o755}},
		tarEntry{header: tar.Header{Typeflag: tar.TypeReg, Name: "sub/page.gohtml", Mode: 0o644}, body: `{{.}}`},
	)
	dir := t.TempDir()
	if err := extract(archive, dir); err != nil {
		t.Fatalf("extract = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "sub", "page.gohtml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{{.}}` {
		t.Errorf("sub/page.gohtml = %q, want %q", got, `{{.}}`)
	}
}

// TestWriteArchivedReportsAReadError states that a file the archive stops
// short of is an error, not a file cut short: a template read from a
// truncated copy would be compared as though it had changed.
func TestWriteArchivedReportsAReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.gohtml")
	if err := writeArchived(path, iotest.ErrReader(io.ErrUnexpectedEOF), 0o644); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("writeArchived = %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

// TestExtractRefusesAnEntryOutsideTheTree states that an entry naming a
// path outside the directory is refused rather than written.
func TestExtractRefusesAnEntryOutsideTheTree(t *testing.T) {
	for _, name := range []string{"../escape.txt", "/escape.txt"} {
		dir := t.TempDir()
		archive := tarOf(t, tarEntry{header: tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644}, body: "x"})
		if err := extract(archive, filepath.Join(dir, "tree")); err == nil {
			t.Errorf("extract(%q) = nil, want an error", name)
		}
		if _, err := os.Stat(filepath.Join(dir, "escape.txt")); err == nil {
			t.Errorf("extract(%q) wrote outside the tree", name)
		}
	}
}

// repo is a git repository in a temporary directory.
type repo struct {
	t   *testing.T
	dir string
}

// newRepo starts an empty repository. git runs without the invoking
// user's configuration, so a setting such as commit signing cannot change
// what the test's commits do, and go list without a workspace from the
// invoking environment.
func newRepo(t *testing.T) *repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GOWORK", "off")
	// git translates its messages, and the test reads one.
	t.Setenv("LC_ALL", "C")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(name, "muxt")
	}
	for _, name := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(name, "muxt@example.com")
	}
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q")
	return r
}

func (r *repo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func (r *repo) write(files map[string]string) {
	r.t.Helper()
	for name, content := range files {
		path := filepath.Join(r.dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *repo) commit(message string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "-m", message)
}

// diffTemplates has "page" render "name" twice with the same dot, so the
// second call is trimmed, and "count" once.
const diffTemplates = `{{define "page"}}{{template "name" .Name}}{{template "name" .Name}}{{template "count" .Count}}{{end}}
{{define "name"}}<b>{{.}}</b>{{end}}
{{define "count"}}<i>{{.}}</i>{{end}}
`

// diffGoFile renders "page" with a struct named by the first argument,
// whose Count has the type the second names.
const diffGoFile = `package server

import (
	"embed"
	"html/template"
	"io"
)

//go:embed *.gohtml
var templateFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "*.gohtml"))

type %[1]s struct {
	Name  string
	Count %[2]s
}

func Render(w io.Writer, data %[1]s) error {
	return templates.ExecuteTemplate(w, "page", data)
}
`

// mutatedTemplates names each template a plan mutates, with its dot.
func mutatedTemplates(p *plan) []string {
	var names []string
	for _, group := range p.groups {
		for _, template := range group.Templates {
			names = append(names, template.Template+" "+template.DataType)
		}
	}
	return names
}

func unchangedTemplates(p *plan) []string {
	var names []string
	for _, u := range p.unchanged {
		names = append(names, u.Template+" "+u.DataType)
	}
	return names
}

// reportText renders a plan's report as a verbose dry run would.
func reportText(t *testing.T, p *plan) string {
	t.Helper()
	report := p.report()
	report.DryRun, report.Verbose = true, true
	var out strings.Builder
	if _, err := report.WriteTo(&out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// TestNewPlanWithDiff states what a --diff run mutates in a real
// repository: a template reached with a type of dot it was not reached
// with at the revision, or whose text changed, and nothing else.
//
// The steps run in order, each committing what the one before changed.
// The package sits below the repository root, as most do.
func TestNewPlanWithDiff(t *testing.T) {
	r := newRepo(t)
	r.write(map[string]string{"README.md": "Not yet a Go package.\n"})
	r.commit("empty")
	r.git("tag", "empty")
	r.write(map[string]string{
		"web/go.mod":           "module server\n\ngo 1.24\n",
		"web/templates.gohtml": diffTemplates,
		"web/template.go":      fmt.Sprintf(diffGoFile, "Page", "int"),
	})
	r.commit("base")
	web := filepath.Join(r.dir, "web")
	config := Configuration{TemplatesVariables: []string{"templates"}, Seed: 1, SeedSet: true, Diff: "HEAD"}

	t.Run("a type of dot that changed", func(t *testing.T) {
		// Page becomes Summary, and Count goes from int to float64: "page"
		// and "count" are reached with new types, "name" is not.
		r.write(map[string]string{"web/template.go": fmt.Sprintf(diffGoFile, "Summary", "float64")})
		p, err := newPlan(config, web)
		if err != nil {
			t.Fatal(err)
		}
		if p.diffError != "" {
			t.Fatalf("the templates at HEAD could not be read: %s", p.diffError)
		}
		if got, want := mutatedTemplates(p), []string{"page server.Summary", "count float64"}; !slices.Equal(got, want) {
			t.Errorf("mutated %q, want %q", got, want)
		}
		if got, want := unchangedTemplates(p), []string{"name string"}; !slices.Equal(got, want) {
			t.Errorf("unchanged %q, want %q", got, want)
		}
		if len(p.trimmed) != 0 {
			// The repeated "name" was mutated nowhere, so a trim would
			// point at a mutation that never happened.
			t.Errorf("trimmed %v, want none", p.trimmed)
		}
		text := reportText(t, p)
		for _, want := range []string{
			"4 mutants across 2 templates (complexity 2, seed 1)\n1 template unchanged since HEAD\n",
			"\nunchanged since HEAD, not mutated:\n  \"name\" templates.gohtml (dot: string)\n",
			"\n4 mutants, 4 runnable, 0 skipped\n",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("report does not say %q:\n%s", want, text)
			}
		}
	})
	r.commit("types")

	t.Run("text that changed", func(t *testing.T) {
		r.write(map[string]string{"web/templates.gohtml": strings.Replace(diffTemplates, "<b>{{.}}</b>", "<b>{{.}}</b>!", 1)})
		p, err := newPlan(config, web)
		if err != nil {
			t.Fatal(err)
		}
		if p.diffError != "" {
			t.Fatalf("the templates at HEAD could not be read: %s", p.diffError)
		}
		if got, want := mutatedTemplates(p), []string{"name string"}; !slices.Equal(got, want) {
			t.Errorf("mutated %q, want %q", got, want)
		}
		if got, want := unchangedTemplates(p), []string{"page server.Summary", "count float64"}; !slices.Equal(got, want) {
			t.Errorf("unchanged %q, want %q", got, want)
		}
		if len(p.trimmed) != 1 {
			// "name" is mutated now, so its repeat is trimmed as usual.
			t.Errorf("trimmed %v, want the repeated name", p.trimmed)
		}
		if text := reportText(t, p); !strings.Contains(text, "1 mutant across 1 template (complexity 1, seed 1)\n2 templates unchanged since HEAD\n") {
			t.Errorf("report does not count the unchanged templates:\n%s", text)
		}
	})

	t.Run("a revision the templates cannot be read at", func(t *testing.T) {
		config := config
		config.Diff = "empty"
		p, err := newPlan(config, web)
		if err != nil {
			t.Fatal(err)
		}
		if p.diffError == "" {
			t.Error("diffError is empty, want why the templates could not be read")
		}
		if got, want := mutatedTemplates(p), []string{"page server.Summary", "name string", "count float64"}; !slices.Equal(got, want) {
			t.Errorf("mutated %q, want every template", got)
		}
		if text := reportText(t, p); !strings.Contains(text, "every template counts as changed: the templates at empty could not be read (") {
			t.Errorf("report does not say why everything was mutated:\n%s", text)
		}
	})

	t.Run("a revision git does not know", func(t *testing.T) {
		config := config
		config.Diff = "no-such-revision"
		_, err := newPlan(config, web)
		if err == nil || !strings.Contains(err.Error(), "no-such-revision") {
			t.Fatalf("newPlan = %v, want an error naming the revision", err)
		}
		if !strings.Contains(err.Error(), "fatal:") {
			t.Errorf("newPlan = %v, want what git said", err)
		}
	})
}
