package mutation

import (
	"fmt"
	"strings"
)

// BaselineFailedError reports that the tests do not pass before anything
// is mutated, which makes every verdict meaningless: a mutant would be
// recorded as caught by a failure that was already there.
type BaselineFailedError struct {
	Output string
}

func (e *BaselineFailedError) Error() string {
	return "baseline tests failed before mutation; fix them first:\n" + strings.TrimRight(e.Output, "\n")
}

// GoTestError reports that go test could not run: a flag it refused, a
// package that would not build, the go command failing, or a process the
// OS killed. Output is what it printed, which says why far better than
// its exit status does; for a mutant's run it is the tail of the output.
type GoTestError struct {
	Err    error
	Output string
}

func (e *GoTestError) Error() string {
	out := strings.TrimSpace(e.Output)
	if out == "" {
		return fmt.Sprintf("go test could not run: %v", e.Err)
	}
	return fmt.Sprintf("go test could not run: %v:\n%s", e.Err, out)
}

func (e *GoTestError) Unwrap() error { return e.Err }

// MissedMutantsError reports that a run let mutants through: the tests
// passed with each of them in place. The report lists them; this only
// says how many, for an exit status.
type MissedMutantsError struct {
	Missed int
}

func (e *MissedMutantsError) Error() string {
	return fmt.Sprintf("%d %s missed", e.Missed, pluralize(e.Missed, "mutant"))
}

// NoCallSitesError reports that nothing renders the templates, so there
// is no type of dot to mutate against.
type NoCallSitesError struct {
	Variables []string
}

func (e *NoCallSitesError) Error() string {
	return fmt.Sprintf("no %s.ExecuteTemplate calls found: mutation testing needs a call site to know the type of dot", strings.Join(e.Variables, ", "))
}

// NoMutationsError reports that templates were reached but none of them
// held an action to mutate.
//
// A run that mutates nothing passes, and a passing mutation run reads as
// "the tests catch everything". That is the most misleading thing this
// tool can say, so finding nothing to ask about is an error rather than
// an empty report that exits zero.
//
// It is not raised when --template-pattern or --diff narrowed what is
// mutated: a commit that only touches static templates has nothing to
// vary, and the report says so instead.
type NoMutationsError struct {
	Templates int
}

func (e *NoMutationsError) Error() string {
	return fmt.Sprintf("no mutations available: the %d template(s) reached hold no dynamic or control flow actions, so a run would report every mutant killed without testing anything", e.Templates)
}

// NoTemplateMatchesError reports that --template-pattern matched none of
// the templates the ExecuteTemplate calls reach.
//
// A run that mutates nothing because the pattern was mistyped would pass,
// and read as the selected templates being covered.
type NoTemplateMatchesError struct {
	Pattern string
}

func (e *NoTemplateMatchesError) Error() string {
	return fmt.Sprintf("no template matches --template-pattern %s", e.Pattern)
}

// UnreadableTemplateError reports a template the template set parsed into
// actions and this package re-parsed into none.
//
// The two disagree only when the text was read with delimiters it was not
// written in. The template would otherwise contribute no mutants at all,
// and a run that quietly measured fewer templates than it was given is
// the failure this reports instead.
type UnreadableTemplateError struct {
	Template string
	Path     string
}

func (e *UnreadableTemplateError) Error() string {
	return fmt.Sprintf("template %q in %s holds actions the template set can see and this run cannot: it was read with the wrong delimiters", e.Template, e.Path)
}
