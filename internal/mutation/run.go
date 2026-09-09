// Package mutation varies the actions in a project's templates and
// re-runs its tests, reporting every variation the tests still pass
// through.
//
// A template action that can be changed without failing a test is a gap:
// nothing the suite asserts on depends on what that action does. The
// report names those gaps so they can be closed one at a time.
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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/typelate/check"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/asteval"
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
}

// Result pairs a mutant with the verdict its test run produced.
type Result struct {
	Status   Status `json:"status"`
	Operator string `json:"operator"`
	Template string `json:"template"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`
}

// Report is the outcome of a whole mutation run.
type Report struct {
	Baseline BaselineResult `json:"baseline"`
	Results  []Result       `json:"results"`
	Total    int            `json:"total"`
	Killed   int            `json:"killed"`
	Missed   int            `json:"missed"`
}

// BaselineResult records the unmutated run the mutants are compared against.
type BaselineResult struct {
	// Passed is whether the tests pass with no mutation in place. A
	// mutation run is only meaningful when they do.
	Passed bool `json:"passed"`
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

// Run mutates every action the configuration selects and reports which
// variations the tests catch.
func Run(config Configuration, workingDirectory string, pl []*packages.Package) (*Report, error) {
	mutants, err := enumerate(config, workingDirectory, pl)
	if err != nil {
		return nil, err
	}

	tester := goTest{dir: workingDirectory, packages: config.Packages, match: config.Run}
	if len(tester.packages) == 0 {
		tester.packages = []string{"./..."}
	}

	if out, err := tester.run(nil); err != nil {
		if !isTestFailure(err) {
			return nil, err
		}
		return nil, &BaselineFailedError{Output: out}
	}

	report := &Report{
		Baseline: BaselineResult{Passed: true},
		Total:    len(mutants),
		Results:  make([]Result, 0, len(mutants)),
	}
	if len(mutants) == 0 {
		return report, nil
	}

	scratch, err := os.MkdirTemp("", "muxt-mutation-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	for i, mutant := range mutants {
		overlay, err := writeMutant(scratch, i, mutant)
		if err != nil {
			return nil, err
		}

		status := StatusMissed
		if _, err := tester.run([]string{"-overlay=" + overlay}); err != nil {
			if !isTestFailure(err) {
				return nil, err
			}
			status = StatusKilled
		}

		if status == StatusKilled {
			report.Killed++
		} else {
			report.Missed++
		}
		report.Results = append(report.Results, Result{
			Status:   status,
			Operator: mutant.Operator,
			Template: mutant.Template,
			File:     mutant.Path,
			Line:     mutant.Line,
			Column:   mutant.Column,
			Original: mutant.Action(),
			Mutated:  mutant.Replacement(),
		})
	}
	return report, nil
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

// enumerate collects every mutant the configuration selects, in a stable
// order.
func enumerate(config Configuration, workingDirectory string, pl []*packages.Package) ([]Mutant, error) {
	include := func(string) bool { return true }
	if config.TemplatePattern != nil {
		include = config.TemplatePattern.MatchString
	}

	funcs := make(map[string]any)
	collector := &sourceCollector{
		workingDirectory: workingDirectory,
		packages:         pl,
		files:            make(map[string]string),
		byKey:            make(map[sourceKey]*templateSource),
	}

	for _, templatesVariable := range config.TemplatesVariables {
		lt, err := asteval.LoadTemplates(workingDirectory, templatesVariable, pl)
		if err != nil {
			return nil, err
		}
		for name := range lt.Templates.Functions() {
			funcs[name] = func() string { return "" }
		}
		for _, t := range lt.HTML.Templates() {
			definition, ok := lt.Templates.FindDefinition(t.Name())
			if !ok {
				continue
			}
			if err := collector.add(definition); err != nil {
				return nil, err
			}
		}
	}

	var all []Mutant
	for _, src := range collector.sorted() {
		found, err := mutantsInSource(src, funcs, include)
		if err != nil {
			return nil, err
		}
		all = append(all, found...)
	}
	return all, nil
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
// runs over an unchanged project report the same mutants in the same
// sequence.
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
