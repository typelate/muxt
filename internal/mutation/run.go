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
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"
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

	// Parallel is how many mutants run at once. Each is a separate go
	// test invocation against its own overlay, so no mutant sees
	// another's mutation -- but they do share whatever the tests
	// themselves share, such as a port, a database or a file they write.
	// Raise it only for a suite that tolerates running beside itself.
	// Zero or less means one.
	Parallel int

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

// Status is the verdict on one mutant.
type Status string

const (
	// StatusKilled means the tests failed while the mutation was in
	// place, which is the outcome to want: something asserts on the
	// behaviour the mutated action controls.
	StatusKilled Status = "KILL"

	// StatusMissed means the tests still passed, so nothing observes
	// what the action does.
	StatusMissed Status = "MISS"

	// StatusSkipped means the mutation was never run, because it does
	// not survive type checking against the dot it would render with.
	// Running it would report a kill earned by a render error rather
	// than by a test observing a behaviour change.
	StatusSkipped Status = "SKIP"

	// StatusPending means the mutant was enumerated but not run, which
	// is every mutant in a dry run.
	StatusPending Status = "PEND"
)

// Result is one mutant and the verdict its test run produced.
type Result struct {
	Status   Status   `json:"status"`
	Operator Operator `json:"operator"`
	Line     int      `json:"line"`
	Column   int      `json:"column"`
	Original string   `json:"original"`
	Mutated  string   `json:"mutated"`

	// Reason says why a skipped mutant was not run.
	Reason string `json:"reason,omitempty"`

	// Seconds is how long the mutant's test run took.
	Seconds float64 `json:"seconds,omitempty"`

	// mutantIndex locates the mutant this result came from, so the run
	// does not have to carry the mutants inside the report it prints.
	mutantIndex int
}

// TemplateReport gathers the mutants found in one template, rendered with
// one type of dot.
type TemplateReport struct {
	Template   string   `json:"template"`
	File       string   `json:"file"`
	DataType   string   `json:"data_type"`
	Via        bool     `json:"via_template_call"`
	Complexity int      `json:"complexity"`
	Results    []Result `json:"results"`
}

// Group gathers the templates reachable from one ExecuteTemplate call.
type Group struct {
	CallSite  string           `json:"call_site"`
	Entry     string           `json:"entry"`
	DataType  string           `json:"data_type"`
	Templates []TemplateReport `json:"templates"`
}

// TrimmedTemplate is a subtree the traversal did not descend into because
// the same template had already been reached with the same type of dot.
type TrimmedTemplate struct {
	CallSite    string `json:"call_site"`
	Template    string `json:"template"`
	DataType    string `json:"data_type"`
	FirstSeenAt string `json:"first_seen_at"`
}

// BaselineResult records the unmutated run the mutants are compared
// against.
type BaselineResult struct {
	// Passed is whether the tests pass with no mutation in place. A
	// mutation run is only meaningful when they do.
	Passed bool `json:"passed"`

	// Seconds is how long the unmutated run took, which is what the
	// estimate for the whole run is built from.
	Seconds float64 `json:"seconds,omitempty"`
}

// Report is the outcome of a whole mutation run.
type Report struct {
	Baseline   BaselineResult    `json:"baseline,omitzero"`
	DryRun     bool              `json:"dry_run"`
	Seed       uint64            `json:"seed"`
	Verbose    bool              `json:"-"`
	Templates  int               `json:"templates"`
	Complexity int               `json:"complexity"`
	Total      int               `json:"total"`
	Killed     int               `json:"killed"`
	Missed     int               `json:"missed"`
	Skipped    int               `json:"skipped"`
	Groups     []Group           `json:"groups"`
	Trimmed    []TrimmedTemplate `json:"trimmed"`

	// parallel is how many mutants run at once, which the estimate
	// divides the work by.
	parallel int
}

// BaselineFailedError reports that the tests do not pass before anything
// is mutated, which makes every verdict meaningless: a mutant would be
// recorded as caught by a failure that was already there.
type BaselineFailedError struct {
	Output string
}

func (e *BaselineFailedError) Error() string {
	return "baseline tests failed before mutation; fix them first:\n" + strings.TrimRight(e.Output, "\n")
}

// NoCallSitesError reports that nothing renders the templates, so there
// is no type of dot to mutate against.
type NoCallSitesError struct {
	Variables []string
}

func (e *NoCallSitesError) Error() string {
	return fmt.Sprintf("no %s.ExecuteTemplate calls found: mutation testing needs a call site to know the type of dot", strings.Join(e.Variables, ", "))
}

// NoMutationsError reports that templates were reached but none of them
// held an action to mutate.
//
// A run that mutates nothing passes, and a passing mutation run reads as
// "the tests catch everything". That is the most misleading thing this
// tool can say, so finding nothing to ask about is an error rather than
// an empty report that exits zero.
type NoMutationsError struct {
	Templates int
}

func (e *NoMutationsError) Error() string {
	return fmt.Sprintf("no mutations available: the %d template(s) reached hold no dynamic or control flow actions, so a run would report every mutant killed without testing anything", e.Templates)
}

// UnreadableTemplateError reports a template the template set parsed into
// actions and this package re-parsed into none.
//
// The two disagree only when the text was read with delimiters it was not
// written in. The template would otherwise contribute no mutants at all,
// and a run that quietly measured fewer templates than it was given is
// the failure this reports instead.
type UnreadableTemplateError struct {
	Template string
	Path     string
}

func (e *UnreadableTemplateError) Error() string {
	return fmt.Sprintf("template %q in %s holds actions the template set can see and this run cannot: it was read with the wrong delimiters", e.Template, e.Path)
}

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
	report.parallel = max(config.Parallel, 1)

	if status != nil {
		_, _ = fmt.Fprintf(status, "%d %s across %d %s (complexity %d), baseline %s, estimated %s\n",
			report.Total, pluralize(report.Total, "mutant"),
			report.Templates, pluralize(report.Templates, "template"),
			report.Complexity, roundDuration(baseline), report.Estimate())
	}
	reportTrims(progress, report.Trimmed)

	scratch, err := os.MkdirTemp("", "muxt-mutation-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	runner := &mutantRunner{
		plan:     plan,
		tester:   tester,
		scratch:  scratch,
		progress: progress,
		clock:    &estimate{perMutant: baseline, remaining: plan.runnable(), parallel: report.parallel},
	}
	if err := runner.runAll(report, report.parallel); err != nil {
		return nil, err
	}
	return report, nil
}

// countVerdict tallies one mutant's outcome.
func countVerdict(report *Report, status Status) {
	switch status {
	case StatusKilled:
		report.Killed++
	case StatusMissed:
		report.Missed++
	}
}

// mutantRunner runs the mutants a plan enumerated and writes each
// verdict into the report.
type mutantRunner struct {
	plan     *plan
	tester   goTest
	scratch  string
	progress io.Writer

	// mu guards everything below it, which every run updates as it
	// finishes.
	mu    sync.Mutex
	clock *estimate
	done  int
	err   error
}

// runAll runs every mutant in the report, at most parallel at a time.
//
// Each mutant writes its own overlay under scratch and runs its own go
// test, so no run can see another's mutation. A verdict is written into
// the report's own slot for it, so the report reads in plan order however
// the runs finish; only the progress stream comes out in the order they
// complete. The first error that is not a test failure stops new runs
// starting, and is returned once the ones already running have finished.
func (r *mutantRunner) runAll(report *Report, parallel int) error {
	total := 0
	for group := range report.eachTemplate() {
		total += len(group.Results)
	}

	slots := make(chan struct{}, parallel)
	var running sync.WaitGroup
	index := 0
dispatch:
	for group := range report.eachTemplate() {
		for i := range group.Results {
			if r.failed() {
				break dispatch
			}
			slots <- struct{}{}
			running.Add(1)
			go func(index int, group *TemplateReport, result *Result) {
				defer running.Done()
				defer func() { <-slots }()
				r.finish(group, result, total, r.run(index, result))
			}(index, group, &group.Results[i])
			index++
		}
	}
	running.Wait()

	if r.err != nil {
		return r.err
	}
	for group := range report.eachTemplate() {
		for _, result := range group.Results {
			countVerdict(report, result.Status)
		}
	}
	return nil
}

// run runs one mutant's tests and writes the verdict into result.
func (r *mutantRunner) run(index int, result *Result) error {
	if result.Status == StatusSkipped {
		return nil
	}
	overlay, err := writeMutant(r.scratch, index, r.plan.mutants[result.mutantIndex])
	if err != nil {
		return err
	}
	started := time.Now()
	_, testErr := r.tester.run([]string{"-overlay=" + overlay})
	result.Seconds = time.Since(started).Seconds()
	switch {
	case testErr == nil:
		result.Status = StatusMissed
	case isTestFailure(testErr):
		result.Status = StatusKilled
	default:
		return testErr
	}
	return nil
}

// finish records that one mutant is done, reporting its verdict or
// keeping the first error.
func (r *mutantRunner) finish(group *TemplateReport, result *Result, total int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		if r.err == nil {
			r.err = err
		}
		return
	}
	if result.Status != StatusSkipped {
		r.clock.observe(time.Duration(result.Seconds * float64(time.Second)))
	}
	reportProgress(r.progress, r.done, total, group, *result, r.clock)
	r.done++
}

func (r *mutantRunner) failed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err != nil
}

// eachTemplate iterates the report's templates in the order they were
// traversed, which is depth first from each call site.
func (r *Report) eachTemplate() func(func(*TemplateReport) bool) {
	return func(yield func(*TemplateReport) bool) {
		for i := range r.Groups {
			for j := range r.Groups[i].Templates {
				if !yield(&r.Groups[i].Templates[j]) {
					return
				}
			}
		}
	}
}

func reportTrims(progress io.Writer, trimmed []TrimmedTemplate) {
	if progress == nil {
		return
	}
	for _, t := range trimmed {
		_, _ = fmt.Fprintf(progress, "trimmed %s at %s: already mutated with %s from %s\n",
			strconv.Quote(t.Template), t.CallSite, t.DataType, t.FirstSeenAt)
	}
}

func reportProgress(progress io.Writer, index, total int, group *TemplateReport, result Result, clock *estimate) {
	if progress == nil {
		return
	}
	line := fmt.Sprintf("[%*d/%d] %s %s:%d:%d %s %s",
		len(fmt.Sprint(total)), index+1, total,
		result.Status, group.File, result.Line, result.Column,
		strconv.Quote(group.Template), result.Operator)
	if (result.Operator == OperatorOperands || result.Operator == OperatorCondition) && result.Mutated != "" {
		line += " " + result.Mutated
	}
	switch {
	case result.Status == StatusSkipped:
		line += " (" + result.Reason + ")"
	default:
		line += fmt.Sprintf(" (%s, ~%s left)", roundDuration(time.Duration(result.Seconds*float64(time.Second))), clock.left())
	}
	_, _ = fmt.Fprintln(progress, line)
}

// estimate predicts how much longer a run has to go.
//
// The first prediction comes from the unmutated run, which is the only
// timing available before any mutant has been tried. Each mutant that
// finishes moves the average, so the estimate converges on what this
// project's tests actually cost.
type estimate struct {
	perMutant time.Duration
	observed  time.Duration
	count     int
	remaining int

	// parallel is how many mutants run at once, so the remaining work
	// takes that many times fewer rounds.
	parallel int
}

func (e *estimate) observe(d time.Duration) {
	e.observed += d
	e.count++
	e.perMutant = e.observed / time.Duration(e.count)
	if e.remaining > 0 {
		e.remaining--
	}
}

func (e *estimate) total() time.Duration {
	parallel := max(e.parallel, 1)
	rounds := (e.remaining + parallel - 1) / parallel
	return time.Duration(rounds) * e.perMutant
}

func (e *estimate) left() string {
	return roundDuration(e.total())
}

func roundDuration(d time.Duration) string {
	if d >= time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(100 * time.Millisecond).String()
}

// writeMutant writes the mutated file and the overlay pointing at it,
// returning the overlay's path.
func writeMutant(scratch string, index int, mutant Mutant) (string, error) {
	dir := filepath.Join(scratch, fmt.Sprintf("%06d", index))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	mutated := filepath.Join(dir, filepath.Base(mutant.File))
	if err := os.WriteFile(mutated, []byte(mutant.Apply()), 0o600); err != nil {
		return "", err
	}
	overlay := filepath.Join(dir, "overlay.json")
	b, err := json.Marshal(struct {
		Replace map[string]string
	}{Replace: map[string]string{mutant.File: mutated}})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(overlay, b, 0o600); err != nil {
		return "", err
	}
	return overlay, nil
}

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

// sourceKey identifies the text a template was written in: a template
// file, or one string literal within a Go file.
type sourceKey struct {
	file     string
	litStart int
}

// sourceCollector turns the definitions a template set reports into the
// distinct texts holding them, reading each file once.
type sourceCollector struct {
	workingDirectory string
	packages         []*packages.Package
	files            map[string]string
	byKey            map[sourceKey]*templateSource
	keys             []sourceKey

	// delims are the delimiters each source was parsed with, read off the
	// definitions before any source is built. A source scans its actions
	// as it is constructed, so the delimiters have to be known by then,
	// and the definition that reveals them is not necessarily the first
	// one filed for that source.
	delims map[sourceKey][2]string
}

// resolveDelimiters reads the delimiters each source was parsed with off
// the definitions written in it.
//
// They are keyed by source rather than by file because a construction
// chain may call Delims more than once, and one Go file can hold several
// parsed literals. Keying by file would give every literal in a file the
// pair of whichever definition was seen first, and the rest would be read
// with delimiters they were not written in -- which yields a tree with no
// actions in it and a template that silently contributes no mutants.
//
// Within one source the pair is fixed, so the first definition that
// reveals it answers for the whole source. A source whose only template
// has no define clause reveals nothing and keeps the defaults.
func (c *sourceCollector) resolveDelimiters(defs []check.Definition) {
	for _, definition := range defs {
		file := definition.Define.Position.Filename
		if file == "" {
			continue
		}
		key, ok := c.keyFor(definition)
		if !ok {
			continue
		}
		if _, known := c.delims[key]; known {
			continue
		}
		text, err := c.read(file)
		if err != nil {
			continue
		}
		if left, right, ok := delimiters(text, definition); ok {
			c.delims[key] = [2]string{left, right}
		}
	}
}

// keyFor names the source a definition was written in.
//
// It is the same key add files the definition under, so the delimiters
// resolved here reach the source they were read from.
func (c *sourceCollector) keyFor(definition check.Definition) (sourceKey, bool) {
	file := definition.Define.Position.Filename
	if filepath.Ext(file) != ".go" {
		return sourceKey{file: file}, true
	}
	litStart, _, ok := findStringLiteral(c.packages, file, definition.Define.Offset)
	if !ok {
		return sourceKey{}, false
	}
	return sourceKey{file: file, litStart: litStart}, true
}

func newSourceCollector(workingDirectory string, pl []*packages.Package, defs []check.Definition) *sourceCollector {
	c := &sourceCollector{
		workingDirectory: workingDirectory,
		packages:         pl,
		files:            make(map[string]string),
		byKey:            make(map[sourceKey]*templateSource),
		delims:           make(map[sourceKey][2]string),
	}
	c.resolveDelimiters(defs)
	return c
}

// delimitersFor reports the delimiters a source was parsed with, empty
// for the text/template defaults.
func (c *sourceCollector) delimitersFor(key sourceKey) (string, string) {
	pair := c.delims[key]
	return pair[0], pair[1]
}

// add files one definition under the text it was written in, reading
// that text at most once, and reports the source it was filed under.
//
// A definition the collector cannot place -- a Go string literal it has
// no package for -- is reported as no source rather than as an error,
// since the caller may hold others it can still use.
func (c *sourceCollector) add(definition check.Definition) (*templateSource, error) {
	file := definition.Define.Position.Filename
	if file == "" {
		return nil, nil
	}
	fileText, err := c.read(file)
	if err != nil {
		return nil, err
	}

	var src *templateSource
	if filepath.Ext(file) != ".go" {
		left, right := c.delimitersFor(sourceKey{file: file})
		src, err = c.source(sourceKey{file: file}, func() (*templateSource, error) {
			return newFileSource(file, c.relative(file), fileText, left, right), nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		litStart, litEnd, ok := findStringLiteral(c.packages, file, definition.Define.Offset)
		if !ok {
			return nil, nil
		}
		left, right := c.delimitersFor(sourceKey{file: file, litStart: litStart})
		src, err = c.source(sourceKey{file: file, litStart: litStart}, func() (*templateSource, error) {
			return newLiteralSource(file, c.relative(file), definition.Name, fileText, left, right, litStart, litEnd)
		})
		if err != nil {
			return nil, err
		}
		if !definition.TemplateName.IsValid() {
			// A definition with no define clause is the template the
			// literal's own text carries, so its name is the root name
			// the text has to be parsed under.
			src.rootName = definition.Name
		}
	}

	return src, nil
}

func (c *sourceCollector) source(key sourceKey, build func() (*templateSource, error)) (*templateSource, error) {
	if existing, ok := c.byKey[key]; ok {
		return existing, nil
	}
	src, err := build()
	if err != nil {
		return nil, err
	}
	c.byKey[key] = src
	c.keys = append(c.keys, key)
	return src, nil
}

func (c *sourceCollector) read(file string) (string, error) {
	if text, ok := c.files[file]; ok {
		return text, nil
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	c.files[file] = string(b)
	return string(b), nil
}

func (c *sourceCollector) relative(file string) string {
	path, err := filepath.Rel(c.workingDirectory, file)
	if err != nil {
		return file
	}
	return filepath.ToSlash(path)
}

// sorted returns the collected sources in a stable order, so that two
// runs over an unchanged project read the same way.
func (c *sourceCollector) sorted() []*templateSource {
	keys := slices.Clone(c.keys)
	slices.SortFunc(keys, func(a, b sourceKey) int {
		return cmp.Or(
			cmp.Compare(a.file, b.file),
			cmp.Compare(a.litStart, b.litStart),
		)
	})
	sources := make([]*templateSource, 0, len(keys))
	for _, key := range keys {
		sources = append(sources, c.byKey[key])
	}
	return sources
}
