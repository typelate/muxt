package cli

import (
	"bytes"
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

func TestWriteGeneratedFilesReportsRollbackFailure(t *testing.T) {
	dir := t.TempDir()
	twice := filepath.Join(dir, "a.go")
	files := []generate.GeneratedFile{
		{Path: twice},
		{Path: twice},
		{Path: filepath.Join(dir, "missing", "b.go")},
	}
	_, err := writeGeneratedFiles(&bytes.Buffer{}, files, generate.RoutesFileConfiguration{})
	require.Error(t, err, "writeGeneratedFiles() want both the failed write and the failed removal")
	require.ErrorContains(t, err, "b.go", "the failed write")
	require.ErrorContains(t, err, "a.go", "the failed removal")
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
