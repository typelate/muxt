package cli

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/overlay"
	"github.com/typelate/muxt/internal/templatecover"
)

// goTestCall is what a stand-in go test was asked to run.
type goTestCall struct {
	Dir  string
	Env  []string
	Args []string
}

// testRun is a stand-in go test and engine for testRunner, recording what
// the runner asked of each.
type testRun struct {
	t       *testing.T
	Prepare []string // the directory each engine was written to
	Calls   []goTestCall

	// Write is what each go test writes to its stdout and stderr, and
	// Result what it returns, by call.
	Write  func(call int, stdout, stderr io.Writer)
	Result func(call int) error
}

func (r *testRun) goTest(_ context.Context, dir string, env, args []string, stdout, stderr io.Writer) error {
	call := len(r.Calls)
	r.Calls = append(r.Calls, goTestCall{Dir: dir, Env: env, Args: args})
	if r.Write != nil {
		r.Write(call, stdout, stderr)
	}
	if r.Result != nil {
		return r.Result(call)
	}
	return nil
}

func (r *testRun) prepare(_ context.Context, _ overlay.Toolchain, dir string) (templatecover.Engine, error) {
	r.Prepare = append(r.Prepare, dir)
	return templatecover.Engine{
		Files:    overlay.Files{ModFile: filepath.Join(dir, "go.mod"), Overlay: filepath.Join(dir, "overlay.json")},
		CoverDir: filepath.Join(dir, "profiles"),
	}, nil
}

// run runs jobs through testRunner and returns what went to stdout and
// stderr.
func (r *testRun) run(jobs []packageJob, config testConfiguration) (stdout, stderr string, err error) {
	r.t.Helper()
	var out, errOut bytes.Buffer
	cmd := &cobra.Command{Use: testCommandName}
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())
	err = testRunner(r.goTest, r.prepare)(cmd, jobs, config)
	return out.String(), errOut.String(), err
}

func newTestRun(t *testing.T) *testRun {
	t.Helper()
	// The runner's scratch directory is under TMPDIR, which the test owns.
	t.Setenv("TMPDIR", t.TempDir())
	return &testRun{t: t}
}

// TestTestRunsGoTestOnTheEngine states how muxt test runs go test in a
// package directory: once, there, with the engine's flags ahead of the
// caller's.
func TestTestRunsGoTestOnTheEngine(t *testing.T) {
	run := newTestRun(t)
	_, _, err := run.run([]packageJob{{Dir: "/work/pkg"}}, testConfiguration{GoTestArgs: []string{"-run", "TestIndex"}})
	require.NoError(t, err, "muxt test")

	require.Len(t, run.Calls, 1, "go test runs")
	require.Len(t, run.Prepare, 1, "engines")
	assert.Equal(t, "/work/pkg", run.Calls[0].Dir, "go test directory")
	want := []string{
		"test", "-count=1",
		"-mod=mod",
		"-modfile=" + filepath.Join(run.Prepare[0], "go.mod"),
		"-overlay=" + filepath.Join(run.Prepare[0], "overlay.json"),
		"-run", "TestIndex",
	}
	assert.Equal(t, want, run.Calls[0].Args, "go test arguments")
}
