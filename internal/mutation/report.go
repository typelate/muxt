package mutation

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"time"
)

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
		if result.Operator == OperatorOperands && result.Mutated != "" {
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
		if r.Baseline.Seconds > 0 {
			_, _ = fmt.Fprintf(out, "estimated %s\n", r.Estimate())
		}
		return
	}
	_, _ = fmt.Fprintf(out, "%d %s, %d killed, %d missed, %d skipped\n",
		r.Total, pluralize(r.Total, "mutant"), r.Killed, r.Missed, r.Skipped)
}

// Estimate is how long the whole run is expected to take, from the
// unmutated run's duration and the number of mutants that will be run.
func (r *Report) Estimate() string {
	runnable := r.Total - r.Skipped
	total := time.Duration(float64(runnable) * r.Baseline.Seconds * float64(time.Second))
	return roundDuration(total)
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
