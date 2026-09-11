package mutation

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

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

// WriteTo renders the report as text.
//
// A preamble says how much work there is, so the run can be left alone or
// a CI timeout set from it. Then one block per ExecuteTemplate call, and
// within it one block per template, because a reader closing gaps works
// through a template at a time rather than a file at a time.
//
// Only misses are listed unless the report is verbose: a caught mutation
// is the expected outcome and says nothing that needs acting on.
func (r *Report) WriteTo(w io.Writer) (int64, error) {
	counter := &countingWriter{w: w}
	out := bufio.NewWriter(counter)

	r.writePreamble(out)

	for _, group := range r.Groups {
		blocks := r.templateBlocks(group)
		if len(blocks) == 0 {
			continue
		}
		_, _ = fmt.Fprintf(out, "\n%s ExecuteTemplate %s (dot: %s)\n",
			group.CallSite, strconv.Quote(group.Entry), group.DataType)
		for _, block := range blocks {
			r.writeTemplate(out, block)
		}
	}

	r.writeTrimmed(out)
	r.writeSummary(out)

	if err := out.Flush(); err != nil {
		return counter.n, err
	}
	return counter.n, nil
}

func (r *Report) writePreamble(out *bufio.Writer) {
	_, _ = fmt.Fprintf(out, "%d %s across %d %s (complexity %d, seed %d)\n",
		r.Total, pluralize(r.Total, "mutant"),
		r.Templates, pluralize(r.Templates, "template"),
		r.Complexity, r.Seed)

	switch {
	case r.DryRun:
		_, _ = fmt.Fprintln(out, "dry run: no tests were run")
	case r.Baseline.Passed:
		_, _ = fmt.Fprintln(out, "baseline ok")
	default:
		_, _ = fmt.Fprintln(out, "baseline failed")
	}
}

// templateBlocks returns the templates in a group that have something to
// report, which without verbose means the ones holding a miss.
func (r *Report) templateBlocks(group Group) []TemplateReport {
	var blocks []TemplateReport
	for _, template := range group.Templates {
		results := r.visibleResults(template)
		if len(results) == 0 {
			continue
		}
		template.Results = results
		blocks = append(blocks, template)
	}
	return blocks
}

func (r *Report) visibleResults(template TemplateReport) []Result {
	if r.Verbose || r.DryRun {
		// A dry run has no verdicts to filter by: listing what would
		// run is the whole point of it.
		return template.Results
	}
	var missed []Result
	for _, result := range template.Results {
		if result.Status == StatusMissed {
			missed = append(missed, result)
		}
	}
	return missed
}

func (r *Report) writeTemplate(out *bufio.Writer, template TemplateReport) {
	via := ""
	if template.Via {
		via = " via {{template}}"
	}
	_, _ = fmt.Fprintf(out, "  %s %s%s (complexity %d, dot: %s)\n",
		strconv.Quote(template.Template), template.File, via,
		template.Complexity, template.DataType)

	for _, result := range template.Results {
		line := fmt.Sprintf("    %s %d:%d %s",
			result.Status, result.Line, result.Column, result.Operator)
		if (result.Operator == OperatorOperands || result.Operator == OperatorCondition) && result.Mutated != "" {
			// Which operands were substituted is the whole content of a
			// combination mutant; without it the lines are identical.
			line += " " + result.Mutated
		}
		if result.Reason != "" {
			line += " (" + result.Reason + ")"
		}
		_, _ = fmt.Fprintln(out, line)
	}
}

func (r *Report) writeTrimmed(out *bufio.Writer) {
	if !r.Verbose || len(r.Trimmed) == 0 {
		return
	}
	_, _ = fmt.Fprintf(out, "\ntrimmed %d %s already mutated with the same dot:\n",
		len(r.Trimmed), pluralize(len(r.Trimmed), "subtree"))
	for _, trimmed := range r.Trimmed {
		_, _ = fmt.Fprintf(out, "  %s at %s, first reached from %s\n",
			strconv.Quote(trimmed.Template), trimmed.CallSite, trimmed.FirstSeenAt)
	}
}

func (r *Report) writeSummary(out *bufio.Writer) {
	_, _ = fmt.Fprintln(out)
	if r.DryRun {
		_, _ = fmt.Fprintf(out, "%d %s, %d runnable, %d skipped\n",
			r.Total, pluralize(r.Total, "mutant"), r.Total-r.Skipped, r.Skipped)
		return
	}
	_, _ = fmt.Fprintf(out, "%d %s, %d killed, %d missed, %d skipped\n",
		r.Total, pluralize(r.Total, "mutant"), r.Killed, r.Missed, r.Skipped)
}

func pluralize(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// countingWriter counts the bytes written through it so WriteTo can
// report a total without buffering the whole report.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
