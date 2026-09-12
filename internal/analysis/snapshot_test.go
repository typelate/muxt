package analysis

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"text/template/parse"

	"github.com/typelate/check"
	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/templateset"
	"github.com/typelate/muxt/internal/typestest"
)

var update = flag.Bool("update", false, "rewrite the want/ files of the snapshot archives in testdata")

// snapshotConfig is an archive's config.json.
type snapshotConfig struct {
	// Command is what runs: check, routes, callers or calls.
	Command string

	// Verbose is check's --verbose.
	Verbose bool

	// ReceiverType names the receiver type for routes.
	ReceiverType string

	// Match filters callers and calls by template name.
	Match []string
}

// TestSnapshots runs an analysis for each archive in testdata and
// compares what it reports with the archive's want/ files.
//
// An archive holds a case's inputs and what the analysis reports:
//
//   - config.json is a snapshotConfig.
//   - Go files are type checked, as example.com/server, against the stub
//     standard library in internal/typestest. The package declares the
//     templates variable, and its templates.ExecuteTemplate calls are the
//     ones checked.
//   - .gohtml files are parsed into the templates variable, each under its
//     file name, as ParseFS would.
//   - want/ files are what was reported: want/stdout.txt for a command's
//     output, want/log.txt for what check logged, and want/error.txt for
//     the error returned.
//
// Run with -update to rewrite the want/ files, then read the diff.
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
}

func snapshot(t *testing.T, archive *txtar.Archive) map[string]string {
	t.Helper()
	var config snapshotConfig
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
	checked, err := typestest.CheckSyntax("example.com/server", goFiles)
	if err != nil {
		t.Fatal(err)
	}
	pkg := muxt.Package{Fset: typestest.FileSet, Types: checked.Types, Lookup: typestest.Lookup}
	templates := memoryTemplates(t, checked, set, templateFiles)

	got := make(map[string]string)
	var stdout bytes.Buffer
	var runErr error
	switch config.Command {
	case "check":
		var logs strings.Builder
		var n int
		n, runErr = Check(CheckConfiguration{Verbose: config.Verbose, TemplatesVariables: []string{"templates"}}, log.New(&logs, "", 0), pkg, []templateset.Variable{templates})
		fmt.Fprintf(&stdout, "checked %d\n", n)
		if logs.Len() > 0 {
			got["log.txt"] = logs.String()
		}
	case "routes":
		src := muxt.Source{Package: pkg, Templates: []muxt.Templates{templates.Templates}}
		if config.ReceiverType != "" {
			src.Receiver = checked.Types.Scope().Lookup(config.ReceiverType).Type().(*types.Named)
		}
		var results []*Routes
		results, runErr = NewRoutes(src)
		for _, result := range results {
			writeTo(t, &stdout, result)
		}
	case "callers":
		var result *TemplateCallers
		result, runErr = NewTemplateCallers(TemplateCallersConfiguration{TemplatesVariable: "templates", FilterTemplates: patterns(t, config.Match)}, pkg, templates)
		if result != nil {
			writeTo(t, &stdout, result)
		}
	case "calls":
		var result *TemplateCalls
		result, runErr = NewTemplateCalls(TemplateCallsConfiguration{TemplatesVariable: "templates", FilterTemplates: patterns(t, config.Match)}, pkg, templates)
		if result != nil {
			writeTo(t, &stdout, result)
		}
	default:
		t.Fatalf("config.json Command %q is not check, routes, callers or calls", config.Command)
	}
	if stdout.Len() > 0 {
		got["stdout.txt"] = stdout.String()
	}
	if runErr != nil {
		text := runErr.Error()
		if multiLine, ok := runErr.(muxt.MultiLineError); ok {
			text = multiLine.MultiLineError()
		}
		got["error.txt"] = text + "\n"
	}
	return got
}

func writeTo(t *testing.T, w io.Writer, result io.WriterTo) {
	t.Helper()
	if _, err := result.WriteTo(w); err != nil {
		t.Fatal(err)
	}
}

func patterns(t *testing.T, sources []string) []*regexp.Regexp {
	t.Helper()
	var list []*regexp.Regexp
	for _, source := range sources {
		list = append(list, regexp.MustCompile(source))
	}
	return list
}

// memoryTemplates wires the templates variable the way internal/load
// does, from a template set parsed in memory: the checked package's
// templates.ExecuteTemplate calls, and trees and definitions found in the
// set.
func memoryTemplates(t *testing.T, checked *typestest.Checked, set *template.Template, files []txtar.File) templateset.Variable {
	t.Helper()
	variable := checked.Types.Scope().Lookup("templates")
	if variable == nil {
		t.Fatal("the Go files declare no templates variable")
	}
	namePosition := func(name string) (token.Position, bool) {
		for _, file := range files {
			text := string(file.Data)
			for _, keyword := range []string{"define", "block"} {
				i := strings.Index(text, keyword+` "`+name+`"`)
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
	findTree := func(name string) (*parse.Tree, bool) {
		found := set.Lookup(name)
		if found == nil || found.Tree == nil {
			return nil, false
		}
		return found.Tree, true
	}
	findDefinition := func(name string) (check.Definition, bool) {
		pos, ok := namePosition(name)
		if !ok {
			return check.Definition{}, false
		}
		tree, _ := findTree(name)
		// The name span includes its quotes.
		pos.Offset--
		pos.Column--
		return check.Definition{Name: name, TemplateName: check.Span{Position: pos, Length: len(name) + 2}, Tree: tree}, true
	}

	var calls []check.ExecuteTemplateCall
	for _, file := range checked.Syntax {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "ExecuteTemplate" {
				return true
			}
			receiver, ok := sel.X.(*ast.Ident)
			if !ok || checked.Info.Uses[receiver] != variable {
				return true
			}
			literal, ok := call.Args[1].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			definition, _ := findDefinition(name)
			calls = append(calls, check.ExecuteTemplateCall{
				Call:         call,
				TemplateName: name,
				DataType:     checked.Info.TypeOf(call.Args[2]),
				Definition:   definition,
			})
			return true
		})
	}

	return templateset.Variable{
		Templates: muxt.Templates{
			Variable:     "templates",
			Set:          set,
			NamePosition: namePosition,
		},
		Trees:       check.FindTreeFunc(findTree),
		Definitions: check.FindDefinitionFunc(findDefinition),
		Functions:   check.DefaultFunctions(checked.Types),
		Calls:       calls,
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
