package composite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// TestFindRejectsUnknownKeys states that a key no muxt.json reader knows is an
// error naming the closest known key, when one is close, wherever it sits.
func TestFindRejectsUnknownKeys(t *testing.T) {
	for _, tt := range []struct {
		name, file, want string
	}{
		{
			name: "top level typo",
			file: `{"package_default": {}, "packages": {"example.com/m": {}}}`,
			want: `unknown key "package_default"; did you mean "package_defaults"?`,
		},
		{
			name: "top level nothing close",
			file: `{"zzzzzzzz": {}, "packages": {"example.com/m": {}}}`,
			want: `unknown key "zzzzzzzz"`,
		},
		{
			name: "entry typo",
			file: `{"packages": {"example.com/m": {"use_reciever_type": "T"}}}`,
			want: `packages["example.com/m"]: unknown key "use_reciever_type"; did you mean "use_receiver_type"?`,
		},
		{
			name: "defaults typo",
			file: `{"package_defaults": {"output_htmx_": true}, "packages": {"example.com/m": {}}}`,
			want: `package_defaults: unknown key "output_htmx_"; did you mean "output_htmx"?`,
		},
		{
			name: "entry nothing close",
			file: `{"packages": {"example.com/m": {"zzz": true}}}`,
			want: `packages["example.com/m"]: unknown key "zzz"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n"), 0o644))
			path := filepath.Join(dir, composite.FileName)
			require.NoError(t, os.WriteFile(path, []byte(tt.file), 0o644))

			_, _, err := composite.Find(dir, nil)

			var unknown *composite.UnknownKeyError
			require.ErrorAs(t, err, &unknown, "Find error %v", err)
			assert.Equal(t, path+": "+tt.want, err.Error(), "message")
		})
	}
}
