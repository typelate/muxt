package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	Prepare []string            // the directory each engine was written to
	Targets []overlay.Toolchain // the toolchain each engine was prepared for
	Calls   []goTestCall

	// Write is what each go test writes to its stdout and stderr, and
	// Result what it returns, by call.
	Write  func(call int, stdout, stderr io.Writer)
	Result func(call int) error

	// PrepareResult is what each engine's preparation returns, by call.
	PrepareResult func(call int) error
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

func (r *testRun) prepare(_ context.Context, tc overlay.Toolchain, dir string) (templatecover.Engine, error) {
	call := len(r.Prepare)
	r.Prepare = append(r.Prepare, dir)
	r.Targets = append(r.Targets, tc)
	if r.PrepareResult != nil {
		if err := r.PrepareResult(call); err != nil {
			return templatecover.Engine{}, err
		}
	}
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

// TestTestStopsAtTheFirstFailingPackage states that a failure ends the run:
// the packages after it are not tested.
func TestTestStopsAtTheFirstFailingPackage(t *testing.T) {
	run := newTestRun(t)
	run.Result = func(call int) error {
		if call == 1 {
			return &exec.ExitError{}
		}
		return nil
	}
	jobs := []packageJob{
		{Key: "example.com/a", Dir: "/work/a", Keyed: true},
		{Key: "example.com/b", Dir: "/work/b", Keyed: true},
		{Key: "example.com/c", Dir: "/work/c", Keyed: true},
	}
	stdout, _, err := run.run(jobs, testConfiguration{})

	require.Error(t, err, "muxt test")
	require.Len(t, run.Calls, 2, "go test runs")
	assert.Equal(t, "/work/b", run.Calls[1].Dir, "the last package tested")
	assert.NotContains(t, stdout, "example.com/c", "stdout names the package that never ran")
}

// TestTestPreparesEachEngineInItsPackageForMuxtTest states that an engine
// is prepared in the package's directory, and that the toolchain says muxt
// test is the command whose -modfile the go command may refuse.
func TestTestPreparesEachEngineInItsPackageForMuxtTest(t *testing.T) {
	run := newTestRun(t)
	_, _, err := run.run([]packageJob{{Dir: "/work/a"}, {Dir: "/work/b"}}, testConfiguration{})
	require.NoError(t, err, "muxt test")

	want := []overlay.Toolchain{
		{Go: "go", Dir: "/work/a", Purpose: "muxt test"},
		{Go: "go", Dir: "/work/b", Purpose: "muxt test"},
	}
	assert.Equal(t, want, run.Targets, "toolchains")
}

// TestTestRunsNothingWhenAnEngineCannotBePrepared states that the engines
// of every package are in place before the first go test runs, so a package
// that cannot have one costs no one a run of the packages before it.
func TestTestRunsNothingWhenAnEngineCannotBePrepared(t *testing.T) {
	run := newTestRun(t)
	run.PrepareResult = func(call int) error {
		if call == 1 {
			return errors.New("go.work is in use")
		}
		return nil
	}
	jobs := []packageJob{
		{Key: "example.com/a", Dir: "/work/a", Keyed: true},
		{Key: "example.com/b", Dir: "/work/b", Keyed: true},
	}
	stdout, _, err := run.run(jobs, testConfiguration{})

	require.Error(t, err, "muxt test")
	assert.Equal(t, "cannot run the tests on muxt's template engine: go.work is in use", err.Error(), "error")
	assert.Empty(t, run.Calls, "go test runs")
	assert.Empty(t, stdout, "stdout")
}

// TestTestNamesTheFailingKeyedPackage states that when one of several
// packages fails, the last line of stderr says which, because go test's own
// report may have scrolled away.
func TestTestNamesTheFailingKeyedPackage(t *testing.T) {
	run := newTestRun(t)
	run.Result = func(call int) error {
		if call == 1 {
			return &exec.ExitError{}
		}
		return nil
	}
	base := newFieldkit(t, fieldkitEntries)
	var stdout, stderr bytes.Buffer
	err := commands(base, []string{testCommandName}, func(string) string { return "" }, cliVersion, strings.NewReader(""), &stdout, &stderr, runners{
		test: testRunner(run.goTest, run.prepare),
	})

	require.Error(t, err, "muxt test")
	failure, ok := errors.AsType[*goTestFailure](err)
	require.True(t, ok, "the error is a *goTestFailure, got %T: %v", err, err)
	assert.Equal(t, "example.com/fieldkit/inventory", failure.Key, "failing package")
	assert.Equal(t, "Error: example.com/fieldkit/inventory: go test failed\n", stderr.String(), "stderr")
}

// TestTestRemovesItsScratchDirectory states that the engine files and
// profiles are gone when the command returns, however it ends.
func TestTestRemovesItsScratchDirectory(t *testing.T) {
	for _, tt := range []struct {
		name   string
		result func(ctx context.Context) error
		check  func(t *testing.T, err error)
	}{
		{
			name:   "tests that pass",
			result: func(context.Context) error { return nil },
			check:  func(t *testing.T, err error) { assert.NoError(t, err, "muxt test") },
		},
		{
			name:   "tests that fail",
			result: func(context.Context) error { return &exec.ExitError{} },
			check:  func(t *testing.T, err error) { assert.Error(t, err, "muxt test") },
		},
		{
			name: "an interrupt",
			result: func(ctx context.Context) error {
				self, err := os.FindProcess(os.Getpid())
				if err == nil {
					err = self.Signal(os.Interrupt)
				}
				if err != nil {
					t.Skipf("cannot interrupt this process: %v", err)
				}
				<-ctx.Done()
				return ctx.Err()
			},
			check: func(t *testing.T, err error) { assert.ErrorIs(t, err, context.Canceled, "muxt test") },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			var out, errOut bytes.Buffer
			cmd := &cobra.Command{Use: testCommandName}
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetContext(context.Background())
			var written []string
			prepare := func(_ context.Context, _ overlay.Toolchain, dir string) (templatecover.Engine, error) {
				// Write what the real Prepare writes, so there is something to remove.
				if err := os.MkdirAll(filepath.Join(dir, "profiles"), 0o700); err != nil {
					return templatecover.Engine{}, err
				}
				written = append(written, dir)
				return templatecover.Engine{CoverDir: filepath.Join(dir, "profiles")}, nil
			}
			goTest := func(ctx context.Context, _ string, _, _ []string, _, _ io.Writer) error { return tt.result(ctx) }
			err := testRunner(goTest, prepare)(cmd, []packageJob{{Dir: "/work/pkg"}}, testConfiguration{})
			tt.check(t, err)

			require.Len(t, written, 1, "engines written")
			assert.NoDirExists(t, filepath.Dir(written[0]), "scratch directory")
			left, readErr := os.ReadDir(tmp)
			require.NoError(t, readErr, "reading TMPDIR")
			assert.Empty(t, left, "files left in TMPDIR")
		})
	}
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

// TestTestNamesEachKeyedPackage states that a run over every entry of a
// muxt.json writes each entry's key on its own line before that package's
// go test output, so a reader can tell whose output follows.
func TestTestNamesEachKeyedPackage(t *testing.T) {
	run := newTestRun(t)
	run.Write = func(call int, stdout, _ io.Writer) {
		_, _ = io.WriteString(stdout, "ok "+strconv.Itoa(call)+"\n")
	}
	jobs := []packageJob{
		{Key: "example.com/a", Dir: "/work/a", Keyed: true},
		{Key: "example.com/b", Dir: "/work/b", Keyed: true},
	}
	stdout, stderr, err := run.run(jobs, testConfiguration{})
	require.NoError(t, err, "muxt test")

	assert.Equal(t, "example.com/a\nok 0\nexample.com/b\nok 1\n", stdout, "stdout")
	assert.Empty(t, stderr, "stderr")
}

// TestTestNamesNoKeyWhenGoTestWritesJSON states that with -json after -- the
// output stays a stream of JSON events: a key line would break every
// reader of it. A -json that go test reads as off leaves the key lines in.
func TestTestNamesNoKeyWhenGoTestWritesJSON(t *testing.T) {
	for _, tt := range []struct {
		args       []string
		wantStdout string
	}{
		{args: []string{"-json"}, wantStdout: "{}\n{}\n"},
		{args: []string{"-json=true"}, wantStdout: "{}\n{}\n"},
		{args: []string{"-json=false"}, wantStdout: "example.com/a\n{}\nexample.com/b\n{}\n"},
		{args: []string{"-json", "-json=false"}, wantStdout: "example.com/a\n{}\nexample.com/b\n{}\n"},
		{args: []string{"-json=false", "-json"}, wantStdout: "{}\n{}\n"},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			run := newTestRun(t)
			run.Write = func(_ int, stdout, _ io.Writer) {
				_, _ = io.WriteString(stdout, "{}\n")
			}
			jobs := []packageJob{
				{Key: "example.com/a", Dir: "/work/a", Keyed: true},
				{Key: "example.com/b", Dir: "/work/b", Keyed: true},
			}
			stdout, _, err := run.run(jobs, testConfiguration{GoTestArgs: tt.args})
			require.NoError(t, err, "muxt test")

			assert.Equal(t, tt.wantStdout, stdout, "muxt test -- %s stdout", strings.Join(tt.args, " "))
		})
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
