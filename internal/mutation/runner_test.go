package mutation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	for _, workers := range []int{1, 4} {
		report, p := runnerFixture(t, kills)
		r := &mutantRunner{
			plan:    p,
			scratch: t.TempDir(),
			clock:   &estimate{remaining: len(kills), workers: workers},
			test: func(overlay string) (Status, error) {
				if strings.Contains(readMutated(t, overlay), "K") {
					return StatusKilled, nil
				}
				return StatusMissed, nil
			},
		}
		if err := r.runAll(report, workers); err != nil {
			t.Fatalf("workers %d: runAll = %v", workers, err)
		}

		results := report.Groups[0].Templates[0].Results
		for i, kill := range kills {
			want := StatusMissed
			if kill {
				want = StatusKilled
			}
			if results[i].Status != want {
				t.Errorf("workers %d: result %d = %s, want %s", workers, i, results[i].Status, want)
			}
		}
		if report.Killed != 4 || report.Missed != 4 {
			t.Errorf("workers %d: killed %d, missed %d, want 4 and 4", workers, report.Killed, report.Missed)
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
		clock:   &estimate{remaining: 3, workers: 1},
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

// TestRunAllReportsEachMutantAsItFinishes states the progress stream: a
// line per mutant, numbered as it finishes, with the time left falling as
// runs complete. A skipped mutant says why, and since it costs nothing it
// does not move the estimate.
func TestRunAllReportsEachMutantAsItFinishes(t *testing.T) {
	report, p := runnerFixture(t, []bool{true, false, false})
	skipped := &report.Groups[0].Templates[0].Results[1]
	skipped.Status, skipped.Reason = StatusSkipped, "does not type check"

	var progress strings.Builder
	r := &mutantRunner{
		plan:     p,
		scratch:  t.TempDir(),
		progress: &progress,
		// An hour a mutant until a run says otherwise: each run that
		// finishes has to bring the estimate down with it.
		clock: &estimate{perMutant: time.Hour, remaining: 2, workers: 1},
		test: func(overlay string) (Status, error) {
			if strings.Contains(readMutated(t, overlay), "K") {
				return StatusKilled, nil
			}
			return StatusMissed, nil
		},
	}
	if err := r.runAll(report, 1); err != nil {
		t.Fatalf("runAll = %v", err)
	}

	want := []string{
		fmt.Sprintf(`[1/3] KILL page.gohtml:0:0 "page" %s (0s, ~0s left)`, OperatorActionEmpty),
		fmt.Sprintf(`[2/3] SKIP page.gohtml:0:0 "page" %s (does not type check)`, OperatorActionEmpty),
		fmt.Sprintf(`[3/3] MISS page.gohtml:0:0 "page" %s (0s, ~0s left)`, OperatorActionEmpty),
	}
	if got := strings.Split(strings.TrimSpace(progress.String()), "\n"); !slices.Equal(got, want) {
		t.Errorf("progress:\n got %q\nwant %q", got, want)
	}
}

// TestReportTrims states the progress line for a trimmed subtree, which
// says where the same template was already mutated with the same dot. A
// nil writer means progress is not wanted.
func TestReportTrims(t *testing.T) {
	trimmed := []TrimmedTemplate{{CallSite: "page.go:12:9", Template: "row", DataType: "server.Row", FirstSeenAt: "page.go:9:9"}}
	var out strings.Builder
	reportTrims(&out, trimmed)
	if got, want := out.String(), "trimmed \"row\" at page.go:12:9: already mutated with server.Row from page.go:9:9\n"; got != want {
		t.Errorf("reportTrims wrote %q, want %q", got, want)
	}
	reportTrims(nil, trimmed)
}

// TestRoundDuration states how a duration is shown: to a tenth of a second
// under a minute, and to the second above.
func TestRoundDuration(t *testing.T) {
	for _, tt := range []struct {
		d    time.Duration
		want string
	}{
		{d: 0, want: "0s"},
		{d: 1234 * time.Millisecond, want: "1.2s"},
		{d: 90*time.Second + 600*time.Millisecond, want: "1m31s"},
	} {
		if got := roundDuration(tt.d); got != tt.want {
			t.Errorf("roundDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
