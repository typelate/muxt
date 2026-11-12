package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/muxtjson"
)

// TestEveryGenerateFlagHasMuxtJSONKey states that the generate flags, minus
// deprecated ones and verbose, are the keys a muxt.json entry accepts. A flag
// without a key cannot be set from muxt.json, and a key without a flag is read
// into nothing. The flag set is built the way the generate command builds it.
func TestEveryGenerateFlagHasMuxtJSONKey(t *testing.T) {
	var config generate.RoutesFileConfiguration
	var deprecatedTemplatesVar string
	flags := pflag.NewFlagSet("generate", pflag.ContinueOnError)
	addGenerateFlags(flags, &config, &deprecatedTemplatesVar)

	var flagNames []string
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Deprecated == "" && f.Name != "verbose" {
			flagNames = append(flagNames, strings.ReplaceAll(f.Name, "-", "_"))
		}
	})
	keys := muxtJSONEntryKeys()

	assert.Empty(t, namesNotIn(flagNames, keys), "generate flags with no muxt.json key")
	assert.Empty(t, namesNotIn(keys, flagNames), "muxt.json keys with no generate flag")
}

// muxtJSONEntryKeys returns the json key of each field of muxtjson.Entry.
func muxtJSONEntryKeys() []string {
	var keys []string
	entry := reflect.TypeFor[muxtjson.Entry]()
	for i := range entry.NumField() {
		name, _, _ := strings.Cut(entry.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

// namesNotIn returns the names in a that are not in b, sorted.
func namesNotIn(a, b []string) []string {
	var missing []string
	for _, name := range a {
		if !slices.Contains(b, name) {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	return missing
}

// TestMuxtJSONEntryAcceptsEveryGenerateFlag states that a muxt.json entry
// holding one generate flag's key, with no flags on the command line, gives
// the same configuration as that flag alone on the command line. Each row
// is one flag given both ways in a directory with no other configuration.
// Static assets flags must be given together, so their rows hold both keys.
func TestMuxtJSONEntryAcceptsEveryGenerateFlag(t *testing.T) {
	for _, tt := range []struct {
		name  string
		args  string
		entry map[string]any
	}{
		{name: "use_templates_variable", args: "--use-templates-variable data", entry: map[string]any{"use_templates_variable": []string{"data"}}},
		{name: "use_receiver_type", args: "--use-receiver-type Server", entry: map[string]any{"use_receiver_type": "Server"}},
		{name: "use_receiver_type_package", args: "--use-receiver-type-package example.com/pkg", entry: map[string]any{"use_receiver_type_package": "example.com/pkg"}},
		{name: "output_file", args: "--output-file routes_gen.go", entry: map[string]any{"output_file": "routes_gen.go"}},
		{name: "output_receiver_interface", args: "--output-receiver-interface Receiver", entry: map[string]any{"output_receiver_interface": "Receiver"}},
		{name: "output_routes_func", args: "--output-routes-func Routes", entry: map[string]any{"output_routes_func": "Routes"}},
		{name: "output_template_data_type", args: "--output-template-data-type Data", entry: map[string]any{"output_template_data_type": "Data"}},
		{name: "output_sse_template_data_type", args: "--output-sse-template-data-type SSEData", entry: map[string]any{"output_sse_template_data_type": "SSEData"}},
		{name: "output_template_route_paths_type", args: "--output-template-route-paths-type Paths", entry: map[string]any{"output_template_route_paths_type": "Paths"}},
		{name: "output_template_route_type", args: "--output-template-route-type Route", entry: map[string]any{"output_template_route_type": "Route"}},
		{name: "output_template_route_builder_type", args: "--output-template-route-builder-type RouteBuilder", entry: map[string]any{"output_template_route_builder_type": "RouteBuilder"}},
		{name: "output_routes_func_with_logger_param", args: "--output-routes-func-with-logger-param", entry: map[string]any{"output_routes_func_with_logger_param": true}},
		{name: "output_routes_func_with_path_prefix_param", args: "--output-routes-func-with-path-prefix-param", entry: map[string]any{"output_routes_func_with_path_prefix_param": true}},
		{name: "output_routes_func_with_middleware_param", args: "--output-routes-func-with-middleware-param", entry: map[string]any{"output_routes_func_with_middleware_param": true}},
		{name: "output_multiple_files", args: "--output-multiple-files", entry: map[string]any{"output_multiple_files": true}},
		{name: "output_htmx", args: "--output-htmx", entry: map[string]any{"output_htmx": true}},
		{name: "output_datastar", args: "--output-datastar", entry: map[string]any{"output_datastar": true}},
		{name: "output_exported_default_identifiers", args: "--output-exported-default-identifiers=false", entry: map[string]any{"output_exported_default_identifiers": false}},
		{name: "output_multipart_max_memory", args: "--output-multipart-max-memory 64MB", entry: map[string]any{"output_multipart_max_memory": "64MB"}},
		{name: "output_muxt_version", args: "--output-muxt-version=false", entry: map[string]any{"output_muxt_version": false}},
		{name: "input_static_assets_directory", args: "--input-static-assets-directory assets --output-static-assets-path /static", entry: map[string]any{"input_static_assets_directory": "assets", "output_static_assets_path": "/static"}},
		{name: "output_static_assets_path", args: "--input-static-assets-directory assets --output-static-assets-path /static", entry: map[string]any{"input_static_assets_directory": "assets", "output_static_assets_path": "/static"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := moduleDir(t)
			want := generateConfigurationOf(t, dir, "generate "+tt.args)

			file, err := json.Marshal(map[string]any{"packages": map[string]any{".": tt.entry}})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "muxt.json"), file, 0o644))
			got := generateConfigurationOf(t, dir, "generate")

			assert.Equal(t, want, got, "entry %s: Config = %+v, want %+v", tt.name, got, want)
		})
	}
}

// TestMuxtJSONEntryNameSurvivesUnexportedDefaults states that an entry's
// explicit routes function name is kept when the entry turns off exported
// default identifiers. Those defaults rename every name not given explicitly,
// so the entry must mark its name as given to keep it.
func TestMuxtJSONEntryNameSurvivesUnexportedDefaults(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {".": {"output_exported_default_identifiers": false, "output_routes_func": "Mount"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "muxt.json"), []byte(file), 0o644))

	got := generateConfigurationOf(t, dir, "generate")

	assert.Equal(t, "Mount", got.RoutesFunction, "routes function")
}

// generateConfigurationOf runs a generate command line in dir and returns the
// configuration of the one job it is handed. Nothing is loaded.
func generateConfigurationOf(t *testing.T, dir, commandLine string) generate.RoutesFileConfiguration {
	t.Helper()
	var jobs []generateJob
	err := commands(dir, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []generateJob) error {
			jobs = got
			return nil
		},
	})
	require.NoError(t, err, "running %q", commandLine)
	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	return jobs[0].Config
}

// TestGenerateWithMuxtJSONEntry states that a muxt.json entry for the working
// directory configures a flagless muxt generate there. Nothing is loaded: the
// generate runner records the job it is handed.
func TestGenerateWithMuxtJSONEntry(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {".": {"use_receiver_type": "Server", "output_routes_func": "templateRoutes"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "muxt.json"), []byte(file), 0o644))

	// The defaults a flagless run gets with no muxt.json, and the same
	// version, so the only differences are the entry's two fields.
	want, err := configurationOf(t, "generate", nil)
	require.NoError(t, err)
	defaults, ok := want.(generate.RoutesFileConfiguration)
	require.True(t, ok, "configurationOf(generate) = %T, want generate.RoutesFileConfiguration", want)
	defaults.ReceiverType = "Server"
	defaults.RoutesFunction = "templateRoutes"

	var jobs []generateJob
	err = commands(dir, strings.Fields("generate"), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []generateJob) error {
			jobs = got
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	assert.Equal(t, defaults, jobs[0].Config, "generate job config")
}

// TestGenerateAppliesPackageDefaults states that a package_defaults key an
// entry leaves unset reaches that entry's configuration on a flagless muxt
// generate. Nothing is loaded: the generate runner records the job it is handed.
func TestGenerateAppliesPackageDefaults(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"package_defaults": {"use_receiver_type": "T"}, "packages": {".": {}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

	jobs := generateJobsOf(t, dir, "generate")

	require.Len(t, jobs, 1, "generate jobs")
	assert.Equal(t, "T", jobs[0].Config.ReceiverType, "receiver type")
}

// TestGenerateEntryOverridesPackageDefaults states that a key an entry sets
// itself wins over package_defaults on a flagless muxt generate. A string and
// a boolean the entry sets replace the default. A list is replaced, not
// extended, and a boolean the entry sets to false overrides a default of true.
func TestGenerateEntryOverridesPackageDefaults(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"package_defaults": {"use_receiver_type": "T"}, "packages": {".": {"use_receiver_type": "Server"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

		jobs := generateJobsOf(t, dir, "generate")

		require.Len(t, jobs, 1, "generate jobs")
		assert.Equal(t, "Server", jobs[0].Config.ReceiverType, "receiver type")
	})

	t.Run("list is replaced", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"package_defaults": {"use_templates_variable": ["templates", "pages"]}, "packages": {".": {"use_templates_variable": ["admin"]}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

		jobs := generateJobsOf(t, dir, "generate")

		require.Len(t, jobs, 1, "generate jobs")
		assert.Equal(t, []string{"admin"}, jobs[0].Config.TemplatesVariables, "templates variables")
	})

	t.Run("false overrides a default of true", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"package_defaults": {"output_routes_func_with_logger_param": true}, "packages": {".": {"output_routes_func_with_logger_param": false}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

		jobs := generateJobsOf(t, dir, "generate")

		require.Len(t, jobs, 1, "generate jobs")
		assert.False(t, jobs[0].Config.Logger, "logger parameter")
	})
}

// TestGenerateRejectsPackageDefaultsKey states that a package_defaults key
// that an entry could not have fails a flagless muxt generate, naming the key
// under package_defaults. The runner is not called.
func TestGenerateRejectsPackageDefaultsKey(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"package_defaults": {"zzz": true}, "packages": {".": {}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

		called, err := generateFailureOf(t, dir)

		var unknown *muxtjson.UnknownKeyError
		require.ErrorAs(t, err, &unknown, "error %v", err)
		assert.Equal(t, "package_defaults", unknown.Where, "where")
		assert.Equal(t, "zzz", unknown.Key, "key")
		assert.Equal(t, `muxt.json: package_defaults: unknown key "zzz"`, err.Error(), "message")
		assert.False(t, called, "generate runner called")
	})

	t.Run("dashed", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"package_defaults": {"use-receiver-type": "T"}, "packages": {".": {}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

		called, err := generateFailureOf(t, dir)

		var dashed *muxtjson.DashedKeyError
		require.ErrorAs(t, err, &dashed, "error %v", err)
		assert.Equal(t, "package_defaults", dashed.Where, "where")
		assert.Equal(t, `muxt.json: package_defaults: key "use-receiver-type" is written with underscores; write "use_receiver_type"`, err.Error(), "message")
		assert.False(t, called, "generate runner called")
	})

	t.Run("deprecated", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"package_defaults": {"receiver_type": "T"}, "packages": {".": {}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

		called, err := generateFailureOf(t, dir)

		var deprecated *muxtjson.DeprecatedKeyError
		require.ErrorAs(t, err, &deprecated, "error %v", err)
		assert.Equal(t, "package_defaults", deprecated.Where, "where")
		assert.Equal(t, `muxt.json: package_defaults: key "receiver_type" is deprecated; write "use_receiver_type"`, err.Error(), "message")
		assert.False(t, called, "generate runner called")
	})
}

// TestGenerateIgnoresMuxtJSONAboveModuleRoot states that a muxt.json in a
// directory above the module root is not read: the walk stops at the directory
// holding go.mod. Nothing is loaded: the generate runner records the job it is
// handed.
func TestGenerateIgnoresMuxtJSONAboveModuleRoot(t *testing.T) {
	parent := moduleDir(t)
	module := filepath.Join(parent, "module")
	require.NoError(t, os.Mkdir(module, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(module, "go.mod"), []byte("module example.com/m\n"), 0o644))
	const file = `{"packages": {"module": {"use_receiver_type": "T1"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(parent, "muxt.json"), []byte(file), 0o644))

	var jobs []generateJob
	err := commands(module, strings.Fields("generate"), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []generateJob) error {
			jobs = got
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	assert.Equal(t, "", jobs[0].Key, "generate job key")
	assert.Equal(t, "", jobs[0].Config.ReceiverType, "generate job receiver type")
}

// TestGenerateRejectsMalformedMuxtJSON states that a muxt.json that is not
// valid JSON fails a flagless muxt generate at the byte the parser rejects,
// naming the file and the line and column. The runner is not called.
func TestGenerateRejectsMalformedMuxtJSON(t *testing.T) {
	dir := moduleDir(t)
	const file = "{\n  \"packages\": {\n    \".\": x\n  }\n}\n"
	path := filepath.Join(dir, muxtjson.FileName)
	require.NoError(t, os.WriteFile(path, []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var parseErr *muxtjson.ParseError
	require.ErrorAs(t, err, &parseErr, "error %v", err)
	assert.Equal(t, muxtjson.FileName, parseErr.Path, "path")
	assert.Equal(t, 3, parseErr.Line, "line")
	assert.Equal(t, 10, parseErr.Col, "column")
	assert.Equal(t, "muxt.json:3:10: invalid character 'x' looking for beginning of value", err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsMalformedMuxtJSONInParentDirectory states that a muxt.json
// in the parent of the working directory that is not valid JSON fails a
// flagless muxt generate, naming the file relative to the working directory.
// The runner is not called.
func TestGenerateRejectsMalformedMuxtJSONInParentDirectory(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = "{\n  \"packages\": {\n    \"workspace\": x\n  }\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(parent, muxtjson.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, workspace)

	var parseErr *muxtjson.ParseError
	require.ErrorAs(t, err, &parseErr, "error %v", err)
	assert.Equal(t, filepath.Join("..", muxtjson.FileName), parseErr.Path, "path")
	assert.Equal(t, 3, parseErr.Line, "line")
	assert.Equal(t, 18, parseErr.Col, "column")
	assert.Equal(t, "../muxt.json:3:18: invalid character 'x' looking for beginning of value", err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsUnknownMuxtJSONKey states that a muxt.json entry key no
// generate flag reads fails a flagless muxt generate, naming the key, its
// entry, and the file. The runner is not called.
func TestGenerateRejectsUnknownMuxtJSONKey(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {".": {"nope": true}}}`
	path := filepath.Join(dir, muxtjson.FileName)
	require.NoError(t, os.WriteFile(path, []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var unknown *muxtjson.UnknownKeyError
	require.ErrorAs(t, err, &unknown, "error %v", err)
	assert.Equal(t, muxtjson.FileName, unknown.Path, "path")
	assert.Equal(t, `packages["."]`, unknown.Where, "where")
	assert.Equal(t, "nope", unknown.Key, "key")
	assert.Equal(t, `muxt.json: packages["."]: unknown key "nope"`, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsDeprecatedMuxtJSONKey states that a muxt.json entry key
// that is a deprecated flag name fails a flagless muxt generate, naming the
// key and the key to write instead. The runner is not called.
func TestGenerateRejectsDeprecatedMuxtJSONKey(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {".": {"receiver_type": "Server"}}}`
	path := filepath.Join(dir, muxtjson.FileName)
	require.NoError(t, os.WriteFile(path, []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var deprecated *muxtjson.DeprecatedKeyError
	require.ErrorAs(t, err, &deprecated, "error %v", err)
	assert.Equal(t, muxtjson.FileName, deprecated.Path, "path")
	assert.Equal(t, `packages["."]`, deprecated.Where, "where")
	assert.Equal(t, "receiver_type", deprecated.Key, "key")
	assert.Equal(t, "use_receiver_type", deprecated.Replacement, "replacement")
	assert.Equal(t, `muxt.json: packages["."]: key "receiver_type" is deprecated; write "use_receiver_type"`, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateMuxtJSONKeysUseUnderscores states that a muxt.json entry key is
// the flag name without "--" with each "-" written as "_". A key with the
// underscores reaches the generator. A key with dashes, which the underscore
// form would accept, fails a flagless muxt generate naming the underscore form.
func TestGenerateMuxtJSONKeysUseUnderscores(t *testing.T) {
	t.Run("underscored keys reach the generator", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"packages": {".": {"use_receiver_type": "Server", "output_routes_func": "Mount"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

		got := generateConfigurationOf(t, dir, "generate")

		assert.Equal(t, "Server", got.ReceiverType, "receiver type")
		assert.Equal(t, "Mount", got.RoutesFunction, "routes function")
	})

	t.Run("dashed key fails naming the underscore form", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"packages": {".": {"use-receiver-type": "Server"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

		called, err := generateFailureOf(t, dir)

		var dashed *muxtjson.DashedKeyError
		require.ErrorAs(t, err, &dashed, "error %v", err)
		assert.Equal(t, muxtjson.FileName, dashed.Path, "path")
		assert.Equal(t, `packages["."]`, dashed.Where, "where")
		assert.Equal(t, "use-receiver-type", dashed.Key, "key")
		assert.Equal(t, "use_receiver_type", dashed.Underscored, "underscored")
		assert.Equal(t, `muxt.json: packages["."]: key "use-receiver-type" is written with underscores; write "use_receiver_type"`, err.Error(), "message")
		assert.False(t, called, "generate runner called")
	})
}

// TestGenerateRejectsMuxtJSONEntryFlagConflict states that a muxt.json entry
// whose values a generate flag rejects fails a flagless muxt generate with the
// flag's own error under the entry's location. The runner is not called.
func TestGenerateRejectsMuxtJSONEntryFlagConflict(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {".": {"output_htmx": true, "output_datastar": true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	const flagMessage = "--output-htmx and --output-datastar are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)"
	var invalid *muxtjson.InvalidEntryError
	require.ErrorAs(t, err, &invalid, "error %v", err)
	assert.Equal(t, muxtjson.FileName, invalid.Path, "path")
	assert.Equal(t, `packages["."]`, invalid.Where, "where")
	assert.EqualError(t, invalid.Err, flagMessage, "flag error")
	assert.Equal(t, `muxt.json: packages["."]: `+flagMessage, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsMuxtJSONMultipartMaxMemoryValue states that a muxt.json
// entry whose output_multipart_max_memory the flag cannot parse fails a
// flagless muxt generate, naming the file once, the entry, and the key, then
// the flag's own error. The runner is not called.
func TestGenerateRejectsMuxtJSONMultipartMaxMemoryValue(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {".": {"output_multipart_max_memory": "lots"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var flagCfg generate.RoutesFileConfiguration
	flagErr := (&multipartMaxMemoryFlag{cfg: &flagCfg}).Set("lots")
	require.Error(t, flagErr)
	var invalid *muxtjson.InvalidEntryError
	require.ErrorAs(t, err, &invalid, "error %v", err)
	assert.Equal(t, muxtjson.FileName, invalid.Path, "path")
	assert.Equal(t, `packages["."]`, invalid.Where, "where")
	assert.Equal(t, `muxt.json: packages["."]: output_multipart_max_memory: `+flagErr.Error(), err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsUncleanMuxtJSONKey states that a muxt.json packages key
// not in clean form fails a muxt generate, even one with flags, naming the key
// and its clean form. The runner is not called.
func TestGenerateRejectsUncleanMuxtJSONKey(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {"./workspace/": {}}}`
	path := filepath.Join(dir, muxtjson.FileName)
	require.NoError(t, os.WriteFile(path, []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var keyForm *muxtjson.KeyFormError
	require.ErrorAs(t, err, &keyForm, "error %v", err)
	assert.Equal(t, muxtjson.FileName, keyForm.Path, "path")
	assert.Equal(t, "./workspace/", keyForm.Key, "key")
	assert.Equal(t, "workspace", keyForm.Clean, "clean form")
	assert.Equal(t, `muxt.json: packages key "./workspace/" is not in clean form; write "workspace"`, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsDirectoryWithoutMuxtJSONEntry states that a flagless muxt
// generate in a directory below a muxt.json with no entry for it fails, naming
// the file and the missing key. With a generate flag the same directory is
// generated from the flags. The runner is not called for the failure.
func TestGenerateRejectsDirectoryWithoutMuxtJSONEntry(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {".": {}}}`
	require.NoError(t, os.WriteFile(filepath.Join(parent, muxtjson.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, workspace)

	var missing *muxtjson.MissingEntryError
	require.ErrorAs(t, err, &missing, "error %v", err)
	assert.Equal(t, filepath.Join("..", muxtjson.FileName), missing.Path, "path")
	assert.Equal(t, "workspace", missing.Key, "key")
	assert.Equal(t, `../muxt.json: no packages entry "workspace"; add one to generate this directory`, err.Error(), "message")
	assert.False(t, called, "generate runner called")

	config := generateConfigurationOf(t, workspace, "generate --use-receiver-type T1")
	assert.Equal(t, "T1", config.ReceiverType, "receiver type from flags")
}

// TestGenerateRejectsFlagsForConfiguredDirectory states that a generate flag
// given where muxt.json has an entry for the working directory fails, telling
// the person to configure the directory in the file. The flag names are the
// ones given, sorted. The runner is not called.
func TestGenerateRejectsFlagsForConfiguredDirectory(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {"workspace": {}}}`
	require.NoError(t, os.WriteFile(filepath.Join(parent, muxtjson.FileName), []byte(file), 0o644))

	called, err := generateFailureOfLine(t, workspace, "generate --output-htmx")

	var configured *muxtjson.ConfiguredDirectoryError
	require.ErrorAs(t, err, &configured, "error %v", err)
	assert.Equal(t, filepath.Join("..", muxtjson.FileName), configured.Path, "path")
	assert.Equal(t, []string{"output-htmx"}, configured.Flags, "flags")
	assert.Equal(t, "configure generate with fields in ../muxt.json instead of flags", err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateVerboseGeneratesFromConfiguredDirectory states that -v with no
// other generate flag, in a directory its muxt.json entry configures, generates
// from that entry. Verbose is not a generate setting, so it does not reject the
// entry. The runner records the job it is handed.
func TestGenerateVerboseGeneratesFromConfiguredDirectory(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {".": {"use_receiver_type": "Server"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

	var jobs []generateJob
	err := commands(dir, strings.Fields("generate -v"), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []generateJob) error {
			jobs = got
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	assert.Equal(t, ".", jobs[0].Key, "generate job key")
	assert.Equal(t, dir, jobs[0].Dir, "generate job dir")
	assert.Equal(t, "Server", jobs[0].Config.ReceiverType, "generate job receiver type")
}

// TestGenerateVerboseSetsEveryEntryJob states that muxt generate -v in a
// muxt.json directory with entries for "." and "workspace" hands the runner
// jobs with verbose on, and that without -v they have it off. Verbose is a run
// setting, so it comes from the command line and no entry sets it.
func TestGenerateVerboseSetsEveryEntryJob(t *testing.T) {
	for _, tc := range []struct {
		commandLine string
		want        bool
	}{
		{commandLine: "generate -v", want: true},
		{commandLine: "generate", want: false},
	} {
		t.Run(tc.commandLine, func(t *testing.T) {
			dir := moduleDir(t)
			require.NoError(t, os.Mkdir(filepath.Join(dir, "workspace"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(`{"packages": {".": {}, "workspace": {}}}`), 0o644))

			jobs := generateJobsOf(t, dir, tc.commandLine)

			require.Len(t, jobs, 2, "got %d generate jobs, want 2", len(jobs))
			for _, job := range jobs {
				assert.Equal(t, tc.want, job.Config.Verbose, "verbose of the %s job", job.Key)
			}
		})
	}
}

// TestGenerateChangeDirectorySelectsMuxtJSONEntry states that muxt -C workspace
// generate, run from the directory holding a muxt.json with entries for the
// directory and for workspace, generates only the workspace entry. -C is not a
// generate flag, so it does not reject the entries. The runner records the job
// it is handed.
func TestGenerateChangeDirectorySelectsMuxtJSONEntry(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {".": {"use_receiver_type": "Root"}, "workspace": {"use_receiver_type": "T1"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(parent, muxtjson.FileName), []byte(file), 0o644))

	var jobs []generateJob
	err := commands(parent, strings.Fields("-C workspace generate"), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []generateJob) error {
			jobs = got
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	assert.Equal(t, "workspace", jobs[0].Key, "generate job key")
	assert.Equal(t, workspace, jobs[0].Dir, "generate job dir")
	assert.Equal(t, "T1", jobs[0].Config.ReceiverType, "generate job receiver type")
}

// TestGenerateEveryMuxtJSONEntry states that a flagless muxt generate in the
// directory holding a muxt.json hands the generate runner one job per entry,
// in key order, each configured by its own entry. Nothing is loaded.
func TestGenerateEveryMuxtJSONEntry(t *testing.T) {
	dir := moduleDir(t)
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {"workspace": {"use_receiver_type": "T1"}, ".": {"use_receiver_type": "Root"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

	jobs := generateJobsOf(t, dir, "generate")

	require.Len(t, jobs, 2, "generate jobs")
	assert.Equal(t, ".", jobs[0].Key, "first job key")
	assert.Equal(t, dir, jobs[0].Dir, "first job dir")
	assert.Equal(t, "Root", jobs[0].Config.ReceiverType, "first job receiver type")
	assert.Equal(t, "workspace", jobs[1].Key, "second job key")
	assert.Equal(t, workspace, jobs[1].Dir, "second job dir")
	assert.Equal(t, "T1", jobs[1].Config.ReceiverType, "second job receiver type")
}

// TestGenerateEveryMuxtJSONEntryValidatesEachEntry states that a flagless muxt
// generate in the muxt.json directory fails when any entry has values a flag
// rejects, naming that entry, and the generate runner is not called.
func TestGenerateEveryMuxtJSONEntryValidatesEachEntry(t *testing.T) {
	dir := moduleDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "workspace"), 0o755))
	const file = `{"packages": {".": {"use_receiver_type": "Root"}, "workspace": {"output_htmx": true, "output_datastar": true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	const flagMessage = "--output-htmx and --output-datastar are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)"
	var invalid *muxtjson.InvalidEntryError
	require.ErrorAs(t, err, &invalid, "error %v", err)
	assert.Equal(t, muxtjson.FileName, invalid.Path, "path")
	assert.Equal(t, `packages["workspace"]`, invalid.Where, "where")
	assert.Equal(t, `muxt.json: packages["workspace"]: `+flagMessage, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateMuxtJSONWithOnlyWorkspaceEntry states that a muxt.json whose only
// entry is workspace, run in the muxt.json directory, generates workspace alone:
// the directory itself has no entry, so nothing is generated for it.
func TestGenerateMuxtJSONWithOnlyWorkspaceEntry(t *testing.T) {
	dir := moduleDir(t)
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {"workspace": {"use_receiver_type": "T1"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

	jobs := generateJobsOf(t, dir, "generate")

	require.Len(t, jobs, 1, "generate jobs")
	assert.Equal(t, "workspace", jobs[0].Key, "job key")
	assert.Equal(t, workspace, jobs[0].Dir, "job dir")
	assert.Equal(t, "T1", jobs[0].Config.ReceiverType, "job receiver type")
}

// TestGenerateInSubdirectorySkipsOtherEntries states that a flagless muxt
// generate in workspace, a sub-directory, generates only workspace's entry. An
// invalid entry for the muxt.json directory does not fail that run.
func TestGenerateInSubdirectorySkipsOtherEntries(t *testing.T) {
	dir := moduleDir(t)
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {".": {"output_htmx": true, "output_datastar": true}, "workspace": {"use_receiver_type": "T1"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

	jobs := generateJobsOf(t, workspace, "generate")

	require.Len(t, jobs, 1, "generate jobs")
	assert.Equal(t, "workspace", jobs[0].Key, "job key")
	assert.Equal(t, workspace, jobs[0].Dir, "job dir")
	assert.Equal(t, "T1", jobs[0].Config.ReceiverType, "job receiver type")
}

// TestGenerateRejectsMissingMuxtJSONDirectory states that a muxt.json entry for
// a directory that does not exist, or that is a file, fails a flagless muxt
// generate in the muxt.json directory, naming the key before any entry is
// generated. The generate runner is not called.
func TestGenerateRejectsMissingMuxtJSONDirectory(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{name: "missing", setup: func(*testing.T, string) {}},
		{name: "file", setup: func(t *testing.T, dir string) {
			require.NoError(t, os.WriteFile(filepath.Join(dir, "gone"), []byte("not a directory\n"), 0o644))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := moduleDir(t)
			tt.setup(t, dir)
			const file = `{"packages": {".": {"use_receiver_type": "Root"}, "gone": {"use_receiver_type": "T1"}}}`
			require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

			called, err := generateFailureOf(t, dir)

			var missing *muxtjson.MissingDirectoryError
			require.ErrorAs(t, err, &missing, "error %v", err)
			assert.Equal(t, muxtjson.FileName, missing.Path, "path")
			assert.Equal(t, "gone", missing.Key, "key")
			assert.Equal(t, "gone", missing.Dir, "dir")
			assert.Equal(t, `muxt.json: packages["gone"]: directory gone does not exist`, err.Error(), "message")
			assert.False(t, called, "generate runner called")
		})
	}
}

// generateJobsOf runs a generate command line in dir and returns the jobs the
// generate runner is handed. Nothing is loaded.
func generateJobsOf(t *testing.T, dir, commandLine string) []generateJob {
	t.Helper()
	var jobs []generateJob
	err := commands(dir, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []generateJob) error {
			jobs = got
			return nil
		},
	})
	require.NoError(t, err, "running %q", commandLine)
	return jobs
}

// generateFailureOf runs a flagless muxt generate in dir. It returns whether
// the generate runner was called and the error the command failed with.
func generateFailureOf(t *testing.T, dir string) (called bool, err error) {
	t.Helper()
	return generateFailureOfLine(t, dir, "generate")
}

// generateFailureOfLine runs a muxt command line in dir and returns whether the
// generate runner was called and the error the command failed with.
func generateFailureOfLine(t *testing.T, dir, commandLine string) (called bool, err error) {
	t.Helper()
	err = commands(dir, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, _ []generateJob) error {
			called = true
			return nil
		},
	})
	require.Error(t, err, "muxt generate in %s", dir)
	return called, err
}

// TestGenerateWithMuxtJSONInParentDirectory states that a flagless muxt generate
// in a sub-directory uses the entry keyed by that directory's path relative to
// the muxt.json found in a parent. Nothing is loaded: the generate runner
// records the job it is handed.
func TestGenerateWithMuxtJSONInParentDirectory(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {"workspace": {"use_receiver_type": "T1"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(parent, "muxt.json"), []byte(file), 0o644))

	var jobs []generateJob
	err := commands(workspace, strings.Fields("generate"), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []generateJob) error {
			jobs = got
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	assert.Equal(t, "workspace", jobs[0].Key, "generate job key")
	assert.Equal(t, workspace, jobs[0].Dir, "generate job dir")
	assert.Equal(t, "T1", jobs[0].Config.ReceiverType, "generate job receiver type")
}

// TestGenerateRejectsMuxtJSONKeyOutsideDirectory states that a muxt.json
// packages key that leaves the file's directory fails a muxt generate, naming
// the key, so no job directory is built from it. The runner is not called.
func TestGenerateRejectsMuxtJSONKeyOutsideDirectory(t *testing.T) {
	for _, key := range []string{"../other", "/abs", ".."} {
		t.Run(key, func(t *testing.T) {
			dir := moduleDir(t)
			file, err := json.Marshal(map[string]any{"packages": map[string]any{key: map[string]any{}}})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), file, 0o644))

			called, err := generateFailureOf(t, dir)

			var outside *muxtjson.KeyOutsideError
			require.ErrorAs(t, err, &outside, "error %v", err)
			assert.Equal(t, muxtjson.FileName, outside.Path, "path")
			assert.Equal(t, key, outside.Key, "key")
			assert.Equal(t, `muxt.json: packages key "`+key+`" is outside the file's directory`, err.Error(), "message")
			assert.False(t, called, "generate runner called")
		})
	}
}

// TestGenerateRejectsMuxtJSONLinkOutsideDirectory states that a packages key
// whose directory is a symbolic link to a directory outside the muxt.json's
// directory fails a flagless muxt generate, naming the key, before the generate
// runner is called.
func TestGenerateRejectsMuxtJSONLinkOutsideDirectory(t *testing.T) {
	root := moduleDir(t)
	project := filepath.Join(root, "project")
	elsewhere := filepath.Join(root, "elsewhere")
	require.NoError(t, os.Mkdir(project, 0o755))
	require.NoError(t, os.Mkdir(elsewhere, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, muxtjson.FileName), []byte(`{"packages": {"linked": {}}}`), 0o644))
	if err := os.Symlink(elsewhere, filepath.Join(project, "linked")); err != nil {
		t.Skipf("creating a symbolic link: %v", err)
	}

	called, err := generateFailureOf(t, project)

	var outside *muxtjson.OutsideDirectoryError
	require.ErrorAs(t, err, &outside, "error %v", err)
	assert.Equal(t, muxtjson.FileName, outside.Path, "path")
	assert.Equal(t, "linked", outside.Key, "key")
	assert.Equal(t, "linked", outside.Dir, "dir")
	assert.Equal(t, `muxt.json: packages["linked"]: directory linked resolves outside the file's directory`, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateAcceptsMuxtJSONLinkInsideDirectory states that a packages key
// whose directory is a symbolic link to another directory inside the muxt.json's
// directory is a job like any other. The runner records the job it is handed.
func TestGenerateAcceptsMuxtJSONLinkInsideDirectory(t *testing.T) {
	dir := moduleDir(t)
	inside := filepath.Join(dir, "inside")
	require.NoError(t, os.Mkdir(inside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(`{"packages": {"linked": {}}}`), 0o644))
	if err := os.Symlink(inside, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("creating a symbolic link: %v", err)
	}

	jobs := generateJobsOf(t, dir, "generate")

	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	assert.Equal(t, "linked", jobs[0].Key, "generate job key")
}
