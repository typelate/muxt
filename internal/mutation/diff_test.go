package mutation

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

// TestExtractWritesASymlink states that a symlink in the tree is made as
// one, pointing where it pointed.
func TestExtractWritesASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("making a symlink needs privileges on Windows")
	}
	archive := tarOf(t,
		tarEntry{header: tar.Header{Typeflag: tar.TypeReg, Name: "sub/page.gohtml", Mode: 0o644}, body: `{{.}}`},
		tarEntry{header: tar.Header{Typeflag: tar.TypeSymlink, Name: "link/page.gohtml", Linkname: "../sub/page.gohtml"}},
	)
	dir := t.TempDir()
	if err := extract(archive, dir); err != nil {
		t.Fatalf("extract = %v", err)
	}
	link := filepath.Join(dir, "link", "page.gohtml")
	if target, err := os.Readlink(link); err != nil || target != "../sub/page.gohtml" {
		t.Errorf("Readlink = %q, %v, want %q", target, err, "../sub/page.gohtml")
	}
	if got, err := os.ReadFile(link); err != nil || string(got) != `{{.}}` {
		t.Errorf("reading through the link = %q, %v, want %q", got, err, `{{.}}`)
	}
}

// TestExtractRefusesALinkOutOfTheTree states that a symlink pointing
// outside the copy is refused: it would read what the revision does not
// hold.
func TestExtractRefusesALinkOutOfTheTree(t *testing.T) {
	for _, target := range []string{"../../escape", "/etc/hosts"} {
		archive := tarOf(t, tarEntry{header: tar.Header{Typeflag: tar.TypeSymlink, Name: "link/escape", Linkname: target}})
		dir := t.TempDir()
		if err := extract(archive, dir); err == nil {
			t.Errorf("extract of a link to %q = nil, want an error", target)
		}
		if _, err := os.Lstat(filepath.Join(dir, "link", "escape")); err == nil {
			t.Errorf("extract made the link to %q", target)
		}
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

// diffRepo starts a repository whose web/ package renders "page" with a
// Page whose Count is an int, committed as "base". An earlier commit,
// tagged empty, holds no Go package at all.
//
// The package sits below the repository root, as most do.
func diffRepo(t *testing.T) (*repo, string) {
	t.Helper()
	r := newRepo(t)
	r.write(map[string]string{"README.md": "Not yet a Go package.\n"})
	r.commit("empty")
	r.git("tag", "empty")
	r.write(map[string]string{
		"web/go.mod":           goMod,
		"web/templates.gohtml": diffTemplates,
		"web/template.go":      fmt.Sprintf(diffGoFile, "Page", "int"),
	})
	r.commit("base")
	return r, filepath.Join(r.dir, "web")
}

// renameType is a branch that renames the page's data type and changes
// what Count is, leaving the templates as they were.
func renameType(r *repo) {
	r.write(map[string]string{"web/template.go": fmt.Sprintf(diffGoFile, "Summary", "float64")})
}

// editTemplate is a branch that commits that type change and then edits
// what "name" renders.
func editTemplate(r *repo) {
	renameType(r)
	r.commit("types")
	r.write(map[string]string{"web/templates.gohtml": strings.Replace(diffTemplates, "<b>{{.}}</b>", "<b>{{.}}</b>!", 1)})
}

// diffConfig is a --diff run against the last commit.
func diffConfig() Configuration {
	return Configuration{TemplatesVariables: []string{"templates"}, Seed: 1, SeedSet: true, Diff: "HEAD", env: goEnv()}
}

// TestNewPlanWithDiff states what a --diff run mutates: a template reached
// with a type of dot it was not reached with at the revision, or whose
// text changed, and nothing else.
func TestNewPlanWithDiff(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		change     func(r *repo)
		env        func(r *repo) []string
		mutated    []string
		unchanged  []string
		trimmed    int
		reportSays []string
	}{
		{
			name:   "a type of dot that changed",
			change: renameType,
			// "page" and "count" are reached with types they were not
			// reached with before; "name" still gets a string. The repeat
			// of "name" is mutated nowhere, so no trim points at a
			// mutation that never happened.
			mutated:   []string{"page server.Summary", "count float64"},
			unchanged: []string{"name string"},
			reportSays: []string{
				"4 mutants across 2 templates (complexity 2, seed 1)\n1 template unchanged since HEAD\n",
				"\nunchanged since HEAD, not mutated:\n  \"name\" templates.gohtml (dot: string)\n",
				"\n4 mutants, 4 runnable, 0 skipped\n",
			},
		},
		{
			name:      "text that changed",
			change:    editTemplate,
			mutated:   []string{"name string"},
			unchanged: []string{"page server.Summary", "count float64"},
			// "name" is mutated now, so its repeat is trimmed as usual.
			trimmed:    1,
			reportSays: []string{"1 mutant across 1 template (complexity 1, seed 1)\n2 templates unchanged since HEAD\n"},
		},
		{
			name: "a workspace the caller names",
			change: func(r *repo) {
				editTemplate(r)
				r.write(map[string]string{"go.work": "go 1.24\n\nuse ./web\n"})
			},
			// The working tree loads within the workspace; the copy of the
			// revision sits outside it and loads as the module it is.
			env:       func(r *repo) []string { return append(os.Environ(), "GOWORK="+filepath.Join(r.dir, "go.work")) },
			mutated:   []string{"name string"},
			unchanged: []string{"page server.Summary", "count float64"},
			trimmed:   1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r, web := diffRepo(t)
			tt.change(r)
			config := diffConfig()
			if tt.env != nil {
				config.env = tt.env(r)
			}

			p, err := newPlan(config, web)
			if err != nil {
				t.Fatal(err)
			}
			if p.diffError != "" {
				t.Fatalf("the templates at HEAD could not be read: %s", p.diffError)
			}
			if got := mutatedTemplates(p); !slices.Equal(got, tt.mutated) {
				t.Errorf("mutated %q, want %q", got, tt.mutated)
			}
			if got := unchangedTemplates(p); !slices.Equal(got, tt.unchanged) {
				t.Errorf("unchanged %q, want %q", got, tt.unchanged)
			}
			if len(p.trimmed) != tt.trimmed {
				t.Errorf("trimmed %v, want %d", p.trimmed, tt.trimmed)
			}
			text := reportText(t, p)
			for _, want := range tt.reportSays {
				if !strings.Contains(text, want) {
					t.Errorf("report does not say %q:\n%s", want, text)
				}
			}
		})
	}
}

// TestNewPlanWithDiffAtARevisionItCannotRead states that a revision with
// no readable templates, such as one from before the package existed,
// leaves nothing to compare with: every template counts as changed and the
// report says why.
func TestNewPlanWithDiffAtARevisionItCannotRead(t *testing.T) {
	t.Parallel()
	r, web := diffRepo(t)
	renameType(r)
	config := diffConfig()
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
}

// TestNewPlanWithDiffAtARevisionGitDoesNotKnow states that an unknown
// revision stops the run, naming the revision and saying what git said.
func TestNewPlanWithDiffAtARevisionGitDoesNotKnow(t *testing.T) {
	// muxt runs git with the environment it inherits, and git translates
	// its messages; the assertion below reads one.
	t.Setenv("LC_ALL", "C")
	_, web := diffRepo(t)
	config := diffConfig()
	config.Diff = "no-such-revision"

	_, err := newPlan(config, web)
	if err == nil || !strings.Contains(err.Error(), "no-such-revision") {
		t.Fatalf("newPlan = %v, want an error naming the revision", err)
	}
	if !strings.Contains(err.Error(), "fatal:") {
		t.Errorf("newPlan = %v, want what git said", err)
	}
}
