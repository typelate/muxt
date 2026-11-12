package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCommand runs a muxt command line in dir, with no configuration runners,
// and returns what it printed to stdout and the error it returned.
func runCommand(t *testing.T, dir string, commandLine string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	err := commands(dir, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), &stdout, io.Discard, runners{})
	return stdout.String(), err
}

// TestGenerateConfigWritesEmptyMuxtJSON states that muxt generate config in a
// directory with no muxt.json creates one with an empty packages object.
func TestGenerateConfigWritesEmptyMuxtJSON(t *testing.T) {
	dir := t.TempDir()

	stdout, err := runCommand(t, dir, "generate config")

	require.NoError(t, err)
	assert.Equal(t, "wrote muxt.json\n", stdout, "stdout")
	got, err := os.ReadFile(filepath.Join(dir, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, "{\n\t\"packages\": {}\n}\n", string(got), "muxt.json")
}

// TestGenerateConfigLeavesExistingMuxtJSON states that muxt generate config
// fails, and writes nothing, when the working directory has a muxt.json.
func TestGenerateConfigLeavesExistingMuxtJSON(t *testing.T) {
	dir := t.TempDir()
	const existing = "{\n  \"packages\": {\".\": {\"output_htmx\": true}}\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "muxt.json"), []byte(existing), 0o644))

	_, err := runCommand(t, dir, "generate config")

	require.EqualError(t, err, "muxt.json already exists; nothing was written")
	got, err := os.ReadFile(filepath.Join(dir, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, existing, string(got), "muxt.json is unchanged")
}

// TestGeneratePackageAddsEntryToMuxtJSON states that muxt generate package
// adds an entry keyed by the new directory's path from the muxt.json's
// directory, keeps the entries already there, and rewrites the file in its
// stable form.
func TestGeneratePackageAddsEntryToMuxtJSON(t *testing.T) {
	const existing = `{"package_defaults": {"use_templates_variable": ["pages"]}, "packages": {"admin": {"output_htmx": true}}}`
	const want = "{\n" +
		"\t\"package_defaults\": {\n" +
		"\t\t\"use_templates_variable\": [\n" +
		"\t\t\t\"pages\"\n" +
		"\t\t]\n" +
		"\t},\n" +
		"\t\"packages\": {\n" +
		"\t\t\"admin\": {\n" +
		"\t\t\t\"output_htmx\": true\n" +
		"\t\t},\n" +
		"\t\t\"workspace\": {}\n" +
		"\t}\n" +
		"}\n"

	t.Run("in the file's directory", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(existing), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(root, "admin"), 0o755))

		_, err := runCommand(t, root, "generate package workspace")

		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(root, "muxt.json"))
		require.NoError(t, err)
		assert.Equal(t, want, string(got), "muxt.json")
	})

	t.Run("in a sub-directory", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "muxt.json"), []byte(`{"packages": {}}`), 0o644))
		admin := filepath.Join(root, "admin")
		require.NoError(t, os.Mkdir(admin, 0o755))

		_, err := runCommand(t, admin, "generate package workspace")

		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(root, "muxt.json"))
		require.NoError(t, err)
		assert.Equal(t, "{\n\t\"packages\": {\n\t\t\"admin/workspace\": {}\n\t}\n}\n", string(got), "muxt.json")
	})
}

// TestGenerateConfigDoesNotFollowDanglingMuxtJSONLink states that muxt generate
// config fails, and creates nothing at the link's target, when muxt.json is a
// symbolic link to a file that does not exist.
func TestGenerateConfigDoesNotFollowDanglingMuxtJSONLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere.json")
	link := filepath.Join(dir, "muxt.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	_, err := runCommand(t, dir, "generate config")

	require.EqualError(t, err, "muxt.json already exists; nothing was written")
	_, statErr := os.Lstat(target)
	assert.ErrorIs(t, statErr, os.ErrNotExist, "nothing is created at the link's target")
}

// TestGenerateConfigurationIsAliasOfConfig states that muxt generate
// configuration does what muxt generate config does.
func TestGenerateConfigurationIsAliasOfConfig(t *testing.T) {
	dir := t.TempDir()

	stdout, err := runCommand(t, dir, "generate configuration")

	require.NoError(t, err)
	assert.Equal(t, "wrote muxt.json\n", stdout, "stdout")
	got, err := os.ReadFile(filepath.Join(dir, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, "{\n\t\"packages\": {}\n}\n", string(got), "muxt.json")
}
