package check

import "testing"

func TestErrorContext(t *testing.T) {
	for _, tt := range []struct {
		source, want string
	}{
		{source: `{{.Result.Name}}`, want: `{{.Result.Name}}`},
		{source: `{{-  .Result.Name   -}}`, want: `{{.Result.Name}}`},
		{source: `.Result.Name`, want: `.Result.Name`},
		// A range's pipeline runs through the right delimiter of its
		// action; text/template rendered the pipeline without it.
		{source: `.Result.Name}}`, want: `.Result.Name`},
		{source: `$i, $x := .Result.Name}}`, want: `$i, $x := .Result.Name`},
		{source: `$i, $x :=  .Result.Name -}}`, want: `$i, $x := .Result.Name`},
		{source: `.Result.Name  }}`, want: `.Result.Name`},
		{source: `print "}}"`, want: `print "}}"`},
		// text/template rendered a parenthesized pipeline with no space
		// just inside its parentheses.
		{source: `{{ ( .Result ).Missing.X }}`, want: `{{(.Result).Missing.X}}`},
		{source: `print ( .X | printf "( %s )" ) .Y`, want: `print (.X | printf "( %s )") .Y`},
	} {
		if got := errorContext(tt.source); got != tt.want {
			t.Errorf("errorContext(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}
