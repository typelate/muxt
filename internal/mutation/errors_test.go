package mutation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestErrorMessages states what each error says, which is all a reader of
// a failed run has to go on.
func TestErrorMessages(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{
			name: "baseline failed",
			err:  &BaselineFailedError{Output: "--- FAIL: TestIndex (0.00s)\nFAIL\n"},
			want: "baseline tests failed before mutation; fix them first:\n--- FAIL: TestIndex (0.00s)\nFAIL",
		},
		{
			name: "no call sites",
			err:  &NoCallSitesError{Variables: []string{"templates", "pages"}},
			want: "no templates, pages.ExecuteTemplate calls found: mutation testing needs a call site to know the type of dot",
		},
		{
			name: "no mutations",
			err:  &NoMutationsError{Templates: 3},
			want: "no mutations available: the 3 template(s) reached hold no dynamic or control flow actions, so a run would report every mutant killed without testing anything",
		},
		{
			name: "no template matches",
			err:  &NoTemplateMatchesError{Pattern: "^footer$"},
			want: "no template matches --template-pattern ^footer$",
		},
		{
			name: "go test could not run",
			err:  &GoTestError{Err: errors.New("exit status 2"), Output: "invalid value \"many\" for flag -count: parse error\n"},
			want: "go test could not run: exit status 2:\ninvalid value \"many\" for flag -count: parse error",
		},
		{
			name: "go test could not run and said nothing",
			err:  &GoTestError{Err: errors.New("signal: killed")},
			want: "go test could not run: signal: killed",
		},
		{
			name: "one mutant missed",
			err:  &MissedMutantsError{Missed: 1},
			want: "1 mutant missed",
		},
		{
			name: "mutants missed",
			err:  &MissedMutantsError{Missed: 3},
			want: "3 mutants missed",
		},
		{
			name: "unreadable template",
			err:  &UnreadableTemplateError{Template: "page", Path: "page.gohtml"},
			want: `template "page" in page.gohtml holds actions the template set can see and this run cannot: it was read with the wrong delimiters`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualError(t, tt.err, tt.want)
		})
	}
}
