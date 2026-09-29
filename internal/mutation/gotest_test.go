package mutation

import (
	"errors"
	"os/exec"
	"regexp"
	"slices"
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

// TestGoTestArgsPutTheCallersFlagsAfterThePackages states the order go
// test needs: a flag it does not know ends the package list.
func TestGoTestArgsPutTheCallersFlagsAfterThePackages(t *testing.T) {
	for _, tt := range []struct {
		name  string
		test  goTest
		flags []string
		want  []string
	}{
		{
			name: "nothing else",
			test: goTest{packages: []string{"./..."}},
			want: []string{"test", "-count=1", "./..."},
		},
		{
			name:  "everything",
			test:  goTest{packages: []string{"./a", "./b"}, match: regexp.MustCompile("TestPage"), extra: []string{"-update", "-v"}},
			flags: []string{"-overlay=o.json"},
			want:  []string{"test", "-count=1", "-overlay=o.json", "-run=TestPage", "./a", "./b", "-update", "-v"},
		},
		{
			name: "user flags only",
			test: goTest{packages: []string{"./..."}, extra: []string{"-update"}},
			want: []string{"test", "-count=1", "./...", "-update"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.test.args(tt.flags...); !slices.Equal(got, tt.want) {
				t.Errorf("args(%q) = %q, want %q", tt.flags, got, tt.want)
			}
		})
	}
}

func TestConfigurationGoTest(t *testing.T) {
	match := regexp.MustCompile("TestPage")
	for _, tt := range []struct {
		name   string
		config Configuration
		want   goTest
	}{
		{
			name:   "packages default to everything",
			config: Configuration{},
			want:   goTest{dir: "/work", packages: []string{"./..."}},
		},
		{
			name:   "configured fields carry over",
			config: Configuration{Packages: []string{"./web"}, Run: match, GoTestArgs: []string{"-v"}, env: []string{"A=b"}},
			want:   goTest{dir: "/work", packages: []string{"./web"}, match: match, extra: []string{"-v"}, env: []string{"A=b"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.goTest("/work")
			if got.dir != tt.want.dir || !slices.Equal(got.packages, tt.want.packages) || got.match != tt.want.match ||
				!slices.Equal(got.extra, tt.want.extra) || !slices.Equal(got.env, tt.want.env) {
				t.Errorf("goTest(/work) = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestVerdictOf states how a go test result reads as a verdict.
func TestVerdictOf(t *testing.T) {
	cannotRun := errors.New("go: no such tool")
	for _, tt := range []struct {
		name       string
		err        error
		want       Status
		wantErr    error
		wantHasErr bool
	}{
		{name: "tests passed", err: nil, want: StatusMissed},
		{name: "tests failed", err: exitStatusOne(t), want: StatusKilled},
		{name: "go test could not run", err: cannotRun, wantErr: cannotRun, wantHasErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := verdictOf(tt.err)
			if got != tt.want || (err != nil) != tt.wantHasErr || (tt.wantHasErr && !errors.Is(err, tt.wantErr)) {
				t.Errorf("verdictOf(%v) = %q, %v, want %q, %v", tt.err, got, err, tt.want, tt.wantErr)
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
// --workers.
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
