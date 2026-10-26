package mutation

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		p.mutants = append(p.mutants, mutant{
			Operator: OperatorActionEmpty,
			File:     src.file,
			src:      src,
			edits:    []edit{{start: 0, end: len(text), text: marker}},
		})
		results[i] = Result{Status: StatusPending, Operator: OperatorActionEmpty, mutantIndex: i}
	}
	// The group hangs off the plan, so the report a run builds from it
	// holds these results: a verdict written through one is the other's.
	p.templates = 1
	p.groups = []Group{{Templates: []TemplateReport{{
		Template: "page",
		File:     "page.gohtml",
		Results:  results,
	}}}}
	return p.report(), p
}

// readMutated returns the mutated text an overlay points at.
func readMutated(t *testing.T, overlay string) string {
	t.Helper()
	b, err := os.ReadFile(overlay)
	require.NoError(t, err)
	var o struct{ Replace map[string]string }
	require.NoError(t, json.Unmarshal(b, &o))
	for _, mutated := range o.Replace {
		text, err := os.ReadFile(mutated)
		require.NoError(t, err)
		return string(text)
	}
	require.Fail(t, "overlay replaces nothing")
	return ""
}

// TestWriteMutantMapsTheFileToItsMutatedCopy states that the overlay
// replaces the mutant's file with a copy holding the mutation, and that no
// two mutants share a directory.
func TestWriteMutantMapsTheFileToItsMutatedCopy(t *testing.T) {
	_, p := runnerFixture(t, []bool{true, false})
	scratch := t.TempDir()

	var dirs []string
	for i, mutant := range p.mutants {
		overlay, err := writeMutant(scratch, mutant)
		require.NoError(t, err)
		b, err := os.ReadFile(overlay)
		require.NoError(t, err)
		var got struct{ Replace map[string]string }
		require.NoError(t, json.Unmarshal(b, &got))
		mutated, ok := got.Replace[mutant.File]
		require.True(t, ok, "mutant %d overlay = %v, want %s replaced", i, got.Replace, mutant.File)
		require.Len(t, got.Replace, 1, "mutant %d overlay replaces only %s", i, mutant.File)

		assert.Equal(t, mutant.apply(), readMutated(t, overlay), "mutant %d copy", i)
		assert.Equal(t, filepath.Base(mutant.File), filepath.Base(mutated), "mutant %d copy name", i)
		assert.Equal(t, filepath.Dir(overlay), filepath.Dir(mutated), "mutant %d copy and overlay share a directory", i)
		assert.Equal(t, scratch, filepath.Dir(filepath.Dir(overlay)), "mutant %d directory is directly under the scratch directory", i)
		dirs = append(dirs, filepath.Dir(overlay))
	}
	assert.NotEqual(t, dirs[0], dirs[1], "mutants share a directory")
}

// TestRunAllWritesVerdictsInPlanOrder states that however many mutants run
// at once, each verdict lands in its own mutant's slot and the counts are
// the same as a serial run's.
func TestRunAllWritesVerdictsInPlanOrder(t *testing.T) {
	kills := []bool{true, false, true, true, false, false, true, false}
	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprintf("workers %d", workers), func(t *testing.T) {
			report, p := runnerFixture(t, kills)
			r := &mutantRunner{
				plan:    p,
				scratch: t.TempDir(),
				clock:   &estimate{remaining: len(kills), workers: workers},
				test: func(_ context.Context, overlay string) (Status, []string, error) {
					if strings.Contains(readMutated(t, overlay), "K") {
						return StatusKilled, nil, nil
					}
					return StatusMissed, nil, nil
				},
			}
			require.NoError(t, r.runAll(t.Context(), report, workers))

			results := report.Groups[0].Templates[0].Results
			for i, kill := range kills {
				want := StatusMissed
				if kill {
					want = StatusKilled
				}
				assert.Equal(t, want, results[i].Status, "result %d", i)
			}
			assert.Equal(t, 4, report.Killed, "killed")
			assert.Equal(t, 4, report.Missed, "missed")
		})
	}
}

// TestRunAllReportsAKillThatDoesNotRepeatAsFlaky states the rule for
// workers beyond one: a kill is repeated alone, once, and counts only if
// the failing tests fail again. One the tests pass the second time is
// flaky, and names the tests that failed.
func TestRunAllReportsAKillThatDoesNotRepeatAsFlaky(t *testing.T) {
	kills := []bool{true, true, false, true}
	for _, tt := range []struct {
		name         string
		workers      int
		wantStatus   []Status
		wantRechecks int
	}{
		{name: "beside other mutants", workers: 3, wantStatus: []Status{StatusKilled, StatusFlaky, StatusMissed, StatusFlaky}, wantRechecks: 3},
		{name: "one at a time nothing is repeated", workers: 1, wantStatus: []Status{StatusKilled, StatusKilled, StatusMissed, StatusKilled}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			report, p := runnerFixture(t, kills)
			var rechecks atomic.Int32
			r := &mutantRunner{
				plan:    p,
				scratch: t.TempDir(),
				clock:   &estimate{remaining: len(kills), workers: tt.workers},
				test: func(_ context.Context, overlay string) (Status, []string, error) {
					if strings.Contains(readMutated(t, overlay), "K") {
						return StatusKilled, []string{"TestPage"}, nil
					}
					return StatusMissed, nil, nil
				},
				// Only the first mutant's failure survives being repeated.
				recheck: func(_ context.Context, overlay string, failed []string) (Status, error) {
					rechecks.Add(1)
					assert.Equal(t, []string{"TestPage"}, failed, "tests handed to the repeat")
					if overlay == report.Groups[0].Templates[0].Results[0].overlay {
						return StatusKilled, nil
					}
					return StatusMissed, nil
				},
			}
			require.NoError(t, r.runAll(t.Context(), report, tt.workers))

			var got []Status
			for _, result := range report.Groups[0].Templates[0].Results {
				got = append(got, result.Status)
			}
			assert.Equal(t, tt.wantStatus, got, "statuses")
			assert.Equal(t, int32(tt.wantRechecks), rechecks.Load(), "kills repeated")
			if tt.workers > 1 {
				assert.Equal(t, 1, report.Killed, "killed")
				assert.Equal(t, 2, report.Flaky, "flaky")
				assert.Equal(t, "TestPage failed beside other mutants, passed when run alone", report.Groups[0].Templates[0].Results[1].Reason, "reason")
			}
		})
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
		test: func(context.Context, string) (Status, []string, error) {
			calls.Add(1)
			return "", nil, errors.New("go could not run")
		},
	}
	require.Error(t, r.runAll(t.Context(), report, 1), "runAll: the error the go command gave")
	assert.Equal(t, int32(1), calls.Load(), "mutants started: nothing should start after the first error")
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
		test: func(_ context.Context, overlay string) (Status, []string, error) {
			if strings.Contains(readMutated(t, overlay), "K") {
				return StatusKilled, nil, nil
			}
			return StatusMissed, nil, nil
		},
	}
	require.NoError(t, r.runAll(t.Context(), report, 1))

	want := []string{
		fmt.Sprintf(`[1/3] KILL page.gohtml:0:0 "page" %s (0s, ~0s left)`, OperatorActionEmpty),
		fmt.Sprintf(`[2/3] SKIP page.gohtml:0:0 "page" %s (does not type check)`, OperatorActionEmpty),
		fmt.Sprintf(`[3/3] MISS page.gohtml:0:0 "page" %s (0s, ~0s left)`, OperatorActionEmpty),
	}
	assert.Equal(t, want, strings.Split(strings.TrimSpace(progress.String()), "\n"), "progress")
}

// TestRunAllReportsEachTemplateAsItFinishes states the template progress
// stream: one line when the last mutant of a template is done, numbered
// among the templates that have mutants, and nothing per mutant.
func TestRunAllReportsEachTemplateAsItFinishes(t *testing.T) {
	report, p := runnerFixture(t, []bool{true, false, false})
	first := &report.Groups[0].Templates[0]
	first.Results[1].Status, first.Results[1].Reason = StatusSkipped, "does not type check"
	second := TemplateReport{Template: "row", File: "row.gohtml", Results: []Result{{Status: StatusPending, Operator: OperatorActionEmpty, mutantIndex: 0}}}
	report.Groups[0].Templates = append(report.Groups[0].Templates, second, TemplateReport{Template: "empty", File: "empty.gohtml"})

	for _, workers := range []int{1, 3} {
		t.Run(fmt.Sprintf("workers %d", workers), func(t *testing.T) {
			var templates strings.Builder
			r := &mutantRunner{
				plan:      p,
				scratch:   t.TempDir(),
				templates: &templates,
				clock:     &estimate{remaining: 3, workers: workers},
				test: func(_ context.Context, overlay string) (Status, []string, error) {
					if strings.Contains(readMutated(t, overlay), "K") {
						return StatusKilled, nil, nil
					}
					return StatusMissed, nil, nil
				},
			}
			for i := range report.Groups[0].Templates {
				for j := range report.Groups[0].Templates[i].Results {
					if report.Groups[0].Templates[i].Results[j].Status != StatusSkipped {
						report.Groups[0].Templates[i].Results[j].Status = StatusPending
					}
				}
			}
			require.NoError(t, r.runAll(t.Context(), report, workers))

			lines := strings.Split(strings.TrimSpace(templates.String()), "\n")
			assert.ElementsMatch(t, []string{
				`[1/2] page.gohtml "page": 3 mutants, 1 killed, 1 missed, 1 skipped, 0s`,
				`[1/2] row.gohtml "row": 1 mutant, 1 killed, 0 missed, 0s`,
			}, stripIndex(lines), "template lines")
			assert.Len(t, lines, 2, "one line per template with mutants:\n%s", templates.String())
		})
	}
}

// stripIndex drops the [n/N] prefix so lines can be compared in any
// completion order.
func stripIndex(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		_, rest, _ := strings.Cut(l, "] ")
		out[i] = "[1/2] " + rest
	}
	return out
}

// TestReportTrims states the progress line for a trimmed subtree, which
// says where the same template was already mutated with the same dot. A
// nil writer means progress is not wanted.
func TestReportTrims(t *testing.T) {
	trimmed := []TrimmedTemplate{{CallSite: "page.go:12:9", Template: "row", DataType: "server.Row", FirstSeenAt: "page.go:9:9"}}
	var out strings.Builder
	reportTrims(&out, trimmed)
	assert.Equal(t, "trimmed \"row\" at page.go:12:9: already mutated with server.Row from page.go:9:9\n", out.String(), "reportTrims")
	assert.NotPanics(t, func() { reportTrims(nil, trimmed) }, "reportTrims without a writer")
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
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, roundDuration(tt.d), "roundDuration(%v)", tt.d)
		})
	}
}

// TestRunAllKeepsAKillWhoseRepeatCannotRun states that a repeat which
// fails to run does not discard the report: the kill stands, with the
// error in its reason, and the other kills are still repeated.
func TestRunAllKeepsAKillWhoseRepeatCannotRun(t *testing.T) {
	report, p := runnerFixture(t, []bool{true, true})
	var templates strings.Builder
	r := &mutantRunner{
		plan:      p,
		scratch:   t.TempDir(),
		templates: &templates,
		clock:     &estimate{remaining: 2, workers: 2},
		test: func(context.Context, string) (Status, []string, error) {
			return StatusKilled, []string{"TestPage"}, nil
		},
		recheck: func(_ context.Context, overlay string, _ []string) (Status, error) {
			if overlay == report.Groups[0].Templates[0].Results[0].overlay {
				return "", errors.New("go: cannot start\nmore detail")
			}
			return StatusMissed, nil
		},
	}
	require.NoError(t, r.runAll(t.Context(), report, 2))

	results := report.Groups[0].Templates[0].Results
	assert.Equal(t, StatusKilled, results[0].Status, "the kill whose repeat failed")
	assert.Equal(t, "not re-checked alone: go: cannot start", results[0].Reason, "its reason")
	assert.Equal(t, StatusFlaky, results[1].Status, "the other kill was still repeated")
	assert.Equal(t, 1, report.Killed, "killed")
	assert.Equal(t, 1, report.Flaky, "flaky")
	assert.Contains(t, templates.String(), `[recheck 1/2] page.gohtml "page" 0:0 action-empty: kept as killed, not re-checked alone: go: cannot start`, "repeat progress")
	assert.Contains(t, templates.String(), `[recheck 2/2] page.gohtml "page" 0:0 action-empty: flaky`, "repeat progress")
	assert.Contains(t, templates.String(), "2 killed (unconfirmed)", "the template line says its kills are not yet confirmed")
}
