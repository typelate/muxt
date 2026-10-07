package mutation

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func greetingConfig() Configuration {
	return Configuration{TemplatesVariables: []string{"templates"}, Seed: 1, SeedSet: true, env: goEnv()}
}

// TestRun states a whole run over a template written as a Go string
// literal: the unmutated tests pass, each mutant is written back into the
// literal and tested, and every verdict lands in the report while progress
// streams to status.
func TestRun(t *testing.T) {
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": goMod, "template.go": greetingGo, "render_test.go": greetingTest})
	config := greetingConfig()
	config.Verbose = true
	config.Workers = 2
	config.Run = regexp.MustCompile("TestGreeting")

	var status strings.Builder
	report, err := Run(t.Context(), config, dir, &status)
	require.NoError(t, err)
	assert.True(t, report.Baseline.Passed, "baseline passed")
	var got []string
	for _, result := range report.Groups[0].Templates[0].Results {
		got = append(got, string(result.Status)+" "+string(result.Operator))
	}
	assert.Equal(t, []string{"KILL action-zero", "MISS if-false", "KILL if-true"}, got, "results")
	assert.Equal(t, 2, report.Killed, "killed")
	assert.Equal(t, 1, report.Missed, "missed")

	lines := strings.Split(strings.TrimSpace(status.String()), "\n")
	assert.Len(t, lines, 4, "status has the preamble and a line per mutant:\n%s", status.String())
	assert.True(t, strings.HasPrefix(lines[0], "3 mutants across 1 template (complexity 2), baseline "), "status opens with the preamble:\n%s", status.String())
}

// TestRunDryRun states that a dry run enumerates and tests nothing: here a
// test that always fails would otherwise fail the baseline.
func TestRunDryRun(t *testing.T) {
	t.Parallel()
	dir := module(t, map[string]string{
		"go.mod":         goMod,
		"template.go":    greetingGo,
		"render_test.go": "package server\n\nimport \"testing\"\n\nfunc TestNothingRuns(t *testing.T) { t.Fatal(\"a dry run ran the tests\") }\n",
	})
	config := greetingConfig()
	config.DryRun = true
	var status strings.Builder
	report, err := Run(t.Context(), config, dir, &status)
	require.NoError(t, err)
	assert.True(t, report.DryRun, "the report is a dry run")
	assert.Equal(t, 3, report.Total, "mutants")
	assert.Empty(t, status.String(), "status: no baseline ran")
}

// TestRunStopsWhenTheBaselineFails states that tests failing with nothing
// mutated stop the run, since every mutant would be recorded as caught.
func TestRunStopsWhenTheBaselineFails(t *testing.T) {
	t.Parallel()
	dir := module(t, map[string]string{
		"go.mod":         goMod,
		"template.go":    greetingGo,
		"render_test.go": "package server\n\nimport \"testing\"\n\nfunc TestBroken(t *testing.T) { t.Fatal(\"broken before mutation\") }\n",
	})
	_, err := Run(t.Context(), greetingConfig(), dir, nil)
	baseline, ok := errors.AsType[*BaselineFailedError](err)
	require.True(t, ok, "Run = %v, want a failing baseline", err)
	assert.Contains(t, baseline.Output, "broken before mutation", "baseline output holds the failure")
}

// TestRunStopsWhenGoTestCannotRun states that go test refusing to run, here
// over a flag value it cannot read, is an error of its own rather than a
// failing baseline, and that it says what go test said.
func TestRunStopsWhenGoTestCannotRun(t *testing.T) {
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": goMod, "template.go": greetingGo, "render_test.go": greetingTest})
	config := greetingConfig()
	config.GoTestArgs = []string{"-count=many"}
	_, err := Run(t.Context(), config, dir, nil)
	require.Error(t, err, "Run")
	assert.NotErrorAs(t, err, new(*BaselineFailedError), "Run: an error other than a failing baseline")
	assert.ErrorAs(t, err, new(*GoTestError), "Run: go test could not run")
	assert.Contains(t, err.Error(), `invalid value "many" for flag -count`, "Run: the error carries what go test printed")
}

// TestNewPlanIncludesTestCallersWhenAsked states that a template rendered
// only from a test is mutated only when test callers are included. Its
// text is an interpreted string literal, which a mutation is re-quoted
// into.
func TestNewPlanIncludesTestCallersWhenAsked(t *testing.T) {
	t.Parallel()
	dir := module(t, map[string]string{
		"go.mod": goMod,
		"template.go": "package server\n\nimport \"html/template\"\n\n" +
			"var templates = template.Must(template.New(\"greeting\").Parse(\"Hello, {{.Name}}!\\n\"))\n\n" +
			"type Greeting struct{ Name string }\n",
		"render_test.go": "package server\n\nimport (\n\t\"io\"\n\t\"testing\"\n)\n\n" +
			"func TestGreeting(t *testing.T) {\n\tif err := templates.ExecuteTemplate(io.Discard, \"greeting\", Greeting{Name: \"World\"}); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
	})

	_, err := newPlan(t.Context(), greetingConfig(), dir)
	require.ErrorAs(t, err, new(*NoCallSitesError), "newPlan: the only call site is in a test")

	config := greetingConfig()
	config.IncludeTests = true
	p, err := newPlan(t.Context(), config, dir)
	require.NoError(t, err)
	require.Len(t, p.mutants, 1, "the one action")
	m := p.mutants[0]
	assert.Equal(t, "template.go", m.Path, "mutant path")
	assert.Equal(t, 5, m.Line, "mutant line")
	assert.Contains(t, m.apply(), `Parse("Hello, {{\"\"}}!\n")`, "mutated file")
}

// neverRun is a suite that fails the test if anything runs it.
func neverRun(t *testing.T) (func(context.Context) (string, error), func(context.Context, string) (Status, error)) {
	t.Helper()
	return func(context.Context) (string, error) {
			assert.Fail(t, "the suite ran")
			return "", nil
		}, func(context.Context, string) (Status, error) {
			assert.Fail(t, "a mutant ran")
			return StatusMissed, nil
		}
}

// TestRunPlanDryRun states that a dry run reports the plan and runs
// nothing: no baseline, no mutant, and nothing on the status stream.
func TestRunPlanDryRun(t *testing.T) {
	t.Parallel()
	_, p := runnerFixture(t, []bool{true, false})
	baseline, verdict := neverRun(t)

	var status strings.Builder
	report, err := runPlan(t.Context(), p, Configuration{DryRun: true}, &status, baseline, verdict)
	require.NoError(t, err)
	assert.True(t, report.DryRun, "the report says it was a dry run")
	assert.Empty(t, status.String(), "status")
}

// TestRunPlanStopsWhenTheBaselineFails states that tests failing with
// nothing mutated stop the run before a single mutant is tried, carrying
// the output that shows what failed.
func TestRunPlanStopsWhenTheBaselineFails(t *testing.T) {
	t.Parallel()
	_, p := runnerFixture(t, []bool{true})
	_, verdict := neverRun(t)
	failed := exitStatusOne(t)

	_, err := runPlan(t.Context(), p, Configuration{}, nil, func(context.Context) (string, error) {
		return "--- FAIL: TestIndex (0.00s)\n", failed
	}, verdict)

	baselineErr, ok := errors.AsType[*BaselineFailedError](err)
	require.True(t, ok, "runPlan = %v, want a failing baseline", err)
	assert.Contains(t, baselineErr.Output, "--- FAIL: TestIndex", "the error carries what failed")
}

// TestRunPlanStopsWhenTheSuiteCannotRun states that go test failing to run
// at all is that error, not a failing baseline: reading it as one would
// report the tests as broken when the command was.
func TestRunPlanStopsWhenTheSuiteCannotRun(t *testing.T) {
	t.Parallel()
	_, p := runnerFixture(t, []bool{true})
	_, verdict := neverRun(t)
	cannotRun := errors.New("go: no such tool")

	_, err := runPlan(t.Context(), p, Configuration{}, nil, func(context.Context) (string, error) {
		return "", cannotRun
	}, verdict)

	require.ErrorIs(t, err, cannotRun, "runPlan: the error go gave")
	assert.NotErrorAs(t, err, new(*BaselineFailedError), "a command that could not run is not a failing baseline")
}

// TestRunPlanStopsWhenCancelled states that a run whose context is done
// stops starting mutants, returns the context's error rather than a
// report, and removes the files it wrote for the mutants it had started.
func TestRunPlanStopsWhenCancelled(t *testing.T) {
	t.Parallel()
	_, p := runnerFixture(t, []bool{true, false, true, false})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var (
		mu       sync.Mutex
		overlays []string
	)
	_, err := runPlan(ctx, p, Configuration{Workers: 2}, nil, func(context.Context) (string, error) {
		return "", nil
	}, func(ctx context.Context, overlay string) (Status, error) {
		mu.Lock()
		overlays = append(overlays, overlay)
		if len(overlays) == 2 {
			cancel()
		}
		mu.Unlock()
		<-ctx.Done()
		return "", ctx.Err()
	})

	require.ErrorIs(t, err, context.Canceled, "runPlan after cancelling")
	assert.Len(t, overlays, 2, "mutants started: none after the context was cancelled")
	for _, overlay := range overlays {
		_, statErr := os.Stat(overlay)
		assert.ErrorIs(t, statErr, fs.ErrNotExist, "overlay %s outlived the run", overlay)
	}
}

// TestRunPlanReportsWhatTheSuiteSaid states a whole run without a suite of
// its own: the preamble says how much work there is, and each verdict
// lands in the report.
func TestRunPlanReportsWhatTheSuiteSaid(t *testing.T) {
	t.Parallel()
	_, p := runnerFixture(t, []bool{true, false, true})

	var status strings.Builder
	got, err := runPlan(t.Context(), p, Configuration{Verbose: true}, &status, func(context.Context) (string, error) {
		return "", nil
	}, func(_ context.Context, overlay string) (Status, error) {
		if strings.Contains(readMutated(t, overlay), "K") {
			return StatusKilled, nil
		}
		return StatusMissed, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, got.Killed, "killed")
	assert.Equal(t, 1, got.Missed, "missed")
	assert.True(t, got.Baseline.Passed, "the report says the baseline passed")
	assert.True(t, strings.HasPrefix(status.String(), "3 mutants across 1 template (complexity 0), baseline "), "status opens with the preamble:\n%s", status.String())
	lines := strings.Count(strings.TrimSpace(status.String()), "\n") + 1
	assert.Equal(t, 4, lines, "status has the preamble and one line per mutant:\n%s", status.String())
}
