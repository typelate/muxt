package muxt

import (
	"errors"
	"go/token"
	"html/template"
	"maps"
	"slices"
	"testing"

	"github.com/typelate/muxt/internal/source"
)

func TestDefinitionBySourceThenName(t *testing.T) {
	at := func(file string, offset int, name string) Definition {
		return Definition{sourceFile: file, namePosition: token.Position{Offset: offset}, name: name}
	}
	for _, tt := range []struct {
		name     string
		a, b     Definition
		wantSign int
	}{
		{name: "earlier file", a: at("a.gohtml", 9, "z"), b: at("b.gohtml", 0, "a"), wantSign: -1},
		{name: "later file", a: at("b.gohtml", 0, "a"), b: at("a.gohtml", 9, "z"), wantSign: 1},
		{name: "earlier offset", a: at("a.gohtml", 10, "z"), b: at("a.gohtml", 20, "a"), wantSign: -1},
		{name: "later offset", a: at("a.gohtml", 20, "a"), b: at("a.gohtml", 10, "z"), wantSign: 1},
		{name: "earlier name", a: at("a.gohtml", 5, "a"), b: at("a.gohtml", 5, "b"), wantSign: -1},
		{name: "later name", a: at("a.gohtml", 5, "b"), b: at("a.gohtml", 5, "a"), wantSign: 1},
		{name: "same", a: at("a.gohtml", 5, "a"), b: at("a.gohtml", 5, "a")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.bySourceThenName(tt.b); (got > 0) != (tt.wantSign > 0) || (got < 0) != (tt.wantSign < 0) {
				t.Errorf("bySourceThenName() = %d, want sign %d", got, tt.wantSign)
			}
		})
	}
}

func TestCombineNameFailuresIsSorted(t *testing.T) {
	failures := []nameFailure{
		{def: Definition{sourceFile: "b.gohtml", name: "x"}, err: errors.New("second")},
		{def: Definition{sourceFile: "a.gohtml", name: "y"}, err: errors.New("first")},
	}
	list, ok := combineNameFailures(failures).(ErrorList)
	if !ok {
		t.Fatalf("combineNameFailures() is not an ErrorList")
	}
	var got []string
	for _, err := range list {
		got = append(got, err.Error())
	}
	if want := []string{"first", "second"}; !slices.Equal(got, want) {
		t.Errorf("combineNameFailures() = %q, want %q", got, want)
	}
}

func TestCheckForDuplicatePatternsOrdersLocations(t *testing.T) {
	at := func(file string, line, offset int, name string) Definition {
		return Definition{
			path: "/", sourceFile: file, name: name,
			namePosition: token.Position{Filename: file, Line: line, Column: 1, Offset: offset},
		}
	}
	for _, tt := range []struct {
		name string
		defs []Definition
		want []string
	}{
		{name: "by file", defs: []Definition{at("b.gohtml", 1, 0, "x"), at("a.gohtml", 1, 0, "y")}, want: []string{"a.gohtml:1:1", "b.gohtml:1:1"}},
		{name: "by offset", defs: []Definition{at("a.gohtml", 2, 20, "x"), at("a.gohtml", 1, 10, "y")}, want: []string{"a.gohtml:1:1", "a.gohtml:2:1"}},
		{name: "by name", defs: []Definition{at("a.gohtml", 2, 5, "b"), at("a.gohtml", 1, 5, "a")}, want: []string{"a.gohtml:1:1", "a.gohtml:2:1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckForDuplicatePatterns(tt.defs)
			dup, ok := err.(*DuplicatePatternError)
			if !ok {
				t.Fatalf("CheckForDuplicatePatterns() = %v, want a *DuplicatePatternError", err)
			}
			if !slices.Equal(dup.Locations, tt.want) {
				t.Errorf("CheckForDuplicatePatterns() locations = %q, want %q", dup.Locations, tt.want)
			}
		})
	}
}

func TestDefinitionsSourceFile(t *testing.T) {
	ts := template.New("root")
	template.Must(ts.New("a.gohtml").Parse(`{{define "GET /a A()"}}x{{end}}`))
	ts.New("GET /b B()") // declared, never parsed: no tree
	defs, err := Definitions(source.Variable{Name: "templates", Set: ts})
	if err != nil {
		t.Fatalf("Definitions() error = %v", err)
	}
	got := make(map[string]string)
	for _, def := range defs {
		got[def.Name()] = def.SourceFile()
	}
	want := map[string]string{"GET /a A()": "a.gohtml", "GET /b B()": ""}
	if !maps.Equal(got, want) {
		t.Errorf("Definitions() source files = %v, want %v", got, want)
	}
}
