package mutation

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"
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
	report, err := Run(config, dir, &status)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Baseline.Passed {
		t.Error("baseline did not pass")
	}
	var got []string
	for _, result := range report.Groups[0].Templates[0].Results {
		got = append(got, string(result.Status)+" "+string(result.Operator))
	}
	if want := []string{"KILL action-zero", "MISS if-false", "KILL if-true"}; !slices.Equal(got, want) {
		t.Errorf("results %q, want %q", got, want)
	}
	if report.Killed != 2 || report.Missed != 1 {
		t.Errorf("killed %d, missed %d, want 2 and 1", report.Killed, report.Missed)
	}

	lines := strings.Split(strings.TrimSpace(status.String()), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "3 mutants across 1 template (complexity 2), baseline ") {
		t.Errorf("status:\n%s\nwant the preamble and a line per mutant", status.String())
	}
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
	report, err := Run(config, dir, &status)
	if err != nil {
		t.Fatal(err)
	}
	if !report.DryRun || report.Total != 3 {
		t.Errorf("dry run %t with %d mutants, want a dry run of 3", report.DryRun, report.Total)
	}
	if status.Len() != 0 {
		t.Errorf("status = %q, want nothing: no baseline ran", status.String())
	}
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
	_, err := Run(greetingConfig(), dir, nil)
	baseline, ok := errors.AsType[*BaselineFailedError](err)
	if !ok {
		t.Fatalf("Run = %v, want a failing baseline", err)
	}
	if !strings.Contains(baseline.Output, "broken before mutation") {
		t.Errorf("baseline output does not hold the failure:\n%s", baseline.Output)
	}
}

// TestRunStopsWhenGoTestCannotRun states that go test refusing to run, here
// over a flag value it cannot read, is an error of its own rather than a
// failing baseline.
func TestRunStopsWhenGoTestCannotRun(t *testing.T) {
	t.Parallel()
	dir := module(t, map[string]string{"go.mod": goMod, "template.go": greetingGo, "render_test.go": greetingTest})
	config := greetingConfig()
	config.GoTestArgs = []string{"-count=many"}
	_, err := Run(config, dir, nil)
	if err == nil {
		t.Fatal("Run = nil, want an error")
	}
	if _, ok := errors.AsType[*BaselineFailedError](err); ok {
		t.Errorf("Run = %v, want an error other than a failing baseline", err)
	}
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

	if _, err := newPlan(greetingConfig(), dir); err == nil {
		t.Fatal("newPlan = nil, want no call sites: the only one is in a test")
	} else if _, ok := errors.AsType[*NoCallSitesError](err); !ok {
		t.Fatalf("newPlan = %v, want no call sites", err)
	}

	config := greetingConfig()
	config.IncludeTests = true
	p, err := newPlan(config, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.mutants) != 1 {
		t.Fatalf("mutants = %d, want the one action", len(p.mutants))
	}
	m := p.mutants[0]
	if m.Path != "template.go" || m.Line != 5 {
		t.Errorf("mutant at %s:%d, want template.go:5", m.Path, m.Line)
	}
	if want := `Parse("Hello, {{\"\"}}!\n")`; !strings.Contains(m.Apply(), want) {
		t.Errorf("mutated file does not hold %s:\n%s", want, m.Apply())
	}
}

// neverRun is a suite that fails the test if anything runs it.
func neverRun(t *testing.T) (func([]string) (string, error), func(string) (Status, error)) {
	t.Helper()
	return func([]string) (string, error) {
			t.Error("the suite ran")
			return "", nil
		}, func(string) (Status, error) {
			t.Error("a mutant ran")
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
	report, err := runPlan(p, Configuration{DryRun: true}, &status, baseline, verdict)
	if err != nil {
		t.Fatal(err)
	}
	if !report.DryRun {
		t.Error("the report does not say it was a dry run")
	}
	if status.Len() != 0 {
		t.Errorf("status = %q, want nothing", status.String())
	}
}

// TestRunPlanStopsWhenTheBaselineFails states that tests failing with
// nothing mutated stop the run before a single mutant is tried, carrying
// the output that shows what failed.
func TestRunPlanStopsWhenTheBaselineFails(t *testing.T) {
	t.Parallel()
	_, p := runnerFixture(t, []bool{true})
	_, verdict := neverRun(t)
	failed := exitStatusOne(t)

	_, err := runPlan(p, Configuration{}, nil, func([]string) (string, error) {
		return "--- FAIL: TestIndex (0.00s)\n", failed
	}, verdict)

	baselineErr, ok := errors.AsType[*BaselineFailedError](err)
	if !ok {
		t.Fatalf("runPlan = %v, want a failing baseline", err)
	}
	if !strings.Contains(baselineErr.Output, "--- FAIL: TestIndex") {
		t.Errorf("the error does not carry what failed: %q", baselineErr.Output)
	}
}

// TestRunPlanStopsWhenTheSuiteCannotRun states that go test failing to run
// at all is that error, not a failing baseline: reading it as one would
// report the tests as broken when the command was.
func TestRunPlanStopsWhenTheSuiteCannotRun(t *testing.T) {
	t.Parallel()
	_, p := runnerFixture(t, []bool{true})
	_, verdict := neverRun(t)
	cannotRun := errors.New("go: no such tool")

	_, err := runPlan(p, Configuration{}, nil, func([]string) (string, error) {
		return "", cannotRun
	}, verdict)

	if !errors.Is(err, cannotRun) {
		t.Fatalf("runPlan = %v, want the error go gave", err)
	}
	if _, ok := errors.AsType[*BaselineFailedError](err); ok {
		t.Error("a command that could not run was reported as a failing baseline")
	}
}

// TestRunPlanReportsWhatTheSuiteSaid states a whole run without a suite of
// its own: the preamble says how much work there is, and each verdict
// lands in the report.
func TestRunPlanReportsWhatTheSuiteSaid(t *testing.T) {
	t.Parallel()
	_, p := runnerFixture(t, []bool{true, false, true})

	var status strings.Builder
	got, err := runPlan(p, Configuration{Verbose: true}, &status, func([]string) (string, error) {
		return "", nil
	}, func(overlay string) (Status, error) {
		if strings.Contains(readMutated(t, overlay), "K") {
			return StatusKilled, nil
		}
		return StatusMissed, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Killed != 2 || got.Missed != 1 {
		t.Errorf("killed %d, missed %d, want 2 and 1", got.Killed, got.Missed)
	}
	if !got.Baseline.Passed {
		t.Error("the report does not say the baseline passed")
	}
	if !strings.HasPrefix(status.String(), "3 mutants across 1 template (complexity 0), baseline ") {
		t.Errorf("status does not open with the preamble:\n%s", status.String())
	}
	lines := strings.Count(strings.TrimSpace(status.String()), "\n") + 1
	if lines != 4 {
		t.Errorf("status has %d lines, want the preamble and one per mutant:\n%s", lines, status.String())
	}
}
