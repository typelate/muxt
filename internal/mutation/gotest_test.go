package mutation

import (
	"os/exec"
	"testing"
)

func TestCheckGoTestArgs(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    []string
		refused bool
	}{
		{name: "a build flag", args: []string{"-tags=x"}},
		{name: "overlay with a value", args: []string{"-overlay=o.json"}, refused: true},
		{name: "overlay with two dashes", args: []string{"--overlay=o.json"}, refused: true},
		{name: "overlay with its value as the next argument", args: []string{"-overlay", "o.json"}, refused: true},
		{name: "overlay with an empty value", args: []string{"-overlay="}, refused: true},
		{name: "the word overlay as another flag's value", args: []string{"-run", "overlay"}},
		{name: "a flag that only starts with overlay", args: []string{"-overlayx"}},
		{name: "an overlay flag for the test binary", args: []string{"-args", "-overlay=o.json"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckGoTestArgs(tt.args)
			if refused := err != nil; refused != tt.refused {
				t.Errorf("CheckGoTestArgs(%q) = %v, want refused %t", tt.args, err, tt.refused)
			}
		})
	}
}

// TestIsTestFailureOnlyCountsATestThatRan states which go test exits mean
// the tests failed.
//
// A mutant is recorded as killed when its tests fail, so anything else
// read as a test failure is a kill the tests did not earn: a usage error
// in a pass-through flag, or a go test the OS killed for memory under
// --parallel.
func TestIsTestFailureOnlyCountsATestThatRan(t *testing.T) {
	for _, tt := range []struct {
		name   string
		script string
		want   bool
	}{
		{name: "the tests failed", script: "exit 1", want: true},
		{name: "a usage error", script: "exit 2", want: false},
		{name: "killed by a signal", script: "kill -9 $$", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := exec.Command("sh", "-c", tt.script).Run()
			if got := isTestFailure(err); got != tt.want {
				t.Errorf("isTestFailure(%v) = %t, want %t", err, got, tt.want)
			}
		})
	}
}
