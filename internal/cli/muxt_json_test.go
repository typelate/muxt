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

	"github.com/typelate/muxt/internal/generate"
)

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
