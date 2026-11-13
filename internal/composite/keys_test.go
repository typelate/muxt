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

			_, _, err := composite.Find(dir, testKeys())

			var unknown *composite.UnknownKeyError
			require.ErrorAs(t, err, &unknown, "Find error %v", err)
			assert.Equal(t, path+": "+tt.want, err.Error(), "message")
		})
	}
}

// TestFindRejectsDeprecatedKeys states that a key Keys lists as deprecated is
// an error naming the key to write instead, not an unknown key.
func TestFindRejectsDeprecatedKeys(t *testing.T) {
	dir := writeFile(t, `{"packages": {"example.com/m": {"output_htmx_helpers": true}}}`)
	keys := testKeys()
	keys.Deprecated = map[string]string{"output_htmx_helpers": "output_htmx"}

	_, _, err := composite.Find(dir, keys)

	var deprecated *composite.DeprecatedKeyError
	require.ErrorAs(t, err, &deprecated, "Find error %v", err)
	assert.Equal(t, filepath.Join(dir, composite.FileName)+`: packages["example.com/m"]: key "output_htmx_helpers" is deprecated; write "output_htmx"`, err.Error(), "message")
}

// TestFindRejectsKeyWithOtherLetterCase states that a packages key that names a
// directory by a different letter case than the disk spells it is an error
// naming the spelling on disk, so two spellings of one directory on a
// case-insensitive file system never become two packages. The key is compared
// with the names in the directory listing, so the test creates the directory
// under its own spelling and runs on a case-sensitive file system too.
func TestFindRejectsKeyWithOtherLetterCase(t *testing.T) {
	for _, tt := range []struct {
		name, key, want string
	}{
		{name: "one element", key: "example.com/m/Contacts", want: "example.com/m/contacts"},
		{name: "a later element", key: "example.com/m/internal/Hypertext", want: "example.com/m/internal/hypertext"},
		{name: "every element", key: "example.com/m/INTERNAL/HYPERTEXT", want: "example.com/m/internal/hypertext"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n"), 0o644))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "contacts"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "hypertext"), 0o755))
			path := filepath.Join(dir, composite.FileName)
			require.NoError(t, os.WriteFile(path, []byte(`{"packages": {"`+tt.key+`": {}}}`), 0o644))

			_, _, err := composite.Find(dir, testKeys())

			var typed *composite.KeyCaseError
			require.ErrorAs(t, err, &typed, "Find error %v", err)
			assert.Equal(t, path+`: packages key "`+tt.key+`" is spelled "`+tt.want+`" on disk; write that`, err.Error(), "message")
		})
	}
}

// TestFindAcceptsKeysSpelledAsOnDisk states that a key spelled as its directory
// is, or naming a directory that does not exist, is not a letter-case error:
// whether the package is there is for the go command to say.
func TestFindAcceptsKeysSpelledAsOnDisk(t *testing.T) {
	for _, key := range []string{"example.com/m/contacts", "example.com/m/Missing", "example.com/m"} {
		t.Run(key, func(t *testing.T) {
			dir := writeFile(t, `{"packages": {"`+key+`": {}}}`)
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "contacts"), 0o755))

			_, found, err := composite.Find(dir, testKeys())

			require.NoError(t, err, "Find with key %q", key)
			assert.True(t, found, "found")
		})
	}
}
