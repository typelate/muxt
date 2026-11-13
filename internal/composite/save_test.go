package composite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// TestSaveWritesEachValueAsTheFileSpelledIt states that rewriting a file keeps
// every value another entry holds: an empty list stays, so the entry still does
// not inherit the default's list, and an empty package_defaults is not
// written.
func TestSaveWritesEachValueAsTheFileSpelledIt(t *testing.T) {
	for _, tt := range []struct {
		name, file, want string
	}{
		{
			name: "an empty list stays",
			file: `{"packages": {"example.com/m": {"use_templates": []}}}`,
			want: "{\n\t\"packages\": {\n\t\t\"example.com/m\": {\n\t\t\t\"use_templates\": []\n\t\t}\n\t}\n}\n",
		},
		{
			name: "defaults that hold only an empty list are written",
			file: `{"package_defaults": {"use_templates": []}, "packages": {}}`,
			want: "{\n\t\"package_defaults\": {\n\t\t\"use_templates\": []\n\t},\n\t\"packages\": {}\n}\n",
		},
		{
			name: "an empty package_defaults is dropped",
			file: `{"package_defaults": {}, "packages": {"example.com/m": {}}}`,
			want: "{\n\t\"packages\": {\n\t\t\"example.com/m\": {}\n\t}\n}\n",
		},
		{
			name: "markup characters are not escaped",
			file: `{"packages": {"example.com/m": {"use_receiver_type": "T[<&>]"}}}`,
			want: "{\n\t\"packages\": {\n\t\t\"example.com/m\": {\n\t\t\t\"use_receiver_type\": \"T[<&>]\"\n\t\t}\n\t}\n}\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFile(t, tt.file)
			keys := composite.Keys{Kinds: map[string]composite.Kind{
				"use_templates":     composite.KindList,
				"use_receiver_type": composite.KindString,
			}}
			f, found, err := composite.Find(dir, keys)
			require.NoError(t, err, "Find")
			require.True(t, found, "found")

			require.NoError(t, f.Save(), "Save")

			got, err := os.ReadFile(filepath.Join(dir, composite.FileName))
			require.NoError(t, err, "reading muxt.json")
			assert.Equal(t, tt.want, string(got), "muxt.json")
		})
	}
}
