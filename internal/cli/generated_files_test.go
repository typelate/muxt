package cli

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/header"
)

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestPlural(t *testing.T) {
	for _, tt := range []struct {
		n    int
		want string
	}{
		{0, "0 routes"},
		{1, "1 route"},
		{2, "2 routes"},
		{12, "12 routes"},
	} {
		assert.Equal(t, tt.want, plural(tt.n, "route"), "plural(%d, %q)", tt.n, "route")
	}
}

func TestOwnedGeneratedFiles(t *testing.T) {
	dir := t.TempDir()
	generated := func(name string, args ...string) string {
		return writeTestFile(t, dir, name, header.Format(args, "v1")+"package main\n")
	}
	current := generated("current.go")
	explicitDefault := generated("explicit_default.go", "--output-routes-func=TemplateRoutes")
	deprecatedFlag := generated("deprecated_flag.go", "--routes-func=TemplateRoutes")
	other := generated("other.go", "--output-routes-func=AdminRoutes")
	writeTestFile(t, dir, "handwritten.go", "package main\n")
	writeTestFile(t, dir, "notes.txt", header.Format(nil, ""))

	t.Run("the default routes function", func(t *testing.T) {
		got, err := ownedGeneratedFiles(dir, "TemplateRoutes", log.New(io.Discard, "", 0))
		require.NoError(t, err, "ownedGeneratedFiles()")
		want := map[string]bool{current: true, explicitDefault: true, deprecatedFlag: true}
		assert.Equal(t, want, got, "ownedGeneratedFiles(%q) (not %s)", "TemplateRoutes", other)
	})

	t.Run("another routes function", func(t *testing.T) {
		got, err := ownedGeneratedFiles(dir, "AdminRoutes", log.New(io.Discard, "", 0))
		require.NoError(t, err, "ownedGeneratedFiles()")
		assert.Equal(t, map[string]bool{other: true}, got, "ownedGeneratedFiles(%q)", "AdminRoutes")
	})
}

// A header this version cannot read is not shown to belong to the current
// routes function, so the file is left alone, as the warning says.
func TestOwnedGeneratedFilesIgnoresUnreadableHeaders(t *testing.T) {
	dir := t.TempDir()
	unreadable := writeTestFile(t, dir, "unreadable.go", header.Format([]string{"--no-such-flag"}, "")+"package main\n")

	var stderr bytes.Buffer
	got, err := ownedGeneratedFiles(dir, "TemplateRoutes", log.New(&stderr, "", 0))
	require.NoError(t, err, "ownedGeneratedFiles()")
	assert.Empty(t, got, "ownedGeneratedFiles() want the unreadable file ignored")
	want := "WARNING: ignored generated file " + unreadable + " because arguments failed to parse: unknown flag: --no-such-flag\n"
	assert.Equal(t, want, stderr.String(), "log")
}

func TestOwnedGeneratedFilesMissingDirectory(t *testing.T) {
	_, err := ownedGeneratedFiles(filepath.Join(t.TempDir(), "missing"), "TemplateRoutes", log.New(io.Discard, "", 0))
	require.Error(t, err, "ownedGeneratedFiles(missing directory)")
}

func TestWriteGeneratedFiles(t *testing.T) {
	dir := t.TempDir()
	config := generate.RoutesFileConfiguration{
		MuxtVersion:                      "v9",
		OutputExportedDefaultIdentifiers: true,
		OutputMuxtVersion:                true,
		TemplatesVariables:               []string{defaultTemplatesVariableName},
		OutputFileName:                   defaultOutputFileName,
		ReceiverInterface:                defaultReceiverInterfaceName,
		RoutesFunction:                   defaultRoutesFunctionName,
		TemplateDataType:                 defaultTemplateDataTypeName,
		SSETemplateDataType:              defaultSSETemplateDataTypeName,
		TemplateRoutePathsTypeName:       defaultTemplateRoutePathsTypeName,
		TemplateRouteTypeName:            defaultTemplateRouteTypeName,
		TemplateRouteBuilderTypeName:     defaultTemplateRouteTypeName + "Builder",
	}
	files := []generate.GeneratedFile{
		{Path: filepath.Join(dir, "a.go"), Content: "package a\n", Routes: 1},
		{Path: filepath.Join(dir, "b.go"), Content: "package b\n", Routes: 3},
	}
	var stdout bytes.Buffer
	written, err := writeGeneratedFiles(&stdout, files, config)
	require.NoError(t, err, "writeGeneratedFiles()")

	assert.Equal(t, "wrote a.go: 1 route\nwrote b.go: 3 routes\n", stdout.String(), "stdout")
	assert.Equal(t, map[string]bool{files[0].Path: true, files[1].Path: true}, written, "written")
	content, err := os.ReadFile(files[0].Path)
	require.NoError(t, err)
	assert.Equal(t, header.Format(nil, "v9")+"package a\n", string(content), "a.go")
}

func TestWriteGeneratedFilesRollsBack(t *testing.T) {
	dir := t.TempDir()
	files := []generate.GeneratedFile{
		{Path: filepath.Join(dir, "a.go"), Content: "package a\n"},
		{Path: filepath.Join(dir, "missing", "b.go"), Content: "package b\n"},
	}
	written, err := writeGeneratedFiles(&bytes.Buffer{}, files, generate.RoutesFileConfiguration{})
	require.Error(t, err, "writeGeneratedFiles() want the failed write")
	assert.Nil(t, written, "written after a failure")
	_, statErr := os.Stat(files[0].Path)
	assert.ErrorIs(t, statErr, os.ErrNotExist, "a.go still exists after rollback")
}

// A failed run puts back the files an earlier run wrote: their content and
// their mode, not a deletion.
func TestWriteGeneratedFilesRestoresReplacedFiles(t *testing.T) {
	dir := t.TempDir()
	previous := writeTestFile(t, dir, "old_template_routes_gen.go", "package old\n")
	require.NoError(t, os.Chmod(previous, 0o600))
	// A directory with something in it cannot be renamed over.
	blocked := filepath.Join(dir, "template_routes.go")
	require.NoError(t, os.Mkdir(blocked, 0o755))
	writeTestFile(t, blocked, "child", "")

	files := []generate.GeneratedFile{
		{Path: previous, Content: "package new\n"},
		{Path: blocked, Content: "package new\n"},
	}
	_, err := writeGeneratedFiles(&bytes.Buffer{}, files, generate.RoutesFileConfiguration{})
	require.Error(t, err, "writeGeneratedFiles() want the failed write")

	content, err := os.ReadFile(previous)
	require.NoError(t, err, "old_template_routes_gen.go after a failed run")
	assert.Equal(t, "package old\n", string(content), "old_template_routes_gen.go after a failed run")
	info, err := os.Stat(previous)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "old_template_routes_gen.go mode after a failed run")
	assert.Equal(t, []string{"old_template_routes_gen.go", "template_routes.go"}, dirNames(t, dir), "files left, want no temporary file")
}

func TestWriteGeneratedFilesKeepsTheModeOfAReplacedFile(t *testing.T) {
	dir := t.TempDir()
	previous := writeTestFile(t, dir, "a.go", "package old\n")
	require.NoError(t, os.Chmod(previous, 0o600))

	_, err := writeGeneratedFiles(&bytes.Buffer{}, []generate.GeneratedFile{{Path: previous, Content: "package a\n"}}, generate.RoutesFileConfiguration{})
	require.NoError(t, err, "writeGeneratedFiles()")
	info, err := os.Stat(previous)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a.go mode")
	assert.Equal(t, []string{"a.go"}, dirNames(t, dir), "files left, want no temporary file")
}

// TestWriteGeneratedFilesWritesThroughASymlink states that a generated file
// that is a symlink keeps being one: the file it points at is rewritten, and
// a failed run puts that file's content back and leaves the link.
func TestWriteGeneratedFilesWritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	target := writeTestFile(t, dir, "real.go", "old\n")
	link := filepath.Join(dir, "template_routes.go")
	require.NoError(t, os.Symlink("real.go", link))

	undo, err := replaceFile(link, []byte("new\n"))
	require.NoError(t, err, "replaceFile()")

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the generated path is still a symlink")
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(got), "the file the link points at")

	require.NoError(t, undo(), "undo()")
	info, err = os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the generated path is a symlink after undo")
	got, err = os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "old\n", string(got), "the file the link points at after undo")
}

// TestWriteGeneratedFilesRefusesASymlinkOutOfItsDirectory states that a
// generated file that links to a file elsewhere is not written: a
// checked-out repository must not be able to point `muxt generate` at a
// file outside the package, by an absolute link, a relative one, or one
// through a directory that is itself a link.
func TestWriteGeneratedFilesRefusesASymlinkOutOfItsDirectory(t *testing.T) {
	for _, tt := range []struct {
		name    string
		link    func(t *testing.T, dir, victim string) string
		wantErr string
	}{
		{
			name: "an absolute link",
			link: func(t *testing.T, dir, victim string) string {
				link := filepath.Join(dir, "template_routes.go")
				require.NoError(t, os.Symlink(victim, link))
				return link
			},
			wantErr: "absolute path",
		},
		{
			name: "a relative link that leaves the directory",
			link: func(t *testing.T, dir, victim string) string {
				rel, err := filepath.Rel(dir, victim)
				require.NoError(t, err)
				link := filepath.Join(dir, "template_routes.go")
				require.NoError(t, os.Symlink(rel, link))
				return link
			},
			wantErr: "outside its directory",
		},
		{
			name: "a chain of links that ends outside",
			link: func(t *testing.T, dir, victim string) string {
				rel, err := filepath.Rel(dir, victim)
				require.NoError(t, err)
				require.NoError(t, os.Symlink(rel, filepath.Join(dir, "hop.go")))
				link := filepath.Join(dir, "template_routes.go")
				require.NoError(t, os.Symlink("hop.go", link))
				return link
			},
			wantErr: "outside its directory",
		},
		{
			name: "a link through a subdirectory that is a link out",
			link: func(t *testing.T, dir, victim string) string {
				require.NoError(t, os.Symlink(filepath.Dir(victim), filepath.Join(dir, "sub")))
				link := filepath.Join(dir, "template_routes.go")
				require.NoError(t, os.Symlink(filepath.Join("sub", "victim.txt"), link))
				return link
			},
			wantErr: "escapes",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			victim := writeTestFile(t, t.TempDir(), "victim.txt", "keep\n")
			dir := t.TempDir()
			link := tt.link(t, dir, victim)

			_, err := replaceFile(link, []byte("generated\n"))
			require.Error(t, err, "replaceFile() through a link out of the directory")
			assert.Contains(t, err.Error(), tt.wantErr)

			got, readErr := os.ReadFile(victim)
			require.NoError(t, readErr)
			assert.Equal(t, "keep\n", string(got), "the file the link points at")
		})
	}
}

// TestWriteGeneratedFilesFollowsALinkInsideItsDirectory states what is
// still allowed: a relative link, or a chain of them, that stays in the
// directory, including into a subdirectory.
func TestWriteGeneratedFilesFollowsALinkInsideItsDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	target := writeTestFile(t, filepath.Join(dir, "sub"), "real.go", "old\n")
	require.NoError(t, os.Symlink(filepath.Join("sub", "real.go"), filepath.Join(dir, "hop.go")))
	link := filepath.Join(dir, "template_routes.go")
	require.NoError(t, os.Symlink("hop.go", link))

	_, err := replaceFile(link, []byte("new\n"))
	require.NoError(t, err, "replaceFile()")

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new\n", string(got), "the file the chain ends at")
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the generated path is still a link")
}

func TestWriteGeneratedFilesReportsRollbackFailure(t *testing.T) {
	replace := func(path string, _ []byte) (func() error, error) {
		if filepath.Base(path) == "b.go" {
			return nil, errors.New("writing b.go failed")
		}
		return func() error { return errors.New("restoring a.go failed") }, nil
	}
	files := []generate.GeneratedFile{{Path: "a.go"}, {Path: "b.go"}}
	_, err := writeGeneratedFilesWith(&bytes.Buffer{}, files, generate.RoutesFileConfiguration{}, replace)
	require.Error(t, err, "writeGeneratedFiles() want both the failed write and the failed restore")
	require.ErrorContains(t, err, "writing b.go failed", "the failed write")
	require.ErrorContains(t, err, "restoring a.go failed", "the failed restore")
}

// A path written twice in one run is left as it was before the run.
func TestWriteGeneratedFilesUndoesInReverse(t *testing.T) {
	dir := t.TempDir()
	twice := writeTestFile(t, dir, "a.go", "package before\n")
	files := []generate.GeneratedFile{
		{Path: twice, Content: "package first\n"},
		{Path: twice, Content: "package second\n"},
		{Path: filepath.Join(dir, "missing", "b.go")},
	}
	_, err := writeGeneratedFiles(&bytes.Buffer{}, files, generate.RoutesFileConfiguration{})
	require.Error(t, err, "writeGeneratedFiles()")
	content, err := os.ReadFile(twice)
	require.NoError(t, err)
	assert.Equal(t, "package before\n", string(content), "a.go after a failed run")
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

func TestRemoveOrphans(t *testing.T) {
	dir := t.TempDir()
	kept := writeTestFile(t, dir, "kept.go", "")
	orphan := writeTestFile(t, dir, "orphan.go", "")
	writeTestFile(t, dir, "unowned.go", "")
	gone := filepath.Join(dir, "gone.go")

	owned := map[string]bool{kept: true, orphan: true, gone: true}
	require.NoError(t, removeOrphans(owned, map[string]bool{kept: true}), "removeOrphans()")
	var left []string
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	slices.Sort(left)
	assert.Equal(t, []string{"kept.go", "unowned.go"}, left, "files left")
}

func TestRemoveOrphansReportsFailure(t *testing.T) {
	dir := t.TempDir()
	stuck := filepath.Join(dir, "stuck")
	require.NoError(t, os.Mkdir(stuck, 0o755))
	writeTestFile(t, stuck, "child", "")
	err := removeOrphans(map[string]bool{stuck: true}, nil)
	require.Error(t, err, "removeOrphans() want a failed to remove orphaned file error")
	require.True(t, strings.HasPrefix(err.Error(), "failed to remove orphaned file "+stuck+": "), "removeOrphans() = %v, want a failed to remove orphaned file error", err)
}
