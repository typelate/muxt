package mutation

import (
	"errors"
	"os/exec"
	"regexp"
	"strings"
)

// goTest runs the project's tests, optionally with extra flags.
type goTest struct {
	dir      string
	packages []string
	match    *regexp.Regexp

	// extra are the caller's own go test flags, passed on every run.
	extra []string
}

func (t goTest) run(extra []string) (string, error) {
	// go test reads [flags] [packages] [flags and test binary flags]. A
	// flag it does not know -- a test binary's own, such as -update --
	// ends the package list, so the caller's flags go after the packages,
	// where they cannot cut it short.
	args := []string{"test", "-count=1"}
	args = append(args, extra...)
	if t.match != nil {
		args = append(args, "-run="+t.match.String())
	}
	args = append(args, t.packages...)
	args = append(args, t.extra...)

	cmd := exec.Command("go", args...)
	cmd.Dir = t.dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// testedPackages is the package patterns a run tests, with the default
// applied.
func testedPackages(config Configuration) []string {
	if len(config.Packages) == 0 {
		return []string{"./..."}
	}
	return config.Packages
}

// isTestFailure reports whether go test exited the way it does when a
// test fails, as opposed to not running at all.
//
// Only exit status 1 counts. A usage error exits 2, and a go test the OS
// killed -- for memory, say, under --parallel -- has no exit status at
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
