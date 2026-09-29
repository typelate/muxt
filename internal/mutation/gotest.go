package mutation

import (
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

func (t goTest) run(flags ...string) (string, error) {
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
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// verdict runs the tests against one mutant's overlay and reports whether
// they caught it. An error means go test could not run at all.
func (t goTest) verdict(overlay string) (Status, error) {
	_, err := t.run("-overlay=" + overlay)
	return verdictOf(err)
}

func verdictOf(err error) (Status, error) {
	switch {
	case err == nil:
		return StatusMissed, nil
	case isTestFailure(err):
		return StatusKilled, nil
	default:
		return "", err
	}
}

// isTestFailure reports whether go test exited the way it does when a
// test fails, as opposed to not running at all.
//
// Only exit status 1 counts. A usage error exits 2, and a go test the OS
// killed -- for memory, say, under --workers -- has no exit status at
// all; read as test failures, both would be recorded as mutants the tests
// caught.
func isTestFailure(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
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
