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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

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
	}
	if len(mutants) == 0 {
		return report, nil
	}

	scratch, err := os.MkdirTemp("", "muxt-mutation-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	texts := make(map[string]string)
	for i, mutant := range mutants {
		text, ok := texts[mutant.File]
		if !ok {
			b, err := os.ReadFile(mutant.File)
			if err != nil {
				return nil, err
			}
			text = string(b)
			texts[mutant.File] = text
		}

		overlay, err := writeMutant(scratch, i, mutant, text)
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
			Original: mutant.Pipeline(text),
			Mutated:  mutant.Replacement(),
		})
	}
	return report, nil
}

// writeMutant writes the mutated file and the overlay pointing at it,
// returning the overlay's path.
func writeMutant(scratch string, index int, mutant Mutant, text string) (string, error) {
	dir := filepath.Join(scratch, fmt.Sprintf("%06d", index))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	mutated := filepath.Join(dir, filepath.Base(mutant.File))
	if err := os.WriteFile(mutated, []byte(mutant.Apply(text)), 0o600); err != nil {
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

	var (
		all   []Mutant
		files []string
		seen  = make(map[string]struct{})
		funcs = make(map[string]any)
	)

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
			file := definition.Define.Position.Filename
			if file == "" || filepath.Ext(file) == ".go" {
				// Templates written as Go string literals are
				// reported but not yet mutated.
				continue
			}
			if _, ok := seen[file]; ok {
				continue
			}
			seen[file] = struct{}{}
			files = append(files, file)
		}
	}

	slices.Sort(files)
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		path, err := filepath.Rel(workingDirectory, file)
		if err != nil {
			path = file
		}
		found, err := mutantsInFile(file, filepath.ToSlash(path), filepath.Base(file), string(b), funcs, include)
		if err != nil {
			return nil, err
		}
		all = append(all, found...)
	}
	return all, nil
}
