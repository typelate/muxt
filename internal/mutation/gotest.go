package mutation

import (
	"bytes"
	"errors"
	"os/exec"
	"regexp"
	"slices"
	"strings"
)

// goTest runs the project's tests, optionally with extra flags.
type goTest struct {
	dir      string
	packages []string
	match    *regexp.Regexp

	// extra are the caller's own go test flags, passed on every run.
	extra []string

	// env is the environment go test runs in, nil for the process's own.
	env []string
}

func (c Configuration) goTest(dir string) goTest {
	packages := c.Packages
	if len(packages) == 0 {
		packages = []string{"./..."}
	}
	return goTest{dir: dir, packages: packages, match: c.Run, extra: c.GoTestArgs, env: c.env}
}

// args is the go test command line. go test reads [flags] [packages]
// [flags and test binary flags]. A flag it does not know -- a test
// binary's own, such as -update -- ends the package list, so the caller's
// flags go after the packages, where they cannot cut it short.
func (t goTest) args(flags ...string) []string {
	args := []string{"test", "-count=1"}
	args = append(args, flags...)
	if t.match != nil {
		args = append(args, "-run="+t.match.String())
	}
	args = append(args, t.packages...)
	return append(args, t.extra...)
}

// run runs go test and returns what it printed, stdout and stderr
// together.
//
// limit bounds how much of the output is kept, from the end; zero keeps
// all of it. Whether a test failed or a package did not build is read off
// every line as it is printed, so the bound costs no verdict.
func (t goTest) run(limit int, flags ...string) (*goTestOutput, error) {
	cmd := exec.Command("go", t.args(flags...)...)
	cmd.Dir = t.dir
	if t.env != nil {
		// os/exec points PWD at Dir only when it supplies the environment
		// itself. The go command addresses files the way PWD says, and an
		// overlay keyed on the paths the loader reported is ignored when
		// the two disagree -- every mutant then builds unmutated and is
		// reported as a mutation no test caught.
		cmd.Env = append(slices.Clip(t.env), "PWD="+t.dir)
	}
	out := &goTestOutput{limit: limit}
	// One writer for both streams is one pipe, so lines arrive whole and
	// in the order go test wrote them.
	cmd.Stdout, cmd.Stderr = out, out
	err := cmd.Run()
	return out, err
}

// baseline runs the tests with nothing mutated and returns everything they
// printed, which is what a failing baseline shows.
func (t goTest) baseline() (string, error) {
	out, err := t.run(0)
	return out.String(), err
}

// mutantOutputLimit is how much of one mutant's go test output is kept:
// enough for the end of a failure, and bounded however many mutants run
// at once or however much their tests print.
const mutantOutputLimit = 64 << 10

// verdict runs the tests against one mutant's overlay and reports whether
// they caught it. An error means go test could not run at all.
func (t goTest) verdict(overlay string) (Status, error) {
	out, err := t.run(mutantOutputLimit, "-overlay="+overlay)
	return verdictOf(out, err)
}

// verdictOf reads a go test run as a verdict on the mutant it ran against.
//
// A mutant is killed only when a test failed. go test also exits 1 when a
// package does not build or set up, and when the go command itself fails
// -- a module that will not download, an overlay it cannot read -- and a
// compiler the OS killed for memory under --workers is a build failure. A
// mutant cannot break the build, since it only changes template text, so
// all of those are a run that could not happen rather than a kill.
func verdictOf(out *goTestOutput, err error) (Status, error) {
	switch {
	case err == nil:
		return StatusMissed, nil
	case isTestFailure(err) && out.testFailed && !out.couldNotRun:
		return StatusKilled, nil
	default:
		return "", &GoTestError{Err: err, Output: out.String()}
	}
}

// isTestFailure reports whether go test exited the way it does when a
// test fails. It is necessary for a kill and not sufficient: go test exits
// 1 for a package that does not build too.
//
// A usage error exits 2, and a go test the OS killed -- for memory, say,
// under --workers -- has no exit status at all; read as test failures,
// both would be recorded as mutants the tests caught.
func isTestFailure(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
}

// goTestOutput collects what go test prints, keeping at most limit bytes
// from the end when limit is positive, and notes as each line arrives
// whether it says a test failed or a package could not be tested.
type goTestOutput struct {
	limit int
	kept  []byte

	// line is the line being written, up to maxMarkerLine bytes of it; a
	// longer one is test output and never a marker.
	line     []byte
	overflow bool

	// testFailed is set by a "--- FAIL" line or a package's "FAIL" result
	// line, couldNotRun by a package that did not build or set up, or by
	// an error from the go command itself.
	testFailed  bool
	couldNotRun bool
}

const maxMarkerLine = 1 << 10

func (o *goTestOutput) Write(p []byte) (int, error) {
	o.keep(p)
	for rest := p; len(rest) > 0; {
		chunk, after, ended := bytes.Cut(rest, []byte("\n"))
		if !o.overflow && len(o.line)+len(chunk) <= maxMarkerLine {
			o.line = append(o.line, chunk...)
		} else {
			o.overflow = true
		}
		if !ended {
			break
		}
		if !o.overflow {
			o.mark(string(o.line))
		}
		o.line, o.overflow = o.line[:0], false
		rest = after
	}
	return len(p), nil
}

func (o *goTestOutput) keep(p []byte) {
	o.kept = append(o.kept, p...)
	if o.limit > 0 && len(o.kept) > o.limit {
		o.kept = append(o.kept[:0], o.kept[len(o.kept)-o.limit:]...)
	}
}

// mark notes what one line of go test output says about the run.
//
// A package's result line is FAIL, a tab, its import path, and then either
// a tab and how long its tests took, or a bracketed reason it was never
// tested.
func (o *goTestOutput) mark(line string) {
	line = strings.TrimSuffix(line, "\r")
	switch {
	case strings.HasPrefix(strings.TrimLeft(line, " \t"), "--- FAIL"):
		o.testFailed = true
	case strings.HasPrefix(line, "FAIL\t"):
		if strings.HasSuffix(line, "[build failed]") || strings.HasSuffix(line, "[setup failed]") {
			o.couldNotRun = true
		} else if strings.Count(line, "\t") >= 2 {
			o.testFailed = true
		}
	case strings.HasPrefix(line, "go: ") && !strings.HasPrefix(line, "go: downloading "):
		o.couldNotRun = true
	}
}

// String returns the output kept, which is all of it or its tail.
func (o *goTestOutput) String() string {
	if o == nil {
		return ""
	}
	return string(o.kept)
}

// CheckGoTestArgs refuses a pass-through flag muxt sets itself.
//
// -overlay is how a mutant reaches the build. A second one would either be
// ignored or replace the mutant, and either way the run would measure the
// templates as written rather than as mutated.
func CheckGoTestArgs(args []string) error {
	for _, arg := range args {
		if arg == "-args" || arg == "--args" {
			// Everything after -args belongs to the test binary.
			return nil
		}
		if !strings.HasPrefix(arg, "-") {
			// A flag's value, such as the pattern after -run.
			continue
		}
		name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if name == "overlay" {
			return errors.New("go test flag -overlay cannot be passed through: muxt uses -overlay to deliver each mutant")
		}
	}
	return nil
}
