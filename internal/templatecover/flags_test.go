package templatecover_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/templatecover"
)

// TestFlags states how the caller's go test arguments are walked:
// a flag's name without its dashes, its value inline or as the next
// argument when it is a flag that takes one, an argument that is no flag
// with no name, and everything from -args on as one last step.
func TestFlags(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want []templatecover.Flag
	}{
		{name: "none"},
		{
			name: "a boolean flag",
			args: []string{"-v", "--short"},
			want: []templatecover.Flag{{Name: "v", Args: []string{"-v"}}, {Name: "short", Args: []string{"--short"}}},
		},
		{
			name: "a value after =",
			args: []string{"-run=TestA", "-overlay="},
			want: []templatecover.Flag{
				{Name: "run", Value: "TestA", HasValue: true, Args: []string{"-run=TestA"}},
				{Name: "overlay", HasValue: true, Args: []string{"-overlay="}},
			},
		},
		{
			name: "a value as the next argument",
			args: []string{"-coverpkg", "./...", "--covermode", "set"},
			want: []templatecover.Flag{
				{Name: "coverpkg", Value: "./...", HasValue: true, Args: []string{"-coverpkg", "./..."}},
				{Name: "covermode", Value: "set", HasValue: true, Args: []string{"--covermode", "set"}},
			},
		},
		{
			name: "a value that looks like a flag",
			args: []string{"-run", "-json"},
			want: []templatecover.Flag{{Name: "run", Value: "-json", HasValue: true, Args: []string{"-run", "-json"}}},
		},
		{
			name: "a flag that takes a value, last",
			args: []string{"-run"},
			want: []templatecover.Flag{{Name: "run", Args: []string{"-run"}}},
		},
		{
			name: "a boolean flag does not take the next argument",
			args: []string{"-cover", "-short"},
			want: []templatecover.Flag{{Name: "cover", Args: []string{"-cover"}}, {Name: "short", Args: []string{"-short"}}},
		},
		{
			name: "a test binary's own flag and an argument that is no flag",
			args: []string{"-update", "golden"},
			want: []templatecover.Flag{{Name: "update", Args: []string{"-update"}}, {Args: []string{"golden"}}},
		},
		{
			name: "the test binary's arguments",
			args: []string{"-v", "-args", "-overlay=o.json", "x"},
			want: []templatecover.Flag{{Name: "v", Args: []string{"-v"}}, {Name: "args", Args: []string{"-args", "-overlay=o.json", "x"}}},
		},
		{
			name: "the test binary's arguments with two dashes",
			args: []string{"--args", "-json"},
			want: []templatecover.Flag{{Name: "args", Args: []string{"--args", "-json"}}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got []templatecover.Flag
			for f := range templatecover.Flags(tt.args) {
				got = append(got, f)
			}
			assert.Equal(t, tt.want, got, "Flags(%q)", tt.args)
		})
	}
}

// TestFlagsStopsWhenAsked states that the walk ends when the loop over it
// does.
func TestFlagsStopsWhenAsked(t *testing.T) {
	n := 0
	for range templatecover.Flags([]string{"-v", "-short", "-run=x"}) {
		n++
		break
	}
	assert.Equal(t, 1, n, "steps")
}
