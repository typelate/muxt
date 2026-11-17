package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/typelate/htt/template"

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

// TestTestPassesGoTestOutputThrough states that what go test writes is what
// the command writes, byte for byte, and nothing else.
func TestTestPassesGoTestOutputThrough(t *testing.T) {
	run := newTestRun(t)
	run.Write = func(_ int, stdout, stderr io.Writer) {
		_, _ = io.WriteString(stdout, "ok  \texample.com/pkg\t0.003s\n")
		_, _ = io.WriteString(stderr, "# example.com/pkg\r\nwarning\n")
		_, _ = io.WriteString(stdout, "no final newline")
	}
	stdout, stderr, err := run.run([]packageJob{{Dir: "/work/pkg"}}, testConfiguration{})
	require.NoError(t, err, "muxt test")

	assert.Equal(t, "ok  \texample.com/pkg\t0.003s\nno final newline", stdout, "stdout")
	assert.Equal(t, "# example.com/pkg\r\nwarning\n", stderr, "stderr")
}

// TestTestFailsWhenTheTestsFail states that a failing go test fails the
// command, and that go test's own report is the whole of what is printed:
// cobra's "Error:" line would only repeat it.
func TestTestFailsWhenTheTestsFail(t *testing.T) {
	run := newTestRun(t)
	run.Write = func(_ int, stdout, _ io.Writer) {
		_, _ = io.WriteString(stdout, "--- FAIL: TestIndex (0.00s)\nFAIL\n")
	}
	run.Result = func(int) error { return &exec.ExitError{} }
	var stdout, stderr bytes.Buffer
	err := commands("/work", []string{testCommandName}, func(string) string { return "" }, cliVersion, strings.NewReader(""), &stdout, &stderr, runners{
		test: testRunner(run.goTest, run.prepare),
	})

	require.Error(t, err, "muxt test")
	_, ok := errors.AsType[*goTestFailure](err)
	assert.True(t, ok, "the error is a *goTestFailure, got %T: %v", err, err)
	assert.Equal(t, "--- FAIL: TestIndex (0.00s)\nFAIL\n", stdout.String(), "stdout")
	assert.Empty(t, stderr.String(), "stderr")
}

// TestTestGivesEachJobItsOwnCoverDirectory states that two packages' test
// binaries never write profiles to the same place, so a profile line is
// the package's whose go test wrote it.
func TestTestGivesEachJobItsOwnCoverDirectory(t *testing.T) {
	run := newTestRun(t)
	_, _, err := run.run([]packageJob{{Dir: "/work/a"}, {Dir: "/work/b"}}, testConfiguration{})
	require.NoError(t, err, "muxt test")

	require.Len(t, run.Calls, 2, "go test runs")
	require.Len(t, run.Prepare, 2, "engines")
	assert.NotEqual(t, run.Prepare[0], run.Prepare[1], "engine directories")
	assert.Equal(t, filepath.Dir(run.Prepare[0]), filepath.Dir(run.Prepare[1]), "the engine directories' parent, the command's scratch")
	for i, call := range run.Calls {
		want := template.CoverEnv + "=" + filepath.Join(run.Prepare[i], "profiles")
		assert.Contains(t, call.Env, want, "go test %d environment", i)
	}
}

// TestTestPassesNoPackagePattern states that with nothing after -- go test
// is given no package pattern, so it tests the package in the directory.
func TestTestPassesNoPackagePattern(t *testing.T) {
	run := newTestRun(t)
	_, _, err := run.run([]packageJob{{Dir: "/work/pkg"}}, testConfiguration{})
	require.NoError(t, err, "muxt test")

	require.Len(t, run.Calls, 1, "go test runs")
	for _, arg := range run.Calls[0].Args[1:] {
		assert.True(t, strings.HasPrefix(arg, "-"), "go test argument %q is not a flag, want no package pattern", arg)
	}
}
