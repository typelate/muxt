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
