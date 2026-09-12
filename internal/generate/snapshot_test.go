package generate

import (
	"bytes"
	"encoding/json"
	"flag"
	"go/token"
	"go/types"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/typestest"
)

var update = flag.Bool("update", false, "rewrite the want/ files of the snapshot archives in testdata")

// TestSnapshots generates the routes files for each archive in testdata
// and compares them with the archive's want/ files.
//
// An archive holds a case's inputs and what it generates:
//
//   - config.json overrides fields of testConfig, by field name.
//   - Go files are type checked, as example.com/server, against the stub
//     standard library in internal/typestest.
//   - .gohtml files are parsed into the templates variable, each under its
//     file name, as ParseFS would.
//   - want/ files are the expected output: one per generated file, named
//     for it, and want/log.txt and want/error.txt for what generation
//     logged and the error it returned.
//
// Run with -update to rewrite the want/ files from the generator, then
// read the diff: the snapshot says what the generator does, not what it
// should do. Whether generated code compiles and serves requests is the
// integration suite's job, under cmd/muxt/testdata.
func TestSnapshots(t *testing.T) {
	archives, err := filepath.Glob(filepath.Join("testdata", "*.txtar"))
	if err != nil {
		t.Fatal(err)
	}
	for _, archivePath := range archives {
		t.Run(strings.TrimSuffix(filepath.Base(archivePath), ".txtar"), func(t *testing.T) {
			archive, err := txtar.ParseFile(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			got := snapshot(t, archive)
			if *update {
				writeSnapshot(t, archivePath, archive, got)
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
}

// snapshot runs generation over an archive's inputs and returns the
// want/ files it produces, by name.
func snapshot(t *testing.T, archive *txtar.Archive) map[string]string {
	t.Helper()
	config := testConfig()
	goFiles := make(map[string]string)
	set := template.New("templates")
	var templateFiles []txtar.File
	for _, file := range archive.Files {
		switch {
		case strings.HasPrefix(file.Name, "want/"):
		case file.Name == "config.json":
			decoder := json.NewDecoder(bytes.NewReader(file.Data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&config); err != nil {
				t.Fatalf("config.json: %v", err)
			}
		case filepath.Ext(file.Name) == ".go":
			goFiles[file.Name] = string(file.Data)
		case filepath.Ext(file.Name) == ".gohtml":
			if _, err := set.New(file.Name).Parse(string(file.Data)); err != nil {
				t.Fatalf("%s: %v", file.Name, err)
			}
			templateFiles = append(templateFiles, file)
		default:
			t.Fatalf("archive file %s is not config.json, Go, .gohtml, or want/", file.Name)
		}
	}
	pkg, err := typestest.Check("example.com/server", goFiles)
	if err != nil {
		t.Fatal(err)
	}
	src := muxt.Source{
		Package: muxt.Package{Fset: typestest.FileSet, Types: pkg, Lookup: typestest.Lookup},
		Templates: []muxt.Templates{{
			Variable:     "templates",
			Set:          set,
			NamePosition: namePositions(templateFiles),
		}},
	}
	if config.ReceiverType != "" {
		obj := pkg.Scope().Lookup(config.ReceiverType)
		if obj == nil {
			t.Fatalf("config.json names receiver %s, which the Go files do not declare", config.ReceiverType)
		}
		src.Receiver = obj.Type().(*types.Named)
	}

	var logs strings.Builder
	files, err := TemplateRoutesFiles("/work", config, src, log.New(&logs, "", 0))
	got := make(map[string]string)
	for _, file := range files {
		got[strings.TrimPrefix(file.Path, "/work/")] = file.Content
	}
	if logs.Len() > 0 {
		got["log.txt"] = logs.String()
	}
	if err != nil {
		got["error.txt"] = errorText(err) + "\n"
	}
	return got
}

// errorText is what the command line prints for err: the multi-line
// form when there is one.
func errorText(err error) string {
	if multiLine, ok := err.(muxt.MultiLineError); ok {
		return multiLine.MultiLineError()
	}
	return err.Error()
}

// namePositions finds where each template name is written in the
// template files, the way the loader reports it: at the first byte
// inside the name's quotes.
func namePositions(files []txtar.File) func(string) (token.Position, bool) {
	return func(name string) (token.Position, bool) {
		for _, file := range files {
			text := string(file.Data)
			for _, keyword := range []string{"define", "block"} {
				needle := keyword + ` "` + name + `"`
				i := strings.Index(text, needle)
				if i < 0 {
					continue
				}
				offset := i + len(keyword) + len(` "`)
				line := 1 + strings.Count(text[:offset], "\n")
				column := offset - strings.LastIndex(text[:offset], "\n")
				return token.Position{Filename: file.Name, Offset: offset, Line: line, Column: column}, true
			}
		}
		return token.Position{}, false
	}
}

func writeSnapshot(t *testing.T, archivePath string, archive *txtar.Archive, got map[string]string) {
	t.Helper()
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
