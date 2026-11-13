package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// TestGenerateReportsTheFirstErrorOfAFile states which error a muxt.json with
// several broken entries gives: the first entry in key order, and in an entry
// the first rule in the order generate checks them, so fixing the one named
// reveals the next.
func TestGenerateReportsTheFirstErrorOfAFile(t *testing.T) {
	for _, tt := range []struct {
		name string
		file string
		want string
	}{
		{
			name: "two entries",
			file: `{"packages": {"example.com/m/b": {"output_routes_func": "1x"}, "example.com/m/a": {"output_htmx": true, "output_datastar": true}}}`,
			want: `packages["example.com/m/a"]: output_htmx and output_datastar are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)`,
		},
		{
			name: "one entry",
			file: `{"packages": {"example.com/m/a": {"output_routes_func": "1x", "output_htmx": true, "output_datastar": true, "output_receiver_interface": ""}}}`,
			want: `packages["example.com/m/a"]: output_receiver_interface value must not be empty`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := moduleDir(t)
			require.NoError(t, os.Mkdir(filepath.Join(dir, "a"), 0o755))
			require.NoError(t, os.Mkdir(filepath.Join(dir, "b"), 0o755))
			writeMuxtJSON(t, dir, tt.file)

			called, err := generateFailureOf(t, dir)

			assert.Equal(t, filepath.Join(dir, composite.FileName)+": "+tt.want, err.Error(), "message")
			assert.False(t, called, "generate runner called")
		})
	}
}

// muxtJSONAfter runs commandLine in a module holding file as its muxt.json and
// returns the muxt.json text afterwards.
func muxtJSONAfter(t *testing.T, file, commandLine string) string {
	t.Helper()
	root := moduleDir(t)
	writeMuxtJSON(t, root, file)
	_, err := runCommand(t, root, commandLine)
	require.NoError(t, err, commandLine)
	got, err := os.ReadFile(filepath.Join(root, composite.FileName))
	require.NoError(t, err, "reading muxt.json")
	return string(got)
}

// TestGeneratePackageRecordsDeprecatedTemplatesVariableUnderCurrentKey states
// that muxt generate package writes the deprecated --templates-variable flag
// into the entry as use_templates_variable, the key a muxt.json holds.
func TestGeneratePackageRecordsDeprecatedTemplatesVariableUnderCurrentKey(t *testing.T) {
	for _, tt := range []struct {
		name, commandLine, want string
	}{
		{
			name:        "the deprecated templates variable is recorded under the current key",
			commandLine: "generate package a --templates-variable x",
			want:        "{\n\t\"packages\": {\n\t\t\"example.com/m/a\": {\n\t\t\t\"use_receiver_type\": \"T\",\n\t\t\t\"use_templates_variable\": [\n\t\t\t\t\"x\"\n\t\t\t]\n\t\t}\n\t}\n}\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, muxtJSONAfter(t, `{"packages": {}}`, tt.commandLine), "muxt.json")
		})
	}
}

// TestGeneratePackageRewritesOtherEntries states what rewriting the file does
// to what it already holds: a list the file spelled empty stays in its entry,
// so the entry still does not inherit the default's list, and an empty
// package_defaults is not written.
func TestGeneratePackageRewritesOtherEntries(t *testing.T) {
	const file = `{"package_defaults": {}, "packages": {"example.com/m/a": {"output_routes_func": "Root", "use_templates_variable": []}}}`
	root := moduleDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(root, "a"), 0o755))
	writeMuxtJSON(t, root, file)

	_, err := runCommand(t, root, "generate package b")

	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(root, composite.FileName))
	require.NoError(t, err)
	const want = "{\n\t\"packages\": {\n\t\t\"example.com/m/a\": {\n\t\t\t\"output_routes_func\": \"Root\",\n\t\t\t\"use_templates_variable\": []\n\t\t},\n\t\t\"example.com/m/b\": {\n\t\t\t\"use_receiver_type\": \"T\"\n\t\t}\n\t}\n}\n"
	assert.Equal(t, want, string(got), "muxt.json")
}

// TestGeneratePackageKeepsDefaultsThatHeldOnlyAnEmptyList states that a
// package_defaults holding only an empty list is rewritten with the list.
func TestGeneratePackageKeepsDefaultsThatHeldOnlyAnEmptyList(t *testing.T) {
	got := muxtJSONAfter(t, `{"package_defaults": {"use_templates_variable": []}, "packages": {}}`, "generate package a")

	const want = "{\n\t\"package_defaults\": {\n\t\t\"use_templates_variable\": []\n\t},\n\t\"packages\": {\n\t\t\"example.com/m/a\": {\n\t\t\t\"use_receiver_type\": \"T\"\n\t\t}\n\t}\n}\n"
	assert.Equal(t, want, got, "muxt.json")
}
