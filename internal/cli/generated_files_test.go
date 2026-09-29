package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/header"
)

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
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
		if got := plural(tt.n, "route"); got != tt.want {
			t.Errorf("plural(%d, %q) = %q, want %q", tt.n, "route", got, tt.want)
		}
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

	got, err := ownedGeneratedFiles(dir, "TemplateRoutes")
	if err != nil {
		t.Fatalf("ownedGeneratedFiles() error = %v", err)
	}
	want := map[string]bool{current: true, explicitDefault: true, deprecatedFlag: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ownedGeneratedFiles(%q) = %v, want %v (not %s)", "TemplateRoutes", got, want, other)
	}

	got, err = ownedGeneratedFiles(dir, "AdminRoutes")
	if err != nil {
		t.Fatalf("ownedGeneratedFiles() error = %v", err)
	}
	if want := (map[string]bool{other: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("ownedGeneratedFiles(%q) = %v, want %v", "AdminRoutes", got, want)
	}
}

func TestOwnedGeneratedFilesMissingDirectory(t *testing.T) {
	if _, err := ownedGeneratedFiles(filepath.Join(t.TempDir(), "missing"), "TemplateRoutes"); err == nil {
		t.Fatal("ownedGeneratedFiles(missing directory) = nil error, want one")
	}
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
	}
	files := []generate.GeneratedFile{
		{Path: filepath.Join(dir, "a.go"), Content: "package a\n", Routes: 1},
		{Path: filepath.Join(dir, "b.go"), Content: "package b\n", Routes: 3},
	}
	var stdout bytes.Buffer
	written, err := writeGeneratedFiles(&stdout, files, config)
	if err != nil {
		t.Fatalf("writeGeneratedFiles() error = %v", err)
	}
	if want := "wrote a.go: 1 route\nwrote b.go: 3 routes\n"; stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
	if want := (map[string]bool{files[0].Path: true, files[1].Path: true}); !reflect.DeepEqual(written, want) {
		t.Errorf("written = %v, want %v", written, want)
	}
	content, err := os.ReadFile(files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if want := header.Format(nil, "v9") + "package a\n"; string(content) != want {
		t.Errorf("a.go = %q, want %q", content, want)
	}
}

func TestWriteGeneratedFilesRollsBack(t *testing.T) {
	dir := t.TempDir()
	files := []generate.GeneratedFile{
		{Path: filepath.Join(dir, "a.go"), Content: "package a\n"},
		{Path: filepath.Join(dir, "missing", "b.go"), Content: "package b\n"},
	}
	written, err := writeGeneratedFiles(&bytes.Buffer{}, files, generate.RoutesFileConfiguration{})
	if err == nil {
		t.Fatal("writeGeneratedFiles() = nil error, want the failed write")
	}
	if written != nil {
		t.Errorf("written = %v, want nil after a failure", written)
	}
	if _, statErr := os.Stat(files[0].Path); !os.IsNotExist(statErr) {
		t.Errorf("a.go still exists after rollback: %v", statErr)
	}
}

func TestRemoveOrphans(t *testing.T) {
	dir := t.TempDir()
	kept := writeTestFile(t, dir, "kept.go", "")
	orphan := writeTestFile(t, dir, "orphan.go", "")
	writeTestFile(t, dir, "unowned.go", "")
	gone := filepath.Join(dir, "gone.go")

	owned := map[string]bool{kept: true, orphan: true, gone: true}
	if err := removeOrphans(owned, map[string]bool{kept: true}); err != nil {
		t.Fatalf("removeOrphans() error = %v", err)
	}
	var left []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		left = append(left, e.Name())
	}
	slices.Sort(left)
	if want := []string{"kept.go", "unowned.go"}; !slices.Equal(left, want) {
		t.Errorf("files left = %v, want %v", left, want)
	}
}

func TestRemoveOrphansReportsFailure(t *testing.T) {
	dir := t.TempDir()
	stuck := filepath.Join(dir, "stuck")
	if err := os.Mkdir(stuck, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, stuck, "child", "")
	err := removeOrphans(map[string]bool{stuck: true}, nil)
	if err == nil || !strings.HasPrefix(err.Error(), "failed to remove orphaned file "+stuck+": ") {
		t.Fatalf("removeOrphans() = %v, want a failed to remove orphaned file error", err)
	}
}
