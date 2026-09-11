package mutation

import (
	"archive/tar"
	"bytes"
	"errors"
	"go/types"
	"io"
	"os"
	"path/filepath"
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
