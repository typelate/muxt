package mutation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/overlay"
)

// loudTest renders the greeting loud from a second package, so its test
// binary reaches the block the first one never does.
const loudTest = `package loud

import (
	"strings"
	"testing"

	"server"
)

func TestLoud(t *testing.T) {
	var buf strings.Builder
	if err := server.Render(&buf, server.Greeting{Name: "World", Loud: true}); err != nil {
		t.Fatal(err)
	}
}
`

// TestTemplateCoverageOverAModule runs the pre-pass for real: two test
// binaries each append the blocks they reach, and the profile is the
// union of the two.
func TestTemplateCoverageOverAModule(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test binaries with html/template overlaid")
	}
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": goMod, "template.go": greetingGo, "render_test.go": greetingTest})
	config := greetingConfig()
	tester := config.goTest(dir)

	var warnings strings.Builder
	got, err := templateCoverage(t.Context(), config.toolchain(dir), t.TempDir(), tester.coverage, &warnings)
	require.NoError(t, err)
	require.NotNil(t, got, "coverage is known; warnings:\n%s", warnings.String())
	assert.Empty(t, warnings.String(), "warnings")
	greeting := got["greeting"]
	assert.Contains(t, greeting, coverBlock{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}, "the text before the name is reached: %v", got)
	assert.NotContains(t, greeting, coverBlock{StartLine: 1, StartCol: 30, EndLine: 1, EndCol: 34}, "no test renders the greeting loud: %v", got)

	writeFiles(t, dir, map[string]string{"loud/loud_test.go": loudTest})
	got, err = templateCoverage(t.Context(), config.toolchain(dir), t.TempDir(), tester.coverage, &warnings)
	require.NoError(t, err)
	require.NotNil(t, got, "coverage is known; warnings:\n%s", warnings.String())
	greeting = got["greeting"]
	assert.Contains(t, greeting, coverBlock{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}, "reached by both packages: %v", got)
	assert.Contains(t, greeting, coverBlock{StartLine: 1, StartCol: 30, EndLine: 1, EndCol: 34}, "reached by the loud package: %v", got)
}

// TestTemplateCoverageRunsTheEngineOverlay states what the pre-pass hands
// go test: the engine overlay, written under the run's scratch directory,
// and an empty directory for the profiles, under it too.
func TestTemplateCoverageRunsTheEngineOverlay(t *testing.T) {
	realGOROOT(t)
	scratch := t.TempDir()
	runs := 0
	run := func(_ context.Context, overlayPath, coverDir string) (*goTestOutput, error) {
		runs++
		assert.True(t, filepath.IsAbs(overlayPath), "overlay %s is absolute", overlayPath)
		assert.True(t, under(scratch, overlayPath), "overlay %s is under the scratch directory %s", overlayPath, scratch)
		assert.True(t, under(scratch, coverDir), "cover directory %s is under the scratch directory %s", coverDir, scratch)
		_, err := os.Stat(overlayPath)
		assert.NoError(t, err, "the overlay is written before go test runs")
		entries, err := os.ReadDir(coverDir)
		assert.NoError(t, err, "the cover directory exists")
		assert.Empty(t, entries, "the cover directory starts empty")
		writeFiles(t, coverDir, map[string]string{
			"1.cov": "mode: set\npage:1.1,1.8 1 1\n",
			"2.cov": "mode: set\npage:1.1,1.8 1 1\npage:2.3,2.9 1 1\nother:1.1,1.4 1 1\n",
		})
		return testOutput("ok  \tserver\t0.1s\n"), nil
	}
	var warnings strings.Builder
	got, err := templateCoverage(t.Context(), overlay.Toolchain{}, scratch, run, &warnings)
	require.NoError(t, err)
	assert.Equal(t, 1, runs, "go test runs once")
	assert.Empty(t, warnings.String(), "warnings")
	assert.Equal(t, coverage{
		"page": {
			{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}: {},
			{StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 9}: {},
		},
		"other": {{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}: {}},
	}, got, "the union of every profile, by block")
}

// TestTemplateCoverageReachingNothing states that tests that execute no
// template leave a known, empty coverage: nothing is reached.
func TestTemplateCoverageReachingNothing(t *testing.T) {
	realGOROOT(t)
	run := func(context.Context, string, string) (*goTestOutput, error) {
		return testOutput("ok  \tserver\t0.1s\n"), nil
	}
	got, err := templateCoverage(t.Context(), overlay.Toolchain{}, t.TempDir(), run, nil)
	require.NoError(t, err)
	assert.NotNil(t, got, "coverage is known")
	assert.Empty(t, got, "nothing is reached")
}

// TestTemplateCoverageUnknown states the pre-passes that leave coverage
// unknown: each warns once, returns nil coverage and no error, so the run
// goes on without it.
func TestTemplateCoverageUnknown(t *testing.T) {
	for _, tt := range []struct {
		name string
		// toolchain is the probe's; empty for the real go command.
		toolchain func(t *testing.T) overlay.Toolchain
		out       string
		err       error
		warning   string
	}{
		{
			name:      "the probe fails",
			toolchain: refusingToolchain,
			warning:   "refuses an overlay",
		},
		{
			name:    "an export was disabled",
			out:     "=== RUN   TestA\nmuxt: template coverage export disabled: open /x/1.cov: permission denied\n--- PASS: TestA (0.00s)\nok  \tserver\t0.1s\n",
			warning: "muxt: template coverage export disabled: open /x/1.cov: permission denied",
		},
		{
			name:    "the tests do not build with the engine",
			out:     "# server\n./page.go:9:12: ts.Tree.Root undefined\nFAIL\tserver [build failed]\n",
			err:     exitError(t, 1),
			warning: "ts.Tree.Root undefined",
		},
		{
			name:    "go test exits without a failed test",
			out:     "go: some failure\n",
			err:     exitError(t, 2),
			warning: "go: some failure",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tc := overlay.Toolchain{}
			if tt.toolchain != nil {
				tc = tt.toolchain(t)
			} else {
				realGOROOT(t)
			}
			run := func(_ context.Context, _, coverDir string) (*goTestOutput, error) {
				if tt.toolchain != nil {
					t.Error("go test ran though the probe failed")
				}
				writeFiles(t, coverDir, map[string]string{"1.cov": "mode: set\npage:1.1,1.8 1 1\n"})
				return testOutput(tt.out), tt.err
			}
			var warnings strings.Builder
			got, err := templateCoverage(t.Context(), tc, t.TempDir(), run, &warnings)
			require.NoError(t, err)
			assert.Nil(t, got, "coverage is unknown")
			lines := strings.Split(strings.TrimRight(warnings.String(), "\n"), "\n")
			require.Len(t, lines, 1, "one warning line:\n%s", warnings.String())
			assert.True(t, strings.HasPrefix(lines[0], "warning: template coverage unknown: "), "warning %q", lines[0])
			assert.Contains(t, lines[0], tt.warning, "warning")
		})
	}
}

// TestTemplateCoverageStopsWhenATestFails states that the tests failing in
// the pre-pass stop the run as a failing baseline does.
func TestTemplateCoverageStopsWhenATestFails(t *testing.T) {
	realGOROOT(t)
	out := "--- FAIL: TestBroken (0.00s)\n    a_test.go:5: broken\nFAIL\nFAIL\tserver\t0.1s\n"
	run := func(context.Context, string, string) (*goTestOutput, error) {
		return testOutput(out), exitError(t, 1)
	}
	_, err := templateCoverage(t.Context(), overlay.Toolchain{}, t.TempDir(), run, nil)
	baseline, ok := errors.AsType[*BaselineFailedError](err)
	require.True(t, ok, "templateCoverage = %v, want a failing baseline", err)
	assert.Equal(t, out, baseline.Output, "what go test printed")
}

// TestTemplateCoverageStopsWhenCancelled states that an interrupted
// pre-pass is not a verdict on anything: the run stops with ctx's error.
func TestTemplateCoverageStopsWhenCancelled(t *testing.T) {
	realGOROOT(t)
	ctx, cancel := context.WithCancel(t.Context())
	run := func(context.Context, string, string) (*goTestOutput, error) {
		cancel()
		return testOutput(""), context.Canceled
	}
	_, err := templateCoverage(ctx, overlay.Toolchain{}, t.TempDir(), run, nil)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestGoTestOutputNotesTheCoverageMarker(t *testing.T) {
	for _, tt := range []struct {
		name string
		out  string
		want string
	}{
		{name: "none", out: "=== RUN   TestA\n--- PASS: TestA (0.00s)\nok  \tserver\t0.1s\n"},
		{name: "a line of its own", out: "muxt: template coverage export disabled: no space left on device\n", want: "muxt: template coverage export disabled: no space left on device"},
		{name: "after a test's unterminated output", out: "partialmuxt: template coverage export disabled: x\n", want: "muxt: template coverage export disabled: x"},
		{name: "the first of several", out: "muxt: template coverage export disabled: a\nmuxt: template coverage export disabled: b\n", want: "muxt: template coverage export disabled: a"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, testOutput(tt.out).coverDisabled, "coverDisabled after %q", tt.out)
		})
	}
}

func TestCoverageArgs(t *testing.T) {
	test := goTest{packages: []string{"./..."}, extra: []string{"-short"}}
	assert.Equal(t,
		[]string{"test", "-count=1", "-v", "-vet=off", "-overlay=/s/overlay.json", "./...", "-short"},
		test.args(coverageFlags("/s/overlay.json")...),
		"go test arguments of the pre-pass")
}

// TestNewPlanHoldsTheCoverage states that the profile reaches the plan.
func TestNewPlanHoldsTheCoverage(t *testing.T) {
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": goMod, "template.go": greetingGo})
	cov := coverage{"greeting": {{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}: {}}}
	p, err := newPlan(t.Context(), greetingConfig(), dir, cov)
	require.NoError(t, err)
	assert.Equal(t, cov, p.coverage, "plan coverage")
}

func testOutput(s string) *goTestOutput {
	out := new(goTestOutput)
	_, _ = out.Write([]byte(s))
	return out
}

// exitError is a real *exec.ExitError with the given status.
func exitError(t *testing.T, code int) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	err := exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Run()
	require.Error(t, err)
	return err
}

// refusingToolchain is a go command that reports a GOROOT and refuses any
// overlay of it.
func refusingToolchain(t *testing.T) overlay.Toolchain {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake go command is a shell script")
	}
	goroot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(goroot, "src", "html", "template"), 0o755))
	script := `#!/bin/sh
case "$1" in
env) printf '%s\n%s\n' '` + goroot + `' '` + t.TempDir() + `' ;;
*) echo "go: overlay contains a replacement for GOROOT" >&2; exit 1 ;;
esac
`
	goBin := filepath.Join(t.TempDir(), "go")
	require.NoError(t, os.WriteFile(goBin, []byte(script), 0o755))
	return overlay.Toolchain{Go: goBin}
}

// realGOROOT skips the test when there is no html/template source to
// overlay.
func realGOROOT(t *testing.T) {
	t.Helper()
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Skipf("go env GOROOT: %v", err)
	}
	if _, err := os.Stat(filepath.Join(strings.TrimSpace(string(out)), "src", "html", "template")); err != nil {
		t.Skipf("no html/template source under GOROOT: %v", err)
	}
}

func under(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && filepath.IsLocal(rel)
}

// TestCoverageSeesADisabledExport runs the pre-pass for real into a
// directory no test binary can write to: the binary passes, and what it
// printed about the export still reaches the output, which is why the
// pre-pass runs verbose.
func TestCoverageSeesADisabledExport(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test binaries with html/template overlaid")
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the test cannot write to")
	}
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": goMod, "template.go": greetingGo, "render_test.go": greetingTest})
	config := greetingConfig()
	scratch := t.TempDir()
	overlayPath, err := overlay.Write(t.Context(), config.toolchain(dir), scratch)
	require.NoError(t, err)
	coverDir := filepath.Join(scratch, "profiles")
	require.NoError(t, os.Mkdir(coverDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(coverDir, 0o700) })

	out, err := config.goTest(dir).coverage(t.Context(), overlayPath, coverDir)
	require.NoError(t, err, "the tests pass whatever the export does:\n%s", out.String())
	assert.True(t, strings.HasPrefix(out.coverDisabled, coverExportDisabled), "coverDisabled = %q; output:\n%s", out.coverDisabled, out.String())
}
