package generate_test

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/generate"
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

// TestSnapshots generates the routes files for each archive in testdata
// and compares them with the archive's want/ files.
//
// An archive holds a case's inputs and what it generates, and snapshots,
// in snapshots_test.go, the configuration it is generated with:
//
//   - Go files and .gohtml files are loaded as example.com/server by
//     internal/load/loadtest -- type checked against the stub standard
//     library, without the go command -- and hydrated by
//     load.GenerateSource, as muxt generate does. An archive with no
//     templates.go gets one declaring the templates variable over every
//     .gohtml file.
//   - want/ files are the expected output: one per generated file, named
//     for it, and want/log.txt and want/error.txt for what generation
//     logged and the error loading or generating returned.
//
// Paths in the output are relative to the directory the package was
// written to.
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
		name := strings.TrimSuffix(filepath.Base(archivePath), ".txtar")
		if !slices.ContainsFunc(snapshots, func(c snapshotCase) bool { return c.archive == name }) {
			t.Errorf("testdata/%s.txtar has no configuration in snapshots", name)
		}
	}
	for _, tt := range snapshots {
		t.Run(tt.archive, func(t *testing.T) {
			archivePath := filepath.Join("testdata", tt.archive+".txtar")
			archive, err := txtar.ParseFile(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			got := snapshot(t, tt.config, archive)
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

// snapshot loads an archive's package, generates from it, and returns the
// want/ files it produces, by name.
func snapshot(t *testing.T, config generate.RoutesFileConfiguration, archive *txtar.Archive) map[string]string {
	t.Helper()
	files := make(map[string]string)
	for _, file := range archive.Files {
		if strings.HasPrefix(file.Name, "want/") {
			continue
		}
		files[file.Name] = string(file.Data)
	}
	if _, declared := files["templates.go"]; !declared {
		files["templates.go"] = templatesGo
	}

	dir := t.TempDir()
	relative := func(text string) string { return strings.ReplaceAll(text, dir+string(filepath.Separator), "") }
	got := make(map[string]string)
	fail := func(err error) map[string]string {
		text := err.Error()
		if multiLine, ok := err.(muxt.MultiLineError); ok {
			text = multiLine.MultiLineError()
		}
		got["error.txt"] = relative(text) + "\n"
		return got
	}

	pkg, receiver, err := load.GenerateSource(dir, loadtest.Package(t, dir, "example.com/server", files), config)
	if err != nil {
		return fail(err)
	}
	var logs strings.Builder
	generated, err := generate.TemplateRoutesFiles(dir, config, pkg, receiver, log.New(&logs, "", 0))
	for _, file := range generated {
		got[relative(file.Path)] = file.Content
		for _, name := range unusedImports(t, file.Content) {
			t.Errorf("%s imports %s without using it", relative(file.Path), name)
		}
	}
	if logs.Len() > 0 {
		got["log.txt"] = relative(logs.String())
	}
	if err != nil {
		return fail(err)
	}
	return got
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

// unusedImports names the imports a generated file declares but does not
// refer to. Generated code that compiles has none; a file that did would
// be a generator registering an import for a declaration it did not write.
func unusedImports(t *testing.T, content string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", content, 0)
	if err != nil {
		t.Fatal(err)
	}
	referenced := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		if sel, ok := node.(*ast.SelectorExpr); ok {
			// A package name resolves to nothing in the file; a local
			// variable of the same name does.
			if id, ok := sel.X.(*ast.Ident); ok && id.Obj == nil {
				referenced[id.Name] = true
			}
		}
		return true
	})
	var unused []string
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		name := path.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if !referenced[name] {
			unused = append(unused, importPath)
		}
	}
	return unused
}
