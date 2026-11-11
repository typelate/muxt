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
