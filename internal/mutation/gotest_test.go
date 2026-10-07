package mutation

import (
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
			if tt.refused {
				assert.Error(t, err, "CheckGoTestArgs(%q)", tt.args)
			} else {
				assert.NoError(t, err, "CheckGoTestArgs(%q)", tt.args)
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
			assert.Equal(t, tt.want, tt.test.args(tt.flags...), "args(%q)", tt.flags)
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
			assert.Equal(t, tt.want.dir, got.dir, "goTest(/work) dir")
			assert.Equal(t, tt.want.packages, got.packages, "goTest(/work) packages")
			assert.Same(t, tt.want.match, got.match, "goTest(/work) match")
			assert.Equal(t, tt.want.extra, got.extra, "goTest(/work) extra")
			assert.Equal(t, tt.want.env, got.env, "goTest(/work) env")
		})
	}
}

// TestVerdictOf states how a go test result reads as a verdict: a kill
// needs a test that failed, and anything else go test exits 1 for is a
// run that could not happen, reported with what go test printed.
func TestVerdictOf(t *testing.T) {
	cannotRun := errors.New("go: no such tool")
	exitOne := exitStatusOne(t)
	exitTwo := exec.Command("sh", "-c", "exit 2").Run()
	for _, tt := range []struct {
		name    string
		output  string
		err     error
		want    Status
		wantErr error
	}{
		{name: "tests passed", output: "ok  \tserver\t0.1s\n", want: StatusMissed},
		{
			name:   "a test failed",
			output: "--- FAIL: TestGreeting (0.00s)\n    render_test.go:9: got \"\"\nFAIL\nFAIL\tserver\t0.1s\nFAIL\n",
			err:    exitOne,
			want:   StatusKilled,
		},
		{
			name:   "a test binary failed without a test failing",
			output: "panic: boom\nFAIL\tserver\t0.1s\nFAIL\n",
			err:    exitOne,
			want:   StatusKilled,
		},
		{
			name:   "a test failed on a CRLF stream",
			output: "--- FAIL: TestGreeting (0.00s)\r\nFAIL\tserver\t0.1s\r\n",
			err:    exitOne,
			want:   StatusKilled,
		},
		{
			name:    "a package did not build",
			output:  "# server\n./server.go:3:28: cannot use \"x\" (untyped string constant) as int value in return statement\nFAIL\tserver [build failed]\nFAIL\n",
			err:     exitOne,
			wantErr: exitOne,
		},
		{
			name:    "a package did not build beside one whose test failed",
			output:  "--- FAIL: TestA (0.00s)\nFAIL\tserver/a\t0.1s\nFAIL\tserver/b [build failed]\nFAIL\n",
			err:     exitOne,
			wantErr: exitOne,
		},
		{
			name:    "a package did not set up",
			output:  "FAIL\tserver [setup failed]\nFAIL\n",
			err:     exitOne,
			wantErr: exitOne,
		},
		{
			name:    "the go command failed",
			output:  "go: reading overlay file: open /tmp/overlay.json: no such file or directory\n",
			err:     exitOne,
			wantErr: exitOne,
		},
		{
			name:    "exit status 1 saying nothing",
			err:     exitOne,
			wantErr: exitOne,
		},
		{
			name:    "a usage error",
			output:  "invalid value \"many\" for flag -count: parse error\n",
			err:     exitTwo,
			wantErr: exitTwo,
		},
		{name: "go test could not start", err: cannotRun, wantErr: cannotRun},
	} {
		t.Run(tt.name, func(t *testing.T) {
			out := &goTestOutput{}
			_, _ = out.Write([]byte(tt.output))
			got, err := verdictOf(out, tt.err)
			assert.Equal(t, tt.want, got, "verdictOf(%q, %v) status", tt.output, tt.err)
			if tt.wantErr == nil {
				assert.NoError(t, err, "verdictOf(%q, %v) error", tt.output, tt.err)
				return
			}
			assert.ErrorIs(t, err, tt.wantErr, "verdictOf(%q, %v) error", tt.output, tt.err)
			goTestErr, ok := errors.AsType[*GoTestError](err)
			if assert.True(t, ok, "verdictOf(%q, %v) error = %v, want a GoTestError", tt.output, tt.err, err) {
				assert.Equal(t, tt.output, goTestErr.Output, "the error carries what go test printed")
			}
		})
	}
}

// TestGoTestOutputKeepsATailAndEveryMarker states that the output kept
// for a mutant is bounded, from the end, while what decides its verdict
// is read off every line, however early it was printed.
func TestGoTestOutputKeepsATailAndEveryMarker(t *testing.T) {
	out := &goTestOutput{limit: 16}
	_, _ = out.Write([]byte("FAIL\tserver/b [build failed]\n"))
	for range 100 {
		_, _ = out.Write([]byte("--- FAIL: TestA (0.00s)\n"))
	}
	// A line split across writes is still read as one.
	_, _ = out.Write([]byte("FAIL\tserv"))
	_, _ = out.Write([]byte("er/a\t0.1s\nlast\n"))

	assert.Equal(t, "ver/a\t0.1s\nlast\n", out.String(), "kept tail")
	assert.True(t, out.testFailed, "a test failed")
	assert.True(t, out.couldNotRun, "a package did not build")
}

// TestGoTestOutputIgnoresALongLine states that a line too long to be a
// marker is not read as one, even when it starts like one.
func TestGoTestOutputIgnoresALongLine(t *testing.T) {
	out := &goTestOutput{}
	_, _ = out.Write([]byte("go: " + strings.Repeat("x", 2*maxMarkerLine) + "\n--- FAIL: TestA (0.00s)\n"))
	assert.False(t, out.couldNotRun, "the long line is not a go command error")
	assert.True(t, out.testFailed, "the line after it is still read")
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
			assert.Equal(t, tt.want, isTestFailure(err), "isTestFailure(%v)", err)
		})
	}
}
