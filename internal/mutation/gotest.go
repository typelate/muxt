package mutation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/typelate/muxt/internal/template/html"
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
//
// When ctx is done the run is interrupted, along with the compilers and
// test binaries it started, and the error is ctx's.
func (t goTest) run(ctx context.Context, limit int, flags ...string) (*goTestOutput, error) {
	cmd := exec.CommandContext(ctx, "go", t.args(flags...)...)
	interruptTogether(cmd)
	// A go test that ignores the interrupt is killed after this long.
	cmd.WaitDelay = 10 * time.Second
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
	if ctxErr := ctx.Err(); ctxErr != nil {
		// Whatever the interrupted run said, it was not a verdict.
		return out, ctxErr
	}
	return out, err
}

// baseline runs the tests with nothing mutated and returns everything they
// printed, which is what a failing baseline shows.
func (t goTest) baseline(ctx context.Context) (string, error) {
	out, err := t.run(ctx, 0)
	return out.String(), err
}

// coverageOutputLimit is how much of the pre-pass's output is kept. It
// runs verbose, so that a passing package's stderr is printed too, and a
// failure shows the end of that.
const coverageOutputLimit = 1 << 20

// coverageFlags are the go test flags of the coverage pre-pass.
//
// The engine adds packages under GOROOT that have no directory on disk,
// which vet cannot read, hence -vet=off. -v is required: see coverage.
func coverageFlags(overlay string) []string {
	return []string{"-v", "-vet=off", "-overlay=" + overlay}
}

// coverage runs the tests once, unmutated, with overlay laying muxt's
// template engine over html/template and coverDir, named by
// html.CoverEnv, receiving the template blocks each test binary executes.
//
// It leaves out the caller's Go coverage flags; see withoutCoverFlags.
//
// It runs go test with -v, and must: without it go test prints nothing a
// passing package's test binary wrote, and a binary whose export failed
// says so only on its stderr, with html.CoverDisabledMarker. Without -v a
// broken export would read as templates no test reached.
func (t goTest) coverage(ctx context.Context, overlay, coverDir string) (*goTestOutput, error) {
	env := t.env
	if env == nil {
		env = os.Environ()
	}
	t.env = append(slices.Clip(env), html.CoverEnv+"="+coverDir)
	t.extra = withoutCoverFlags(t.extra)
	return t.run(ctx, coverageOutputLimit, coverageFlags(overlay)...)
}

// withoutCoverFlags is the caller's go test flags less the ones for Go
// coverage, which the pre-pass must not pass on: -coverprofile would be
// written once by it and again by the baseline, and -coverpkg or -cover
// would instrument the engine's packages, which have no directory on
// disk. The baseline and the kill runs keep them. A flag's value as the
// next argument goes with it; everything from -args on is the test
// binary's and is kept.
func withoutCoverFlags(args []string) []string {
	kept := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-args" || arg == "--args" {
			return append(kept, args[i:]...)
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if !strings.HasPrefix(arg, "-") {
			kept = append(kept, arg)
			continue
		}
		switch name {
		case "cover":
			continue
		case "covermode", "coverprofile", "coverpkg":
			if !hasValue && i+1 < len(args) {
				i++
			}
			continue
		}
		kept = append(kept, arg)
	}
	return kept
}

// mutantOutputLimit is how much of one mutant's go test output is kept:
// enough for the end of a failure, and bounded however many mutants run
// at once or however much their tests print.
const mutantOutputLimit = 64 << 10

// verdict runs the tests against one mutant's overlay and reports whether
// they caught it. An error means go test could not run at all.
//
// For a kill it also returns the tests that failed, by their top level
// names, so a kill that cannot be repeated can name them.
func (t goTest) verdict(ctx context.Context, overlay string) (Status, []string, error) {
	out, err := t.run(ctx, mutantOutputLimit, "-overlay="+overlay)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", nil, ctxErr
	}
	status, err := verdictOf(out, err)
	if status != StatusKilled || out.unnamed {
		// A package that failed without naming a test leaves nothing to
		// narrow a repeat by: some other package's named failure would
		// leave it out.
		return status, nil, err
	}
	return status, out.failed, nil
}

// onlyTests is the -run pattern selecting exactly the named top level
// tests.
func onlyTests(names []string) *regexp.Regexp {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = regexp.QuoteMeta(name)
	}
	return regexp.MustCompile("^(" + strings.Join(quoted, "|") + ")$")
}

// hasRunFlag reports whether the caller's go test flags select tests.
func hasRunFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-args" || arg == "--args" {
			return false
		}
		name, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if strings.HasPrefix(arg, "-") && name == "run" {
			return true
		}
	}
	return false
}

// recheck runs only the tests that failed against a mutant, again, and
// says whether they still do.
//
// It is for a kill earned beside other runs, where a test can fail from
// load alone. With no names to narrow by, because the failure was a
// package and not a test, it repeats the whole run.
func (t goTest) recheck(ctx context.Context, overlay string, failed []string) (Status, error) {
	status, _, err := t.repeating(failed).verdict(ctx, overlay)
	return status, err
}

// repeating is t narrowed to the tests that failed.
//
// The caller's own -run, as --run or after --, already says which tests
// the baseline ran; replacing it could run subtests it never did, so the
// repeat keeps it and runs everything it selects.
func (t goTest) repeating(failed []string) goTest {
	if len(failed) > 0 && t.match == nil && !hasRunFlag(t.extra) {
		t.match = onlyTests(failed)
	}
	return t
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
	// line, couldNotRun by a package that did not build or set up.
	testFailed  bool
	couldNotRun bool

	// failed holds the top level name of each test a "--- FAIL" line
	// named, once each, in the order they were printed.
	failed []string

	// named is whether a "--- FAIL" has been read since the last package
	// result, and unnamed whether a package failed without one: a test
	// binary that timed out, panicked outside a test, or exited from
	// TestMain.
	named, unnamed bool

	// coverDisabled is the first line in which a test binary said it
	// stopped exporting template coverage, from html.CoverDisabledMarker
	// on, cut at maxMarkerLine bytes. It is looked for in everything
	// written, not only in lines short enough to be markers: a test's own
	// output with no newline can precede it on a long line.
	coverDisabled string

	// markerTail is the end of what has been written, a byte short of a
	// whole marker, so that one split across writes is still found.
	// capturing is set from the marker to the end of its line, and
	// markerFound after.
	markerTail             []byte
	capturing, markerFound bool
}

const maxMarkerLine = 1 << 10

func (o *goTestOutput) Write(p []byte) (int, error) {
	o.keep(p)
	o.findMarker(p)
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

// findMarker looks through p for the first html.CoverDisabledMarker and
// collects the rest of its line into coverDisabled.
func (o *goTestOutput) findMarker(p []byte) {
	marker := []byte(html.CoverDisabledMarker)
	for len(p) > 0 && !o.markerFound {
		if o.capturing {
			chunk, _, ended := bytes.Cut(p, []byte("\n"))
			chunk = chunk[:min(len(chunk), maxMarkerLine-len(o.coverDisabled))]
			o.coverDisabled += string(chunk)
			if ended || len(o.coverDisabled) >= maxMarkerLine {
				o.coverDisabled = strings.TrimSuffix(o.coverDisabled, "\r")
				o.capturing, o.markerFound = false, true
			}
			return
		}
		// A marker that starts in what came before ends in the first
		// len(marker)-1 bytes of p.
		window := append(slices.Clip(o.markerTail), p[:min(len(p), len(marker)-1)]...)
		start := -1
		if i := bytes.Index(window, marker); i >= 0 {
			start = i - len(o.markerTail)
		} else if i := bytes.Index(p, marker); i >= 0 {
			start = i
		}
		if start == -1 {
			o.markerTail = lastBytes(append(slices.Clip(o.markerTail), lastBytes(p, len(marker)-1)...), len(marker)-1)
			return
		}
		o.coverDisabled = string(marker)
		o.capturing = true
		o.markerTail = nil
		p = p[start+len(marker):]
	}
}

// lastBytes is a copy of at most the last n bytes of b.
func lastBytes(b []byte, n int) []byte {
	return slices.Clone(b[max(len(b)-n, 0):])
}

// keep appends p, and drops the front once more than twice the limit is
// held, so that trimming costs one copy per limit bytes written rather than
// one per write. String returns the last limit bytes of what is held.
func (o *goTestOutput) keep(p []byte) {
	o.kept = append(o.kept, p...)
	if o.limit > 0 && len(o.kept) > 2*o.limit {
		o.kept = append(o.kept[:0], o.kept[len(o.kept)-o.limit:]...)
	}
}

// mark notes what one line of go test output says about the run.
//
// A package's result line is FAIL, a tab, its import path, and then either
// a tab and how long its tests took, or a bracketed reason it was never
// tested. An error from the go command itself, a line starting "go: ", is
// not read: a test that runs the go command prints such lines too, and a
// run where the go command failed has no test failure to find, which is
// already not a kill.
func (o *goTestOutput) mark(line string) {
	line = strings.TrimSuffix(line, "\r")
	switch {
	case strings.HasPrefix(strings.TrimLeft(line, " \t"), "--- FAIL"):
		o.testFailed = true
		o.named = true
		o.noteFailed(strings.TrimLeft(line, " \t"))
	case strings.HasPrefix(line, "ok  \t"):
		o.named = false
	case strings.HasPrefix(line, "FAIL\t"):
		if strings.HasSuffix(line, "[build failed]") || strings.HasSuffix(line, "[setup failed]") {
			o.couldNotRun = true
		} else if strings.Count(line, "\t") >= 2 {
			o.testFailed = true
			o.unnamed = o.unnamed || !o.named
			o.named = false
		}
	}
}

// noteFailed records the test a "--- FAIL: Name (0.00s)" line names. A
// subtest is recorded as the test it belongs to, which is what -run can
// select on its own.
func (o *goTestOutput) noteFailed(line string) {
	name, ok := strings.CutPrefix(line, "--- FAIL: ")
	if !ok {
		return
	}
	name, _, _ = strings.Cut(name, " ")
	name, _, _ = strings.Cut(name, "/")
	if name != "" && !slices.Contains(o.failed, name) {
		o.failed = append(o.failed, name)
	}
}

// String returns the output kept, which is all of it or its tail.
func (o *goTestOutput) String() string {
	if o == nil {
		return ""
	}
	kept := o.kept
	if o.limit > 0 && len(kept) > o.limit {
		kept = kept[len(kept)-o.limit:]
	}
	return string(kept)
}

// CheckGoTestArgs refuses a pass-through flag muxt sets itself, or one
// that cannot work beside what it sets.
//
// -overlay is how a mutant reaches the build. A second one would either be
// ignored or replace the mutant, and either way the run would measure the
// templates as written rather than as mutated.
//
// -json is refused because a mutant is killed when go test's text output
// says a test failed; the JSON stream has none of those lines, so every
// failing mutant would be read as a run that could not happen.
//
// -coverpkg is refused when it covers html/template, by name, by a pattern
// matching it, or as std or all: the coverage pre-pass replaces that
// package with muxt's template engine, whose files have no directory on
// disk for coverage instrumentation to work from.
func CheckGoTestArgs(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-args" || arg == "--args" {
			// Everything after -args belongs to the test binary.
			return nil
		}
		if !strings.HasPrefix(arg, "-") {
			// A flag's value, such as the pattern after -run.
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch name {
		case "overlay":
			return errors.New("go test flag -overlay cannot be passed through: muxt uses -overlay to deliver each mutant")
		case "json":
			return errors.New("go test flag -json cannot be passed through: muxt reads go test's text output to tell a failed test from a failed build")
		case "coverpkg":
			if !hasValue && i+1 < len(args) {
				i++
				value = args[i]
			}
			if pattern, ok := coversHTMLTemplate(value); ok {
				return fmt.Errorf("go test flag -coverpkg cannot cover html/template (%q does): muxt replaces html/template with its own template engine to learn which template actions the tests reach; use -coverpkg with your own packages, such as ./... instead", pattern)
			}
		}
	}
	return nil
}

// coversHTMLTemplate returns the first of a -coverpkg value's
// comma-separated patterns that takes in html/template.
func coversHTMLTemplate(value string) (string, bool) {
	for pattern := range strings.SplitSeq(value, ",") {
		switch {
		case pattern == "std", pattern == "all":
			return pattern, true
		case strings.Contains(pattern, "..."):
			parts := strings.Split(pattern, "...")
			for j := range parts {
				parts[j] = regexp.QuoteMeta(parts[j])
			}
			if regexp.MustCompile("^" + strings.Join(parts, ".*") + "$").MatchString("html/template") {
				return pattern, true
			}
		case pattern == "html/template":
			return pattern, true
		}
	}
	return "", false
}
