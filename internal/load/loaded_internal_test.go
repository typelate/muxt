package load

import (
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestLoadedHasParseErrors(t *testing.T) {
	parseError := packages.Error{Kind: packages.ParseError, Pos: "a.go:1:1", Msg: "expected 'package'"}
	typeError := packages.Error{Kind: packages.TypeError, Pos: "a.go:2:1", Msg: "undefined: X"}

	for _, tt := range []struct {
		name string
		pl   []*packages.Package
		want bool
	}{
		{name: "no packages"},
		{name: "clean", pl: []*packages.Package{{}}},
		{name: "type errors are expected before generate", pl: []*packages.Package{{Errors: []packages.Error{typeError}}}},
		{name: "syntax errors", pl: []*packages.Package{{Errors: []packages.Error{parseError}}}, want: true},
		{name: "syntax errors in a later package", pl: []*packages.Package{{}, {Errors: []packages.Error{parseError}}}, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := (&Loaded{pl: tt.pl}).HasParseErrors(); got != tt.want {
				t.Errorf("HasParseErrors() = %t, want %t", got, tt.want)
			}
		})
	}
}
