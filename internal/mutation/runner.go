package mutation

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// mutantRunner runs the mutants a plan enumerated and writes each
// verdict into the report.
type mutantRunner struct {
	plan     *plan
	scratch  string
	progress io.Writer

	// test runs the suite against one mutant's overlay and says whether
	// the tests caught it. An error means the tests could not be run at
	// all, which stops the run.
	test func(overlay string) (Status, error)

	// mu guards everything below it, which every run updates as it
	// finishes.
	mu    sync.Mutex
	clock *estimate
	done  int
	err   error
}

// runAll runs every mutant in the report, at most workers at a time.
//
// Each mutant writes its own overlay under scratch and runs its own go
// test, so no run can see another's mutation. A verdict is written into
// the report's own slot for it, so the report reads in plan order however
// the runs finish; only the progress stream comes out in the order they
// complete. The first error that is not a test failure stops new runs
// starting, and is returned once the ones already running have finished.
func (r *mutantRunner) runAll(report *Report, workers int) error {
	total := 0
	for group := range report.eachTemplate() {
		total += len(group.Results)
	}

	slots := make(chan struct{}, workers)
	var running sync.WaitGroup
dispatch:
	for group := range report.eachTemplate() {
		for i := range group.Results {
			slots <- struct{}{}
			// Checked only once a slot is free: the run that failed frees
			// its slot after recording the error, so this is the first
			// point at which a failure is certain to be seen.
			if r.failed() {
				<-slots
				break dispatch
			}
			running.Add(1)
			go func(group *TemplateReport, result *Result) {
				defer running.Done()
				defer func() { <-slots }()
				r.finish(group, result, total, r.run(result))
			}(group, &group.Results[i])
		}
	}
	running.Wait()

	if r.err != nil {
		return r.err
	}
	report.tally()
	return nil
}

// run runs one mutant's tests and writes the verdict into result.
func (r *mutantRunner) run(result *Result) error {
	if result.Status == StatusSkipped {
		return nil
	}
	overlay, err := writeMutant(r.scratch, r.plan.mutants[result.mutantIndex])
	if err != nil {
		return err
	}
	started := time.Now()
	status, err := r.test(overlay)
	result.Seconds = time.Since(started).Seconds()
	if err != nil {
		return err
	}
	result.Status = status
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
		strconv.Quote(group.Template), result.label())
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

	// workers is how many mutants run at once, so the remaining work
	// takes that many times fewer rounds.
	workers int
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
	workers := max(e.workers, 1)
	rounds := (e.remaining + workers - 1) / workers
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

// writeMutant writes the mutated file and the overlay pointing at it into
// a directory of their own under scratch, returning the overlay's path.
// No two mutants share a directory, so none can read another's mutation.
func writeMutant(scratch string, mutant Mutant) (string, error) {
	dir, err := os.MkdirTemp(scratch, "mutant-")
	if err != nil {
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
