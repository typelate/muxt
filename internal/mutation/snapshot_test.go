package mutation

import (
	"encoding/json/v2"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/configjson"
	"github.com/typelate/muxt/internal/load/loadtest"
	"github.com/typelate/muxt/internal/muxt"
)

var update = flag.Bool("update", false, "rewrite the want/ files of the snapshot archives in testdata")

// TestSnapshots plans a dry run for each archive in
// testdata/test-template-mutations and compares the report with the
// archive's want/ files. The directory is the command, so one case runs
// with -run TestSnapshots/test-template-mutations/diff.
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
	const command = "test-template-mutations"
	stray, _ := filepath.Glob(filepath.Join("testdata", "*.txtar"))
	require.Empty(t, stray, "archives outside a command's directory; the mutation run's archives are in testdata/%s", command)
	archives, err := filepath.Glob(filepath.Join("testdata", command, "*.txtar"))
	require.NoError(t, err)
	require.NotEmpty(t, archives, "no archives in testdata/%s", command)
	t.Run(command, func(t *testing.T) {
		for _, archivePath := range archives {
			runSnapshot(t, archivePath)
		}
	})
}

// runSnapshot compares one archive's want/ files with the report planning
// it produces, or with -update rewrites them.
func runSnapshot(t *testing.T, archivePath string) {
	t.Helper()
	t.Run(strings.TrimSuffix(filepath.Base(archivePath), ".txtar"), func(t *testing.T) {
		archive, err := txtar.ParseFile(archivePath)
		require.NoError(t, err)
		config := configuration(t, archive)
		require.True(t, config.DryRun, "a snapshot plans a dry run; config.json must set DryRun")
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
			require.NoError(t, os.WriteFile(archivePath, txtar.Format(archive), 0o644))
			return
		}
		want := make(map[string]string)
		for _, file := range archive.Files {
			if name, ok := strings.CutPrefix(file.Name, "want/"); ok {
				want[name] = string(file.Data)
			}
		}
		for _, name := range []string{"error.txt", "report.txt"} {
			assert.Equal(t, want[name], got[name], "want/%s differs (run go test -run TestSnapshots -update to rewrite)", name)
		}
	})
}

// configuration reads the archive's config.json: the configuration to plan
// with.
func configuration(t *testing.T, archive *txtar.Archive) Configuration {
	t.Helper()
	for _, file := range archive.Files {
		if file.Name != "config.json" {
			continue
		}
		var config Configuration
		require.NoError(t, json.Unmarshal(file.Data, &config, configjson.Options()), "config.json")
		return config
	}
	require.Fail(t, "the archive has no config.json")
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
	report, err := runPlan(p, config, nil, func() (string, error) {
		require.Fail(t, "a dry run ran the baseline")
		return "", nil
	}, func(string) (Status, error) {
		require.Fail(t, "a dry run ran a mutant")
		return "", nil
	})
	if err != nil {
		return fail(err)
	}
	var out strings.Builder
	_, err = report.WriteTo(&out)
	require.NoError(t, err)
	got["report.txt"] = out.String()
	return got
}
