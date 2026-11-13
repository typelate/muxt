package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/source"
	"github.com/typelate/muxt/template"
)

// TestCheckGeneratedFileNames states which template files clash when each
// writes <base name>_template_routes_gen.go into the package's directory: two
// with one base name, whether both are the package's or one is shared, and
// that the package's own file is named first so the error says which one the
// package can rename.
func TestCheckGeneratedFileNames(t *testing.T) {
	const (
		wd     = "/m/workspace"
		own    = "/m/workspace/layout.gohtml"
		shared = "/m/fragments/layout.gohtml"
	)
	definedIn := func(files ...string) source.Package {
		definitions := make(map[string]source.Definition)
		for _, file := range files {
			definitions[file] = source.Definition{Name: file, Package: "example.com/m/fragments", Tree: &template.Tree{Filepath: file}}
		}
		return source.Package{Variables: []source.Variable{{Definitions: definitions}}}
	}
	for _, tt := range []struct {
		name    string
		files   []string
		pkg     source.Package
		wantErr *GeneratedFileClashError
	}{
		{
			name:  "base names differ",
			files: []string{own, "/m/fragments/footer.gohtml"},
			pkg:   definedIn("/m/fragments/footer.gohtml"),
		},
		{
			name:    "a shared file and an own file share a base name",
			files:   []string{shared, own},
			pkg:     definedIn(shared),
			wantErr: &GeneratedFileClashError{FileName: "layout_template_routes_gen.go", First: "layout.gohtml", Second: "../fragments/layout.gohtml"},
		},
		{
			name:    "the own file is first whatever the input order",
			files:   []string{own, shared},
			pkg:     definedIn(shared),
			wantErr: &GeneratedFileClashError{FileName: "layout_template_routes_gen.go", First: "layout.gohtml", Second: "../fragments/layout.gohtml"},
		},
		{
			name:    "two own files in different directories",
			files:   []string{"/m/workspace/a/layout.gohtml", own},
			pkg:     definedIn(),
			wantErr: &GeneratedFileClashError{FileName: "layout_template_routes_gen.go", First: "a/layout.gohtml", Second: "layout.gohtml"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkGeneratedFileNames(wd, tt.files, tt.pkg)

			if tt.wantErr == nil {
				require.NoError(t, err, "checkGeneratedFileNames(%q)", tt.files)
				return
			}
			clash, ok := err.(*GeneratedFileClashError)
			require.True(t, ok, "checkGeneratedFileNames(%q) = %v, want a *GeneratedFileClashError", tt.files, err)
			assert.Equal(t, tt.wantErr, clash, "checkGeneratedFileNames(%q)", tt.files)
			assert.Equal(t, clash.FileName+" would be written for both "+clash.First+" and "+clash.Second+"; rename one template file", err.Error(), "message")
		})
	}
}
