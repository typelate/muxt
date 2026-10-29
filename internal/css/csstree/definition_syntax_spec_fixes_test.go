package csstree

import "testing"

// {0} repeats a term exactly zero times (CSS Values 4 §2.3); upstream reads
// it as *. Unbounded multipliers still repeat without limit.
func TestDefinitionSyntaxMatchSpecFixes(t *testing.T) {
	for _, test := range []definitionSyntaxMatchTest{
		{Syntax: "a{0} b", Valid: []string{"b"}, Invalid: []string{"a b", "a a b", "a"}},
		{Syntax: "a{0,0} b", Valid: []string{"b"}, Invalid: []string{"a b"}},
		{Syntax: "a#{0} b", Valid: []string{"b"}, Invalid: []string{"a b", "a, a b"}},
		{Syntax: "a{0}? b", Valid: []string{"b"}, Invalid: []string{"a b"}},
		{Syntax: "a{0,} b", Valid: []string{"b", "a b", "a a a b"}, Invalid: []string{"a"}},
		{Syntax: "a* b", Valid: []string{"b", "a b", "a a a b"}},
		{Syntax: "a+", Valid: []string{"a", "a a a"}, Invalid: []string{""}},
		{Syntax: "a#", Valid: []string{"a", "a, a, a"}, Invalid: []string{"a a"}},
		{Syntax: "a#?", Valid: []string{"", "a", "a, a"}},
		{Syntax: "a{2,}", Valid: []string{"a a", "a a a"}, Invalid: []string{"a"}},
		{Syntax: "a{1,2}", Valid: []string{"a", "a a"}, Invalid: []string{"a a a"}},
	} {
		createSyntaxTest(t, test.Syntax, test)
	}
}
