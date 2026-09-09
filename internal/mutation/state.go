package mutation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// DefaultStatePath is where a run records what it learned, so the next
// run only has to try what changed.
//
// It sits in testdata because it belongs to the tests: it is the record
// of which template behaviour the suite was shown to cover, and it should
// be reviewed and committed alongside the tests that produced it.
const DefaultStatePath = "testdata/template-mutations.json"

// stateVersion guards the file's shape. A run that finds a version it
// does not know starts from nothing rather than trusting a record it may
// read wrongly.
const stateVersion = 1

// State is what a previous run learned.
type State struct {
	Version int `json:"version"`

	// Seed is the seed those verdicts were reached with. It is recorded
	// so the file says how to reproduce them; it also feeds each
	// action's fingerprint, so changing it retries everything.
	Seed uint64 `json:"seed"`

	// Engine is the muxt version that reached them, recorded for the
	// same reason: it feeds every fingerprint, so an improvement to the
	// mutation engine retries everything rather than trusting verdicts
	// reached by an older one.
	Engine string `json:"engine"`

	// Actions maps an action's fingerprint to the verdicts its mutants
	// reached.
	Actions map[string]ActionState `json:"actions"`
}

// ActionState is one action's record.
type ActionState struct {
	Template string        `json:"template"`
	File     string        `json:"file"`
	Results  []StateResult `json:"results"`
}

// StateResult is one mutant's recorded verdict.
type StateResult struct {
	Operator string `json:"operator"`
	Mutated  string `json:"mutated"`
	Status   Status `json:"status"`
}

// loadState reads the state file, returning an empty state when there is
// none or when it cannot be trusted.
//
// A missing or unreadable state is not an error: it only means everything
// has to be run, which is what a first run does anyway.
func loadState(path string) *State {
	empty := &State{Version: stateVersion, Actions: make(map[string]ActionState)}
	if path == "" {
		return empty
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return empty
	}
	var state State
	if err := json.Unmarshal(b, &state); err != nil || state.Version != stateVersion {
		return empty
	}
	if state.Actions == nil {
		state.Actions = make(map[string]ActionState)
	}
	return &state
}

// verdict returns what a mutant was found to be last time, if the action
// it varies is unchanged.
func (s *State) verdict(fingerprint, operator, mutated string) (Status, bool) {
	action, ok := s.Actions[fingerprint]
	if !ok {
		return "", false
	}
	for _, result := range action.Results {
		if result.Operator == operator && result.Mutated == mutated {
			return result.Status, true
		}
	}
	return "", false
}

// save writes the state, creating the directory it lives in.
func (s *State) save(path string) error {
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	// Sorted for a readable diff: this file is reviewed and committed.
	for fingerprint, action := range s.Actions {
		slices.SortFunc(action.Results, func(a, b StateResult) int {
			if a.Operator != b.Operator {
				return compareStrings(a.Operator, b.Operator)
			}
			return compareStrings(a.Mutated, b.Mutated)
		})
		s.Actions[fingerprint] = action
	}
	b, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// record adds a mutant's verdict.
func (s *State) record(fingerprint, template, file, operator, mutated string, status Status) {
	action, ok := s.Actions[fingerprint]
	if !ok {
		action = ActionState{Template: template, File: file}
	}
	for i, result := range action.Results {
		if result.Operator == operator && result.Mutated == mutated {
			action.Results[i].Status = status
			s.Actions[fingerprint] = action
			return
		}
	}
	action.Results = append(action.Results, StateResult{Operator: operator, Mutated: mutated, Status: status})
	s.Actions[fingerprint] = action
}

