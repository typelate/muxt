package mutation

import (
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
