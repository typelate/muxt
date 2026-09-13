package mutation

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/load/loadtest"
	"github.com/typelate/muxt/internal/muxt"
)

var update = flag.Bool("update", false, "rewrite the want/ files of the snapshot archives in testdata")

// TestSnapshots plans a dry run for each archive in testdata and compares
// the report with the archive's want/ files.
//
// An archive holds a case's inputs and what the plan reports, and
// snapshots, in snapshots_test.go, the configuration it runs with:
//
//   - Go files and template files are written to a directory and loaded as
//     example.com/server by internal/load/loadtest: type checked against
//     the stub standard library, without the go command.
//   - Files under before/ are the package at the --diff revision, loaded
//     the same way, when the configuration names one.
//   - want/report.txt is the dry run's report and want/error.txt the error
//     planning returned.
//
// Run with -update to rewrite the want/ files, then read the diff. Whether
// the tests catch a mutant is decided by running them, which is what
// run_test.go and the integration suite do.
func TestSnapshots(t *testing.T) {
	archives, err := filepath.Glob(filepath.Join("testdata", "*.txtar"))
	if err != nil {
		t.Fatal(err)
	}
	for _, archivePath := range archives {
		name := strings.TrimSuffix(filepath.Base(archivePath), ".txtar")
		if !slices.ContainsFunc(snapshots, func(c snapshotCase) bool { return c.archive == name }) {
			t.Errorf("testdata/%s.txtar has no configuration in snapshots", name)
		}
	}
	for _, tt := range snapshots {
		t.Run(tt.archive, func(t *testing.T) {
			if !tt.config.DryRun {
				t.Fatal("a snapshot plans a dry run; the configuration must set DryRun")
			}
			archivePath := filepath.Join("testdata", tt.archive+".txtar")
			archive, err := txtar.ParseFile(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			got := dryRunSnapshot(t, tt.config, archive)
			if *update {
				files := slices.DeleteFunc(slices.Clone(archive.Files), func(file txtar.File) bool {
					return strings.HasPrefix(file.Name, "want/")
				})
				for _, name := range []string{"error.txt", "report.txt"} {
					if text, ok := got[name]; ok {
						files = append(files, txtar.File{Name: "want/" + name, Data: []byte(text)})
					}
				}
				archive.Files = files
				if err := os.WriteFile(archivePath, txtar.Format(archive), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want := make(map[string]string)
			for _, file := range archive.Files {
				if name, ok := strings.CutPrefix(file.Name, "want/"); ok {
					want[name] = string(file.Data)
				}
			}
			for _, name := range []string{"error.txt", "report.txt"} {
				if got[name] != want[name] {
					t.Errorf("want/%s differs (run go test -run TestSnapshots -update to rewrite):\n--- got\n%s\n--- want\n%s", name, got[name], want[name])
				}
			}
		})
	}
}

// dryRunSnapshot plans and dry runs an archive's package under config.
func dryRunSnapshot(t *testing.T, config Configuration, archive *txtar.Archive) map[string]string {
	t.Helper()
	current, before := make(map[string]string), make(map[string]string)
	for _, file := range archive.Files {
		switch {
		case strings.HasPrefix(file.Name, "want/"):
		case strings.HasPrefix(file.Name, "before/"):
			before[strings.TrimPrefix(file.Name, "before/")] = string(file.Data)
		default:
			current[file.Name] = string(file.Data)
		}
	}

	got := make(map[string]string)
	fail := func(err error) map[string]string {
		text := err.Error()
		if multiLine, ok := errors.AsType[muxt.MultiLineError](err); ok {
			text = multiLine.MultiLineError()
		}
		got["error.txt"] = text + "\n"
		return got
	}

	dir := t.TempDir()
	in, err := inputFrom(dir, loadtest.Package(t, dir, "example.com/server", current), config.TemplatesVariables)
	if err != nil {
		return fail(err)
	}

	var (
		previous  revision
		diffError string
	)
	if config.Diff != "" {
		beforeDir := t.TempDir()
		beforeInput, err := inputFrom(beforeDir, loadtest.Package(t, beforeDir, "example.com/server", before), config.TemplatesVariables)
		if err == nil {
			previous, err = revisionOf(beforeInput)
		}
		if err != nil {
			diffError = err.Error()
		}
	}

	p, err := planFrom(config, in, previous, diffError)
	if err != nil {
		return fail(err)
	}
	report, err := runPlan(p, config, nil, func([]string) (string, error) {
		t.Fatal("a dry run ran the baseline")
		return "", nil
	}, func(string) (Status, error) {
		t.Fatal("a dry run ran a mutant")
		return "", nil
	})
	if err != nil {
		return fail(err)
	}
	var out strings.Builder
	if _, err := report.WriteTo(&out); err != nil {
		t.Fatal(err)
	}
	got["report.txt"] = out.String()
	return got
}
