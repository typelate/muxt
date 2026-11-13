package cli

import (
	"bytes"
	"encoding/json"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/composite"
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))
}

// TestRoutesListsMuxtJSONEntry states that bare muxt with no flags, in a
// directory a muxt.json entry configures, lists with the entry's templates
// variable and receiver type, with package_defaults applied. The shared flags
// that run settings (-v, --format) do not count as configuration.
func TestRoutesListsMuxtJSONEntry(t *testing.T) {
	dir := moduleDir(t)
	writeMuxtJSON(t, dir, `{"package_defaults": {"use_receiver_type_package": "example.com/m/types"}, "packages": {"example.com/m": {"use_templates_variable": ["pages"], "use_receiver_type": "Server"}}}`)

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
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {"example.com/m/workspace": {}}}`)

	jobs, err := routesJobsOf(t, workspace, "--use-receiver-type=Server --use-templates-variable=pages")

	var configured *composite.ConfiguredDirectoryError
	require.ErrorAs(t, err, &configured, "error %v", err)
	assert.Equal(t, filepath.Join(parent, composite.FileName), configured.Path, "path")
	assert.Equal(t, []string{"use-receiver-type", "use-templates-variable"}, configured.Flags, "flags")
	assert.Equal(t, "configure muxt with fields in "+filepath.Join(parent, composite.FileName)+" instead of flags", err.Error(), "message")
	assert.Empty(t, jobs, "jobs")
}

// TestRoutesAcceptsDirectoryWithoutMuxtJSONEntry states that bare muxt in a
// directory below a muxt.json with no entry for it lists with its own flags and
// defaults, as it does with no muxt.json.
func TestRoutesAcceptsDirectoryWithoutMuxtJSONEntry(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {"example.com/m": {}}}`)

	jobs, err := routesJobsOf(t, workspace, "")

	require.NoError(t, err, "without flags")
	require.Len(t, jobs, 1, "jobs without flags")
	assert.Equal(t, []string{"templates"}, jobs[0].definitions().TemplatesVariables, "default templates variables")

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
	dir := moduleDir(t)
	writeMuxtJSON(t, dir, `{"packages": {"example.com/m": {"use_receiver_type": "Server"}}}`)

	text := reportOf(t, dir, "")

	assert.True(t, strings.HasPrefix(text, "Configuration:\n"), "section starts with its heading:\n%s", text)
	for _, key := range muxtJSONEntryKeys() {
		assert.Equal(t, 1, strings.Count(text, "\""+key+"\": "), "lines for %q in:\n%s", key, text)
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
// entry sets is under the heading naming the file by its absolute path, and
// the entry's key, written as its muxt.json key and value, and
// that the setting is not repeated under another heading.
func TestRoutesConfigurationGroupsEntrySettings(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"packages": {"example.com/m/workspace": {"use_receiver_type": "T1"}}}`)

	text := reportOf(t, workspace, "")

	want := "Configuration:\n  " + filepath.Join(parent, composite.FileName) + ":\n    packages[\"example.com/m/workspace\"]:\n      \"use_receiver_type\": \"T1\"\n  defaults:\n"
	assert.True(t, strings.HasPrefix(text, want), "section starts with\n%s\ngot\n%s", want, text)
	assert.Equal(t, 1, strings.Count(text, "\"use_receiver_type\":"), "lines for use_receiver_type")
}

// TestRoutesConfigurationJSONNamesMuxtJSONFile states that with --format json
// the two muxt.json groups hold the file's path in File, and their Source is the
// packages key or package_defaults without it, while the defaults group has no File.
func TestRoutesConfigurationJSONNamesMuxtJSONFile(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	writeMuxtJSON(t, parent, `{"package_defaults": {"output_htmx": true}, "packages": {"example.com/m/workspace": {"use_receiver_type": "T1"}}}`)
	jobs, err := routesJobsOf(t, workspace, "")
	require.NoError(t, err, "muxt")
	require.NotEmpty(t, jobs, "jobs")

	data, err := json.Marshal(jobs[0].Report)
	require.NoError(t, err, "marshaling the report")

	var groups []struct{ Source, File string }
	require.NoError(t, json.Unmarshal(data, &groups), "groups:\n%s", data)
	want := []struct{ Source, File string }{
		{Source: `packages["example.com/m/workspace"]`, File: filepath.Join(parent, composite.FileName)},
		{Source: "package_defaults", File: filepath.Join(parent, composite.FileName)},
		{Source: "defaults"},
	}
	assert.Equal(t, want, groups, "groups")
}

// TestRoutesConfigurationDefaults states that where no muxt.json is found the
// section is the single heading defaults, holding every setting with the value
// generate would use after defaulting, in the order muxt generate --help lists
// the flags.
func TestRoutesConfigurationDefaults(t *testing.T) {
	dir := moduleDir(t)
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

// TestRoutesConfigurationGroupsPackageDefaults states that a setting that comes
// from package_defaults is under the heading naming the file and package_defaults,
// between the packages heading and defaults, and that a setting the entry sets
// itself stays under the packages heading.
func TestRoutesConfigurationGroupsPackageDefaults(t *testing.T) {
	dir := moduleDir(t)
	writeMuxtJSON(t, dir, `{"package_defaults": {"output_htmx": true, "use_receiver_type": "T0"}, "packages": {"example.com/m": {"use_receiver_type": "T1"}}}`)

	text := reportOf(t, dir, "")

	want := "Configuration:\n  " + filepath.Join(dir, composite.FileName) + ":\n    packages[\"example.com/m\"]:\n      \"use_receiver_type\": \"T1\"\n    package_defaults:\n      \"output_htmx\": true\n  defaults:\n"
	assert.True(t, strings.HasPrefix(text, want), "section starts with\n%s\ngot\n%s", want, text)
}

// TestRoutesListsEveryMuxtJSONEntryFromOneLoad states that bare muxt with no
// flags in a muxt.json directory with entries for "." and "workspace" prints each
// entry's key on its own line followed by that package's sections, from one
// package load, in key order; and in import order when one package imports
// another, so the imported one is listed first.
func TestRoutesListsEveryMuxtJSONEntryFromOneLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	for _, tt := range []struct {
		name    string
		imports map[string][]string
		order   []string
	}{
		{name: "key order", order: []string{".", "workspace"}},
		{name: "import order", imports: map[string][]string{".": {"workspace"}}, order: []string{"workspace", "."}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := entryModule(t, map[string]string{".": "Root", "workspace": "Space"}, tt.imports)

			stdout, loads, err := listWithLoader(t, dir)

			require.NoError(t, err, "muxt")
			assert.Equal(t, 1, loads, "package loads")
			sections := map[string]string{".": "GET /Root RootPage()", "workspace": "GET /Space SpacePage()"}
			var want []string
			for _, key := range tt.order {
				want = append(want, `\n*`+regexp.QuoteMeta(importPath(key))+`\nConfiguration:\n.*Template Routes:\n  - `+regexp.QuoteMeta(sections[key])+`\n.*Template Functions:\n`)
			}
			assert.Regexp(t, `(?s)\A`+strings.Join(want, "")+`\n*\z`, stdout, "stdout")
		})
	}
}

// TestRoutesListsOnlyTheEntriesAMuxtJSONHas states that bare muxt in the
// directory of a muxt.json with no "." entry lists the entries it has, and that
// one in a sub-directory prints that package's sections with no key line, the
// Configuration section naming the file.
func TestRoutesListsOnlyTheEntriesAMuxtJSONHas(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{"workspace": "Space", "other": "Other"}, nil)

	atTop, _, err := listWithLoader(t, dir)
	require.NoError(t, err, "muxt at the top")
	assert.Regexp(t, `\A(?s:example\.com/m/other\nConfiguration:\n.*Template Routes:\n  - GET /Other OtherPage\(\)\n.*example\.com/m/workspace\nConfiguration:\n.*Template Routes:\n  - GET /Space SpacePage\(\)\n.*)\z`, atTop, "stdout at the top")
	assert.NotContains(t, atTop, "\nexample.com/m\n", "a key the file does not have")

	below, _, err := listWithLoader(t, filepath.Join(dir, "workspace"))
	require.NoError(t, err, "muxt in workspace")
	assert.Regexp(t, `\A(?s:Configuration:\n  `+regexp.QuoteMeta(filepath.Join(dir, composite.FileName))+`:\n    packages\["example\.com/m/workspace"\]:\n      "use_receiver_type": "Space"\n  defaults:\n.*Template Routes:\n  - GET /Space SpacePage\(\)\n.*)\z`, below, "stdout in workspace")
}

// TestRoutesConfigurationGroupsFlags states that a setting given as a flag, in
// a directory no muxt.json entry configures, is under the heading flags, ahead
// of defaults, so it is not reported as a default it overrode.
func TestRoutesConfigurationGroupsFlags(t *testing.T) {
	dir := moduleDir(t)
	writeTestFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.24\n")

	text := reportOf(t, dir, "--use-receiver-type=T1 --use-templates-variable=pages")

	const want = "Configuration:\n  flags:\n    \"use_receiver_type\": \"T1\"\n    \"use_templates_variable\": [\"pages\"]\n  defaults:\n"
	assert.True(t, strings.HasPrefix(text, want), "section starts with\n%s\ngot\n%s", want, text)
	assert.Equal(t, 1, strings.Count(text, "\"use_receiver_type\":"), "lines for use_receiver_type")
}

// TestRoutesJSONNamesEachEntry states that with --format json a listing over
// several entries gives each listing a Key member holding its packages key, and
// a listing over one package has none.
func TestRoutesJSONNamesEachEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	keysOf := func(t *testing.T, dir string) []string {
		t.Helper()
		stdout, _, err := listWithLoader(t, dir, "--format", "json")
		require.NoError(t, err, "muxt --format json")
		var listings []struct{ Key string }
		if strings.HasPrefix(stdout, "[") {
			require.NoError(t, json.Unmarshal([]byte(stdout), &listings), "output:\n%s", stdout)
		} else {
			var listing struct{ Key string }
			require.NoError(t, json.Unmarshal([]byte(stdout), &listing), "output:\n%s", stdout)
			listings = append(listings, listing)
		}
		var keys []string
		for _, listing := range listings {
			keys = append(keys, listing.Key)
		}
		return keys
	}

	dir := entryModule(t, map[string]string{".": "Root", "workspace": "Space"}, nil)
	assert.Equal(t, []string{testModule, testModule + "/workspace"}, keysOf(t, dir), "keys over two entries")
	assert.Equal(t, []string{""}, keysOf(t, filepath.Join(dir, "workspace")), "keys over one package")
}

// TestRoutesJSONOverSeveralEntriesIsOneDocument states that with --format json
// a listing over several entries is one JSON array holding each package's
// object in import order, and a listing over one package is the object alone.
func TestRoutesJSONOverSeveralEntriesIsOneDocument(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := entryModule(t, map[string]string{".": "Root", "workspace": "Space"}, nil)

	stdout, _, err := listWithLoader(t, dir, "--format", "json")
	require.NoError(t, err, "muxt --format json")
	var several []struct{ Key string }
	require.NoError(t, json.Unmarshal([]byte(stdout), &several), "output:\n%s", stdout)
	require.Len(t, several, 2, "output:\n%s", stdout)
	assert.Equal(t, []string{testModule, testModule + "/workspace"}, []string{several[0].Key, several[1].Key}, "keys")

	stdout, _, err = listWithLoader(t, filepath.Join(dir, "workspace"), "--format", "json")
	require.NoError(t, err, "muxt --format json in one package")
	var one struct{ Key string }
	require.NoError(t, json.Unmarshal([]byte(stdout), &one), "output:\n%s", stdout)
}
