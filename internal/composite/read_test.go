package composite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// writeFile writes a go.mod for example.com/m and a muxt.json holding file in
// a temporary directory, and returns the directory.
func writeFile(t *testing.T, file string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))
	return dir
}

// TestFindWrongJSONTypeIsAJSONError states what a value of the wrong JSON type
// reads today: the json package's message, naming the Go struct field, before
// the module and key checks. A later change replaces it with an error naming
// the key and the type it takes.
func TestFindWrongJSONTypeIsAJSONError(t *testing.T) {
	dir := writeFile(t, `{"packages": {"example.com/m": {"output_htmx": "yes"}}}`)

	_, _, err := composite.Find(dir, nil)

	require.Error(t, err, "Find")
	path := filepath.Join(dir, composite.FileName)
	assert.Equal(t, "parsing "+path+": json: cannot unmarshal string into Go struct field File.packages.example.com~1m.output_htmx of type bool", err.Error(), "message")
}

// TestFindReadsNullAsAbsent states that a null value is the same as a key that
// is not there: a default fills it.
func TestFindReadsNullAsAbsent(t *testing.T) {
	dir := writeFile(t, `{"package_defaults": {"output_htmx": true}, "packages": {"example.com/m": {"output_htmx": null, "use_receiver_type": null}}}`)

	f, found, err := composite.Find(dir, nil)

	require.NoError(t, err, "Find")
	require.True(t, found, "found")
	entry, ok := f.Resolve("example.com/m")
	require.True(t, ok, "entry")
	require.NotNil(t, entry.OutputHTMX, "output_htmx of the resolved entry")
	assert.True(t, *entry.OutputHTMX, "output_htmx of the resolved entry")
	assert.Nil(t, entry.UseReceiverType, "use_receiver_type of the resolved entry")
}
