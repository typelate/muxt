package composite_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// TestMuxtJSONFilesDoNotNest states that two muxt.json files on one path from
// a directory to the module root are an error where Find meets both, so a run
// at the top and a run below it never generate different code for one
// package. Files in sibling directories, and in a nested module, are not on
// one path.
func TestMuxtJSONFilesDoNotNest(t *testing.T) {
	const empty = `{"packages": {}}`
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, root string)
		// wd is where Find starts, relative to root.
		wd string
		// inner and outer are the files the error names, relative to root;
		// both empty when there is no error.
		inner, outer string
	}{
		{
			name: "found file with another above it",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, "muxt.json", empty)
				writeIn(t, root, "web/muxt.json", empty)
			},
			wd:    "web",
			inner: "web/muxt.json", outer: "muxt.json",
		},
		{
			name: "found from a directory below both",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, "muxt.json", empty)
				writeIn(t, root, "a/b/muxt.json", empty)
				writeIn(t, root, "a/b/c/keep", "")
			},
			wd:    "a/b/c",
			inner: "a/b/muxt.json", outer: "muxt.json",
		},
		{
			name: "a directory between them holds none",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, "a/muxt.json", empty)
				writeIn(t, root, "a/b/c/muxt.json", empty)
			},
			wd:    "a/b/c",
			inner: "a/b/c/muxt.json", outer: "a/muxt.json",
		},
		{
			name: "siblings with none above",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, "a/muxt.json", empty)
				writeIn(t, root, "b/muxt.json", empty)
			},
			wd: "a",
		},
		{
			name: "a nested module is its own module",
			setup: func(t *testing.T, root string) {
				writeIn(t, root, "go.mod", "module example.com/m\n")
				writeIn(t, root, "muxt.json", empty)
				writeIn(t, root, "sub/go.mod", "module example.com/m/sub\n")
				writeIn(t, root, "sub/muxt.json", empty)
			},
			wd: "sub",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root)

			f, found, err := composite.Find(filepath.Join(root, tt.wd), testKeys())

			if tt.inner == "" {
				require.NoError(t, err, "Find(%q)", tt.wd)
				assert.True(t, found, "Find(%q) found", tt.wd)
				assert.NotNil(t, f, "Find(%q) file", tt.wd)
				return
			}
			inner, outer := filepath.Join(root, tt.inner), filepath.Join(root, tt.outer)
			nested, ok := errors.AsType[*composite.NestedError](err)
			require.True(t, ok, "Find(%q) = %v, want *composite.NestedError", tt.wd, err)
			assert.Equal(t, inner, nested.Inner, "inner")
			assert.Equal(t, outer, nested.Outer, "outer")
			assert.Equal(t, inner+" is below "+outer+"; a module's muxt.json files do not nest: move its entries into "+outer+" and delete it", err.Error(), "message")
		})
	}
}

// TestCheckNotNestedNamesFileBelowEntry states that a muxt.json in an entry's
// directory, or in a directory between it and the file's directory, is an
// error naming both files, and the file's own directory is not checked.
func TestCheckNotNestedNamesFileBelowEntry(t *testing.T) {
	const empty = `{"packages": {}}`
	root := t.TempDir()
	writeIn(t, root, "go.mod", "module example.com/m\n")
	writeIn(t, root, "muxt.json", empty)
	writeIn(t, root, "a/b/d/keep", "")
	// A file below the root is only reachable by hand-building the walk: the
	// file under test is the root's, found from a directory with none below.
	f, found, err := composite.Find(root, testKeys())
	require.NoError(t, err)
	require.True(t, found)
	writeIn(t, root, "a/b/muxt.json", empty)

	for _, tt := range []struct {
		dir   string
		inner string // relative to root; "" when no error
	}{
		{dir: "."},
		{dir: "a"},
		{dir: "a/b", inner: "a/b/muxt.json"},
		{dir: "a/b/d", inner: "a/b/muxt.json"},
	} {
		err := f.CheckNotNested(filepath.Join(root, tt.dir))
		if tt.inner == "" {
			assert.NoError(t, err, "CheckNotNested(%q)", tt.dir)
			continue
		}
		nested, ok := errors.AsType[*composite.NestedError](err)
		require.True(t, ok, "CheckNotNested(%q) = %v, want *composite.NestedError", tt.dir, err)
		assert.Equal(t, filepath.Join(root, tt.inner), nested.Inner, "CheckNotNested(%q) inner", tt.dir)
		assert.Equal(t, f.Name, nested.Outer, "CheckNotNested(%q) outer", tt.dir)
	}
}

// TestCheckNotNestedStopsAtNestedModule states that a muxt.json in a nested
// module is that module's, so an entry directory inside it, at its root or
// below, is not reported as nested under the outer file.
func TestCheckNotNestedStopsAtNestedModule(t *testing.T) {
	const empty = `{"packages": {}}`
	root := t.TempDir()
	writeIn(t, root, "go.mod", "module example.com/m\n")
	writeIn(t, root, "muxt.json", empty)
	writeIn(t, root, "sub/go.mod", "module example.com/m/sub\n")
	writeIn(t, root, "sub/q/keep", "")
	f, found, err := composite.Find(root, testKeys())
	require.NoError(t, err)
	require.True(t, found)
	writeIn(t, root, "sub/muxt.json", empty)

	for _, dir := range []string{"sub", "sub/q"} {
		assert.NoError(t, f.CheckNotNested(filepath.Join(root, dir)), "CheckNotNested(%q)", dir)
	}
}

// TestCreateRefusesDirectoryWithFileBelow states that Create fails, and writes
// nothing, when a muxt.json is below the directory in the same module, and
// that a nested module, vendor, testdata and names starting with . or _ are
// not looked in.
func TestCreateRefusesDirectoryWithFileBelow(t *testing.T) {
	const empty = `{"packages": {}}`
	skipped := t.TempDir()
	writeIn(t, skipped, "go.mod", "module example.com/m\n")
	writeIn(t, skipped, "sub/go.mod", "module example.com/m/sub\n")
	for _, name := range []string{"sub", "vendor/x", ".hidden", "_x", "testdata", "a/testdata"} {
		writeIn(t, skipped, name+"/muxt.json", empty)
	}
	require.NoError(t, composite.Create(skipped, testKeys()), "Create with only skipped files below")

	root := t.TempDir()
	writeIn(t, root, "go.mod", "module example.com/n\n")
	writeIn(t, root, "web/deeper/muxt.json", empty)

	err := composite.Create(root, testKeys())

	nested, ok := errors.AsType[*composite.NestedError](err)
	require.True(t, ok, "Create = %v, want *composite.NestedError", err)
	assert.Equal(t, filepath.Join(root, "web/deeper/muxt.json"), nested.Inner, "inner")
	assert.Equal(t, filepath.Join(root, "muxt.json"), nested.Outer, "outer")
	assert.NoFileExists(t, filepath.Join(root, "muxt.json"), "Create wrote a file")
}
