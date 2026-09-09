package mutation

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

// WriteTo renders the report as text, one line per mutant.
//
// A line names the verdict, where the mutated pipeline is, the template
// holding it, and the variation applied, so a missed line reads as an
// instruction: assert on this action, in this template, at this place.
func (r *Report) WriteTo(w io.Writer) (int64, error) {
	counter := &countingWriter{w: w}
	out := bufio.NewWriter(counter)

	if r.Baseline.Passed {
		_, _ = fmt.Fprintln(out, "baseline ok")
	} else {
		_, _ = fmt.Fprintln(out, "baseline failed")
	}

	for _, result := range r.Results {
		_, _ = fmt.Fprintf(out, "%s %s:%d:%d %s %s\n",
			result.Status,
			result.File,
			result.Line,
			result.Column,
			strconv.Quote(result.Template),
			result.Operator,
		)
	}

	_, _ = fmt.Fprintf(out, "%d %s, %d killed, %d missed\n",
		r.Total, pluralize(r.Total, "mutant"), r.Killed, r.Missed)

	if err := out.Flush(); err != nil {
		return counter.n, err
	}
	return counter.n, nil
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
