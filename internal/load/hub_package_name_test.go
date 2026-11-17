package load_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/load"
)

func TestHubPackageName(t *testing.T) {
	for _, tt := range []struct {
		name       string
		files      map[string]string
		importPath string
		want       string
	}{
		{name: "no Go file takes the last import path element", importPath: "example.com/m/site", want: "site"},
		{name: "a Go file names it", files: map[string]string{"a.go": "package web\n"}, importPath: "example.com/m", want: "web"},
		{name: "the output file is skipped", files: map[string]string{"route_paths.go": "package stale\n", "a.go": "package web\n"}, importPath: "example.com/m", want: "web"},
		{name: "only the output file", files: map[string]string{"route_paths.go": "package stale\n"}, importPath: "example.com/m", want: "m"},
		{name: "test files are skipped", files: map[string]string{"a_test.go": "package web_test\n"}, importPath: "example.com/m", want: "m"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
			}
			got, err := load.HubPackageName(dir, tt.importPath, "route_paths.go")
			require.NoError(t, err, "HubPackageName()")
			assert.Equal(t, tt.want, got, "HubPackageName(%q)", tt.importPath)
		})
	}
}
