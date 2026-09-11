// Package mutation varies the actions in a project's templates and
// re-runs its tests, reporting every variation the tests still pass
// through.
//
// A template action that can be changed without failing a test is a gap:
// nothing the suite asserts on depends on what that action does. The
// report names those gaps so they can be closed one at a time.
//
// Templates are mutated in the context of the ExecuteTemplate call that
// renders them, which is the only place the type of dot is known. That
// type decides what a sensible variation is, and tells a mutation that
// changes behaviour from one that merely breaks the template.
//
// Variations reach the test run through the go command's -overlay flag.
// An overlay replaces a file for the build without writing to the working
// tree, and it reaches embedded files, so a template a package embeds can
// be varied without touching the checkout. golang.org/x/tools/go/packages
// has an Overlay of its own, but it does not reach //go:embed content:
// go list reports the original path for an embedded file, so a consumer
// reading that path still sees the unmutated bytes.
package mutation

import (
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"regexp"
	"time"
)

// Configuration selects what to mutate and what to run.
type Configuration struct {
	// TemplatesVariables names the package level template variables to
	// read templates from.
	TemplatesVariables []string

	// TemplatePattern, when set, limits mutation to templates whose name
	// it matches.
	TemplatePattern *regexp.Regexp

	// Run, when set, is passed to go test as -run, narrowing both the
	// baseline and every mutant run to the same subset.
	Run *regexp.Regexp

	// Packages are the package patterns to test. It defaults to ./...
	// so that a mutation caught only by a test in another package is
	// still reported as caught.
	Packages []string

	// DryRun enumerates and reports the mutants without running any of
	// them, which is quick enough to answer "how much work is this".
	DryRun bool

	// Verbose reports every mutant rather than only the ones no test
	// caught, and streams progress while the run is in flight.
	Verbose bool

	// IncludeTests also starts traversal from ExecuteTemplate calls in
	// _test.go files. It is off by default: a template rendered only by
	// a test is not rendered in production, and mutating it measures the
	// tests against themselves.
	IncludeTests bool

	// Seed seeds the values substituted for an action's operands. When
	// SeedSet is false one is drawn and reported, so a run can always be
	// repeated with the seed it printed.
	Seed    uint64
	SeedSet bool

	// MaxCases bounds how many combinations one action may contribute.
	// An action over the bound contributes none, and says so.
	MaxCases int

	// Workers is how many mutants run at once. Each is a separate go
	// test invocation against its own overlay, so no mutant sees
	// another's mutation -- but they do share whatever the tests
	// themselves share, such as a port, a database or a file they write.
	// Raise it only for a suite that tolerates running beside itself.
	// Zero or less means one.
	Workers int

	// GoTestArgs are extra flags for every go test invocation, from
	// everything after a -- on the command line. They reach the
	// baseline and each mutant alike.
	GoTestArgs []string
}

// DefaultMaxCases bounds the combinations one action may contribute.
//
// Every case is a full test run, so a wide action can cost more than the
// rest of a template set put together. Eight allows three operands to be
// varied in every combination.
const DefaultMaxCases = 8

// Run mutates every action the configuration selects and reports which
// variations the tests catch.
//
// progress receives a line per mutant as it completes when the
// configuration is verbose; it may be nil.
func Run(config Configuration, workingDirectory string, status io.Writer) (*Report, error) {
	if !config.SeedSet {
		// A drawn seed is reported so the run can be repeated exactly.
		config.Seed = rand.Uint64()
	}
	plan, err := newPlan(config, workingDirectory)
	if err != nil {
		return nil, err
	}

	// Per mutant lines are noise unless asked for; the preamble is not,
	// because it is what tells someone whether to wait or walk away.
	var progress io.Writer
	if config.Verbose {
		progress = status
	}

	report := plan.report()
	report.Verbose = config.Verbose
	if config.DryRun {
		report.DryRun = true
		return report, nil
	}

	tester := goTest{dir: workingDirectory, packages: testedPackages(config), match: config.Run, extra: config.GoTestArgs}

	started := time.Now()
	if out, err := tester.run(nil); err != nil {
		if !isTestFailure(err) {
			return nil, err
		}
		return nil, &BaselineFailedError{Output: out}
	}
	baseline := time.Since(started)
	report.Baseline = BaselineResult{Passed: true, Seconds: baseline.Seconds()}
	workers := max(config.Workers, 1)
	clock := &estimate{perMutant: baseline, remaining: plan.runnable(), workers: workers}

	if status != nil {
		_, _ = fmt.Fprintf(status, "%d %s across %d %s (complexity %d), baseline %s, estimated %s\n",
			report.Total, pluralize(report.Total, "mutant"),
			report.Templates, pluralize(report.Templates, "template"),
			report.Complexity, roundDuration(baseline), clock.left())
	}
	reportTrims(progress, report.Trimmed)

	scratch, err := os.MkdirTemp("", "muxt-mutation-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	runner := &mutantRunner{
		plan:     plan,
		test:     tester.verdict,
		scratch:  scratch,
		progress: progress,
		clock:    clock,
	}
	if err := runner.runAll(report, workers); err != nil {
		return nil, err
	}
	return report, nil
}
