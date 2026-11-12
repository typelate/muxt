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
			flagNames = append(flagNames, f.Name)
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
		{name: "use-templates-variable", args: "--use-templates-variable data", entry: map[string]any{"use-templates-variable": []string{"data"}}},
		{name: "use-receiver-type", args: "--use-receiver-type Server", entry: map[string]any{"use-receiver-type": "Server"}},
		{name: "use-receiver-type-package", args: "--use-receiver-type-package example.com/pkg", entry: map[string]any{"use-receiver-type-package": "example.com/pkg"}},
		{name: "output-file", args: "--output-file routes_gen.go", entry: map[string]any{"output-file": "routes_gen.go"}},
		{name: "output-receiver-interface", args: "--output-receiver-interface Receiver", entry: map[string]any{"output-receiver-interface": "Receiver"}},
		{name: "output-routes-func", args: "--output-routes-func Routes", entry: map[string]any{"output-routes-func": "Routes"}},
		{name: "output-template-data-type", args: "--output-template-data-type Data", entry: map[string]any{"output-template-data-type": "Data"}},
		{name: "output-sse-template-data-type", args: "--output-sse-template-data-type SSEData", entry: map[string]any{"output-sse-template-data-type": "SSEData"}},
		{name: "output-template-route-paths-type", args: "--output-template-route-paths-type Paths", entry: map[string]any{"output-template-route-paths-type": "Paths"}},
		{name: "output-template-route-type", args: "--output-template-route-type Route", entry: map[string]any{"output-template-route-type": "Route"}},
		{name: "output-template-route-builder-type", args: "--output-template-route-builder-type RouteBuilder", entry: map[string]any{"output-template-route-builder-type": "RouteBuilder"}},
		{name: "output-routes-func-with-logger-param", args: "--output-routes-func-with-logger-param", entry: map[string]any{"output-routes-func-with-logger-param": true}},
		{name: "output-routes-func-with-path-prefix-param", args: "--output-routes-func-with-path-prefix-param", entry: map[string]any{"output-routes-func-with-path-prefix-param": true}},
		{name: "output-routes-func-with-middleware-param", args: "--output-routes-func-with-middleware-param", entry: map[string]any{"output-routes-func-with-middleware-param": true}},
		{name: "output-multiple-files", args: "--output-multiple-files", entry: map[string]any{"output-multiple-files": true}},
		{name: "output-htmx", args: "--output-htmx", entry: map[string]any{"output-htmx": true}},
		{name: "output-datastar", args: "--output-datastar", entry: map[string]any{"output-datastar": true}},
		{name: "output-exported-default-identifiers", args: "--output-exported-default-identifiers=false", entry: map[string]any{"output-exported-default-identifiers": false}},
		{name: "output-multipart-max-memory", args: "--output-multipart-max-memory 64MB", entry: map[string]any{"output-multipart-max-memory": "64MB"}},
		{name: "output-muxt-version", args: "--output-muxt-version=false", entry: map[string]any{"output-muxt-version": false}},
		{name: "input-static-assets-directory", args: "--input-static-assets-directory assets --output-static-assets-path /static", entry: map[string]any{"input-static-assets-directory": "assets", "output-static-assets-path": "/static"}},
		{name: "output-static-assets-path", args: "--input-static-assets-directory assets --output-static-assets-path /static", entry: map[string]any{"input-static-assets-directory": "assets", "output-static-assets-path": "/static"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
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
	dir := t.TempDir()
	const file = `{"packages": {".": {"output-exported-default-identifiers": false, "output-routes-func": "Mount"}}}`
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
	dir := t.TempDir()
	const file = `{"packages": {".": {"use-receiver-type": "Server", "output-routes-func": "templateRoutes"}}}`
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

// TestGenerateIgnoresMuxtJSONAboveModuleRoot states that a muxt.json in a
// directory above the module root is not read: the walk stops at the directory
// holding go.mod. Nothing is loaded: the generate runner records the job it is
// handed.
func TestGenerateIgnoresMuxtJSONAboveModuleRoot(t *testing.T) {
	parent := t.TempDir()
	module := filepath.Join(parent, "module")
	require.NoError(t, os.Mkdir(module, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(module, "go.mod"), []byte("module example.com/m\n"), 0o644))
	const file = `{"packages": {"module": {"use-receiver-type": "T1"}}}`
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
	dir := t.TempDir()
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
	parent := t.TempDir()
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
	dir := t.TempDir()
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
	dir := t.TempDir()
	const file = `{"packages": {".": {"receiver-type": "Server"}}}`
	path := filepath.Join(dir, muxtjson.FileName)
	require.NoError(t, os.WriteFile(path, []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var deprecated *muxtjson.DeprecatedKeyError
	require.ErrorAs(t, err, &deprecated, "error %v", err)
	assert.Equal(t, muxtjson.FileName, deprecated.Path, "path")
	assert.Equal(t, `packages["."]`, deprecated.Where, "where")
	assert.Equal(t, "receiver-type", deprecated.Key, "key")
	assert.Equal(t, "use-receiver-type", deprecated.Replacement, "replacement")
	assert.Equal(t, `muxt.json: packages["."]: key "receiver-type" is deprecated; write "use-receiver-type"`, err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsMuxtJSONEntryFlagConflict states that a muxt.json entry
// whose values a generate flag rejects fails a flagless muxt generate with the
// flag's own error under the entry's location. The runner is not called.
func TestGenerateRejectsMuxtJSONEntryFlagConflict(t *testing.T) {
	dir := t.TempDir()
	const file = `{"packages": {".": {"output-htmx": true, "output-datastar": true}}}`
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
// entry whose output-multipart-max-memory the flag cannot parse fails a
// flagless muxt generate, naming the file once, the entry, and the key, then
// the flag's own error. The runner is not called.
func TestGenerateRejectsMuxtJSONMultipartMaxMemoryValue(t *testing.T) {
	dir := t.TempDir()
	const file = `{"packages": {".": {"output-multipart-max-memory": "lots"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

	called, err := generateFailureOf(t, dir)

	var flagCfg generate.RoutesFileConfiguration
	flagErr := (&multipartMaxMemoryFlag{cfg: &flagCfg}).Set("lots")
	require.Error(t, flagErr)
	var invalid *muxtjson.InvalidEntryError
	require.ErrorAs(t, err, &invalid, "error %v", err)
	assert.Equal(t, muxtjson.FileName, invalid.Path, "path")
	assert.Equal(t, `packages["."]`, invalid.Where, "where")
	assert.Equal(t, `muxt.json: packages["."]: output-multipart-max-memory: `+flagErr.Error(), err.Error(), "message")
	assert.False(t, called, "generate runner called")
}

// TestGenerateRejectsUncleanMuxtJSONKey states that a muxt.json packages key
// not in clean form fails a muxt generate, even one with flags, naming the key
// and its clean form. The runner is not called.
func TestGenerateRejectsUncleanMuxtJSONKey(t *testing.T) {
	dir := t.TempDir()
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
	parent := t.TempDir()
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
	parent := t.TempDir()
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
	dir := t.TempDir()
	const file = `{"packages": {".": {"use-receiver-type": "Server"}}}`
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

// TestGenerateChangeDirectorySelectsMuxtJSONEntry states that muxt -C workspace
// generate, run from the directory holding a muxt.json with entries for the
// directory and for workspace, generates only the workspace entry. -C is not a
// generate flag, so it does not reject the entries. The runner records the job
// it is handed.
func TestGenerateChangeDirectorySelectsMuxtJSONEntry(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {".": {"use-receiver-type": "Root"}, "workspace": {"use-receiver-type": "T1"}}}`
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
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {"workspace": {"use-receiver-type": "T1"}, ".": {"use-receiver-type": "Root"}}}`
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
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "workspace"), 0o755))
	const file = `{"packages": {".": {"use-receiver-type": "Root"}, "workspace": {"output-htmx": true, "output-datastar": true}}}`
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
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {"workspace": {"use-receiver-type": "T1"}}}`
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
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {".": {"output-htmx": true, "output-datastar": true}, "workspace": {"use-receiver-type": "T1"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, muxtjson.FileName), []byte(file), 0o644))

	jobs := generateJobsOf(t, workspace, "generate")

	require.Len(t, jobs, 1, "generate jobs")
	assert.Equal(t, "workspace", jobs[0].Key, "job key")
	assert.Equal(t, workspace, jobs[0].Dir, "job dir")
	assert.Equal(t, "T1", jobs[0].Config.ReceiverType, "job receiver type")
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
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	require.NoError(t, os.Mkdir(workspace, 0o755))
	const file = `{"packages": {"workspace": {"use-receiver-type": "T1"}}}`
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
