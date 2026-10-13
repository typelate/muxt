package mutation

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleReport is a finished run over two call sites. "page" holds a mutant
// of every status; "row" and "list" hold only kills.
func sampleReport() *Report {
	return &Report{
		Seed:       1,
		Templates:  3,
		Complexity: 4,
		Total:      7,
		Killed:     3,
		Missed:     3,
		Skipped:    1,
		Baseline:   BaselineResult{Passed: true},
		Groups: []Group{
			{
				CallSite: "page.go:9:9", Entry: "page", DataType: "server.Page",
				Templates: []TemplateReport{
					{
						Template: "page", File: "page.gohtml", DataType: "server.Page", Complexity: 2,
						Results: []Result{
							{Status: StatusKilled, Operator: OperatorActionZero, Line: 1, Column: 18, Mutated: `""`},
							{Status: StatusMissed, Operator: OperatorIfFalse, Line: 1, Column: 30, Mutated: "false"},
							{Status: StatusMissed, Operator: OperatorOperands, Line: 1, Column: 40, Mutated: `.A="abcd" .B=3`},
							{Status: StatusMissed, Operator: OperatorCondition, Line: 1, Column: 45, Mutated: ".A=false"},
							{Status: StatusSkipped, Operator: OperatorActionEmpty, Line: 2, Column: 1, Reason: "does not type check against server.Page"},
						},
					},
					{
						Template: "row", File: "page.gohtml", DataType: "server.Row", Via: true, Complexity: 1,
						Results: []Result{{Status: StatusKilled, Operator: OperatorTemplateDrop, Line: 3, Column: 5}},
					},
				},
			},
			{
				CallSite: "list.go:4:9", Entry: "list", DataType: "[]server.Row",
				Templates: []TemplateReport{{
					Template: "list", File: "list.gohtml", DataType: "[]server.Row", Complexity: 1,
					Results: []Result{{Status: StatusKilled, Operator: OperatorRangeNever, Line: 1, Column: 1}},
				}},
			},
		},
		Trimmed: []TrimmedTemplate{{CallSite: "page.go:12:9", Template: "row", DataType: "server.Row", FirstSeenAt: "page.go:9:9"}},
	}
}

func writeReport(t *testing.T, report *Report) string {
	t.Helper()
	var out strings.Builder
	n, err := report.WriteTo(&out)
	require.NoError(t, err)
	assert.Equal(t, int64(out.Len()), n, "WriteTo count of bytes written")
	return out.String()
}

// TestReportListsOnlyMisses states that a report lists what needs acting
// on: the misses, under their template and call site. A template or call
// site holding nothing but kills is left out entirely.
func TestReportListsOnlyMisses(t *testing.T) {
	const want = `7 mutants across 3 templates (complexity 4, seed 1)
baseline ok

page.go:9:9 ExecuteTemplate "page" (dot: server.Page)
  "page" page.gohtml (complexity 2, dot: server.Page)
    MISS 1:30 if-false
    MISS 1:40 operands .A="abcd" .B=3
    MISS 1:45 condition .A=false

7 mutants, 3 killed, 3 missed, 1 skipped
`
	assert.Equal(t, want, writeReport(t, sampleReport()), "report")
}

// TestVerboseReportListsEverything states that a verbose report lists every
// mutant, why each skipped one was not run, and the subtrees trimmed.
func TestVerboseReportListsEverything(t *testing.T) {
	report := sampleReport()
	report.Verbose = true
	const want = `7 mutants across 3 templates (complexity 4, seed 1)
baseline ok

page.go:9:9 ExecuteTemplate "page" (dot: server.Page)
  "page" page.gohtml (complexity 2, dot: server.Page)
    KILL 1:18 action-zero
    MISS 1:30 if-false
    MISS 1:40 operands .A="abcd" .B=3
    MISS 1:45 condition .A=false
    SKIP 2:1 action-empty (does not type check against server.Page)
  "row" page.gohtml via {{template}} (complexity 1, dot: server.Row)
    KILL 3:5 template-drop

list.go:4:9 ExecuteTemplate "list" (dot: []server.Row)
  "list" list.gohtml (complexity 1, dot: []server.Row)
    KILL 1:1 range-never

trimmed 1 subtree already mutated with the same dot:
  "row" at page.go:12:9, first reached from page.go:9:9

7 mutants, 3 killed, 3 missed, 1 skipped
`
	assert.Equal(t, want, writeReport(t, report), "report")
}

// TestDryRunReportListsWhatWouldRun states that a dry run has no verdicts to
// filter by, so it lists every mutant, and counts what would run.
func TestDryRunReportListsWhatWouldRun(t *testing.T) {
	report := sampleReport()
	report.DryRun = true
	got := writeReport(t, report)
	for _, want := range []string{
		"\ndry run: no tests were run\n",
		"    KILL 3:5 template-drop\n",
		"\n7 mutants, 6 runnable, 1 skipped\n",
	} {
		assert.Contains(t, got, want, "report")
	}
}

// TestReportSaysTheBaselineFailed states the preamble of a report whose
// unmutated run did not pass.
func TestReportSaysTheBaselineFailed(t *testing.T) {
	report := sampleReport()
	report.Baseline = BaselineResult{}
	assert.Contains(t, writeReport(t, report), "(complexity 4, seed 1)\nbaseline failed\n", "report says the baseline failed")
}

// TestReportErr states which runs fail the command once their report is
// written: one that let a mutant through. Skipped mutants are not run, so
// they fail nothing, and a dry run has no verdicts at all.
func TestReportErr(t *testing.T) {
	for _, tt := range []struct {
		name       string
		report     Report
		wantMissed int
	}{
		{name: "a mutant missed", report: Report{Total: 2, Killed: 1, Missed: 1}, wantMissed: 1},
		{name: "mutants missed", report: Report{Total: 5, Killed: 1, Missed: 3, Skipped: 1}, wantMissed: 3},
		{name: "every mutant killed", report: Report{Total: 2, Killed: 2}},
		{name: "killed and skipped", report: Report{Total: 3, Killed: 2, Skipped: 1}},
		{name: "nothing to mutate", report: Report{}},
		{name: "a dry run", report: Report{DryRun: true, Total: 2}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.report.Err()
			if tt.wantMissed == 0 {
				assert.NoError(t, err, "Err()")
				return
			}
			missed, ok := errors.AsType[*MissedMutantsError](err)
			require.True(t, ok, "Err() = %v, want a MissedMutantsError", err)
			assert.Equal(t, tt.wantMissed, missed.Missed, "Err() missed")
		})
	}
}
