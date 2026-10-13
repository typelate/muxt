package mutation

import (
	"archive/tar"
	"bytes"
	"fmt"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRevisionChanged states which scopes a --diff run mutates: one whose
// template reads differently, or one reached with a type of dot the
// revision did not reach it with.
func TestRevisionChanged(t *testing.T) {
	str := types.Typ[types.String]
	before := scopesOf([]scope{scopeOf(t, "t", `<b>{{.}}</b>`, str)})

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
			assert.Equal(t, tt.want, before.changed(scopeOf(t, "t", tt.text, tt.dot)), "changed")
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
		require.NoError(t, w.WriteHeader(&e.header))
		_, err := w.Write([]byte(e.body))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
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
	require.NoError(t, extract(archive, dir))
	got, err := os.ReadFile(filepath.Join(dir, "sub", "page.gohtml"))
	require.NoError(t, err)
	assert.Equal(t, `{{.}}`, string(got), "sub/page.gohtml")
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
	require.NoError(t, extract(archive, dir))
	link := filepath.Join(dir, "link", "page.gohtml")

	target, err := os.Readlink(link)
	assert.NoError(t, err, "Readlink")
	assert.Equal(t, "../sub/page.gohtml", target, "Readlink")

	got, err := os.ReadFile(link)
	assert.NoError(t, err, "reading through the link")
	assert.Equal(t, `{{.}}`, string(got), "reading through the link")
}

// TestExtractRefusesALinkOutOfTheTree states that a symlink pointing
// outside the copy is refused: it would read what the revision does not
// hold.
func TestExtractRefusesALinkOutOfTheTree(t *testing.T) {
	for _, target := range []string{"../../escape", "/etc/hosts"} {
		t.Run(target, func(t *testing.T) {
			archive := tarOf(t, tarEntry{header: tar.Header{Typeflag: tar.TypeSymlink, Name: "link/escape", Linkname: target}})
			dir := t.TempDir()
			assert.Error(t, extract(archive, dir), "extract of a link to %q", target)
			assert.NoFileExists(t, filepath.Join(dir, "link", "escape"), "extract made the link to %q", target)
		})
	}
}

func TestCheckSymlink(t *testing.T) {
	for _, tt := range []struct {
		name, entry, target string
		wantErr             bool
	}{
		{name: "sibling", entry: "web/link", target: "page.gohtml"},
		{name: "up within the tree", entry: "web/deep/link", target: "../page.gohtml"},
		{name: "up to the root", entry: "web/link", target: "../page.gohtml"},
		{name: "past the root", entry: "web/link", target: "../../escape", wantErr: true},
		{name: "past the root from the top", entry: "link", target: "../escape", wantErr: true},
		{name: "absolute", entry: "web/link", target: "/etc/hosts", wantErr: true},
		{name: "absolute in the tree's own name", entry: "web/link", target: "/web/page.gohtml", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkSymlink(tt.entry, tt.target)
			if tt.wantErr {
				assert.Error(t, err, "checkSymlink(%q, %q)", tt.entry, tt.target)
			} else {
				assert.NoError(t, err, "checkSymlink(%q, %q)", tt.entry, tt.target)
			}
		})
	}
}

// TestWriteArchivedReportsAReadError states that a file the archive stops
// short of is an error, not a file cut short: a template read from a
// truncated copy would be compared as though it had changed.
func TestWriteArchivedReportsAReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.gohtml")
	err := writeArchived(path, iotest.ErrReader(io.ErrUnexpectedEOF), 0o644)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF, "writeArchived")
}

// TestExtractRefusesAnEntryOutsideTheTree states that an entry naming a
// path outside the directory is refused rather than written.
func TestExtractRefusesAnEntryOutsideTheTree(t *testing.T) {
	for _, name := range []string{"../escape.txt", "/escape.txt"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			archive := tarOf(t, tarEntry{header: tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644}, body: "x"})
			assert.Error(t, extract(archive, filepath.Join(dir, "tree")), "extract(%q)", name)
			assert.NoFileExists(t, filepath.Join(dir, "escape.txt"), "extract(%q) wrote outside the tree", name)
		})
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

			p, err := newPlan(t.Context(), config, web)
			require.NoError(t, err)
			require.Empty(t, p.diffError, "the templates at HEAD could not be read")
			assert.Equal(t, tt.mutated, mutatedTemplates(p), "mutated")
			assert.Equal(t, tt.unchanged, unchangedTemplates(p), "unchanged")
			assert.Len(t, p.trimmed, tt.trimmed, "trimmed")
			text := reportText(t, p)
			for _, want := range tt.reportSays {
				assert.Contains(t, text, want, "report")
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

	p, err := newPlan(t.Context(), config, web)
	require.NoError(t, err)
	assert.NotEmpty(t, p.diffError, "diffError says why the templates could not be read")
	assert.Equal(t, []string{"page server.Summary", "name string", "count float64"}, mutatedTemplates(p), "mutated: every template")
	assert.Contains(t, reportText(t, p), "every template counts as changed: the templates at empty could not be read (", "report says why everything was mutated")
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

	_, err := newPlan(t.Context(), config, web)
	require.Error(t, err, "newPlan")
	require.ErrorContains(t, err, "no-such-revision", "newPlan names the revision")
	assert.ErrorContains(t, err, "fatal:", "newPlan says what git said")
}
