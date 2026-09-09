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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
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
}

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
	Status   Status `json:"status"`
	Operator string `json:"operator"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`

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
	Verbose    bool              `json:"-"`
	Templates  int               `json:"templates"`
	Complexity int               `json:"complexity"`
	Total      int               `json:"total"`
	Killed     int               `json:"killed"`
	Missed     int               `json:"missed"`
	Skipped    int               `json:"skipped"`
	Groups     []Group           `json:"groups"`
	Trimmed    []TrimmedTemplate `json:"trimmed"`
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

// Run mutates every action the configuration selects and reports which
// variations the tests catch.
//
// progress receives a line per mutant as it completes when the
// configuration is verbose; it may be nil.
func Run(config Configuration, workingDirectory string, status io.Writer) (*Report, error) {
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

	tester := goTest{dir: workingDirectory, packages: config.Packages, match: config.Run}
	if len(tester.packages) == 0 {
		tester.packages = []string{"./..."}
	}

	started := time.Now()
	if out, err := tester.run(nil); err != nil {
		if !isTestFailure(err) {
			return nil, err
		}
		return nil, &BaselineFailedError{Output: out}
	}
	baseline := time.Since(started)
	report.Baseline = BaselineResult{Passed: true, Seconds: baseline.Seconds()}

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

	clock := &estimate{perMutant: baseline, remaining: plan.runnable()}
	index := 0
	for group := range report.eachTemplate() {
		for i := range group.Results {
			result := &group.Results[i]
			if result.Status == StatusSkipped {
				reportProgress(progress, index, plan.total(), group, *result, clock)
				index++
				continue
			}

			mutant := plan.mutants[result.mutantIndex]
			overlay, err := writeMutant(scratch, index, mutant)
			if err != nil {
				return nil, err
			}

			runStarted := time.Now()
			_, testErr := tester.run([]string{"-overlay=" + overlay})
			elapsed := time.Since(runStarted)
			if testErr != nil && !isTestFailure(testErr) {
				return nil, testErr
			}

			result.Seconds = elapsed.Seconds()
			if testErr != nil {
				result.Status = StatusKilled
				report.Killed++
			} else {
				result.Status = StatusMissed
				report.Missed++
			}
			clock.observe(elapsed)
			reportProgress(progress, index, plan.total(), group, *result, clock)
			index++
		}
	}
	return report, nil
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
	return time.Duration(e.remaining) * e.perMutant
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
}

func (t goTest) run(extra []string) (string, error) {
	args := []string{"test", "-count=1"}
	args = append(args, extra...)
	if t.match != nil {
		args = append(args, "-run="+t.match.String())
	}
	args = append(args, t.packages...)

	cmd := exec.Command("go", args...)
	cmd.Dir = t.dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// isTestFailure reports whether the go command exited non zero, which is
// what a failing test looks like, as opposed to the command not running
// at all.
func isTestFailure(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
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
}

func newSourceCollector(workingDirectory string, pl []*packages.Package) *sourceCollector {
	return &sourceCollector{
		workingDirectory: workingDirectory,
		packages:         pl,
		files:            make(map[string]string),
		byKey:            make(map[sourceKey]*templateSource),
	}
}

func (c *sourceCollector) add(definition check.Definition) error {
	file := definition.Define.Position.Filename
	if file == "" {
		return nil
	}
	fileText, err := c.read(file)
	if err != nil {
		return err
	}

	if filepath.Ext(file) != ".go" {
		_, err := c.source(sourceKey{file: file}, func() (*templateSource, error) {
			return newFileSource(file, c.relative(file), fileText), nil
		})
		return err
	}

	litStart, litEnd, ok := findStringLiteral(c.packages, file, definition.Define.Offset)
	if !ok {
		return nil
	}
	src, err := c.source(sourceKey{file: file, litStart: litStart}, func() (*templateSource, error) {
		return newLiteralSource(file, c.relative(file), definition.Name, fileText, litStart, litEnd)
	})
	if err != nil {
		return err
	}
	if !definition.TemplateName.IsValid() {
		// A definition with no define clause is the template the
		// literal's own text carries, so its name is the root name the
		// text has to be parsed under.
		src.rootName = definition.Name
	}
	return nil
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
