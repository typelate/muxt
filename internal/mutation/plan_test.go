package mutation

import (
	"go/token"
	"go/types"
	"slices"
	"testing"
)

// newSelector is a selector with nothing to compare with and no template
// pattern: every scope counts as changed.
func newSelector(before revision) selector {
	return selector{
		before:    before,
		seen:      make(map[string]struct{}),
		unchanged: make(map[string]struct{}),
		include:   func(string) bool { return true },
	}
}

// trimOf is a repeat of a template already reached with the same dot,
// reported from one call site having first been reached from another.
func trimOf(template string, dot types.Type, at, first string) trim {
	position := func(file string) callSite {
		return callSite{Position: token.Position{Filename: file, Line: 1, Column: 1}}
	}
	return trim{
		call:     position(at),
		template: template,
		dataType: dot,
		firstFor: position(first),
	}
}

func names(chosen []scope) []string {
	var out []string
	for _, sc := range chosen {
		out = append(out, sc.template)
	}
	return out
}

// TestSelectorSkipsWhatARevisionAlreadyHeld states the rule a --diff run
// selects by: a scope is mutated when its template reads differently than
// it did at the revision, or when it is reached with a type of dot the
// revision did not reach it with.
func TestSelectorSkipsWhatARevisionAlreadyHeld(t *testing.T) {
	str, count := types.Typ[types.String], types.Typ[types.Int]
	before := scopesOf([]scope{scopeOf(t, "page", `<b>{{.}}</b>`, str)})

	for _, tt := range []struct {
		name          string
		scope         scope
		wantMutated   bool
		wantUnchanged string
	}{
		{
			name:          "the same text and dot",
			scope:         scopeOf(t, "page", `<b>{{.}}</b>`, str),
			wantUnchanged: "page string",
		},
		{
			name:        "text that reads differently",
			scope:       scopeOf(t, "page", `<i>{{.}}</i>`, str),
			wantMutated: true,
		},
		{
			name:        "a type of dot it was not reached with",
			scope:       scopeOf(t, "page", `<b>{{.}}</b>`, count),
			wantMutated: true,
		},
		{
			name:        "a template the revision did not hold",
			scope:       scopeOf(t, "footer", `<p>{{.}}</p>`, str),
			wantMutated: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			chosen := newSelector(before).choose([]scope{tt.scope}, nil)

			var wantMutate []string
			if tt.wantMutated {
				wantMutate = []string{tt.scope.template}
			}
			if got := names(chosen.mutate); !slices.Equal(got, wantMutate) {
				t.Errorf("mutate %q, want %q", got, wantMutate)
			}

			var wantUnchanged []string
			if tt.wantUnchanged != "" {
				wantUnchanged = []string{tt.wantUnchanged}
			}
			var got []string
			for _, u := range chosen.unchanged {
				got = append(got, u.Template+" "+u.DataType)
			}
			if !slices.Equal(got, wantUnchanged) {
				t.Errorf("unchanged %q, want %q", got, wantUnchanged)
			}
		})
	}
}

// TestSelectorMutatesATemplateOnceAcrossAWholeRun states that a template
// reached from two templates variables is mutated once: the second
// traversal to reach it says nothing about it at all.
func TestSelectorMutatesATemplateOnceAcrossAWholeRun(t *testing.T) {
	str := types.Typ[types.String]
	page := scopeOf(t, "page", `<b>{{.}}</b>`, str)
	sel := newSelector(nil)

	if got := names(sel.choose([]scope{page}, nil).mutate); !slices.Equal(got, []string{"page"}) {
		t.Fatalf("the first traversal chose %q, want the page", got)
	}
	second := sel.choose([]scope{page}, nil)
	if len(second.mutate) != 0 || len(second.unchanged) != 0 {
		t.Errorf("the second traversal chose %v and left %v, want neither", names(second.mutate), second.unchanged)
	}
}

// TestSelectorHonoursTheTemplatePattern states that --template-pattern
// decides before anything else: a template it does not match is neither
// mutated nor reported as unchanged.
func TestSelectorHonoursTheTemplatePattern(t *testing.T) {
	str := types.Typ[types.String]
	sel := newSelector(nil)
	sel.include = func(name string) bool { return name == "page" }

	chosen := sel.choose([]scope{
		scopeOf(t, "page", `<b>{{.}}</b>`, str),
		scopeOf(t, "footer", `<p>{{.}}</p>`, str),
	}, nil)
	if got := names(chosen.mutate); !slices.Equal(got, []string{"page"}) {
		t.Errorf("mutate %q, want only the page", got)
	}
	if len(chosen.unchanged) != 0 {
		t.Errorf("unchanged %v, want none: the pattern is not a comparison", chosen.unchanged)
	}
}

// TestSelectorReportsTrims states what a trim says and when it is worth
// saying: once per template and dot however many times it repeats, and not
// at all for a template this run mutated nowhere.
func TestSelectorReportsTrims(t *testing.T) {
	str := types.Typ[types.String]
	page := scopeOf(t, "page", `<b>{{.}}</b>`, str)

	t.Run("a repeat is reported once", func(t *testing.T) {
		chosen := newSelector(nil).choose([]scope{page}, []trim{
			trimOf("row", str, "page.go", "page.go"),
			trimOf("row", str, "page.go", "page.go"),
		})
		if len(chosen.trimmed) != 1 {
			t.Fatalf("trimmed %v, want one", chosen.trimmed)
		}
		want := TrimmedTemplate{CallSite: "page.go:1:1", Template: "row", DataType: "string", FirstSeenAt: "page.go:1:1"}
		if chosen.trimmed[0] != want {
			t.Errorf("trimmed %+v, want %+v", chosen.trimmed[0], want)
		}
	})

	t.Run("a repeat of a template left alone is not reported", func(t *testing.T) {
		before := scopesOf([]scope{page})
		chosen := newSelector(before).choose([]scope{page}, []trim{trimOf("page", str, "page.go", "page.go")})
		if len(chosen.trimmed) != 0 {
			t.Errorf("trimmed %v, want none: the page was mutated nowhere", chosen.trimmed)
		}
	})
}

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
