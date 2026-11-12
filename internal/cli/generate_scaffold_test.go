package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCommand runs a muxt command line in dir, with no configuration runners,
// and returns what it printed to stdout and the error it returned.
func runCommand(t *testing.T, dir string, commandLine string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	err := commands(dir, strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), &stdout, io.Discard, runners{})
	return stdout.String(), err
}

// TestGenerateConfigWritesEmptyMuxtJSON states that muxt generate config in a
// directory with no muxt.json creates one with an empty packages object.
func TestGenerateConfigWritesEmptyMuxtJSON(t *testing.T) {
	dir := t.TempDir()

	stdout, err := runCommand(t, dir, "generate config")

	require.NoError(t, err)
	assert.Equal(t, "wrote muxt.json\n", stdout, "stdout")
	got, err := os.ReadFile(filepath.Join(dir, "muxt.json"))
	require.NoError(t, err)
	assert.Equal(t, "{\n\t\"packages\": {}\n}\n", string(got), "muxt.json")
}
