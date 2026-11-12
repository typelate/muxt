package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/muxtjson"
)

// templatesVariablesOf runs commandLine in wd with runners that record the
// templates variables the command is run with, so nothing is loaded.
func templatesVariablesOf(t *testing.T, wd, commandLine string) ([]string, error) {
	t.Helper()
	var got []string
	err := commands(wd, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		check: func(_ *cobra.Command, _ string, c analysis.CheckConfiguration) error {
			got = c.TemplatesVariables
			return nil
		},
		calls: func(_ *cobra.Command, _ string, c analysis.TemplateCallsConfiguration) error {
			got = c.TemplatesVariables
			return nil
		},
	})
	return got, err
}

// TestCallsReadsMuxtJSONEntry states that muxt list-template-calls with no flags
// in a directory a muxt.json entry configures lists the calls of the templates
// variable that entry names.
func TestCallsReadsMuxtJSONEntry(t *testing.T) {
	dir := t.TempDir()
	writeMuxtJSON(t, dir, `{"packages": {".": {"use_templates_variable": ["pages"]}}}`)

	got, err := templatesVariablesOf(t, dir, "list-template-calls")

	require.NoError(t, err, "muxt list-template-calls")
	assert.Equal(t, []string{"pages"}, got, "templates variables")
}

// TestCheckReadsMuxtJSONEntry states that muxt check with no flags in a
// directory a muxt.json entry configures checks the templates variable that
// entry names.
func TestCheckReadsMuxtJSONEntry(t *testing.T) {
	dir := t.TempDir()
	writeMuxtJSON(t, dir, `{"packages": {".": {"use_templates_variable": ["pages"]}}}`)

	got, err := templatesVariablesOf(t, dir, "check")

	require.NoError(t, err, "muxt check")
	assert.Equal(t, []string{"pages"}, got, "templates variables")
}

// TestCheckLeavesAFileArgumentToTheRunner states that muxt check given a file,
// not a package directory, reaches the runner, which is what says the argument
// is not a package directory; looking for a muxt.json under a file must not
// fail first with a message about muxt.json.
func TestCheckLeavesAFileArgumentToTheRunner(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))

	for _, command := range []string{"check", "list-template-calls", "list-template-callers"} {
		ran := false
		err := commands(dir, []string{command, "./main.go"}, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
			check:   func(*cobra.Command, string, analysis.CheckConfiguration) error { ran = true; return nil },
			calls:   func(*cobra.Command, string, analysis.TemplateCallsConfiguration) error { ran = true; return nil },
			callers: func(*cobra.Command, string, analysis.TemplateCallersConfiguration) error { ran = true; return nil },
		})
		require.NoError(t, err, "muxt %s ./main.go", command)
		assert.True(t, ran, "muxt %s ./main.go: runner called", command)
	}
}

// TestCheckRejectsDirectoryWithoutMuxtJSONEntry states that muxt check with no
// flags in a directory below a muxt.json with no entry for it fails with the
// error generate returns.
func TestCheckRejectsDirectoryWithoutMuxtJSONEntry(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {".": {}}}`)

	_, err := templatesVariablesOf(t, workspace, "check")

	var missing *muxtjson.MissingEntryError
	require.ErrorAs(t, err, &missing, "error %v", err)
	assert.Equal(t, "workspace", missing.Key, "key")
}
