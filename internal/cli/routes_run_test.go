package cli

import (
	"bytes"
	"encoding/json"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/muxtjson"
)

// routesJobsOf runs commandLine in wd with a runner that records the jobs
// bare muxt would list, so nothing is loaded.
func routesJobsOf(t *testing.T, wd, commandLine string) ([]routesJob, error) {
	t.Helper()
	var got []routesJob
	err := commands(wd, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		routes: func(_ *cobra.Command, jobs []routesJob) error {
			got = jobs
			return nil
		},
	})
	return got, err
}

// writeMuxtJSON writes a muxt.json holding file into dir.
func writeMuxtJSON(t *testing.T, dir, file string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))
}

// TestRoutesListsMuxtJSONEntry states that bare muxt with no flags, in a
// directory a muxt.json entry configures, lists with the entry's templates
// variable and receiver type, with package_defaults applied. The shared flags
// that run settings (-v, --format) do not count as configuration.
func TestRoutesListsMuxtJSONEntry(t *testing.T) {
	dir := t.TempDir()
	writeMuxtJSON(t, dir, `{"package_defaults": {"use_receiver_type_package": "example.com/m/types"}, "packages": {".": {"use_templates_variable": ["pages"], "use_receiver_type": "Server"}}}`)

	for _, commandLine := range []string{"", "-v", "--format json"} {
		jobs, err := routesJobsOf(t, dir, commandLine)

		require.NoError(t, err, "muxt %s", commandLine)
		require.Len(t, jobs, 1, "muxt %s jobs", commandLine)
		assert.Equal(t, dir, jobs[0].Dir, "muxt %s directory", commandLine)
		want := analysis.DefinitionsConfiguration{
			Verbose:            strings.Contains(commandLine, "-v"),
			ReceiverPackage:    "example.com/m/types",
			ReceiverType:       "Server",
			TemplatesVariables: []string{"pages"},
		}
		assert.Equal(t, want, jobs[0].definitions(), "muxt %s configuration", commandLine)
	}
}

// TestRoutesRejectsFlagsForConfiguredDirectory states that a flag bare muxt
// shares with an entry, given where muxt.json has an entry for the working
// directory, fails with the error generate returns, and nothing is listed.
func TestRoutesRejectsFlagsForConfiguredDirectory(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {"workspace": {}}}`)

	jobs, err := routesJobsOf(t, workspace, "--use-receiver-type=Server --use-templates-variable=pages")

	var configured *muxtjson.ConfiguredDirectoryError
	require.ErrorAs(t, err, &configured, "error %v", err)
	assert.Equal(t, filepath.Join("..", muxtjson.FileName), configured.Path, "path")
	assert.Equal(t, []string{"use-receiver-type", "use-templates-variable"}, configured.Flags, "flags")
	assert.Empty(t, jobs, "jobs")
}

// TestRoutesRejectsDirectoryWithoutMuxtJSONEntry states that bare muxt with no
// flags in a directory below a muxt.json with no entry for it fails with the
// error generate returns. With a flag, the flags configure the listing.
func TestRoutesRejectsDirectoryWithoutMuxtJSONEntry(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {".": {}}}`)

	jobs, err := routesJobsOf(t, workspace, "")

	var missing *muxtjson.MissingEntryError
	require.ErrorAs(t, err, &missing, "error %v", err)
	assert.Equal(t, filepath.Join("..", muxtjson.FileName), missing.Path, "path")
	assert.Equal(t, "workspace", missing.Key, "key")
	assert.Empty(t, jobs, "jobs")

	jobs, err = routesJobsOf(t, workspace, "--use-receiver-type=T1")
	require.NoError(t, err, "with a flag")
	require.Len(t, jobs, 1, "jobs with a flag")
	assert.Equal(t, "T1", jobs[0].definitions().ReceiverType, "receiver type from the flag")
}

// reportOf returns the Configuration section text bare muxt prints for the
// first job of commandLine run in wd.
func reportOf(t *testing.T, wd, commandLine string) string {
	t.Helper()
	jobs, err := routesJobsOf(t, wd, commandLine)
	require.NoError(t, err, "muxt %s", commandLine)
	require.NotEmpty(t, jobs, "muxt %s jobs", commandLine)
	var text strings.Builder
	_, err = jobs[0].Report.WriteTo(&text)
	require.NoError(t, err, "writing the section")
	return text.String()
}

// TestRoutesConfigurationHasEverySettingOnce states that the Configuration
// section holds each muxt.json entry key exactly once, as a line written the
// way muxt.json writes it, whether the entry sets it or not.
func TestRoutesConfigurationHasEverySettingOnce(t *testing.T) {
	dir := t.TempDir()
	writeMuxtJSON(t, dir, `{"packages": {".": {"use_receiver_type": "Server"}}}`)

	text := reportOf(t, dir, "")

	assert.True(t, strings.HasPrefix(text, "Configuration:\n"), "section starts with its heading:\n%s", text)
	for _, key := range muxtJSONEntryKeys() {
		assert.Equal(t, 1, strings.Count(text, "\n    \""+key+"\": "), "lines for %q in:\n%s", key, text)
	}
}

// listWithLoader runs bare muxt with args in dir with the real listing runner
// and the go command, counting how often packages are loaded.
func listWithLoader(t *testing.T, dir string, args ...string) (stdout string, loads int, err error) {
	t.Helper()
	counting := func(root string, dirs, patterns []string) (*token.FileSet, []*packages.Package, error) {
		loads++
		return loadFromGoCommand(root, dirs, patterns)
	}
	var out bytes.Buffer
	err = commands(dir, args, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), &out, &bytes.Buffer{}, runners{
		routes: routesRunner(counting),
	})
	return out.String(), loads, err
}

// TestRoutesPrintsConfigurationBeforeRoutes states that bare muxt writes the
// Configuration section before Template Routes, and with --format json adds a
// Configuration member to the listing, keeping the members it had.
func TestRoutesPrintsConfigurationBeforeRoutes(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{".": "Root"}, nil)

	text, _, err := listWithLoader(t, dir)
	require.NoError(t, err, "muxt")
	assert.Regexp(t, `(?s)\AConfiguration:\n.*\n\n.*Template Routes:\n  - GET /Root RootPage\(\)\n`, text, "text output")

	asJSON, _, err := listWithLoader(t, dir, "--format", "json")
	require.NoError(t, err, "muxt --format json")
	var listing struct {
		Definitions   []struct{ String string }
		Configuration []struct {
			Source   string
			Settings []struct {
				Key   string
				Value any
			}
		}
	}
	require.NoError(t, json.Unmarshal([]byte(asJSON), &listing), "output:\n%s", asJSON)
	require.Len(t, listing.Definitions, 1, "definitions")
	assert.Equal(t, "GET /Root RootPage()", listing.Definitions[0].String, "definition")
	values := make(map[string]any)
	for _, group := range listing.Configuration {
		for _, s := range group.Settings {
			values[s.Key] = s.Value
		}
	}
	assert.Equal(t, "Root", values["use_receiver_type"], "use_receiver_type in the configuration")
	assert.Len(t, values, len(muxtJSONEntryKeys()), "settings")
}

// TestRoutesConfigurationGroupsEntrySettings states that a setting the muxt.json
// entry sets is under the heading naming the file, relative to the working
// directory, and the entry's key, written as its muxt.json key and value, and
// that the setting is not repeated under another heading.
func TestRoutesConfigurationGroupsEntrySettings(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {"workspace": {"use_receiver_type": "T1"}}}`)

	text := reportOf(t, workspace, "")

	const want = "Configuration:\n  ../muxt.json packages[\"workspace\"]:\n    \"use_receiver_type\": \"T1\"\n  defaults:\n"
	assert.True(t, strings.HasPrefix(text, want), "section starts with\n%s\ngot\n%s", want, text)
	assert.Equal(t, 1, strings.Count(text, "\"use_receiver_type\":"), "lines for use_receiver_type")
}

// TestRoutesConfigurationDefaults states that where no muxt.json is found the
// section is the single heading defaults, holding every setting with the value
// generate would use after defaulting, in the order muxt generate --help lists
// the flags.
func TestRoutesConfigurationDefaults(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.24\n")

	text := reportOf(t, dir, "")

	const want = `Configuration:
  defaults:
    "input_static_assets_directory": ""
    "output_datastar": false
    "output_exported_default_identifiers": true
    "output_file": "template_routes.go"
    "output_htmx": false
    "output_multipart_max_memory": "32 MiB"
    "output_multiple_files": false
    "output_muxt_version": true
    "output_receiver_interface": "RoutesReceiver"
    "output_routes_func": "TemplateRoutes"
    "output_routes_func_with_logger_param": false
    "output_routes_func_with_middleware_param": false
    "output_routes_func_with_path_prefix_param": false
    "output_sse_template_data_type": "SSETemplateData"
    "output_static_assets_path": ""
    "output_template_data_type": "TemplateData"
    "output_template_route_builder_type": "TemplateRouteBuilder"
    "output_template_route_paths_type": "TemplateRoutePaths"
    "output_template_route_type": "TemplateRoute"
    "use_receiver_type": ""
    "use_receiver_type_package": ""
    "use_templates_variable": ["templates"]

`
	assert.Equal(t, want, text, "section")
}
