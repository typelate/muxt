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
	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/mutation"
)

// templatesVariablesOf runs commandLine in wd with runners that record the
// templates variables the command is run with, so nothing is loaded.
func templatesVariablesOf(t *testing.T, wd, commandLine string) ([]string, error) {
	t.Helper()
	var got []string
	err := commands(wd, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		check: func(_ *cobra.Command, _ []packageJob, c analysis.CheckConfiguration) error {
			got = c.TemplatesVariables
			return nil
		},
		calls: func(_ *cobra.Command, _ []packageJob, c analysis.TemplateCallsConfiguration) error {
			got = c.TemplatesVariables
			return nil
		},
		callers: func(_ *cobra.Command, _ []packageJob, c analysis.TemplateCallersConfiguration) error {
			got = c.TemplatesVariables
			return nil
		},
	})
	return got, err
}

// TestCallersReadsMuxtJSONEntry states that muxt list-template-callers with no
// flags in a directory a muxt.json entry configures lists the callers of the
// templates variable that entry names.
func TestCallersReadsMuxtJSONEntry(t *testing.T) {
	dir := moduleDir(t)
	writeMuxtJSON(t, dir, `{"packages": {"example.com/m": {"use_templates_variable": ["pages"]}}}`)

	got, err := templatesVariablesOf(t, dir, "list-template-callers")

	require.NoError(t, err, "muxt list-template-callers")
	assert.Equal(t, []string{"pages"}, got, "templates variables")
}

// TestCallsReadsMuxtJSONEntry states that muxt list-template-calls with no flags
// in a directory a muxt.json entry configures lists the calls of the templates
// variable that entry names.
func TestCallsReadsMuxtJSONEntry(t *testing.T) {
	dir := moduleDir(t)
	writeMuxtJSON(t, dir, `{"packages": {"example.com/m": {"use_templates_variable": ["pages"]}}}`)

	got, err := templatesVariablesOf(t, dir, "list-template-calls")

	require.NoError(t, err, "muxt list-template-calls")
	assert.Equal(t, []string{"pages"}, got, "templates variables")
}

// TestCheckReadsMuxtJSONEntry states that muxt check with no flags in a
// directory a muxt.json entry configures checks the templates variable that
// entry names.
func TestCheckReadsMuxtJSONEntry(t *testing.T) {
	dir := moduleDir(t)
	writeMuxtJSON(t, dir, `{"packages": {"example.com/m": {"use_templates_variable": ["pages"]}}}`)

	got, err := templatesVariablesOf(t, dir, "check")

	require.NoError(t, err, "muxt check")
	assert.Equal(t, []string{"pages"}, got, "templates variables")
}

// TestCheckLeavesAFileArgumentToTheRunner states that muxt check given a file,
// not a package directory, reaches the runner, which is what says the argument
// is not a package directory; looking for a muxt.json under a file must not
// fail first with a message about muxt.json.
func TestCheckLeavesAFileArgumentToTheRunner(t *testing.T) {
	dir := moduleDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644))

	for _, command := range []string{"check", "list-template-calls", "list-template-callers"} {
		ran := false
		err := commands(dir, []string{command, "./main.go"}, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
			check: func(*cobra.Command, []packageJob, analysis.CheckConfiguration) error { ran = true; return nil },
			calls: func(*cobra.Command, []packageJob, analysis.TemplateCallsConfiguration) error { ran = true; return nil },
			callers: func(*cobra.Command, []packageJob, analysis.TemplateCallersConfiguration) error {
				ran = true
				return nil
			},
		})
		require.NoError(t, err, "muxt %s ./main.go", command)
		assert.True(t, ran, "muxt %s ./main.go: runner called", command)
	}
}

// TestCheckAcceptsDirectoryWithoutMuxtJSONEntry states that muxt check in a
// directory below a muxt.json with no entry for it runs with its own flags and
// defaults, as it does with no muxt.json: only a flagless generate writes from
// the file, so only it needs an entry.
func TestCheckAcceptsDirectoryWithoutMuxtJSONEntry(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {"example.com/m": {}}}`)

	got, err := templatesVariablesOf(t, workspace, "check")
	require.NoError(t, err, "check without flags")
	assert.Equal(t, []string{"templates"}, got, "default templates variables")

	got, err = templatesVariablesOf(t, workspace, "check --use-templates-variable=pages")
	require.NoError(t, err, "check with a flag")
	assert.Equal(t, []string{"pages"}, got, "templates variables from the flag")
}

// TestEntryFlagsRejectedWhereMuxtJSONConfigures states that check and the two
// listings given a flag the muxt.json entry for the working directory also sets
// fail with the error generate returns, and the command does not run.
func TestEntryFlagsRejectedWhereMuxtJSONConfigures(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {"example.com/m/workspace": {}}}`)

	for _, tt := range []struct {
		commandLine, command string
	}{
		{"check --use-templates-variable=x", "check"},
		{"list-template-calls --use-templates-variable=x", "list-template-calls"},
		{"list-template-callers --use-templates-variable=x", "list-template-callers"},
	} {
		commandLine := tt.commandLine
		t.Run(commandLine, func(t *testing.T) {
			ran := false
			err := commands(workspace, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
				check: func(*cobra.Command, []packageJob, analysis.CheckConfiguration) error { ran = true; return nil },
				calls: func(*cobra.Command, []packageJob, analysis.TemplateCallsConfiguration) error { ran = true; return nil },
				callers: func(*cobra.Command, []packageJob, analysis.TemplateCallersConfiguration) error {
					ran = true
					return nil
				},
			})

			var configured *composite.ConfiguredDirectoryError
			require.ErrorAs(t, err, &configured, "error %v", err)
			assert.Equal(t, filepath.Join(parent, composite.FileName), configured.Path, "path")
			assert.Equal(t, []string{"use-templates-variable"}, configured.Flags, "flags")
			assert.Equal(t, "configure "+tt.command+" with fields in "+filepath.Join(parent, composite.FileName)+" instead of flags", err.Error(), "message")
			assert.False(t, ran, "the command ran")
		})
	}
}

// TestEntryNonConfigurationFlagsAllowedWhereMuxtJSONConfigures states that the
// flags a command has that a muxt.json entry does not set, such as --verbose,
// --format and --match, do not stop the entry configuring the command.
func TestEntryNonConfigurationFlagsAllowedWhereMuxtJSONConfigures(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {"example.com/m/workspace": {"use_templates_variable": ["pages"]}}}`)

	for _, commandLine := range []string{
		"check --allow-unused -v",
		"list-template-calls --format json --match x -v",
		"list-template-callers --format json -v",
	} {
		t.Run(commandLine, func(t *testing.T) {
			got, err := templatesVariablesOf(t, workspace, commandLine)

			require.NoError(t, err, "muxt %s", commandLine)
			assert.Equal(t, []string{"pages"}, got, "templates variables")
		})
	}
}

// mutationsTemplatesVariables runs commandLine in wd with a mutations runner
// that records the templates variables it is run with.
func mutationsTemplatesVariables(wd, commandLine string) (got []string, ran bool, err error) {
	err = commands(wd, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		mutations: func(_ *cobra.Command, _ string, c mutation.Configuration) error {
			got, ran = c.TemplatesVariables, true
			return nil
		},
	})
	return got, ran, err
}

// TestMutationsReadsMuxtJSONEntry states that muxt test-template-mutations
// with no flags in a directory a muxt.json entry configures mutates the
// templates variable that entry names.
func TestMutationsReadsMuxtJSONEntry(t *testing.T) {
	dir := moduleDir(t)
	writeMuxtJSON(t, dir, `{"packages": {"example.com/m": {"use_templates_variable": ["pages"]}}}`)

	got, _, err := mutationsTemplatesVariables(dir, "test-template-mutations --dry-run")

	require.NoError(t, err, "muxt test-template-mutations")
	assert.Equal(t, []string{"pages"}, got, "templates variables")
}

// TestMutationsRejectsFlagsForConfiguredDirectory states that
// test-template-mutations given a setting flag in a directory a muxt.json
// entry configures fails with the error generate returns, and nothing runs.
func TestMutationsRejectsFlagsForConfiguredDirectory(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {"example.com/m/workspace": {}}}`)

	_, ran, err := mutationsTemplatesVariables(workspace, "test-template-mutations --use-templates-variable=x")

	var configured *composite.ConfiguredDirectoryError
	require.ErrorAs(t, err, &configured, "error %v", err)
	assert.Equal(t, []string{"use-templates-variable"}, configured.Flags, "flags")
	assert.Equal(t, "configure test-template-mutations with fields in "+filepath.Join(parent, composite.FileName)+" instead of flags", err.Error(), "message")
	assert.False(t, ran, "the command ran")
}

// TestMutationsInMuxtJSONDirectoryNamesPackages states that
// test-template-mutations with no flags in the directory of a muxt.json that
// is not itself an entry fails naming the package directories, and runs
// nothing: a mutation run takes minutes, and the file's directory holds no
// package to mutate.
func TestMutationsInMuxtJSONDirectoryNamesPackages(t *testing.T) {
	dir := moduleDir(t)
	writeMuxtJSON(t, dir, `{"packages": {"example.com/m/tasks": {}, "example.com/m/contacts": {}}}`)

	_, ran, err := mutationsTemplatesVariables(dir, "test-template-mutations --dry-run")

	var several *composite.SeveralPackagesError
	require.ErrorAs(t, err, &several, "error %v", err)
	assert.Equal(t, []string{"contacts", "tasks"}, several.Dirs, "directories in key order")
	assert.Equal(t, filepath.Join(dir, composite.FileName)+": test-template-mutations tests one package; pass one of: contacts tasks", err.Error(), "message")
	assert.False(t, ran, "the command ran")

	_, ran, err = mutationsTemplatesVariables(dir, "test-template-mutations --dry-run --use-templates-variable=pages")
	require.NoError(t, err, "with a flag the directory is the package")
	assert.True(t, ran, "the command ran with a flag")
}
