package analysis_test

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/configjson"
	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/load/loadtest"
	"github.com/typelate/muxt/internal/muxt"
)

var update = flag.Bool("update", false, "rewrite the want/ files of the snapshot archives in testdata")

// templatesGo declares the templates variable for an archive that does not
// declare its own: every template file, parsed as ParseFS would.
const templatesGo = `package server

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templateFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "*.gohtml"))
`

// TestSnapshots runs an analysis for each archive in testdata and
// compares what it reports with the archive's want/ files.
//
// An archive's directory names the command it runs -- check, list-routes,
// list-template-callers or list-template-calls -- so one case runs with
// -run TestSnapshots/list-template-calls/calls. The archive holds the rest
// of the case: the configuration it runs with, its inputs, and what it
// reports.
//
//   - config.json is the configuration, as the command line named in the
//     archive's header parses into it. TestCommandLineConfigurations in
//     internal/cli states that parse; this states what the command does
//     with the result.
//   - Go files and .gohtml files are loaded as example.com/server by
//     internal/load/loadtest -- type checked against the official standard
//     library, without loading the package graph -- and hydrated by
//     internal/load, as the command does. An archive with no templates.go
//     gets one declaring the templates variable over every .gohtml file.
//   - want/ files are what was reported: want/stdout.txt for what a listing
//     writes, want/checked.txt for the number of ExecuteTemplate calls
//     check returns, want/log.txt for what check logged, and want/error.txt
//     for the error returned.
//
// Paths in the output are relative to the directory the package was
// written to. Run with -update to rewrite the want/ files, then read the
// diff.
func TestSnapshots(t *testing.T) {
	if stray, _ := filepath.Glob(filepath.Join("testdata", "*.txtar")); len(stray) > 0 {
		t.Fatalf("%s is not in a command's directory", stray[0])
	}
	directories, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range directories {
		if !directory.IsDir() {
			continue
		}
		command := directory.Name()
		newConfiguration, ok := commands[command]
		if !ok {
			t.Errorf("testdata/%s names no command this package snapshots", command)
			continue
		}
		archives, err := filepath.Glob(filepath.Join("testdata", command, "*.txtar"))
		if err != nil {
			t.Fatal(err)
		}
		t.Run(command, func(t *testing.T) {
			for _, archivePath := range archives {
				runSnapshot(t, archivePath, newConfiguration)
			}
		})
	}
}

// commands are the directories in testdata, each named for the command its
// archives run, with the configuration that command's config.json holds.
var commands = map[string]func() any{
	"check":                 func() any { return new(analysis.CheckConfiguration) },
	"list-routes":           func() any { return new(analysis.DefinitionsConfiguration) },
	"list-template-callers": func() any { return new(analysis.TemplateCallersConfiguration) },
	"list-template-calls":   func() any { return new(analysis.TemplateCallsConfiguration) },
}

// runSnapshot compares one archive's want/ files with what its command
// reports, or with -update rewrites them.
func runSnapshot(t *testing.T, archivePath string, newConfiguration func() any) {
	t.Helper()
	t.Run(strings.TrimSuffix(filepath.Base(archivePath), ".txtar"), func(t *testing.T) {
		archive, err := txtar.ParseFile(archivePath)
		if err != nil {
			t.Fatal(err)
		}
		got := snapshot(t, configuration(t, archive, newConfiguration()), archive)
		if *update {
			files := slices.DeleteFunc(slices.Clone(archive.Files), func(file txtar.File) bool {
				return strings.HasPrefix(file.Name, "want/")
			})
			for _, name := range sortedKeys(got) {
				files = append(files, txtar.File{Name: "want/" + name, Data: []byte(got[name])})
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
		for _, name := range sortedKeys(got, want) {
			if got[name] != want[name] {
				t.Errorf("want/%s differs (run go test -run TestSnapshots -update to rewrite):\n--- got\n%s\n--- want\n%s", name, got[name], want[name])
			}
		}
	})
}

// configuration reads the archive's config.json into config, a pointer to
// the configuration its directory's command holds, and returns the
// configuration by value, as a command holds it.
func configuration(t *testing.T, archive *txtar.Archive, config any) any {
	t.Helper()
	for _, file := range archive.Files {
		if file.Name != "config.json" {
			continue
		}
		if err := json.Unmarshal(file.Data, config, configjson.Options()); err != nil {
			t.Fatalf("config.json: %v", err)
		}
		return reflect.ValueOf(config).Elem().Interface()
	}
	t.Fatal("the archive has no config.json")
	return nil
}

func snapshot(t *testing.T, config any, archive *txtar.Archive) map[string]string {
	t.Helper()
	files := make(map[string]string)
	for _, file := range archive.Files {
		if strings.HasPrefix(file.Name, "want/") || file.Name == "config.json" {
			continue
		}
		files[file.Name] = string(file.Data)
	}
	if _, declared := files["templates.go"]; !declared {
		files["templates.go"] = templatesGo
	}
	dir := t.TempDir()
	pl := loadtest.Package(t, dir, "example.com/server", files)
	relative := func(text string) string { return strings.ReplaceAll(text, dir+string(filepath.Separator), "") }

	got := make(map[string]string)
	var stdout bytes.Buffer
	var runErr error
	switch config := config.(type) {
	case analysis.CheckConfiguration:
		pkg, err := load.Package(dir, pl, config.TemplatesVariables)
		if err != nil {
			runErr = err
			break
		}
		var logs strings.Builder
		var n int
		n, runErr = analysis.Check(config, log.New(&logs, "", 0), pkg)
		got["checked.txt"] = fmt.Sprintf("%d\n", n)
		if logs.Len() > 0 {
			got["log.txt"] = relative(logs.String())
		}
	case analysis.DefinitionsConfiguration:
		pkg, receiver, err := load.RoutesSource(dir, pl, config)
		if err != nil {
			runErr = err
			break
		}
		var results []*analysis.Routes
		results, runErr = analysis.NewRoutes(pkg, receiver)
		for _, result := range results {
			writeTo(t, &stdout, result)
		}
	case analysis.TemplateCallersConfiguration:
		pkg, err := load.Package(dir, pl, config.TemplatesVariables)
		if err != nil {
			runErr = err
			break
		}
		var result *analysis.TemplateCallers
		if result, runErr = analysis.NewTemplateCallers(config, pkg); result != nil {
			writeTo(t, &stdout, result)
		}
	case analysis.TemplateCallsConfiguration:
		pkg, err := load.Package(dir, pl, config.TemplatesVariables)
		if err != nil {
			runErr = err
			break
		}
		var result *analysis.TemplateCalls
		if result, runErr = analysis.NewTemplateCalls(config, pkg); result != nil {
			writeTo(t, &stdout, result)
		}
	default:
		t.Fatalf("no analysis runs with a %T", config)
	}
	if stdout.Len() > 0 {
		got["stdout.txt"] = relative(stdout.String())
	}
	if runErr != nil {
		text := runErr.Error()
		if multiLine, ok := errors.AsType[muxt.MultiLineError](runErr); ok {
			text = multiLine.MultiLineError()
		}
		got["error.txt"] = relative(text) + "\n"
	}
	return got
}

func writeTo(t *testing.T, w io.Writer, result io.WriterTo) {
	t.Helper()
	if _, err := result.WriteTo(w); err != nil {
		t.Fatal(err)
	}
}

func sortedKeys(maps ...map[string]string) []string {
	var keys []string
	for _, m := range maps {
		for key := range m {
			if !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	slices.Sort(keys)
	return keys
}
