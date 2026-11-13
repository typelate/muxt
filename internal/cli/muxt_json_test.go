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

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
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

// muxtJSONEntryKeys returns the json key of each field of composite.Entry.
func muxtJSONEntryKeys() []string {
	var keys []string
	entry := reflect.TypeFor[composite.Entry]()
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
	// A static assets setting is rejected without the other, so each of
	// their rows gives both.
	companion := map[string]string{
		inputStaticAssetsDirectory: outputStaticAssetsPath,
		outputStaticAssetsPath:     inputStaticAssetsDirectory,
	}
	settings := generateSettings(new(generate.RoutesFileConfiguration))
	for _, s := range settings {
		given := []setting{s}
		if other, ok := companion[s.name]; ok {
			i := slices.IndexFunc(settings, func(o setting) bool { return o.name == other })
			require.GreaterOrEqual(t, i, 0, "companion %s of %s is not a setting", other, s.name)
			given = append(given, settings[i])
		}
		t.Run(muxtJSONKey(s.name), func(t *testing.T) {
			var args []string
			entry := map[string]any{}
			for _, g := range given {
				arg, value := exampleOf(t, g)
				args = append(args, arg)
				entry[muxtJSONKey(g.name)] = value
			}

			dir := moduleDir(t)
			want := generateConfigurationOf(t, dir, "generate "+strings.Join(args, " "))

			file, err := json.Marshal(map[string]any{"packages": map[string]any{testModule: entry}})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "muxt.json"), file, 0o644))
			got := generateConfigurationOf(t, dir, "generate")

			assert.Equal(t, want, got, "entry %v: Config = %+v, want %+v", entry, got, want)
		})
	}
}

// TestMuxtJSONEntryNameSurvivesUnexportedDefaults states that an entry's
// explicit routes function name is kept when the entry turns off exported
// default identifiers. Those defaults rename every name not given explicitly,
// so the entry must mark its name as given to keep it.
func TestMuxtJSONEntryNameSurvivesUnexportedDefaults(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {"example.com/m": {"output_exported_default_identifiers": false, "output_routes_func": "Mount"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "muxt.json"), []byte(file), 0o644))

	got := generateConfigurationOf(t, dir, "generate")

	assert.Equal(t, "Mount", got.RoutesFunction, "routes function")
}

// generateConfigurationOf runs a generate command line in dir and returns the
// configuration of the one job it is handed. Nothing is loaded.
func generateConfigurationOf(t *testing.T, dir, commandLine string) generate.RoutesFileConfiguration {
	t.Helper()
	var jobs []packageJob
	err := commands(dir, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []packageJob) error {
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
	const file = `{"packages": {"example.com/m": {"use_receiver_type": "Server", "output_routes_func": "templateRoutes"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "muxt.json"), []byte(file), 0o644))

	// The defaults a flagless run gets with no muxt.json, and the same
	// version, so the only differences are the entry's two fields.
	want, err := configurationOf(t, "generate", nil)
	require.NoError(t, err)
	defaults, ok := want.(generate.RoutesFileConfiguration)
	require.True(t, ok, "configurationOf(generate) = %T, want generate.RoutesFileConfiguration", want)
	defaults.ReceiverType = "Server"
	defaults.RoutesFunction = "templateRoutes"

	var jobs []packageJob
	err = commands(dir, strings.Fields("generate"), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []packageJob) error {
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
	const file = `{"package_defaults": {"use_receiver_type": "T"}, "packages": {"example.com/m": {}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

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
		const file = `{"package_defaults": {"use_receiver_type": "T"}, "packages": {"example.com/m": {"use_receiver_type": "Server"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

		jobs := generateJobsOf(t, dir, "generate")

		require.Len(t, jobs, 1, "generate jobs")
		assert.Equal(t, "Server", jobs[0].Config.ReceiverType, "receiver type")
	})

	t.Run("list is replaced", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"package_defaults": {"use_templates_variable": ["templates", "pages"]}, "packages": {"example.com/m": {"use_templates_variable": ["admin"]}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

		jobs := generateJobsOf(t, dir, "generate")

		require.Len(t, jobs, 1, "generate jobs")
		assert.Equal(t, []string{"admin"}, jobs[0].Config.TemplatesVariables, "templates variables")
	})

	t.Run("false overrides a default of true", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"package_defaults": {"output_routes_func_with_logger_param": true}, "packages": {"example.com/m": {"output_routes_func_with_logger_param": false}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

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
		const file = `{"package_defaults": {"zzz": true}, "packages": {"example.com/m": {}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

		called, err := generateFailureOf(t, dir)

		var unknown *composite.UnknownKeyError
		require.ErrorAs(t, err, &unknown, "error %v", err)
		assert.Equal(t, "package_defaults", unknown.Where, "where")
		assert.Equal(t, "zzz", unknown.Key, "key")
		assert.Equal(t, filepath.Join(dir, composite.FileName)+`: package_defaults: unknown key "zzz"`, err.Error(), "message")
		assert.False(t, called, "generate runner called")
	})

	t.Run("dashed", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"package_defaults": {"use-receiver-type": "T"}, "packages": {"example.com/m": {}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

		called, err := generateFailureOf(t, dir)

		var dashed *composite.DashedKeyError
		require.ErrorAs(t, err, &dashed, "error %v", err)
		assert.Equal(t, "package_defaults", dashed.Where, "where")
		assert.Equal(t, filepath.Join(dir, composite.FileName)+`: package_defaults: key "use-receiver-type" uses dashes; write "use_receiver_type"`, err.Error(), "message")
		assert.False(t, called, "generate runner called")
	})

	t.Run("deprecated", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"package_defaults": {"receiver_type": "T"}, "packages": {"example.com/m": {}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

		called, err := generateFailureOf(t, dir)

		var deprecated *composite.DeprecatedKeyError
		require.ErrorAs(t, err, &deprecated, "error %v", err)
		assert.Equal(t, "package_defaults", deprecated.Where, "where")
		assert.Equal(t, filepath.Join(dir, composite.FileName)+`: package_defaults: key "receiver_type" is deprecated; write "use_receiver_type"`, err.Error(), "message")
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

	var jobs []packageJob
	err := commands(module, strings.Fields("generate"), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []packageJob) error {
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
	path := filepath.Join(dir, composite.FileName)
	require.NoError(t, os.WriteFile(path, []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var parseErr *composite.ParseError
	require.ErrorAs(t, err, &parseErr, "error %v", err)
	assert.Equal(t, path, parseErr.Path, "path")
	assert.Equal(t, 3, parseErr.Line, "line")
	assert.Equal(t, 10, parseErr.Col, "column")
	assert.Equal(t, path+":3:10: invalid character 'x' looking for beginning of value", err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsMalformedMuxtJSONInParentDirectory states that a muxt.json
// in the parent of the working directory that is not valid JSON fails a
// flagless muxt generate, naming the file by its absolute path. The runner is
// not called.
func TestGenerateRejectsMalformedMuxtJSONInParentDirectory(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = "{\n  \"packages\": {\n    \"workspace\": x\n  }\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(parent, composite.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, workspace)

	var parseErr *composite.ParseError
	require.ErrorAs(t, err, &parseErr, "error %v", err)
	path := filepath.Join(parent, composite.FileName)
	assert.Equal(t, path, parseErr.Path, "path")
	assert.Equal(t, 3, parseErr.Line, "line")
	assert.Equal(t, 18, parseErr.Col, "column")
	assert.Equal(t, path+":3:18: invalid character 'x' looking for beginning of value", err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsUnknownMuxtJSONKey states that a muxt.json entry key no
// generate flag reads fails a flagless muxt generate, naming the key, its
// entry, and the file. The runner is not called.
func TestGenerateRejectsUnknownMuxtJSONKey(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {"example.com/m": {"nope": true}}}`
	path := filepath.Join(dir, composite.FileName)
	require.NoError(t, os.WriteFile(path, []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var unknown *composite.UnknownKeyError
	require.ErrorAs(t, err, &unknown, "error %v", err)
	assert.Equal(t, path, unknown.Path, "path")
	assert.Equal(t, `packages["example.com/m"]`, unknown.Where, "where")
	assert.Equal(t, "nope", unknown.Key, "key")
	assert.Equal(t, path+`: packages["example.com/m"]: unknown key "nope"`, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsDeprecatedMuxtJSONKey states that a muxt.json entry key
// that is a deprecated flag name fails a flagless muxt generate, naming the
// key and the key to write instead. The runner is not called.
func TestGenerateRejectsDeprecatedMuxtJSONKey(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {"example.com/m": {"receiver_type": "Server"}}}`
	path := filepath.Join(dir, composite.FileName)
	require.NoError(t, os.WriteFile(path, []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var deprecated *composite.DeprecatedKeyError
	require.ErrorAs(t, err, &deprecated, "error %v", err)
	assert.Equal(t, path, deprecated.Path, "path")
	assert.Equal(t, `packages["example.com/m"]`, deprecated.Where, "where")
	assert.Equal(t, "receiver_type", deprecated.Key, "key")
	assert.Equal(t, "use_receiver_type", deprecated.Replacement, "replacement")
	assert.Equal(t, path+`: packages["example.com/m"]: key "receiver_type" is deprecated; write "use_receiver_type"`, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateMuxtJSONKeysUseUnderscores states that a muxt.json entry key is
// the flag name without "--" with each "-" written as "_". A key with the
// underscores reaches the generator. A key with dashes, which the underscore
// form would accept, fails a flagless muxt generate naming the underscore form.
func TestGenerateMuxtJSONKeysUseUnderscores(t *testing.T) {
	t.Run("underscored keys reach the generator", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"packages": {"example.com/m": {"use_receiver_type": "Server", "output_routes_func": "Mount"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

		got := generateConfigurationOf(t, dir, "generate")

		assert.Equal(t, "Server", got.ReceiverType, "receiver type")
		assert.Equal(t, "Mount", got.RoutesFunction, "routes function")
	})

	t.Run("dashed key fails naming the underscore form", func(t *testing.T) {
		dir := moduleDir(t)
		const file = `{"packages": {"example.com/m": {"use-receiver-type": "Server"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

		called, err := generateFailureOf(t, dir)

		var dashed *composite.DashedKeyError
		require.ErrorAs(t, err, &dashed, "error %v", err)
		assert.Equal(t, filepath.Join(dir, composite.FileName), dashed.Path, "path")
		assert.Equal(t, `packages["example.com/m"]`, dashed.Where, "where")
		assert.Equal(t, "use-receiver-type", dashed.Key, "key")
		assert.Equal(t, "use_receiver_type", dashed.Underscored, "underscored")
		assert.Equal(t, filepath.Join(dir, composite.FileName)+`: packages["example.com/m"]: key "use-receiver-type" uses dashes; write "use_receiver_type"`, err.Error(), "message")
		assert.False(t, called, "generate runner called")
	})
}

// TestGenerateRejectsMuxtJSONEntryFlagConflict states that a muxt.json entry
// whose values a generate flag rejects fails a flagless muxt generate with the
// flag's own error under the entry's location. The runner is not called.
func TestGenerateRejectsMuxtJSONEntryFlagConflict(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {"example.com/m": {"output_htmx": true, "output_datastar": true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	const flagMessage = "--output-htmx and --output-datastar are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)"
	var invalid *composite.InvalidEntryError
	require.ErrorAs(t, err, &invalid, "error %v", err)
	path := filepath.Join(dir, composite.FileName)
	assert.Equal(t, path, invalid.Path, "path")
	assert.Equal(t, `packages["example.com/m"]`, invalid.Where, "where")
	assert.EqualError(t, invalid.Err, flagMessage, "flag error")
	assert.Equal(t, path+`: packages["example.com/m"]: `+flagMessage, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsMuxtJSONMultipartMaxMemoryValue states that a muxt.json
// entry whose output_multipart_max_memory the flag cannot parse fails a
// flagless muxt generate, naming the file once, the entry, and the key, then
// the flag's own error. The runner is not called.
func TestGenerateRejectsMuxtJSONMultipartMaxMemoryValue(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {"example.com/m": {"output_multipart_max_memory": "lots"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var flagCfg generate.RoutesFileConfiguration
	flagErr := byteSize{p: &flagCfg.MultipartMaxMemory}.Set("lots")
	require.Error(t, flagErr)
	var invalid *composite.InvalidEntryError
	require.ErrorAs(t, err, &invalid, "error %v", err)
	path := filepath.Join(dir, composite.FileName)
	assert.Equal(t, path, invalid.Path, "path")
	assert.Equal(t, `packages["example.com/m"]`, invalid.Where, "where")
	assert.Equal(t, path+`: packages["example.com/m"]: output_multipart_max_memory: `+flagErr.Error(), err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsRelativeMuxtJSONKey states that a muxt.json packages key
// written as a directory relative to the file, the form keys used to take,
// fails a muxt generate, naming the import path to write. The runner is not
// called.
func TestGenerateRejectsRelativeMuxtJSONKey(t *testing.T) {
	for _, tt := range []struct {
		key, want string
	}{
		{key: ".", want: "example.com/m"},
		{key: "workspace", want: "example.com/m/workspace"},
		{key: "./workspace/", want: "example.com/m/workspace"},
		{key: "example.com/m/workspace/", want: "example.com/m/workspace"},
	} {
		t.Run(tt.key, func(t *testing.T) {
			dir := moduleDir(t)
			file, err := json.Marshal(map[string]any{"packages": map[string]any{tt.key: map[string]any{}}})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), file, 0o644))

			called, err := generateFailureOf(t, dir)

			var keyForm *composite.KeyFormError
			require.ErrorAs(t, err, &keyForm, "error %v", err)
			path := filepath.Join(dir, composite.FileName)
			assert.Equal(t, path, keyForm.Path, "path")
			assert.Equal(t, tt.key, keyForm.Key, "key")
			assert.Equal(t, tt.want, keyForm.ImportPath, "import path")
			assert.Equal(t, path+`: packages key "`+tt.key+`" is not an import path; write "`+tt.want+`"`, err.Error(), "message")
			assert.False(t, called, "generate runner called")
		})
	}
}

// TestGenerateJoinsModulePathAndDirectoryOfMuxtJSON states that the import
// path of a directory is the module path joined with the directory's path from
// the go.mod, so a muxt.json below the module root is keyed by the import paths
// of its own directory and the ones under it. Nothing is loaded.
func TestGenerateJoinsModulePathAndDirectoryOfMuxtJSON(t *testing.T) {
	root := moduleDir(t)
	hypertext := filepath.Join(root, "internal", "hypertext")
	workspace := filepath.Join(hypertext, "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	writeMuxtJSON(t, hypertext, `{"packages": {"example.com/m/internal/hypertext": {"use_receiver_type": "Server"}, "example.com/m/internal/hypertext/workspace": {"use_receiver_type": "T1"}}}`)

	jobs := generateJobsOf(t, hypertext, "generate")

	require.Len(t, jobs, 2, "generate jobs")
	assert.Equal(t, "example.com/m/internal/hypertext", jobs[0].Key, "first job key")
	assert.Equal(t, hypertext, jobs[0].Dir, "first job dir")
	assert.Equal(t, "Server", jobs[0].Config.ReceiverType, "first job receiver type")
	assert.Equal(t, "example.com/m/internal/hypertext/workspace", jobs[1].Key, "second job key")
	assert.Equal(t, workspace, jobs[1].Dir, "second job dir")
	assert.Equal(t, "T1", jobs[1].Config.ReceiverType, "second job receiver type")

	jobs = generateJobsOf(t, workspace, "generate")

	require.Len(t, jobs, 1, "generate jobs from the sub-directory")
	assert.Equal(t, "example.com/m/internal/hypertext/workspace", jobs[0].Key, "job key")
	assert.Equal(t, "T1", jobs[0].Config.ReceiverType, "job receiver type")
}

// TestGenerateRejectsDirectoryWithoutMuxtJSONEntry states that a flagless muxt
// generate in a directory below a muxt.json with no entry for it fails, naming
// the file and the missing key. With a generate flag the same directory is
// generated from the flags. The runner is not called for the failure.
func TestGenerateRejectsDirectoryWithoutMuxtJSONEntry(t *testing.T) {
	parent := moduleDir(t)
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {"example.com/m": {}}}`
	require.NoError(t, os.WriteFile(filepath.Join(parent, composite.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, workspace)

	var missing *composite.MissingEntryError
	require.ErrorAs(t, err, &missing, "error %v", err)
	path := filepath.Join(parent, composite.FileName)
	assert.Equal(t, path, missing.Path, "path")
	assert.Equal(t, "example.com/m/workspace", missing.Key, "key")
	assert.Equal(t, path+`: no packages entry "example.com/m/workspace"; add one to generate this directory`, err.Error(), "message")
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
	const file = `{"packages": {"example.com/m/workspace": {}}}`
	require.NoError(t, os.WriteFile(filepath.Join(parent, composite.FileName), []byte(file), 0o644))

	called, err := generateFailureOfLine(t, workspace, "generate --output-htmx")

	var configured *composite.ConfiguredDirectoryError
	require.ErrorAs(t, err, &configured, "error %v", err)
	path := filepath.Join(parent, composite.FileName)
	assert.Equal(t, path, configured.Path, "path")
	assert.Equal(t, []string{"output-htmx"}, configured.Flags, "flags")
	assert.Equal(t, "configure generate with fields in "+path+" instead of flags", err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateVerboseGeneratesFromConfiguredDirectory states that -v with no
// other generate flag, in a directory its muxt.json entry configures, generates
// from that entry. Verbose is not a generate setting, so it does not reject the
// entry. The runner records the job it is handed.
func TestGenerateVerboseGeneratesFromConfiguredDirectory(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"packages": {"example.com/m": {"use_receiver_type": "Server"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

	var jobs []packageJob
	err := commands(dir, strings.Fields("generate -v"), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []packageJob) error {
			jobs = got
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	assert.Equal(t, testModule, jobs[0].Key, "generate job key")
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
			require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(`{"packages": {"example.com/m": {}, "example.com/m/workspace": {}}}`), 0o644))

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
	const file = `{"packages": {"example.com/m": {"use_receiver_type": "Root"}, "example.com/m/workspace": {"use_receiver_type": "T1"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(parent, composite.FileName), []byte(file), 0o644))

	var jobs []packageJob
	err := commands(parent, strings.Fields("-C workspace generate"), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []packageJob) error {
			jobs = got
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	assert.Equal(t, testModule+"/workspace", jobs[0].Key, "generate job key")
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
	const file = `{"packages": {"example.com/m/workspace": {"use_receiver_type": "T1"}, "example.com/m": {"use_receiver_type": "Root"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

	jobs := generateJobsOf(t, dir, "generate")

	require.Len(t, jobs, 2, "generate jobs")
	assert.Equal(t, testModule, jobs[0].Key, "first job key")
	assert.Equal(t, dir, jobs[0].Dir, "first job dir")
	assert.Equal(t, "Root", jobs[0].Config.ReceiverType, "first job receiver type")
	assert.Equal(t, testModule+"/workspace", jobs[1].Key, "second job key")
	assert.Equal(t, workspace, jobs[1].Dir, "second job dir")
	assert.Equal(t, "T1", jobs[1].Config.ReceiverType, "second job receiver type")
}

// TestGenerateEveryMuxtJSONEntryValidatesEachEntry states that a flagless muxt
// generate in the muxt.json directory fails when any entry has values a flag
// rejects, naming that entry, and the generate runner is not called.
func TestGenerateEveryMuxtJSONEntryValidatesEachEntry(t *testing.T) {
	dir := moduleDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "workspace"), 0o755))
	const file = `{"packages": {"example.com/m": {"use_receiver_type": "Root"}, "example.com/m/workspace": {"output_htmx": true, "output_datastar": true}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	const flagMessage = "--output-htmx and --output-datastar are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)"
	var invalid *composite.InvalidEntryError
	require.ErrorAs(t, err, &invalid, "error %v", err)
	path := filepath.Join(dir, composite.FileName)
	assert.Equal(t, path, invalid.Path, "path")
	assert.Equal(t, `packages["example.com/m/workspace"]`, invalid.Where, "where")
	assert.Equal(t, path+`: packages["example.com/m/workspace"]: `+flagMessage, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateMuxtJSONWithOnlyWorkspaceEntry states that a muxt.json whose only
// entry is workspace, run in the muxt.json directory, generates workspace alone:
// the directory itself has no entry, so nothing is generated for it.
func TestGenerateMuxtJSONWithOnlyWorkspaceEntry(t *testing.T) {
	dir := moduleDir(t)
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {"example.com/m/workspace": {"use_receiver_type": "T1"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

	jobs := generateJobsOf(t, dir, "generate")

	require.Len(t, jobs, 1, "generate jobs")
	assert.Equal(t, testModule+"/workspace", jobs[0].Key, "job key")
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
	const file = `{"packages": {"example.com/m": {"output_htmx": true, "output_datastar": true}, "example.com/m/workspace": {"use_receiver_type": "T1"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

	jobs := generateJobsOf(t, workspace, "generate")

	require.Len(t, jobs, 1, "generate jobs")
	assert.Equal(t, testModule+"/workspace", jobs[0].Key, "job key")
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
			const file = `{"packages": {"example.com/m": {"use_receiver_type": "Root"}, "example.com/m/gone": {"use_receiver_type": "T1"}}}`
			require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

			called, err := generateFailureOf(t, dir)

			var missing *composite.MissingDirectoryError
			require.ErrorAs(t, err, &missing, "error %v", err)
			path := filepath.Join(dir, composite.FileName)
			assert.Equal(t, path, missing.Path, "path")
			assert.Equal(t, "example.com/m/gone", missing.Key, "key")
			assert.Equal(t, filepath.Join(dir, "gone"), missing.Dir, "dir")
			assert.Equal(t, path+`: packages["example.com/m/gone"]: directory `+filepath.Join(dir, "gone")+` does not exist`, err.Error(), "message")
			assert.False(t, called, "generate runner called")
		})
	}
}

// generateJobsOf runs a generate command line in dir and returns the jobs the
// generate runner is handed. Nothing is loaded.
func generateJobsOf(t *testing.T, dir, commandLine string) []packageJob {
	t.Helper()
	var jobs []packageJob
	err := commands(dir, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []packageJob) error {
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
		generate: func(_ *cobra.Command, _ []packageJob) error {
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
	const file = `{"packages": {"example.com/m/workspace": {"use_receiver_type": "T1"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(parent, "muxt.json"), []byte(file), 0o644))

	var jobs []packageJob
	err := commands(workspace, strings.Fields("generate"), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), io.Discard, io.Discard, runners{
		generate: func(_ *cobra.Command, got []packageJob) error {
			jobs = got
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	assert.Equal(t, testModule+"/workspace", jobs[0].Key, "generate job key")
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
			require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), file, 0o644))

			called, err := generateFailureOf(t, dir)

			var outside *composite.KeyOutsideError
			require.ErrorAs(t, err, &outside, "error %v", err)
			path := filepath.Join(dir, composite.FileName)
			assert.Equal(t, path, outside.Path, "path")
			assert.Equal(t, key, outside.Key, "key")
			assert.Equal(t, path+`: packages key "`+key+`" is outside the file's directory`, err.Error(), "message")
			assert.False(t, called, "generate runner called")
		})
	}
}

// TestGenerateRejectsMuxtJSONKeyOutsideModule states that a packages key that
// is an import path of another module fails a muxt generate saying so, and
// naming the module the file's keys belong to. The runner is not called.
func TestGenerateRejectsMuxtJSONKeyOutsideModule(t *testing.T) {
	for _, key := range []string{"example.com/other/x", "example.com/other", "example.com/m2"} {
		t.Run(key, func(t *testing.T) {
			dir := moduleDir(t)
			writeMuxtJSON(t, dir, `{"packages": {"`+key+`": {}}}`)

			called, err := generateFailureOf(t, dir)

			var other *composite.OtherModuleError
			require.ErrorAs(t, err, &other, "error %v", err)
			assert.Equal(t, key, other.Key, "key")
			assert.Equal(t, testModule, other.Module, "module")
			assert.Equal(t, filepath.Join(dir, composite.FileName)+`: packages key "`+key+`" is not in this module (`+testModule+`)`, err.Error(), "message")
			assert.False(t, called, "generate runner called")
		})
	}
}

// TestGenerateRejectsMuxtJSONKeyInModuleOutsideDirectory states that a packages
// key that is an import path in the file's module, but not at or below the
// directory holding the muxt.json, fails a muxt generate. The runner is not
// called.
func TestGenerateRejectsMuxtJSONKeyInModuleOutsideDirectory(t *testing.T) {
	root := moduleDir(t)
	hypertext := filepath.Join(root, "internal", "hypertext")
	require.NoError(t, os.MkdirAll(hypertext, 0o755))
	writeMuxtJSON(t, hypertext, `{"packages": {"example.com/m/internal/other": {}}}`)

	called, err := generateFailureOf(t, hypertext)

	var outside *composite.KeyOutsideError
	require.ErrorAs(t, err, &outside, "error %v", err)
	assert.Equal(t, filepath.Join(hypertext, composite.FileName)+`: packages key "example.com/m/internal/other" is outside the file's directory`, err.Error(), "message")
	assert.False(t, called, "generate runner called")
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
	require.NoError(t, os.WriteFile(filepath.Join(project, composite.FileName), []byte(`{"packages": {"example.com/m/project/linked": {}}}`), 0o644))
	if err := os.Symlink(elsewhere, filepath.Join(project, "linked")); err != nil {
		t.Skipf("creating a symbolic link: %v", err)
	}

	called, err := generateFailureOf(t, project)

	var outside *composite.OutsideDirectoryError
	require.ErrorAs(t, err, &outside, "error %v", err)
	path := filepath.Join(project, composite.FileName)
	assert.Equal(t, path, outside.Path, "path")
	assert.Equal(t, "example.com/m/project/linked", outside.Key, "key")
	assert.Equal(t, filepath.Join(project, "linked"), outside.Dir, "dir")
	assert.Equal(t, path+`: packages["example.com/m/project/linked"]: directory `+filepath.Join(project, "linked")+` resolves outside the file's directory`, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateAcceptsMuxtJSONLinkInsideDirectory states that a packages key
// whose directory is a symbolic link to another directory inside the muxt.json's
// directory is a job like any other. The runner records the job it is handed.
func TestGenerateAcceptsMuxtJSONLinkInsideDirectory(t *testing.T) {
	dir := moduleDir(t)
	inside := filepath.Join(dir, "inside")
	require.NoError(t, os.Mkdir(inside, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(`{"packages": {"example.com/m/linked": {}}}`), 0o644))
	if err := os.Symlink(inside, filepath.Join(dir, "linked")); err != nil {
		t.Skipf("creating a symbolic link: %v", err)
	}

	jobs := generateJobsOf(t, dir, "generate")

	require.Len(t, jobs, 1, "got %d generate jobs, want 1", len(jobs))
	assert.Equal(t, "example.com/m/linked", jobs[0].Key, "generate job key")
}

// TestGenerateRejectsMuxtJSONWithoutGoMod states that a muxt.json with no
// go.mod at or above it fails a muxt generate, since its packages keys are
// import paths and the module path names them. The runner is not called.
func TestGenerateRejectsMuxtJSONWithoutGoMod(t *testing.T) {
	dir := t.TempDir()
	writeMuxtJSON(t, dir, `{"packages": {"example.com/m": {}}}`)

	called, err := generateFailureOf(t, dir)

	var noModule *composite.NoModuleError
	require.ErrorAs(t, err, &noModule, "error %v", err)
	path := filepath.Join(dir, composite.FileName)
	assert.Equal(t, path, noModule.Path, "path")
	assert.Equal(t, path+": no go.mod found at or above the file; package keys are import paths", err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsMuxtJSONWithoutPackages states that a flagless muxt
// generate in the directory of a muxt.json that lists no packages fails,
// naming the file and the command that adds one, rather than generating with
// defaults. With a generate flag the directory is not configured, so the flags
// decide. The runner is not called for the failure.
func TestGenerateRejectsMuxtJSONWithoutPackages(t *testing.T) {
	dir := moduleDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(`{"packages": {}}`), 0o644))

	called, err := generateFailureOf(t, dir)

	var empty *composite.EmptyPackagesError
	require.ErrorAs(t, err, &empty, "error %v", err)
	path := filepath.Join(dir, composite.FileName)
	assert.Equal(t, path, empty.Path, "path")
	assert.Equal(t, path+": packages is empty; add a package with: muxt generate package NAME", err.Error(), "message")
	assert.False(t, called, "generate runner called")

	config := generateConfigurationOf(t, dir, "generate --use-receiver-type T1")
	assert.Equal(t, "T1", config.ReceiverType, "receiver type from flags")
}

// TestGenerateRejectsUnknownTopLevelKey states that a misspelled top-level key
// fails a flagless muxt generate and names the key it was meant to be, rather
// than dropping the defaults it holds. The runner is not called.
func TestGenerateRejectsUnknownTopLevelKey(t *testing.T) {
	dir := moduleDir(t)
	const file = `{"package_default": {"output_htmx": true}, "packages": {"example.com/m": {}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, composite.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var unknown *composite.UnknownKeyError
	require.ErrorAs(t, err, &unknown, "error %v", err)
	assert.Equal(t, filepath.Join(dir, composite.FileName)+`: unknown key "package_default"; did you mean "package_defaults"?`, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}
