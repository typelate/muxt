package cli

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTestFailsInAWorkspace runs muxt test against the real go command: the
// go command refuses -modfile in workspace mode, which is how the engine goes
// in place, so the command stops with that reason before any test runs. A
// script cannot state it, because scripts run muxt in process and so under the
// process's GOWORK=off.
func TestTestFailsInAWorkspace(t *testing.T) {
	wd := t.TempDir()
	writeTestFile(t, wd, "go.mod", "module example.com\n\ngo 1.24\n")
	writeTestFile(t, wd, "go.work", "go 1.24\n\nuse .\n")
	writeTestFile(t, wd, "run_test.go", "package main\n\nimport \"testing\"\n\nfunc TestRuns(t *testing.T) { t.Log(\"ran\") }\n")
	t.Setenv("GOWORK", filepath.Join(wd, "go.work"))

	stdout, _, err := execute(t, wd, testCommandName)

	require.Error(t, err, "muxt test in a workspace")
	assert.Contains(t, err.Error(), "cannot run the tests on muxt's template engine: go.work is in use, and the go command refuses muxt test's -modfile in workspace mode: ", "muxt test in a workspace")
	assert.Empty(t, stdout, "stdout of muxt test in a workspace: no go test ran")
}
