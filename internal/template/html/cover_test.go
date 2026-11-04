package html

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/cover"
)

// useCoverExport replaces the process's coverage export for the duration of
// the test. Tests that call it must not run in parallel.
func useCoverExport(t *testing.T, export *coverExport) {
	t.Helper()
	saved := processCover
	processCover = export
	t.Cleanup(func() { processCover = saved })
}

func TestCoverExportFromEnv(t *testing.T) {
	if got := coverExportFor(""); got != nil {
		t.Errorf(`coverExportFor("") = %+v, want nil`, got)
	}
	dir := t.TempDir()
	got := coverExportFor(dir)
	if got == nil {
		t.Fatalf("coverExportFor(%q) = nil, want an export", dir)
	}
	if want := filepath.Join(dir, strconv.Itoa(os.Getpid())+".cov"); got.path != want {
		t.Errorf("coverExportFor(%q).path = %q, want %q", dir, got.path, want)
	}
}

func TestExecuteAppendsFirstHits(t *testing.T) {
	dir := t.TempDir()
	export := coverExportFor(dir)
	useCoverExport(t, export)

	ts := Must(New("page").Parse(`{{if .}}yes{{end}}END{{define "other"}}<b>{{.}}</b>{{end}}`))
	if err := ts.Execute(io.Discard, false); err != nil {
		t.Fatal(err)
	}
	const afterFirst = "mode: set\npage:1.1,1.9 1 1\npage:1.12,1.19 1 1\npage:1.19,1.22 1 1\n"
	if got := readFile(t, export.path); got != afterFirst {
		t.Fatalf("after the first Execute the profile is\n%s\nwant\n%s", got, afterFirst)
	}

	if err := ts.Execute(io.Discard, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, export.path); got != afterFirst {
		t.Errorf("executing the same nodes again changed the profile to\n%s\nwant\n%s", got, afterFirst)
	}

	if err := ts.ExecuteTemplate(io.Discard, "other", "x"); err != nil {
		t.Fatal(err)
	}
	if err := ts.Execute(io.Discard, true); err != nil {
		t.Fatal(err)
	}
	profiles, err := cover.ParseProfiles(export.path)
	if err != nil {
		t.Fatalf("cover.ParseProfiles(%s): %v", export.path, err)
	}
	if len(profiles) != 1 || profiles[0].FileName != "page" {
		t.Fatalf("profiles = %+v, want one for page", profiles)
	}
	var lines []string
	for _, b := range profiles[0].Blocks {
		if b.Count != 1 {
			t.Errorf("block %+v has count %d, want 1", b, b.Count)
		}
		lines = append(lines, strconv.Itoa(b.StartLine)+"."+strconv.Itoa(b.StartCol)+","+strconv.Itoa(b.EndLine)+"."+strconv.Itoa(b.EndCol))
	}
	// The if, its body, {{end}} and END; then the define's <b, >, {{.}} and </b>.
	want := []string{"1.1,1.9", "1.9,1.12", "1.12,1.19", "1.19,1.22", "1.40,1.42", "1.42,1.43", "1.43,1.48", "1.48,1.52"}
	if len(lines) != len(want) {
		t.Fatalf("blocks %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("block %d = %s, want %s (all: %v)", i, lines[i], want[i], lines)
		}
	}
}

func TestExecuteWithoutCoverExportWritesNothing(t *testing.T) {
	useCoverExport(t, nil)
	dir := t.TempDir()
	// The environment is read when the package initializes; setting it
	// later does not turn the export on.
	t.Setenv(coverEnv, dir)

	ts := Must(New("page").Parse(`<p>{{.}}</p>`))
	if err := ts.Execute(io.Discard, "x"); err != nil {
		t.Fatal(err)
	}
	if err := ts.ExecuteTemplate(io.Discard, "page", "x"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("%s has %d entries, want none", dir, len(entries))
	}
}

func TestExecuteDisablesCoverExportOnFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	export := coverExportFor(dir)
	var stderr strings.Builder
	export.stderr = &stderr
	useCoverExport(t, export)

	ts := Must(New("page").Parse(`<p>{{.}}</p>`))
	for i := range 2 {
		var out strings.Builder
		if err := ts.Execute(&out, "x"); err != nil {
			t.Errorf("Execute %d with the cover directory missing returned %v, want nil", i, err)
		}
		if got, want := out.String(), "<p>x</p>"; got != want {
			t.Errorf("Execute %d wrote %s, want %s", i, got, want)
		}
		if i == 0 {
			// Once disabled, the export does not try again even when it
			// could now succeed.
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := ts.ExecuteTemplate(io.Discard, "page", "y"); err != nil {
		t.Errorf("ExecuteTemplate after the export was disabled returned %v, want nil", err)
	}

	lines := strings.Split(strings.TrimSuffix(stderr.String(), "\n"), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "muxt: template coverage export disabled: ") {
		t.Errorf("stderr = %q, want one line starting %q", stderr.String(), "muxt: template coverage export disabled: ")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("%s has %d entries after the export was disabled, want none", dir, len(entries))
	}
}

func TestCoverExportAppendsToExistingProfile(t *testing.T) {
	export := coverExportFor(t.TempDir())
	useCoverExport(t, export)
	// A predecessor with the same pid (before syscall.Exec, say) already
	// wrote the header and a block.
	const before = "mode: set\nprev:1.1,1.2 1 1\n"
	if err := os.WriteFile(export.path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}

	ts := Must(New("page").Parse(`x`))
	if err := ts.Execute(io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, export.path), before+"page:1.1,1.2 1 1\n"; got != want {
		t.Errorf("profile is\n%s\nwant\n%s", got, want)
	}
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
