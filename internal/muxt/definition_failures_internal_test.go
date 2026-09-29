package muxt

import (
	"errors"
	"go/token"
	"slices"
	"testing"
)

func TestCombineNameFailures(t *testing.T) {
	failure := func(file string, offset int, name string) nameFailure {
		return nameFailure{
			def: Definition{sourceFile: file, namePosition: token.Position{Offset: offset}, name: name},
			err: errors.New(file + ":" + name),
		}
	}
	for _, tt := range []struct {
		name     string
		failures []nameFailure
		want     []string
	}{
		{
			name:     "source file first",
			failures: []nameFailure{failure("b.gohtml", 0, "a"), failure("a.gohtml", 9, "z")},
			want:     []string{"a.gohtml:z", "b.gohtml:a"},
		},
		{
			name:     "then offset within a file",
			failures: []nameFailure{failure("a.gohtml", 20, "a"), failure("a.gohtml", 10, "z")},
			want:     []string{"a.gohtml:z", "a.gohtml:a"},
		},
		{
			name:     "then template name",
			failures: []nameFailure{failure("a.gohtml", 5, "b"), failure("a.gohtml", 5, "a")},
			want:     []string{"a.gohtml:a", "a.gohtml:b"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := combineNameFailures(tt.failures)
			list, ok := err.(ErrorList)
			if !ok {
				t.Fatalf("combineNameFailures() = %T, want ErrorList", err)
			}
			var got []string
			for _, e := range list {
				got = append(got, e.Error())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("combineNameFailures() order = %q, want %q", got, tt.want)
			}
		})
	}
}
