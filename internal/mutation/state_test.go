package mutation

import "testing"

// TestStateVerdictKeyedByActionOperatorAndMutation states that a recorded
// verdict is handed back only for the mutant that reached it.
//
// The key has three parts, and the fingerprint is only one of them. Two
// operators on one action can substitute the same text -- action-empty
// and a one-operand combination both write "" -- so dropping either of
// the other two parts hands a verdict to a mutant that never earned it,
// which is the failure the whole reuse mechanism exists to avoid.
func TestStateVerdictKeyedByActionOperatorAndMutation(t *testing.T) {
	state := &State{Version: stateVersion, Actions: make(map[string]ActionState)}
	state.record("fp-1", "page", "page.gohtml", OperatorActionEmpty, `""`, StatusKilled)
	state.record("fp-1", "page", "page.gohtml", OperatorOperands, `""`, StatusMissed)
	state.record("fp-2", "page", "page.gohtml", OperatorActionEmpty, `""`, StatusMissed)

	for _, tt := range []struct {
		name        string
		fingerprint string
		operator    Operator
		mutated     string
		want        Status
		found       bool
	}{
		{
			name:        "the operator selects between two mutants substituting the same text",
			fingerprint: "fp-1", operator: OperatorActionEmpty, mutated: `""`,
			want: StatusKilled, found: true,
		},
		{
			name:        "and the other one keeps its own verdict",
			fingerprint: "fp-1", operator: OperatorOperands, mutated: `""`,
			want: StatusMissed, found: true,
		},
		{
			name:        "the fingerprint separates the same operator on two actions",
			fingerprint: "fp-2", operator: OperatorActionEmpty, mutated: `""`,
			want: StatusMissed, found: true,
		},
		{
			name:        "a substitution that was never recorded is not answered",
			fingerprint: "fp-1", operator: OperatorActionEmpty, mutated: `"other"`,
			found: false,
		},
		{
			name:        "an operator that was never recorded is not answered",
			fingerprint: "fp-1", operator: OperatorIfTrue, mutated: `""`,
			found: false,
		},
		{
			name:        "an action that was never recorded is not answered",
			fingerprint: "fp-3", operator: OperatorActionEmpty, mutated: `""`,
			found: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, found := state.verdict(tt.fingerprint, tt.operator, tt.mutated)
			if found != tt.found {
				t.Fatalf("verdict found = %t, want %t", found, tt.found)
			}
			if found && got != tt.want {
				t.Errorf("verdict = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestStateRecordReplacesAVerdict states that recording the same mutant
// twice updates it rather than leaving two answers behind, since a stale
// one could then be read back.
func TestStateRecordReplacesAVerdict(t *testing.T) {
	state := &State{Version: stateVersion, Actions: make(map[string]ActionState)}
	state.record("fp-1", "page", "page.gohtml", OperatorIfTrue, "true", StatusMissed)
	state.record("fp-1", "page", "page.gohtml", OperatorIfTrue, "true", StatusKilled)

	if got := len(state.Actions["fp-1"].Results); got != 1 {
		t.Fatalf("results = %d, want 1: recording a mutant again should replace its verdict", got)
	}
	if got, _ := state.verdict("fp-1", OperatorIfTrue, "true"); got != StatusKilled {
		t.Errorf("verdict = %q, want %q", got, StatusKilled)
	}
}
