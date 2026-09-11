package mutation

import "testing"

// TestPlanReportCounts states how a plan's mutants are counted: an action
// too wide to enumerate counts once, as skipped, alongside the mutants
// skipped for not type checking.
func TestPlanReportCounts(t *testing.T) {
	p := &plan{mutants: make([]Mutant, 5), runnableN: 3, overBudget: 2}
	report := p.report()
	if report.Total != 7 || report.Skipped != 4 {
		t.Errorf("total %d, skipped %d, want 7 and 4", report.Total, report.Skipped)
	}
}
