package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/mutation"
)

// fieldkit muxt.json files, one per situation of the selection table.
const (
	// fieldkitEntries lists three packages and not the module's root.
	fieldkitEntries = `{
	"package_defaults": {"use_receiver_type": "Server", "use_receiver_type_package": "example.com/fieldkit/types", "output_htmx": true},
	"packages": {
		"example.com/fieldkit/contacts": {},
		"example.com/fieldkit/inventory": {"use_receiver_type": "Catalog", "use_templates_variable": ["catalogTemplates"]},
		"example.com/fieldkit/tasks": {"output_file": "tasks_routes.go", "output_routes_func_with_logger_param": true}
	}
}`
	// fieldkitWithRoot also lists the module's root, the file's own directory.
	fieldkitWithRoot = `{
	"package_defaults": {"use_receiver_type": "Server", "output_htmx": true},
	"packages": {
		"example.com/fieldkit": {},
		"example.com/fieldkit/contacts": {},
		"example.com/fieldkit/inventory": {"use_receiver_type": "Catalog", "use_templates_variable": ["catalogTemplates"]},
		"example.com/fieldkit/tasks": {"output_file": "tasks_routes.go", "output_routes_func_with_logger_param": true}
	}
}`
	// fieldkitNoReceiver lists contacts with no receiver type anywhere, so
	// generate def may fill one from its flag.
	fieldkitNoReceiver = `{"packages": {"example.com/fieldkit/contacts": {}}}`
	// fieldkitEmpty has package_defaults and no entry.
	fieldkitEmpty = `{"package_defaults": {"use_receiver_type": "Server"}, "packages": {}}`
	// fieldkitStale lists a package whose directory does not exist.
	fieldkitStale = `{
	"package_defaults": {"use_receiver_type": "Server"},
	"packages": {
		"example.com/fieldkit/contacts": {},
		"example.com/fieldkit/ghost": {}
	}
}`
)

// defRoute is the template name generate def is given.
const defRoute = "GET /things/{id} GetThing(ctx, id)"

// newFieldkit writes a module example.com/fieldkit with the package
// directories contacts, inventory, tasks, extra and fragments, and a muxt.json
// holding muxtJSON unless that is empty. It returns the module's directory.
func newFieldkit(t *testing.T, muxtJSON string) string {
	t.Helper()
	base := t.TempDir()
	writeTestFile(t, base, "go.mod", "module example.com/fieldkit\n")
	for _, name := range []string{"contacts", "inventory", "tasks", "extra", "fragments"} {
		require.NoError(t, os.Mkdir(filepath.Join(base, name), 0o755), "creating %s", name)
	}
	if muxtJSON != "" {
		writeMuxtJSON(t, base, muxtJSON)
	}
	return base
}

// selectionRecord is what every command was asked to do on one command line:
// one line per job a runner saw, or the error the command line ended in. Paths
// under the module are written relative to it, so a record reads the same in
// any temporary directory.
type selectionRecord struct {
	base string
	out  strings.Builder
}

func (r *selectionRecord) rel(path string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(r.base, path)
	if err != nil || (!filepath.IsLocal(rel) && rel != ".") {
		return path
	}
	return rel
}

func (r *selectionRecord) rels(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = r.rel(p)
	}
	return out
}

func (r *selectionRecord) line(format string, args ...any) {
	_, _ = fmt.Fprintf(&r.out, format+"\n", args...)
}

// recordSelection runs the command line in wd with runners that record the
// jobs they are given, and a loader that records the directories it is asked
// for, so nothing is loaded. It returns the record and the error the command
// line ended in.
func recordSelection(base, wd string, args ...string) (string, error) {
	r := &selectionRecord{base: base}
	err := commands(wd, args, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), new(bytes.Buffer), new(bytes.Buffer), runners{
		generate: func(_ *cobra.Command, jobs []generateJob) error {
			for _, job := range jobs {
				r.line("generate key=%q file=%q dir=%q receiver=%q receiver-package=%q templates=%v output=%q htmx=%t logger=%t verbose=%t",
					job.Key, r.rel(job.File), r.rel(job.Dir), job.Config.ReceiverType, job.Config.ReceiverPackage, job.Config.TemplatesVariables,
					job.Config.OutputFileName, job.Config.OutputHTMX, job.Config.Logger, job.Config.Verbose)
			}
			return nil
		},
		routes: func(_ *cobra.Command, jobs []routesJob) error {
			for _, job := range jobs {
				var sources []string
				for _, group := range job.Report {
					sources = append(sources, fmt.Sprintf("%s@%s", group.Source, r.rel(group.File)))
				}
				r.line("routes key=%q file=%q dir=%q receiver=%q receiver-package=%q templates=%v htmx=%t verbose=%t report=%v",
					job.Key, r.rel(job.File), r.rel(job.Dir), job.Config.ReceiverType, job.Config.ReceiverPackage, job.Config.TemplatesVariables,
					job.Config.OutputHTMX, job.Config.Verbose, sources)
			}
			return nil
		},
		check: func(_ *cobra.Command, dir string, c analysis.CheckConfiguration) error {
			r.line("check dir=%q templates=%v allow-unused=%t", r.rel(dir), c.TemplatesVariables, c.AllowUnused)
			return nil
		},
		calls: func(_ *cobra.Command, dir string, c analysis.TemplateCallsConfiguration) error {
			r.line("calls dir=%q templates=%v", r.rel(dir), c.TemplatesVariables)
			return nil
		},
		callers: func(_ *cobra.Command, dir string, c analysis.TemplateCallersConfiguration) error {
			r.line("callers dir=%q templates=%v", r.rel(dir), c.TemplatesVariables)
			return nil
		},
		mutations: func(_ *cobra.Command, dir string, c mutation.Configuration) error {
			r.line("mutations dir=%q templates=%v", r.rel(dir), c.TemplatesVariables)
			return nil
		},
		// generate def reaches its package through this loader, and only
		// after it chose the configuration; the receiver package, which
		// package_defaults and entries can set, is what the chosen
		// configuration shows. The receiver type, the templates variables
		// and the method stub need a loaded package, and stay pinned by the
		// generate def tests and scripts. The empty load ends in "no
		// Go package found", where a real load ends in the package's own
		// error; that text says nothing about selection.
		load: func(root string, dirs, patterns []string) (*token.FileSet, []*packages.Package, error) {
			r.line("load root=%q dirs=%q patterns=%q", r.rel(root), r.rels(dirs), patterns)
			return token.NewFileSet(), nil, nil
		},
	})
	if err != nil {
		r.line("error: %s", strings.ReplaceAll(err.Error(), base, "$BASE"))
	}
	return r.out.String(), err
}

// selectionCell is one command line in one directory of one fieldkit.
type selectionCell struct {
	name    string
	muxJSON string
	// wd is the directory the command runs in, relative to the module.
	wd string
	// dashC, when not empty, runs the command from a directory outside the
	// module with -C pointing here, relative to the module.
	dashC string
	args  []string
}

// selectionCommands are the seven commands whose runners record, each with the
// arguments that make it run flagless.
func selectionCommands() []selectionCell {
	return []selectionCell{
		{name: "generate", args: []string{"generate", "-v"}},
		{name: "muxt", args: nil},
		{name: "check", args: []string{"check"}},
		{name: "calls", args: []string{"list-template-calls"}},
		{name: "callers", args: []string{"list-template-callers"}},
		{name: "def", args: []string{"generate", "def", defRoute}},
		{name: "mutations", args: []string{"test-template-mutations", "--dry-run"}},
	}
}

// selectionSituation is a directory, a muxt.json and what a command line
// adds in it. flags holds the setting flags to give each command by name; a
// command not in it is run flagless.
type selectionSituation struct {
	name    string
	muxJSON string
	wd      string
	dashC   string
	flags   map[string][]string
}

func selectionCells() []selectionCell {
	flagsInEntry := map[string][]string{
		"generate":  {"--use-receiver-type=Server", "--use-templates-variable=templates"},
		"muxt":      {"--use-receiver-type=Server", "--use-templates-variable=templates"},
		"check":     {"--use-templates-variable=templates"},
		"calls":     {"--use-templates-variable=templates"},
		"callers":   {"--use-templates-variable=templates"},
		"def":       {"--use-receiver-type=Server"},
		"mutations": {"--use-templates-variable=templates"},
	}
	flagsOutside := map[string][]string{
		"generate":  {"--use-receiver-type=Server"},
		"muxt":      {"--use-templates-variable=templates"},
		"check":     {"--use-templates-variable=pages"},
		"calls":     {"--use-templates-variable=pages"},
		"callers":   {"--use-templates-variable=pages"},
		"def":       {"--use-receiver-type=Server"},
		"mutations": {"--use-templates-variable=pages"},
	}
	var cells []selectionCell
	for _, s := range []selectionSituation{
		{name: "no muxt.json, root", wd: "."},
		{name: "no muxt.json, package", wd: "contacts"},
		{name: "entries, file directory", muxJSON: fieldkitEntries, wd: "."},
		{name: "entries and root entry, file directory", muxJSON: fieldkitWithRoot, wd: "."},
		{name: "entries, contacts", muxJSON: fieldkitEntries, wd: "contacts"},
		{name: "entries, inventory", muxJSON: fieldkitEntries, wd: "inventory"},
		{name: "entries, tasks", muxJSON: fieldkitEntries, wd: "tasks"},
		{name: "entries, no entry for extra", muxJSON: fieldkitEntries, wd: "extra"},
		{name: "setting flags, contacts has an entry", muxJSON: fieldkitEntries, wd: "contacts", flags: flagsInEntry},
		{name: "setting flags, entry names no receiver", muxJSON: fieldkitNoReceiver, wd: "contacts", flags: flagsInEntry},
		{name: "setting flags, file directory is not an entry", muxJSON: fieldkitEntries, wd: ".", flags: flagsOutside},
		{name: "setting flags, extra has no entry", muxJSON: fieldkitEntries, wd: "extra", flags: flagsOutside},
		{name: "empty packages, file directory", muxJSON: fieldkitEmpty, wd: "."},
		{name: "empty packages, contacts", muxJSON: fieldkitEmpty, wd: "contacts"},
		{name: "stale entry, file directory", muxJSON: fieldkitStale, wd: "."},
		{name: "stale entry, contacts", muxJSON: fieldkitStale, wd: "contacts"},
		{name: "listed directory that does not exist", muxJSON: fieldkitStale, wd: "ghost"},
		{name: "listed directory that does not exist, flags", muxJSON: fieldkitStale, wd: "ghost", flags: flagsOutside},
		{name: "-C module", muxJSON: fieldkitEntries, dashC: "."},
		{name: "-C inventory", muxJSON: fieldkitEntries, dashC: "inventory"},
		{name: "-C extra", muxJSON: fieldkitEntries, dashC: "extra"},
		{name: "-C directory that does not exist, below the file", muxJSON: fieldkitEntries, dashC: "typo"},
		{name: "-C directory that does not exist, no muxt.json", dashC: "typo"},
		{name: "-C directory that does not exist, flag", muxJSON: fieldkitEntries, dashC: "typo", flags: flagsOutside},
	} {
		for _, c := range selectionCommands() {
			args := append(append([]string(nil), c.args...), s.flags[c.name]...)
			cells = append(cells, selectionCell{name: s.name + ": " + c.name + " " + strings.Join(args, " "), muxJSON: s.muxJSON, wd: s.wd, dashC: s.dashC, args: args})
		}
	}

	// Directory arguments, from the module and from other directories.
	for _, c := range []selectionCell{
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"check", "inventory"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"list-template-calls", "inventory"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"list-template-callers", "inventory"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"check", "extra"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"check", "typo"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"check", "./main.go"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"test-template-mutations", "--dry-run", "inventory"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"test-template-mutations", "--dry-run", "extra"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"test-template-mutations", "--dry-run", "inventory", "--use-templates-variable=catalogTemplates"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"check", "inventory", "--use-templates-variable=catalogTemplates"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"check", "./..."}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"check", "inventory", "contacts"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"contacts"}},
		{muxJSON: fieldkitEntries, wd: ".", args: []string{"generate", "contacts"}},
		{muxJSON: fieldkitStale, wd: ".", args: []string{"check", "contacts"}},
		{muxJSON: fieldkitStale, wd: ".", args: []string{"list-template-calls", "contacts"}},
		{muxJSON: fieldkitEntries, dashC: ".", args: []string{"check", "inventory"}},
		{muxJSON: fieldkitEntries, dashC: "inventory", args: []string{"check", "."}},
		{muxJSON: fieldkitEntries, dashC: "inventory", args: []string{"check", "../extra"}},
		{muxJSON: fieldkitEntries, dashC: "inventory", args: []string{"list-template-calls", "../extra"}},
		{muxJSON: fieldkitEntries, dashC: "inventory", args: []string{"test-template-mutations", "--dry-run", "../extra"}},
	} {
		c.name = c.wd + c.dashC + ": " + strings.Join(c.args, " ")
		if c.dashC != "" {
			c.name = "-C " + c.name
		}
		cells = append(cells, c)
	}
	return cells
}

// runSelectionCell runs a cell in a fresh fieldkit and returns its record.
func runSelectionCell(t *testing.T, c selectionCell) string {
	t.Helper()
	base := newFieldkit(t, c.muxJSON)
	wd, args := base, c.args
	switch {
	case c.dashC != "":
		wd = t.TempDir()
		args = append([]string{"-C", filepath.Join(base, c.dashC)}, c.args...)
	case c.wd != "":
		wd = filepath.Join(base, c.wd)
	}
	got, _ := recordSelection(base, wd, args...)
	return got
}

// TestEveryCommandSelects pins, for every command that chooses packages and
// settings from a directory, what it chooses on each command line of the
// fieldkit module, quirks included. It is the gate for moving every command
// onto one selection: a change that is meant to alter a cell changes that
// cell in testdata/select_matrix.txt, in the commit that alters it, and the
// diff says which.
//
// The runners record, so a cell is the jobs the command would run and the
// configuration of each, or the error the command line ended in. What a run
// does with a loaded package is not in a cell; the load stands in for it.
// Rewrite the file with UPDATE_SELECT_MATRIX=1 and read the diff.
func TestEveryCommandSelects(t *testing.T) {
	const goldenPath = "testdata/select_matrix.txt"
	cells := selectionCells()
	got := make([]string, len(cells))
	for i, c := range cells {
		got[i] = runSelectionCell(t, c)
	}
	if os.Getenv("UPDATE_SELECT_MATRIX") != "" {
		var sb strings.Builder
		for i, c := range cells {
			fmt.Fprintf(&sb, "== %s\n%s", c.name, got[i])
		}
		require.NoError(t, os.WriteFile(goldenPath, []byte(sb.String()), 0o644), "writing %s", goldenPath)
	}
	file, err := os.ReadFile(goldenPath)
	require.NoError(t, err, "reading %s", goldenPath)
	want := map[string]string{}
	for _, block := range strings.Split("\n"+string(file), "\n== ")[1:] {
		name, record, _ := strings.Cut(block, "\n")
		want[name] = record
		if !strings.HasSuffix(record, "\n") {
			want[name] += "\n"
		}
	}
	require.Len(t, want, len(cells), "cells in %s", goldenPath)
	for i, c := range cells {
		t.Run(c.name, func(t *testing.T) {
			record, ok := want[c.name]
			require.True(t, ok, "no cell %q in %s", c.name, goldenPath)
			assert.Equal(t, strings.TrimSuffix(record, "\n"), strings.TrimSuffix(got[i], "\n"), "muxt %s", strings.Join(c.args, " "))
		})
	}
}

// TestSettingFlagsCount states which flags a configured directory rejects,
// which is what makes a flag a setting: -v, --format, --match and
// --allow-unused never count, a deprecated spelling counts as its flag, and a
// flag only generate or bare muxt registers counts for those alone.
func TestSettingFlagsCount(t *testing.T) {
	for _, tt := range []struct {
		args      []string
		wantFlags []string // nil: the configured directory accepts the line
	}{
		{[]string{"generate", "-v"}, nil},
		{[]string{"generate", "--verbose"}, nil},
		{[]string{"generate", "--use-receiver-type=Server"}, []string{"use-receiver-type"}},
		{[]string{"generate", "--receiver-type=Server"}, []string{"receiver-type"}},
		{[]string{"generate", "--output-htmx"}, []string{"output-htmx"}},
		{[]string{"generate", "--output-htmx-helpers"}, []string{"output-htmx-helpers"}},
		{[]string{"generate", "--find-templates-variable=pages"}, []string{"find-templates-variable"}},
		{[]string{"generate", "--logger"}, []string{"logger"}},
		{[]string{"generate", "--output-file=x.go", "--output-htmx"}, []string{"output-file", "output-htmx"}},
		{[]string{"-v"}, nil},
		{[]string{"--format", "json"}, nil},
		{[]string{"--use-receiver-type-package=example.com/fieldkit/types"}, []string{"use-receiver-type-package"}},
		{[]string{"--use-receiver-type=Server"}, []string{"use-receiver-type"}},
		{[]string{"--use-templates-variable=pages"}, []string{"use-templates-variable"}},
		{[]string{"--templates-variable=pages"}, []string{"templates-variable"}},
		{[]string{"check", "-v"}, nil},
		{[]string{"check", "--allow-unused"}, nil},
		{[]string{"check", "--receiver-type=Server"}, nil},
		{[]string{"check", "--use-templates-variable=pages"}, []string{"use-templates-variable"}},
		{[]string{"check", "--templates-variable=pages"}, []string{"templates-variable"}},
		{[]string{"list-template-calls", "--format", "json"}, nil},
		{[]string{"list-template-calls", "--match", "x"}, nil},
		{[]string{"list-template-calls", "-v"}, nil},
		{[]string{"list-template-calls", "--use-templates-variable=pages"}, []string{"use-templates-variable"}},
		{[]string{"list-template-callers", "--format", "json"}, nil},
		{[]string{"list-template-callers", "--match", "x"}, nil},
		{[]string{"list-template-callers", "-v"}, nil},
		{[]string{"list-template-callers", "--templates-variable=pages"}, []string{"templates-variable"}},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			base := newFieldkit(t, fieldkitEntries)
			_, err := recordSelection(base, filepath.Join(base, "contacts"), tt.args...)

			if tt.wantFlags == nil {
				assert.NoError(t, err, "muxt %s in a configured directory", strings.Join(tt.args, " "))
				return
			}
			configured, ok := errors.AsType[*composite.ConfiguredDirectoryError](err)
			if assert.True(t, ok, "muxt %s error = %v, want *composite.ConfiguredDirectoryError", strings.Join(tt.args, " "), err) {
				assert.Equal(t, tt.wantFlags, configured.Flags, "muxt %s counted flags", strings.Join(tt.args, " "))
			}
		})
	}
}

// TestEveryCommandSelectsForTheModuleListing states which muxt.json entries
// explore-module and generate-fake-server read: only whether a package's
// directory has one, never whether it is valid. Both run the go command for
// the module, and neither loads a package here: generate-fake-server is given
// a directory without a generated file, so it stops after the listing.
func TestEveryCommandSelectsForTheModuleListing(t *testing.T) {
	// tasks has a header and an entry that breaks a rule; ghost has an entry
	// and no directory.
	const muxtJSON = `{
	"packages": {
		"example.com/fieldkit/contacts": {},
		"example.com/fieldkit/tasks": {"output_routes_func": "1Routes"},
		"example.com/fieldkit/ghost": {}
	}
}`
	base := newFieldkit(t, muxtJSON)
	for _, name := range []string{"contacts", "extra", "tasks"} {
		writeTestFile(t, filepath.Join(base, name), "template_routes.go",
			"// Code generated by muxt generate --use-receiver-type=Server. DO NOT EDIT.\n\npackage "+name+"\n")
	}

	// explore-module takes no directory: an argument is ignored.
	for _, args := range [][]string{{"explore-module", "--format", "json"}, {"explore-module", "--format", "json", "contacts"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			testExploreModule(t, base, args)
		})
	}

	t.Run("generate-fake-server", func(t *testing.T) {
		err := commands(base, []string{"generate-fake-server", "fragments"}, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), new(bytes.Buffer), new(bytes.Buffer), runners{})
		require.Error(t, err, "generate-fake-server in a directory with no generated file")
		assert.Equal(t, "no muxt-generated package found at "+filepath.Join(base, "fragments"), err.Error(), "the listing succeeded: the broken entry is not read")
	})
}

func testExploreModule(t *testing.T, base string, args []string) {
	t.Helper()
	var out bytes.Buffer
	err := commands(base, args, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(""), &out, new(bytes.Buffer), runners{})
	require.NoError(t, err, "explore-module with an entry that breaks a rule and one with no directory")

	var module struct {
		Packages []struct {
			Dir      string
			Commands struct{ Generate string }
		}
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &module), "decoding %s", out.String())
	got := map[string]string{}
	for _, p := range module.Packages {
		rel, err := filepath.Rel(base, p.Dir)
		require.NoError(t, err)
		got[rel] = p.Commands.Generate
	}
	assert.Equal(t, map[string]string{
		"contacts": "muxt -C " + filepath.Join(base, "contacts") + " generate",
		"extra":    "muxt -C " + filepath.Join(base, "extra") + " generate --use-receiver-type=Server",
		"tasks":    "muxt -C " + filepath.Join(base, "tasks") + " generate",
	}, got, "generate command of each package: an entry configures its directory, valid or not")
}
