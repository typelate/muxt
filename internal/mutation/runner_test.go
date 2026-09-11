package mutation

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// runnerFixture builds a report of n runnable mutants of one template.
//
// Mutant i replaces the action with "K" when kills[i] is set and "M"
// otherwise, so a fake test run can read the mutated file and answer as
// though the suite caught exactly the mutants marked K.
func runnerFixture(t *testing.T, kills []bool) (*Report, *plan) {
	t.Helper()
	const text = "{{.A}}"
	src := newFileSource(t.TempDir()+"/page.gohtml", "page.gohtml", text, "", "")

	p := &plan{runnableN: len(kills)}
	results := make([]Result, len(kills))
	for i, kill := range kills {
		marker := "M"
		if kill {
			marker = "K"
		}
		p.mutants = append(p.mutants, Mutant{
			Operator: OperatorActionEmpty,
			File:     src.file,
			src:      src,
			edits:    []edit{{start: 0, end: len(text), text: marker}},
		})
		results[i] = Result{Status: StatusPending, Operator: OperatorActionEmpty, mutantIndex: i}
	}
	report := &Report{Groups: []Group{{Templates: []TemplateReport{{
		Template: "page",
		File:     "page.gohtml",
		Results:  results,
	}}}}}
	return report, p
}

// readMutated returns the mutated text an overlay points at.
func readMutated(t *testing.T, overlay string) string {
	t.Helper()
	b, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatal(err)
	}
	var o struct{ Replace map[string]string }
	if err := json.Unmarshal(b, &o); err != nil {
		t.Fatal(err)
	}
	for _, mutated := range o.Replace {
		text, err := os.ReadFile(mutated)
		if err != nil {
			t.Fatal(err)
		}
		return string(text)
	}
	t.Fatal("overlay replaces nothing")
	return ""
}

// TestRunAllWritesVerdictsInPlanOrder states that however many mutants run
// at once, each verdict lands in its own mutant's slot and the counts are
// the same as a serial run's.
func TestRunAllWritesVerdictsInPlanOrder(t *testing.T) {
	kills := []bool{true, false, true, true, false, false, true, false}
	for _, parallel := range []int{1, 4} {
		report, p := runnerFixture(t, kills)
		r := &mutantRunner{
			plan:    p,
			scratch: t.TempDir(),
			clock:   &estimate{remaining: len(kills), parallel: parallel},
			test: func(overlay string) (Status, error) {
				if strings.Contains(readMutated(t, overlay), "K") {
					return StatusKilled, nil
				}
				return StatusMissed, nil
			},
		}
		if err := r.runAll(report, parallel); err != nil {
			t.Fatalf("parallel %d: runAll = %v", parallel, err)
		}

		results := report.Groups[0].Templates[0].Results
		for i, kill := range kills {
			want := StatusMissed
			if kill {
				want = StatusKilled
			}
			if results[i].Status != want {
				t.Errorf("parallel %d: result %d = %s, want %s", parallel, i, results[i].Status, want)
			}
		}
		if report.Killed != 4 || report.Missed != 4 {
			t.Errorf("parallel %d: killed %d, missed %d, want 4 and 4", parallel, report.Killed, report.Missed)
		}
	}
}

// TestRunAllStopsDispatchingAfterAnError states that once the go command
// fails to run at all, no further mutant is started.
//
// Each run is a full go test, so starting one more after an error that
// will be reported anyway can cost minutes.
func TestRunAllStopsDispatchingAfterAnError(t *testing.T) {
	report, p := runnerFixture(t, []bool{false, false, false})
	var calls atomic.Int32
	r := &mutantRunner{
		plan:    p,
		scratch: t.TempDir(),
		clock:   &estimate{remaining: 3, parallel: 1},
		test: func(string) (Status, error) {
			calls.Add(1)
			return "", errors.New("go could not run")
		},
	}
	if err := r.runAll(report, 1); err == nil {
		t.Fatal("runAll = nil, want the error the go command gave")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("mutants started = %d, want 1: nothing should start after the first error", got)
	}
}
