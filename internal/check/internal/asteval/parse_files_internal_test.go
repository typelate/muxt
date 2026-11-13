package asteval

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseFilesParseErrorNamesSharedFile states that a parse error in a file
// of another package names the file by absolute path, so it does not read as
// an error in the package that consumes it, and that an own file's error is
// unchanged.
func TestParseFilesParseErrorNamesSharedFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "layout.gohtml")
	if err := os.WriteFile(file, []byte(`{{upper .}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name, pkgPath, want string
	}{
		{name: "shared file", pkgPath: "example.com/m/fragments", want: file + `:1: function "upper" not defined`},
		{name: "own file", pkgPath: "", want: `template: layout.gohtml:1: function "upper" not defined`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseFiles(nil, false, nil, nil, tt.pkgPath, "", "", file)

			if err == nil {
				t.Fatalf("parseFiles(%q) error = nil, want a parse error", file)
			}
			if got := err.Error(); got != tt.want {
				t.Errorf("parseFiles(%q) error = %q, want %q", file, got, tt.want)
			}
		})
	}
}
