package composite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// TestSaveKeepsTheRoutePathsKey states that rewriting a file keeps its
// route_paths object with the members it holds, an empty object included, and
// that a file without the key is saved without it.
func TestSaveKeepsTheRoutePathsKey(t *testing.T) {
	for _, tt := range []struct {
		name, file, want string
	}{
		{
			name: "a member is kept",
			file: `{"packages": {}, "route_paths": {"output_file": "paths_gen.go"}}`,
			want: "{\n\t\"packages\": {},\n\t\"route_paths\": {\n\t\t\"output_file\": \"paths_gen.go\"\n\t}\n}\n",
		},
		{
			name: "an empty object is kept",
			file: `{"packages": {}, "route_paths": {}}`,
			want: "{\n\t\"packages\": {},\n\t\"route_paths\": {}\n}\n",
		},
		{
			name: "an absent key stays absent",
			file: `{"packages": {}}`,
			want: "{\n\t\"packages\": {}\n}\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFile(t, tt.file)
			f, found, err := composite.Find(dir, composite.Keys{})
			require.NoError(t, err, "Find")
			require.True(t, found, "found")

			require.NoError(t, f.Save(), "Save")

			got, err := os.ReadFile(filepath.Join(dir, composite.FileName))
			require.NoError(t, err, "reading muxt.json")
			assert.Equal(t, tt.want, string(got), "muxt.json")
		})
	}
}
