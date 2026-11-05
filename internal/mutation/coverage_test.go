package mutation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/overlay"
	"github.com/typelate/muxt/template/html"
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
	got, _, err := templateCoverage(t.Context(), config.toolchain(dir), t.TempDir(), tester.coverage, &warnings)
	require.NoError(t, err)
	require.NotNil(t, got, "coverage is known; warnings:\n%s", warnings.String())
	assert.Empty(t, warnings.String(), "warnings")
	greeting := got["greeting"]
	assert.Contains(t, greeting, coverBlock{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}, "the text before the name is reached: %v", got)
	assert.NotContains(t, greeting, coverBlock{StartLine: 1, StartCol: 30, EndLine: 1, EndCol: 34}, "no test renders the greeting loud: %v", got)

	writeFiles(t, dir, map[string]string{"loud/loud_test.go": loudTest})
	got, _, err = templateCoverage(t.Context(), config.toolchain(dir), t.TempDir(), tester.coverage, &warnings)
	require.NoError(t, err)
	require.NotNil(t, got, "coverage is known; warnings:\n%s", warnings.String())
	greeting = got["greeting"]
	assert.Contains(t, greeting, coverBlock{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}, "reached by both packages: %v", got)
	assert.Contains(t, greeting, coverBlock{StartLine: 1, StartCol: 30, EndLine: 1, EndCol: 34}, "reached by the loud package: %v", got)
}

// attributeInBranchGo has an attribute inside a {{with}} in a start tag,
// the shape of a row whose data attribute depends on an optional field.
const attributeInBranchGo = `package server

import (
	"html/template"
	"io"
)

var templates = template.Must(template.New("row").Parse(` + "`" + `<p {{with .}}data-v='{{.}}'{{end}}>x</p>` + "`" + `))

func Render(w io.Writer, version string) error {
	return templates.ExecuteTemplate(w, "row", version)
}
`

// attributeInBranchTest renders the row with a version and checks the
// attribute, so it executes every action in the start tag.
const attributeInBranchTest = `package server

import (
	"strings"
	"testing"
)

func TestRow(t *testing.T) {
	var buf strings.Builder
	if err := Render(&buf, "1"); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, ` + "`" + `data-v="1"` + "`" + `) {
		t.Errorf("row = %q, want data-v", got)
	}
}
`

// TestTemplateCoverageReachesAnAttributeInsideABranch runs the pre-pass
// over a test that renders an attribute inside a {{with}} in a start tag:
// the action in its value is executed, so no mutant of the template may
// be reported unreached. The engine once left such an attribute's counter
// at zero, and the action in it was skipped though a test caught it.
func TestTemplateCoverageReachesAnAttributeInsideABranch(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test binaries with html/template overlaid")
	}
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": goMod, "template.go": attributeInBranchGo, "render_test.go": attributeInBranchTest})
	config := greetingConfig()
	tester := config.goTest(dir)

	var warnings strings.Builder
	got, _, err := templateCoverage(t.Context(), config.toolchain(dir), t.TempDir(), tester.coverage, &warnings)
	require.NoError(t, err)
	require.NotNil(t, got, "coverage is known; warnings:\n%s", warnings.String())

	p, err := newPlan(t.Context(), config, dir)
	require.NoError(t, err)
	p.cover(got, 0)
	require.NotEmpty(t, p.candidates, "mutants planned")
	assert.Empty(t, unreachedMutants(p), "unreached mutants; coverage: %v", got)
}

// TestTemplateCoverageWarnsOfARaisedGoLine runs the pre-pass over a module
// at go 1.24: the engine needs go 1.27, so the go command raises the
// modfile copy's go line and compiles the module at that language
// version. The pre-pass says so once and still uses the coverage.
func TestTemplateCoverageWarnsOfARaisedGoLine(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test binaries with html/template overlaid")
	}
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": "module server\n\ngo 1.24\n", "template.go": greetingGo, "render_test.go": greetingTest})
	config := greetingConfig()

	var warnings strings.Builder
	got, _, err := templateCoverage(t.Context(), config.toolchain(dir), t.TempDir(), config.goTest(dir).coverage, &warnings)
	require.NoError(t, err)
	assert.NotNil(t, got, "the coverage is used; warnings:\n%s", warnings.String())
	assert.Equal(t, "warning: template coverage was computed at go 1.27 (your go.mod says go 1.24): branches that depend on the older language semantics may be reported unreached\n", warnings.String(), "warnings")
}

// TestTemplateCoverageWarnsOfNewerSharedModules states that a pre-pass in
// which the go command raised a module both the engine and the test
// target require, here golang.org/x/net in muxt's own go.mod, warns once,
// naming the module and both versions, and still uses the coverage.
func TestTemplateCoverageWarnsOfNewerSharedModules(t *testing.T) {
	realGOROOT(t)
	run := func(_ context.Context, engine overlay.Files, coverDir string) (*goTestOutput, error) {
		b, err := os.ReadFile(engine.ModFile)
		require.NoError(t, err)
		var raised []string
		for line := range strings.Lines(string(b)) {
			if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "golang.org/x/net" {
				line = strings.Replace(line, fields[1], "v0.999.0", 1)
			}
			raised = append(raised, line)
		}
		require.NoError(t, os.WriteFile(engine.ModFile, []byte(strings.Join(raised, "")), 0o644))
		writeFiles(t, coverDir, map[string]string{"1.cov": "mode: set\npage:1.1,1.8 1 1\n"})
		return testOutput("ok  \tserver\t0.1s\n"), nil
	}
	var warnings strings.Builder
	got, _, err := templateCoverage(t.Context(), overlay.Toolchain{}, t.TempDir(), run, &warnings)
	require.NoError(t, err)
	assert.NotNil(t, got, "the coverage is used")
	w := warnings.String()
	assert.Regexp(t, `^warning: the coverage pre-pass selected newer versions of modules your go.mod requires: golang.org/x/net v\S+ → v0.999.0; tests exercising them may take other paths\n$`, w, "warnings")
}

// TestTemplateCoverageOffline runs the pre-pass where the go command can
// fetch nothing: the engine's golang.org/x/net is not in the module cache
// and GOPROXY=off. The warning quotes the go command's reason, not a
// download line, and names no engine gap, since none is to blame.
func TestTemplateCoverageOffline(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test with html/template overlaid")
	}
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": goMod, "template.go": greetingGo, "render_test.go": greetingTest})
	config := greetingConfig()
	tester := config.goTest(dir)
	tester.env = append(slices.Clip(tester.env), "GOPROXY=off", "GOMODCACHE="+t.TempDir())

	var warnings strings.Builder
	got, _, err := templateCoverage(t.Context(), config.toolchain(dir), t.TempDir(), tester.coverage, &warnings)
	require.NoError(t, err, "the run goes on")
	assert.Nil(t, got, "coverage is unknown")
	w := warnings.String()
	assert.True(t, strings.HasPrefix(w, "warning: template coverage unknown: "), "warning %q", w)
	assert.Contains(t, w, "GOPROXY=off", "the go command's reason")
	assert.NotContains(t, w, "go: downloading", "warning")
	assert.NotContains(t, w, "known gaps", "warning")
}

// TestFirstErrorSkipsDownloads states that a module download line is
// progress, not the error.
func TestFirstErrorSkipsDownloads(t *testing.T) {
	out := "go: downloading golang.org/x/net v0.62.0\ngo: golang.org/x/net@v0.62.0: module lookup disabled by GOPROXY=off\n"
	assert.Equal(t, "go: golang.org/x/net@v0.62.0: module lookup disabled by GOPROXY=off", firstError(out, exitError(t, 1)))
}

// TestTemplateCoverageRunsTheEngineOverlay states what the pre-pass hands
// go test: the engine's overlay, modfile and module, written under the
// run's scratch directory, and an empty directory for the profiles, under
// it too.
func TestTemplateCoverageRunsTheEngineOverlay(t *testing.T) {
	realGOROOT(t)
	scratch := t.TempDir()
	runs := 0
	var ran overlay.Files
	run := func(_ context.Context, engine overlay.Files, coverDir string) (*goTestOutput, error) {
		runs++
		ran = engine
		for _, p := range []string{engine.Overlay, engine.ModFile} {
			assert.True(t, filepath.IsAbs(p), "%s is absolute", p)
			assert.True(t, under(scratch, p), "%s is under the scratch directory %s", p, scratch)
			_, err := os.Stat(p)
			assert.NoError(t, err, "%s is written before go test runs", p)
		}
		assert.True(t, under(scratch, coverDir), "cover directory %s is under the scratch directory %s", coverDir, scratch)
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
	got, engine, err := templateCoverage(t.Context(), overlay.Toolchain{}, scratch, run, &warnings)
	require.NoError(t, err)
	assert.Equal(t, 1, runs, "go test runs once")
	assert.Equal(t, "engine from this module", engine, "muxt's own module requires no engine")
	assert.Empty(t, ran.Version, "the modfile copy of muxt's own go.mod requires nothing")
	assert.Empty(t, warnings.String(), "warnings")
	assert.Equal(t, coverage{
		"page": {
			{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}: {},
			{StartLine: 2, StartCol: 3, EndLine: 2, EndCol: 9}: {},
		},
		"other": {{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 4}: {}},
	}, got, "the union of every profile, by block")
}

// TestTemplateCoverageReachingNothing states that a test binary that ran
// the engine and executed no template, whose profile is only its mode
// line, leaves a known, empty coverage: nothing is reached.
func TestTemplateCoverageReachingNothing(t *testing.T) {
	realGOROOT(t)
	run := func(_ context.Context, _ overlay.Files, coverDir string) (*goTestOutput, error) {
		writeFiles(t, coverDir, map[string]string{"1.cov": "mode: set\n"})
		return testOutput("ok  \tserver\t0.1s\n"), nil
	}
	var warnings strings.Builder
	got, _, err := templateCoverage(t.Context(), overlay.Toolchain{}, t.TempDir(), run, &warnings)
	require.NoError(t, err)
	assert.NotNil(t, got, "coverage is known")
	assert.Empty(t, got, "nothing is reached")
	assert.Empty(t, warnings.String(), "warnings")
}

// TestTemplateCoverageWithoutAProfile states that a pre-pass in which no
// test binary wrote a profile -- a package with no test files, tests run
// elsewhere by -exec, a server the tests start as a binary of its own --
// leaves coverage unknown: nothing says the engine ran, so an empty
// profile directory is not "nothing reached".
func TestTemplateCoverageWithoutAProfile(t *testing.T) {
	realGOROOT(t)
	run := func(context.Context, overlay.Files, string) (*goTestOutput, error) {
		return testOutput("?   \tserver\t[no test files]\n"), nil
	}
	var warnings strings.Builder
	got, _, err := templateCoverage(t.Context(), overlay.Toolchain{}, t.TempDir(), run, &warnings)
	require.NoError(t, err)
	assert.Nil(t, got, "coverage is unknown")
	assert.Equal(t, "warning: template coverage unknown: no test binary ran the templates (no coverage profile was written)\n", warnings.String(), "warnings")
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
			run := func(_ context.Context, _ overlay.Files, coverDir string) (*goTestOutput, error) {
				if tt.toolchain != nil {
					t.Error("go test ran though the probe failed")
				}
				writeFiles(t, coverDir, map[string]string{"1.cov": "mode: set\npage:1.1,1.8 1 1\n"})
				return testOutput(tt.out), tt.err
			}
			var warnings strings.Builder
			got, _, err := templateCoverage(t.Context(), tc, t.TempDir(), run, &warnings)
			require.NoError(t, err)
			assert.Nil(t, got, "coverage is unknown")
			lines := strings.Split(strings.TrimRight(warnings.String(), "\n"), "\n")
			require.Len(t, lines, 1, "one warning line:\n%s", warnings.String())
			assert.True(t, strings.HasPrefix(lines[0], "warning: template coverage unknown: "), "warning %q", lines[0])
			assert.Contains(t, lines[0], tt.warning, "warning")
		})
	}
}

// TestTemplateCoverageFailingUnderTheEngine states that tests failing or
// not building with the engine in place leave coverage unknown, with one
// warning naming the first failure and the engine's known gaps, and do
// not stop the run: kill runs use the real html/template, so the
// baseline, not the pre-pass, says whether the tests pass.
func TestTemplateCoverageFailingUnderTheEngine(t *testing.T) {
	for _, tt := range []struct {
		name  string
		out   string
		first string
	}{
		{
			name:  "a test fails",
			out:   "=== RUN   TestBroken\n    a_test.go:5: broken\n--- FAIL: TestBroken (0.00s)\nFAIL\nFAIL\tserver\t0.1s\n",
			first: "--- FAIL: TestBroken",
		},
		{
			name:  "a package does not build",
			out:   "# server\n./page.go:9:12: ts.Tree.Root undefined\nFAIL\tserver [build failed]\n",
			first: "./page.go:9:12: ts.Tree.Root undefined",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			realGOROOT(t)
			run := func(_ context.Context, _ overlay.Files, coverDir string) (*goTestOutput, error) {
				writeFiles(t, coverDir, map[string]string{"1.cov": "mode: set\npage:1.1,1.8 1 1\n"})
				return testOutput(tt.out), exitError(t, 1)
			}
			var warnings strings.Builder
			got, _, err := templateCoverage(t.Context(), overlay.Toolchain{}, t.TempDir(), run, &warnings)
			require.NoError(t, err, "the run goes on")
			assert.Nil(t, got, "coverage is unknown")
			want := "warning: template coverage unknown: tests fail under muxt's template engine (coverage pre-pass): " + tt.first +
				"; known gaps: Template.Tree, AddParseTree, Error.Node\n"
			assert.Equal(t, want, warnings.String(), "warning")
		})
	}
}

// parseTreeTest compiles against html/template and not against the
// engine, whose Template.Tree is no *parse.Tree.
const parseTreeTest = `package server

import (
	"testing"
	"text/template/parse"
)

func TestTree(t *testing.T) {
	var tree *parse.Tree = templates.Tree
	if tree == nil {
		t.Fatal("no tree")
	}
}
`

// TestRunGoesOnWhenTheEngineDoesNotBuild runs a module whose tests use
// Template.Tree as a *parse.Tree: under the engine they do not compile, so
// the pre-pass warns and the run carries on with the real html/template.
func TestRunGoesOnWhenTheEngineDoesNotBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test binaries with html/template overlaid")
	}
	realGOROOT(t)
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": goMod, "template.go": greetingGo, "render_test.go": greetingTest, "tree_test.go": parseTreeTest})
	var status strings.Builder
	report, err := Run(t.Context(), greetingConfig(), dir, &status)
	require.NoError(t, err, "status:\n%s", status.String())
	assert.True(t, report.Baseline.Passed, "baseline passed")
	assert.Equal(t, 3, report.Killed+report.Missed, "every mutant ran")
	first, _, _ := strings.Cut(status.String(), "\n")
	assert.True(t, strings.HasPrefix(first, "warning: template coverage unknown: tests fail under muxt's template engine (coverage pre-pass): "), "status opens with the warning:\n%s", status.String())
	assert.Contains(t, first, "tree_test.go", "the warning names the build error")
	assert.True(t, strings.HasSuffix(first, "; known gaps: Template.Tree, AddParseTree, Error.Node"), "warning %q", first)
}

// TestTemplateCoverageStopsWhenCancelled states that an interrupted
// pre-pass is not a verdict on anything: the run stops with ctx's error.
func TestTemplateCoverageStopsWhenCancelled(t *testing.T) {
	realGOROOT(t)
	ctx, cancel := context.WithCancel(t.Context())
	run := func(context.Context, overlay.Files, string) (*goTestOutput, error) {
		cancel()
		return testOutput(""), context.Canceled
	}
	_, _, err := templateCoverage(ctx, overlay.Toolchain{}, t.TempDir(), run, nil)
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
		{name: "past the line cap", out: strings.Repeat("x", 3*maxMarkerLine) + "muxt: template coverage export disabled: late\n", want: "muxt: template coverage export disabled: late"},
		{name: "without a newline", out: "muxt: template coverage export disabled: last", want: "muxt: template coverage export disabled: last"},
		{name: "a long reason is cut", out: "muxt: template coverage export disabled: " + strings.Repeat("y", 2*maxMarkerLine) + "\n", want: ("muxt: template coverage export disabled: " + strings.Repeat("y", 2*maxMarkerLine))[:maxMarkerLine]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, testOutput(tt.out).coverDisabled, "coverDisabled after %q", tt.out)
		})
	}
}

// TestGoTestOutputNotesAMarkerSplitAcrossWrites states that the marker is
// found however go test's output is cut into writes.
func TestGoTestOutputNotesAMarkerSplitAcrossWrites(t *testing.T) {
	text := strings.Repeat("x", 3*maxMarkerLine) + "muxt: template coverage export disabled: split reason\nok  \tserver\t0.1s\n"
	for size := 1; size <= 64; size *= 2 {
		out := &goTestOutput{limit: 16}
		for rest := text; len(rest) > 0; {
			n := min(size, len(rest))
			_, _ = out.Write([]byte(rest[:n]))
			rest = rest[n:]
		}
		assert.Equal(t, "muxt: template coverage export disabled: split reason", out.coverDisabled, "coverDisabled in writes of %d bytes", size)
	}
}

// TestWithoutCoverFlags states which of the caller's flags the pre-pass
// drops: its own coverage would otherwise be a second profile written to
// the caller's file, or instrumentation of the engine's packages.
func TestWithoutCoverFlags(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args, want []string
	}{
		{name: "none", args: []string{"-short", "-v"}, want: []string{"-short", "-v"}},
		{name: "cover", args: []string{"-cover", "-short"}, want: []string{"-short"}},
		{name: "cover with a value", args: []string{"-cover=true", "--cover"}, want: []string{}},
		{name: "covermode", args: []string{"-covermode=atomic", "-covermode", "set", "-short"}, want: []string{"-short"}},
		{name: "coverprofile", args: []string{"-coverprofile=c.out", "--coverprofile", "d.out"}, want: []string{}},
		{name: "coverpkg", args: []string{"-coverpkg", "./...", "-coverpkg=./a"}, want: []string{}},
		{name: "a flag only starting with cover", args: []string{"-coverx", "-covers=1"}, want: []string{"-coverx", "-covers=1"}},
		{name: "test binary flags", args: []string{"-cover", "-args", "-coverprofile=x", "-cover"}, want: []string{"-args", "-coverprofile=x", "-cover"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, withoutCoverFlags(tt.args), "withoutCoverFlags(%q)", tt.args)
		})
	}
}

func TestCoverageArgs(t *testing.T) {
	test := goTest{packages: []string{"./..."}, extra: []string{"-short"}}
	engine := overlay.Files{Overlay: "/s/overlay.json", ModFile: "/s/go.mod"}
	assert.Equal(t,
		[]string{"test", "-count=1", "-v", "-mod=mod", "-modfile=/s/go.mod", "-overlay=/s/overlay.json", "./...", "-short"},
		test.args(coverageFlags(engine)...),
		"go test arguments of the pre-pass")
}

// TestNewPlanHoldsTheCoverage states that the profile reaches the plan:
// planning knows no coverage, and cover gives it what the pre-pass saw.
func TestNewPlanHoldsTheCoverage(t *testing.T) {
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": goMod, "template.go": greetingGo})
	cov := coverage{"greeting": {{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}: {}}}
	p, err := newPlan(t.Context(), greetingConfig(), dir)
	require.NoError(t, err)
	assert.Nil(t, p.coverage, "coverage before the pre-pass")
	p.cover(cov, 0)
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

// refusingToolchain is a go command that reports a GOROOT and a go.mod and
// refuses any overlay of the GOROOT.
func refusingToolchain(t *testing.T) overlay.Toolchain {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake go command is a shell script")
	}
	goroot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(goroot, "src", "html", "template"), 0o755))
	goModPath := filepath.Join(t.TempDir(), "go.mod")
	require.NoError(t, os.WriteFile(goModPath, []byte(goMod), 0o644))
	script := `#!/bin/sh
case "$1" in
env) if [ "$2" = GOMOD ]; then echo '` + goModPath + `'; else printf '%s\n%s\n' '` + goroot + `' '` + t.TempDir() + `'; fi ;;
*) echo "go: overlay contains a replacement for GOROOT" >&2; exit 1 ;;
esac
`
	goBin := filepath.Join(t.TempDir(), "go")
	require.NoError(t, os.WriteFile(goBin, []byte(script), 0o755))
	return overlay.Toolchain{Go: goBin, ModuleDir: muxtDir()}
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
	target, err := overlay.Resolve(t.Context(), config.toolchain(dir))
	require.NoError(t, err)
	engine, err := overlay.Write(target, scratch)
	require.NoError(t, err)
	coverDir := filepath.Join(scratch, "profiles")
	require.NoError(t, os.Mkdir(coverDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(coverDir, 0o700) })

	out, err := config.goTest(dir).coverage(t.Context(), engine, coverDir)
	require.NoError(t, err, "the tests pass whatever the export does:\n%s", out.String())
	assert.True(t, strings.HasPrefix(out.coverDisabled, html.CoverDisabledMarker), "coverDisabled = %q; output:\n%s", out.coverDisabled, out.String())
}

// TestRunPlanPreambleCountsThePrePass states that the preamble says the
// coverage pre-pass was one more run of the suite, and how long it took.
func TestRunPlanPreambleCountsThePrePass(t *testing.T) {
	t.Parallel()
	_, p := runnerFixture(t, []bool{true})
	p.prePass = 2 * time.Second
	var status strings.Builder
	_, err := runPlan(t.Context(), p, Configuration{}, t.TempDir(), &status, suite{
		baseline: func(context.Context) (string, error) {
			return "", nil
		},
		verdict: func(context.Context, string) (Status, []string, error) {
			return StatusKilled, nil, nil
		},
	})
	require.NoError(t, err)
	first, _, _ := strings.Cut(status.String(), "\n")
	assert.Contains(t, first, ", coverage pre-pass 2s, ", "preamble %q", first)
}

// TestRunPlanPreambleNamesTheEngine states that a verbose preamble says
// which muxt the pre-pass built the engine from, and a quiet one does not.
func TestRunPlanPreambleNamesTheEngine(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		_, p := runnerFixture(t, []bool{true})
		p.prePass = 2 * time.Second
		p.engine = "engine github.com/typelate/muxt@v0.19.0"
		var status strings.Builder
		_, err := runPlan(t.Context(), p, Configuration{Verbose: verbose}, t.TempDir(), &status, suite{
			baseline: func(context.Context) (string, error) {
				return "", nil
			},
			verdict: func(context.Context, string) (Status, []string, error) {
				return StatusKilled, nil, nil
			},
		})
		require.NoError(t, err)
		first, _, _ := strings.Cut(status.String(), "\n")
		if verbose {
			assert.Contains(t, first, ", coverage pre-pass 2s (engine github.com/typelate/muxt@v0.19.0), ", "verbose preamble %q", first)
		} else {
			assert.Contains(t, first, ", coverage pre-pass 2s, ", "preamble %q", first)
		}
	}
}

// TestEngineNote states how the preamble names the engine's module.
func TestEngineNote(t *testing.T) {
	for _, tt := range []struct {
		files overlay.Files
		want  string
	}{
		{overlay.Files{}, "engine from this module"},
		{overlay.Files{Version: "v0.19.0"}, "engine github.com/typelate/muxt@v0.19.0"},
		{overlay.Files{Version: "v0.19.0", ModuleDir: "/src/muxt"}, "engine github.com/typelate/muxt@v0.19.0 => /src/muxt"},
		{overlay.Files{Version: overlay.PlaceholderVersion, ModuleDir: "/src/muxt"}, "engine github.com/typelate/muxt => /src/muxt"},
		{overlay.Files{Version: "v0.19.0", TargetVersion: "v0.18.0"}, "engine github.com/typelate/muxt@v0.19.0, not your go.mod's v0.18.0"},
	} {
		assert.Equal(t, tt.want, engineNote(tt.files), "engineNote(%+v)", tt.files)
	}
}

// slowTest takes longer than a plan does, so a run that runs the tests
// before failing to plan shows it.
// markingTest leaves a file in its package directory so a test can tell
// whether the runner ran the tests at all.
const markingTest = `package server

import (
	"os"
	"testing"
)

func TestMarks(t *testing.T) {
	if err := os.WriteFile("tests_ran.txt", nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
`

// TestRunPlansBeforeThePrePass states that a run that cannot plan says so
// without running the tests: the coverage pre-pass comes after planning,
// and only when there is a mutant to run.
func TestRunPlansBeforeThePrePass(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	for _, tt := range []struct {
		name   string
		files  map[string]string
		config func(*Configuration)
	}{
		{
			name:   "a template pattern matching nothing",
			files:  map[string]string{"go.mod": goMod, "template.go": greetingGo, "marking_test.go": markingTest},
			config: func(c *Configuration) { c.TemplatePattern = regexp.MustCompile(`^nomatch$`) },
		},
		{
			name:   "no call sites",
			files:  map[string]string{"go.mod": goMod, "template.go": strings.Replace(greetingGo, "return templates.ExecuteTemplate(w, \"greeting\", greeting)", "return nil", 1), "marking_test.go": markingTest},
			config: func(*Configuration) {},
		},
		{
			name:   "a templates variable that does not exist",
			files:  map[string]string{"go.mod": goMod, "template.go": greetingGo, "marking_test.go": markingTest},
			config: func(c *Configuration) { c.TemplatesVariables = []string{"missing"} },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := module(t, tt.files)
			config := greetingConfig()
			tt.config(&config)
			_, err := Run(t.Context(), config, dir, nil)
			require.Error(t, err, "Run")
			assert.NoFileExists(t, filepath.Join(dir, "tests_ran.txt"), "Run returned %v after running the tests", err)
		})
	}
}

// TestRunPlanSkipsUnreachedMutants states that a mutant the coverage
// pre-pass shows no test reaching is not run unless the configuration
// includes uncovered nodes: it is skipped, counted with the other skips
// and still listed as unreached. Coverage that is unknown skips nothing.
//
// The greeting has three mutants: the name action, reached by a block
// over it, and the if's two, which nothing reaches.
func TestRunPlanSkipsUnreachedMutants(t *testing.T) {
	t.Parallel()
	nameReached := coverage{"greeting": {
		{StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}:  {},
		{StartLine: 1, StartCol: 8, EndLine: 1, EndCol: 17}: {},
	}}
	for _, tt := range []struct {
		name         string
		coverage     coverage
		include      bool
		runs         int
		skipped      int
		unreached    int
		summary      string
		includeField bool
	}{
		{name: "unreached skipped by default", coverage: nameReached, runs: 1, skipped: 2, unreached: 2, summary: "\n3 mutants, 1 killed, 0 missed, 2 skipped, 2 unreached\n"},
		{name: "every mutant unreached", coverage: coverage{}, runs: 0, skipped: 3, unreached: 3, summary: "\n3 mutants, 0 killed, 0 missed, 3 skipped, 3 unreached\n"},
		{name: "unreached run when included", coverage: nameReached, include: true, runs: 3, unreached: 2, summary: "\n3 mutants, 3 killed, 0 missed, 0 skipped, 2 unreached (run)\n", includeField: true},
		{name: "coverage unknown", runs: 3, summary: "\n3 mutants, 3 killed, 0 missed, 0 skipped\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := module(t, map[string]string{"go.mod": goMod, "template.go": greetingGo})
			config := greetingConfig()
			config.IncludeUncoveredNodes = tt.include
			p, err := newPlan(t.Context(), config, dir)
			require.NoError(t, err)
			p.cover(tt.coverage, 0)
			var (
				mu   sync.Mutex
				runs int
			)
			report, err := runPlan(t.Context(), p, config, t.TempDir(), nil, suite{
				baseline: func(context.Context) (string, error) {
					return "", nil
				},
				verdict: func(context.Context, string) (Status, []string, error) {
					mu.Lock()
					defer mu.Unlock()
					runs++
					return StatusKilled, nil, nil
				},
			})
			require.NoError(t, err)
			assert.Equal(t, tt.runs, runs, "mutant runs")
			assert.Equal(t, tt.skipped, report.Skipped, "skipped")
			assert.Len(t, report.Unreached, tt.unreached, "unreached")
			assert.Equal(t, tt.includeField, report.IncludeUncoveredNodes, "include uncovered nodes")
			report.Verbose = true
			text := writeReport(t, report)
			assert.Contains(t, text, tt.summary, "summary")
			if tt.skipped > 0 && !tt.include {
				assert.Contains(t, text, "SKIP 8:80 if-false (no test reaches it)", "the skipped mutant says why")
			}
			assert.NoError(t, report.Err(), "nothing missed")
		})
	}
}
