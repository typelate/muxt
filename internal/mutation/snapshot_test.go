package mutation

import (
	"encoding/json"
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
// An archive holds everything a case is: the configuration it plans with,
// its inputs, and what the plan reports.
//
//   - config.json is the configuration, as the command line named in the
//     archive's header parses into it. TestCommandLineConfigurations in
//     internal/cli states that parse; this states what planning does with
//     the result.
//   - Go files and template files are written to a directory and loaded as
//     example.com/server by internal/load/loadtest: type checked against
//     the official standard library, without loading the package graph.
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
	if len(archives) == 0 {
		t.Fatal("no archives in testdata")
	}
	for _, archivePath := range archives {
		t.Run(strings.TrimSuffix(filepath.Base(archivePath), ".txtar"), func(t *testing.T) {
			archive, err := txtar.ParseFile(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			config := configuration(t, archive)
			if !config.DryRun {
				t.Fatal("a snapshot plans a dry run; config.json must set DryRun")
			}
			got := dryRunSnapshot(t, config, archive)
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

// configuration reads the archive's config.json: the command it runs and
// the configuration to run it with.
func configuration(t *testing.T, archive *txtar.Archive) Configuration {
	t.Helper()
	for _, file := range archive.Files {
		if file.Name != "config.json" {
			continue
		}
		var read struct {
			Command string          `json:"command"`
			Config  json.RawMessage `json:"config"`
		}
		if err := json.Unmarshal(file.Data, &read); err != nil {
			t.Fatalf("config.json: %v", err)
		}
		if read.Command != "test-template-mutations" {
			t.Fatalf("config.json runs %q; this package snapshots the mutation run", read.Command)
		}
		var config Configuration
		if err := json.Unmarshal(read.Config, &config); err != nil {
			t.Fatalf("config.json: %v", err)
		}
		return config
	}
	t.Fatal("the archive has no config.json")
	return Configuration{}
}

// dryRunSnapshot plans and dry runs an archive's package under config.
func dryRunSnapshot(t *testing.T, config Configuration, archive *txtar.Archive) map[string]string {
	t.Helper()
	current, before := make(map[string]string), make(map[string]string)
	for _, file := range archive.Files {
		switch {
		case file.Name == "config.json":
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
