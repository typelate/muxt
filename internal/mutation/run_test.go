package mutation

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// module writes a Go module into a temporary directory and returns it. The
// go command runs without a workspace from the invoking environment.
func module(t *testing.T, files map[string]string) string {
	t.Helper()
	t.Setenv("GOWORK", "off")
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const goMod = "module server\n\ngo 1.24\n"

// greetingGo holds its template as a raw string literal.
const greetingGo = `package server

import (
	"html/template"
	"io"
)

var templates = template.Must(template.New("greeting").Parse(` + "`" + `Hello, {{.Name}}!{{if .Loud}} !!!{{end}}` + "`" + `))

type Greeting struct {
	Name string
	Loud bool
}

func Render(w io.Writer, greeting Greeting) error {
	return templates.ExecuteTemplate(w, "greeting", greeting)
}
`

// greetingTest checks the name and never renders with Loud set, so it
// catches the mutants that change the name or force the exclamation, and
// misses the one that removes it.
const greetingTest = `package server

import (
	"strings"
	"testing"
)

func TestGreeting(t *testing.T) {
	var buf strings.Builder
	if err := Render(&buf, Greeting{Name: "World"}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "Hello, World!"; got != want {
		t.Errorf("greeting = %q, want %q", got, want)
	}
}
`

func greetingConfig() Configuration {
	return Configuration{TemplatesVariables: []string{"templates"}, Seed: 1, SeedSet: true}
}

// TestRun states a whole run over a template written as a Go string
// literal: the unmutated tests pass, each mutant is written back into the
// literal and tested, and every verdict lands in the report while progress
// streams to status.
func TestRun(t *testing.T) {
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
