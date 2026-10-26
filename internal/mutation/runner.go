package mutation

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// mutantRunner runs the mutants a plan enumerated and writes each
// verdict into the report.
type mutantRunner struct {
	plan     *plan
	scratch  string
	progress io.Writer

	// templates, when set, receives one line as each template's last
	// mutant finishes. Unlike progress it is short enough for a log.
	templates io.Writer

	// test runs the suite against one mutant's overlay and says whether
	// the tests caught it. An error means the tests could not be run at
	// all, which stops the run.
	// For a kill it also returns the tests that failed, by name.
	test func(ctx context.Context, overlay string) (Status, []string, error)

	// recheck, when set, runs a kill's failing tests again alone. With
	// more than one worker every kill goes through it once the parallel
	// runs are done, and one the tests pass this time is flaky, not
	// killed. It says whether the failure is still there.
	recheck func(ctx context.Context, overlay string, failed []string) (Status, error)

	// mu guards everything below it, which every run updates as it
	// finishes.
	mu    sync.Mutex
	clock *estimate
	done  int
	err   error

	// pending tracks the mutants each template still has to finish.
	pending       map[*TemplateReport]*templateProgress
	templatesDone int
	templateTotal int

	// rechecking is set when kills will be repeated alone, which makes a
	// kill seen by a template's line provisional.
	rechecking bool
}

// templateProgress sums the verdicts of one template as they arrive.
type templateProgress struct {
	remaining             int
	total, killed, missed int
	skipped               int
	seconds               float64
}

// runAll runs every mutant in the report, at most workers at a time.
//
// Each mutant writes its own overlay under scratch and runs its own go
// test, so no run can see another's mutation. A verdict is written into
// the report's own slot for it, so the report reads in plan order however
// the runs finish; only the progress stream comes out in the order they
// complete. The first error that is not a test failure stops new runs
// starting, and is returned once the ones already running have finished.
// ctx being done is such an error, and interrupts the ones running.
func (r *mutantRunner) runAll(ctx context.Context, report *Report, workers int) error {
	total := 0
	r.pending = make(map[*TemplateReport]*templateProgress)
	for group := range report.eachTemplate() {
		total += len(group.Results)
		if n := len(group.Results); n > 0 {
			r.pending[group] = &templateProgress{remaining: n, total: n}
		}
	}
	r.templateTotal = len(r.pending)
	r.rechecking = workers > 1 && r.recheck != nil

	slots := make(chan struct{}, workers)
	var running sync.WaitGroup
dispatch:
	for group := range report.eachTemplate() {
		for i := range group.Results {
			slots <- struct{}{}
			// Checked only once a slot is free: the run that failed frees
			// its slot after recording the error, so this is the first
			// point at which a failure is certain to be seen.
			if r.failed() || ctx.Err() != nil {
				<-slots
				break dispatch
			}
			running.Add(1)
			go func(group *TemplateReport, result *Result) {
				defer running.Done()
				defer func() { <-slots }()
				r.finish(group, result, total, r.run(ctx, result))
			}(group, &group.Results[i])
		}
	}
	running.Wait()

	if r.err != nil {
		return r.err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.rechecking {
		if err := r.confirmKills(ctx, report); err != nil {
			return err
		}
	}
	report.tally()
	return nil
}

// confirmKills repeats every kill alone, one at a time, with nothing else
// running, and turns the ones that do not repeat into flaky results.
//
// A mutant run beside others can fail a test that has nothing to do with
// the mutation, from a shared port, a timeout or a busy machine, and
// would be counted as caught. Each repeat is at most one more run of the
// mutant, narrowed to the tests that failed when it can be, and it is
// serial because contention is what it checks for.
//
// A repeat that cannot run keeps the kill it started with, and says why in
// the result's reason, rather than throwing away a report of runs that
// took minutes. Only ctx being done stops it.
func (r *mutantRunner) confirmKills(ctx context.Context, report *Report) error {
	kills := 0
	for group := range report.eachTemplate() {
		for _, result := range group.Results {
			if result.Status == StatusKilled {
				kills++
			}
		}
	}
	n := 0
	for group := range report.eachTemplate() {
		for i := range group.Results {
			result := &group.Results[i]
			if result.Status != StatusKilled {
				continue
			}
			n++
			status, err := r.recheck(ctx, result.overlay, result.failed)
			outcome := "killed"
			switch {
			case ctx.Err() != nil:
				return ctx.Err()
			case err != nil:
				result.Reason = "not re-checked alone: " + firstLine(err.Error())
				outcome = "kept as killed, " + result.Reason
			case status != StatusKilled:
				result.Status = StatusFlaky
				result.Reason = flakyReason(result.failed)
				outcome = "flaky"
			}
			if r.templates != nil {
				_, _ = fmt.Fprintf(r.templates, "[recheck %*d/%d] %s %s %d:%d %s: %s\n",
					len(fmt.Sprint(kills)), n, kills, group.File, strconv.Quote(group.Template),
					result.Line, result.Column, result.label(), outcome)
			}
		}
	}
	return nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func flakyReason(failed []string) string {
	if len(failed) == 0 {
		return "failed beside other mutants, passed when run alone"
	}
	return strings.Join(failed, ", ") + " failed beside other mutants, passed when run alone"
}

// run runs one mutant's tests and writes the verdict into result.
func (r *mutantRunner) run(ctx context.Context, result *Result) error {
	if result.Status == StatusSkipped {
		return nil
	}
	overlay, err := writeMutant(r.scratch, r.plan.mutants[result.mutantIndex])
	if err != nil {
		return err
	}
	started := time.Now()
	status, failed, err := r.test(ctx, overlay)
	result.Seconds = time.Since(started).Seconds()
	if err != nil {
		return err
	}
	result.Status = status
	result.overlay, result.failed = overlay, failed
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
	r.finishTemplate(group, *result)
}

// finishTemplate adds one verdict to its template, and writes the
// template's line once every mutant in it is done. r.mu is held.
func (r *mutantRunner) finishTemplate(group *TemplateReport, result Result) {
	t := r.pending[group]
	if t == nil {
		return
	}
	switch result.Status {
	case StatusKilled:
		t.killed++
	case StatusMissed:
		t.missed++
	case StatusSkipped:
		t.skipped++
	}
	t.seconds += result.Seconds
	if t.remaining--; t.remaining > 0 {
		return
	}
	delete(r.pending, group)
	r.templatesDone++
	if r.templates == nil {
		return
	}
	killed := fmt.Sprintf("%d killed", t.killed)
	if r.rechecking && t.killed > 0 {
		// Repeated alone once every mutant has run; the lines for that
		// say which of these hold.
		killed += " (unconfirmed)"
	}
	line := fmt.Sprintf("[%*d/%d] %s %s: %d %s, %s, %d missed",
		len(fmt.Sprint(r.templateTotal)), r.templatesDone, r.templateTotal,
		group.File, strconv.Quote(group.Template),
		t.total, pluralize(t.total, "mutant"), killed, t.missed)
	if t.skipped > 0 {
		line += fmt.Sprintf(", %d skipped", t.skipped)
	}
	line += ", " + roundDuration(time.Duration(t.seconds*float64(time.Second)))
	_, _ = fmt.Fprintln(r.templates, line)
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
func writeMutant(scratch string, m mutant) (string, error) {
	dir, err := os.MkdirTemp(scratch, "mutant-")
	if err != nil {
		return "", err
	}
	mutated := filepath.Join(dir, filepath.Base(m.File))
	if err := os.WriteFile(mutated, []byte(m.apply()), 0o600); err != nil {
		return "", err
	}
	overlay := filepath.Join(dir, "overlay.json")
	b, err := json.Marshal(struct {
		Replace map[string]string
	}{Replace: map[string]string{m.File: mutated}})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(overlay, b, 0o600); err != nil {
		return "", err
	}
	return overlay, nil
}
