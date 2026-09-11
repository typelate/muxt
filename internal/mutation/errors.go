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
type NoMutationsError struct {
	Templates int
}

func (e *NoMutationsError) Error() string {
	return fmt.Sprintf("no mutations available: the %d template(s) reached hold no dynamic or control flow actions, so a run would report every mutant killed without testing anything", e.Templates)
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
