package cli

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fragmentsModule writes the module example.com/m with a fragments package that
// exports the embed.FS TemplateSource holding a shared "layout" template, and a
// workspace package whose templates variable chains that FS and then its own
// files. files adds files to the module or replaces the ones written here,
// keyed by slash-separated path from the module root. It returns the module
// root.
func fragmentsModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	module := map[string]string{
		"go.mod": "module example.com/m\n\ngo 1.24\n",
		"fragments/fragments.go": `package fragments

import "embed"

//go:embed *.gohtml
var TemplateSource embed.FS
`,
		"fragments/layout.gohtml": `{{define "layout"}}<main>{{.}}</main>{{end}}`,
		"workspace/workspace.go": `package workspace

import (
	"embed"
	"html/template"

	"example.com/m/fragments"
)

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.Must(template.New("").
	ParseFS(fragments.TemplateSource, "*.gohtml")).
	ParseFS(templateSource, "*.gohtml"))

type T struct{}

func (T) Home() string { return "home" }
`,
		"workspace/page.gohtml": `{{define "GET /{$} Home()"}}{{template "layout" .}}{{end}}`,
	}
	for name, content := range files {
		module[name] = content
	}
	for name, content := range module {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}

// runMuxt runs the muxt command line args in dir with the real loader and the
// go command, and returns what it wrote to stdout and stderr together.
func runMuxt(t *testing.T, dir string, args ...string) (output string, err error) {
	t.Helper()
	var out bytes.Buffer
	err = Commands(dir, args, func(string) string { return "" }, strings.NewReader(""), &out, &out)
	return out.String(), err
}

// resolvedDir returns dir with symlinks resolved, the form muxt prints: the go
// command reports file paths that way, which on macOS turns /var into
// /private/var.
func resolvedDir(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err, "resolve %s", dir)
	return resolved
}

// TestGenerateReadsAnotherPackagesEmbedFS states that muxt generate succeeds in
// a package whose templates variable parses another package's exported
// embed.FS before its own files.
func TestGenerateReadsAnotherPackagesEmbedFS(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, nil)

	output, err := runMuxt(t, filepath.Join(dir, "workspace"), "generate")

	require.NoError(t, err, "generate: %s", output)
	assert.FileExists(t, filepath.Join(dir, "workspace", "template_routes.go"))
}

// TestCheckFindsTemplateFromAnotherPackage states that a route template that
// calls a template only the other package's file defines passes muxt check,
// and the routes muxt generate wrote for it compile.
func TestCheckFindsTemplateFromAnotherPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, nil)
	workspace := filepath.Join(dir, "workspace")
	output, err := runMuxt(t, workspace, "generate", "--use-receiver-type=T")
	require.NoError(t, err, "generate: %s", output)

	output, err = runMuxt(t, workspace, "check")

	require.NoError(t, err, "check: %s", output)
	vet := exec.Command("go", "vet", "./...")
	vet.Dir = dir
	vet.Env = append(os.Environ(), "GOROOT=")
	vetOutput, err := vet.CombinedOutput()
	assert.NoError(t, err, "go vet: %s", vetOutput)
}

// TestGenerateIncludesRouteFromAnotherPackage states that a route template
// defined in the other package's file is a route of the package that parses
// it: its method is in that package's generated receiver interface, and
// nothing is written into the other package's directory.
func TestGenerateIncludesRouteFromAnotherPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/shared.gohtml": `{{define "GET /shared SharedPage()"}}{{template "layout" .}}{{end}}`,
	})
	fragments := filepath.Join(dir, "fragments")
	before, err := os.ReadDir(fragments)
	require.NoError(t, err, "ReadDir")

	output, err := runMuxt(t, filepath.Join(dir, "workspace"), "generate", "--use-receiver-type=T")

	require.NoError(t, err, "generate: %s", output)
	routes, err := os.ReadFile(filepath.Join(dir, "workspace", "template_routes.go"))
	require.NoError(t, err, "routes file")
	assert.Contains(t, string(routes), "SharedPage(", "the receiver interface names the shared route's method")
	after, err := os.ReadDir(fragments)
	require.NoError(t, err, "ReadDir")
	assert.Equal(t, before, after, "files in the fragments directory")
}

// TestGenerateNotesMissingMethodOfSharedRoute states that a route template in
// the other package's file whose method the receiver type lacks gets the note
// a route in the package's own file gets, naming the method.
func TestGenerateNotesMissingMethodOfSharedRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/shared.gohtml": `{{define "GET /shared SharedPage()"}}{{template "layout" .}}{{end}}`,
	})

	output, err := runMuxt(t, filepath.Join(dir, "workspace"), "generate", "--use-receiver-type=T")

	require.NoError(t, err, "generate: %s", output)
	assert.Contains(t, output, "note: T does not define SharedPage() any", "generate output")
}

// TestMutationsSkipSharedFiles states that muxt test-template-mutations in a
// package that parses the other package's files lists mutants of the
// package's own templates and none in the other package's files, though the
// package's templates call the shared one.
func TestMutationsSkipSharedFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/layout.gohtml": `{{define "layout"}}{{if .Result}}<main>{{.Result}}</main>{{end}}{{end}}`,
		"workspace/page.gohtml":   `{{define "GET /{$} Home()"}}{{if .Result}}{{template "layout" .}}{{end}}{{end}}`,
	})
	workspace := filepath.Join(dir, "workspace")
	output, err := runMuxt(t, workspace, "generate", "--use-receiver-type=T")
	require.NoError(t, err, "generate: %s", output)

	output, err = runMuxt(t, workspace, "test-template-mutations", "--dry-run")

	require.NoError(t, err, "test-template-mutations: %s", output)
	assert.Contains(t, output, "page.gohtml", "mutants of the package's own file")
	assert.NotContains(t, output, "layout.gohtml", "mutants of the shared file")
	assert.NotContains(t, output, "fragments", "mutants of the shared file")
}

// TestFmtWriteLeavesSharedFilesAlone states that muxt fmt -w . in a package
// that parses the other package's files rewrites the package's own templates
// and no file in the other package's directory. Naming the other directory
// does rewrite its file, so the shared file is one fmt would change.
func TestFmtWriteLeavesSharedFilesAlone(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	unformatted := "{{define \"%s\"}}<div>\n<p>text</p>\n</div>{{end}}\n"
	dir := fragmentsModule(t, map[string]string{
		"fragments/layout.gohtml": fmt.Sprintf(unformatted, "layout"),
		"workspace/page.gohtml":   fmt.Sprintf(unformatted, "GET /{$} Home()"),
	})
	shared := filepath.Join(dir, "fragments", "layout.gohtml")
	own := filepath.Join(dir, "workspace", "page.gohtml")
	sharedBefore, err := os.ReadFile(shared)
	require.NoError(t, err, "read shared file")
	ownBefore, err := os.ReadFile(own)
	require.NoError(t, err, "read own file")

	output, err := runMuxt(t, filepath.Join(dir, "workspace"), "fmt", "-w", ".")

	require.NoError(t, err, "fmt: %s", output)
	ownAfter, err := os.ReadFile(own)
	require.NoError(t, err, "read own file")
	assert.NotEqual(t, string(ownBefore), string(ownAfter), "the package's own file is formatted")
	sharedAfter, err := os.ReadFile(shared)
	require.NoError(t, err, "read shared file")
	assert.Equal(t, string(sharedBefore), string(sharedAfter), "the shared file")

	output, err = runMuxt(t, filepath.Join(dir, "workspace"), "fmt", "-w", "../fragments")

	require.NoError(t, err, "fmt: %s", output)
	sharedAfter, err = os.ReadFile(shared)
	require.NoError(t, err, "read shared file")
	assert.NotEqual(t, string(sharedBefore), string(sharedAfter), "the shared file, when fmt is told to format it")
}

// TestGenerateMultipleFilesNamesClashWithSharedFile states that with
// --output-multiple-files a route in the other package's layout.gohtml and a
// route in the package's own layout.gohtml, which would write the same
// generated file, fail naming both template files. The rule is stated in
// internal/generate; this is the one test that the real loader gives it the
// shared file.
func TestGenerateMultipleFilesNamesClashWithSharedFile(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/layout.gohtml": `{{define "layout"}}<main>{{.}}</main>{{end}}{{define "GET /shared SharedPage()"}}{{template "layout" .}}{{end}}`,
		"workspace/layout.gohtml": `{{define "GET /own OwnPage()"}}{{template "layout" .}}{{end}}`,
	})

	output, err := runMuxt(t, filepath.Join(dir, "workspace"), "generate", "--output-multiple-files", "--use-receiver-type=T")

	require.Error(t, err, "generate: %s", output)
	assert.Contains(t, output, "layout_template_routes_gen.go would be written for both "+filepath.Join(dir, "workspace", "layout.gohtml")+" and "+filepath.Join(dir, "fragments", "layout.gohtml")+"; rename one template file", "generate output")
}

// TestGenerateMultipleFilesWritesOnlyInThePackage states that with
// --output-multiple-files and no clash, the routes of a shared file are
// written into the package's directory and nothing into the other package's.
func TestGenerateMultipleFilesWritesOnlyInThePackage(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/shared.gohtml": `{{define "GET /shared SharedPage()"}}{{template "layout" .}}{{end}}`,
	})
	before, err := os.ReadDir(filepath.Join(dir, "fragments"))
	require.NoError(t, err, "ReadDir")

	output, err := runMuxt(t, filepath.Join(dir, "workspace"), "generate", "--output-multiple-files", "--use-receiver-type=T")

	require.NoError(t, err, "generate: %s", output)
	assert.FileExists(t, filepath.Join(dir, "workspace", "shared_template_routes_gen.go"), "generated file for the shared route")
	after, err := os.ReadDir(filepath.Join(dir, "fragments"))
	require.NoError(t, err, "ReadDir")
	assert.Equal(t, before, after, "files in the fragments directory")
}

// TestGenerateMuxtJSONEntriesSharingAnEmbedFS states that two muxt.json entries
// that both parse the other package's files are loaded once, each routes file
// includes the shared route, and nothing is written into the other package's
// directory.
func TestGenerateMuxtJSONEntriesSharingAnEmbedFS(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/shared.gohtml": `{{define "GET /shared SharedPage()"}}{{template "layout" .}}{{end}}`,
		"billing/billing.go":      strings.Replace(workspaceReadingFS("example.com/m/fragments", "fragments.TemplateSource"), "package workspace", "package billing", 1),
		"billing/page.gohtml":     `{{define "GET /billing Home()"}}{{template "layout" .}}{{end}}`,
		"muxt.json": `{"packages": {
	"example.com/m/workspace": {"use_receiver_type": "T"},
	"example.com/m/billing": {"use_receiver_type": "T"}
}}`,
	})
	before, err := os.ReadDir(filepath.Join(dir, "fragments"))
	require.NoError(t, err, "ReadDir")

	_, loads, err := generateWithLoader(t, dir)

	require.NoError(t, err, "generate")
	assert.Equal(t, 1, loads, "package loads")
	for _, pkg := range []string{"workspace", "billing"} {
		routes, err := os.ReadFile(filepath.Join(dir, pkg, "template_routes.go"))
		require.NoError(t, err, "routes file of %s", pkg)
		assert.Contains(t, string(routes), "SharedPage(", "%s includes the shared route", pkg)
	}
	after, err := os.ReadDir(filepath.Join(dir, "fragments"))
	require.NoError(t, err, "ReadDir")
	assert.Equal(t, before, after, "files in the fragments directory")
}

// TestCheckReadsOnlyFilesTheOtherPackageEmbeds states that a file in the
// other package's directory its //go:embed pattern does not match is not
// parsed, even when the ParseFS patterns would match it, so a template only
// that file defines is undefined.
func TestCheckReadsOnlyFilesTheOtherPackageEmbeds(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/notes.txt":   `{{define "draft"}}draft{{end}}`,
		"workspace/page.gohtml": `{{define "GET /{$} Home()"}}{{template "draft" .}}{{end}}`,
		"workspace/workspace.go": `package workspace

import (
	"embed"
	"html/template"

	"example.com/m/fragments"
)

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.Must(template.New("").
	ParseFS(fragments.TemplateSource, "*")).
	ParseFS(templateSource, "*.gohtml"))

type T struct{}

func (T) Home() string { return "home" }
`,
	})

	workspace := filepath.Join(dir, "workspace")
	output, err := runMuxt(t, workspace, "generate", "--use-receiver-type=T")
	require.NoError(t, err, "generate: %s", output)

	output, err = runMuxt(t, workspace, "check")

	require.Error(t, err, "check: %s", output)
	assert.Contains(t, output, `"draft"`, "check output")
}

// TestCheckFindsTemplateInOtherPackagesSubdirectory states that when the other
// package embeds a subdirectory and the chain's ParseFS names a pattern in it,
// a template defined there is found.
func TestCheckFindsTemplateInOtherPackagesSubdirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/fragments.go": `package fragments

import "embed"

//go:embed *.gohtml partials/*.gohtml
var TemplateSource embed.FS
`,
		"fragments/partials/nav.gohtml": `{{define "nav"}}<nav></nav>{{end}}`,
		"workspace/page.gohtml":         `{{define "GET /{$} Home()"}}{{template "layout" .}}{{template "nav"}}{{end}}`,
		"workspace/workspace.go": `package workspace

import (
	"embed"
	"html/template"

	"example.com/m/fragments"
)

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.Must(template.New("").
	ParseFS(fragments.TemplateSource, "*.gohtml", "partials/*.gohtml")).
	ParseFS(templateSource, "*.gohtml"))

type T struct{}

func (T) Home() string { return "home" }
`,
	})
	workspace := filepath.Join(dir, "workspace")
	output, err := runMuxt(t, workspace, "generate", "--use-receiver-type=T")
	require.NoError(t, err, "generate: %s", output)

	output, err = runMuxt(t, workspace, "check")

	require.NoError(t, err, "check: %s", output)
}

// workspaceReadingFS is the workspace package's source, with first as the
// argument of the ParseFS that reads the other package's files, and importPath
// the package that argument's qualifier names.
func workspaceReadingFS(importPath, first string) string {
	return `package workspace

import (
	"embed"
	"html/template"

	"` + importPath + `"
)

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.Must(template.New("").
	ParseFS(` + first + `, "*.gohtml")).
	ParseFS(templateSource, "*.gohtml"))

type T struct{}

func (T) Home() string { return "home" }
`
}

// TestGenerateReportsUnexportedSharedFS states that a ParseFS argument naming
// an unexported variable of another package fails with muxt's own message, at
// the argument's position, and not the go compiler's.
func TestGenerateReportsUnexportedSharedFS(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/fragments.go": `package fragments

import "embed"

//go:embed *.gohtml
var templateSource embed.FS
`,
		"workspace/workspace.go": workspaceReadingFS("example.com/m/fragments", "fragments.templateSource"),
	})

	output, err := runMuxt(t, filepath.Join(dir, "workspace"), "generate")

	require.Error(t, err, "generate: %s", output)
	assert.Contains(t, output, "workspace.go:14:10: fragments.templateSource is unexported; ParseFS reads only an exported embed.FS variable of another package", "generate output")
}

// TestGenerateReportsSharedVariableThatIsNotEmbedFS states that a ParseFS
// argument naming an exported variable of another package that is an fs.FS,
// not an embed.FS, fails with muxt's message at the argument's position.
func TestGenerateReportsSharedVariableThatIsNotEmbedFS(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/fragments.go": `package fragments

import (
	"embed"
	"io/fs"
)

//go:embed *.gohtml
var TemplateSource embed.FS

var Other fs.FS = TemplateSource
`,
		"workspace/workspace.go": workspaceReadingFS("example.com/m/fragments", "fragments.Other"),
	})

	output, err := runMuxt(t, filepath.Join(dir, "workspace"), "generate")

	require.Error(t, err, "generate: %s", output)
	assert.Contains(t, output, "workspace.go:14:10: fragments.Other is a io/fs.FS, not an embed.FS; ParseFS reads an exported embed.FS variable of another package", "generate output")
}

// TestGenerateReportsSharedFSCall states that a ParseFS first argument that is
// a call, not a variable, fails with the message that names the shared form
// muxt reads.
func TestGenerateReportsSharedFSCall(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"workspace/workspace.go": workspaceReadingFS("example.com/m/fragments", "fragments.FS()"),
	})

	output, err := runMuxt(t, filepath.Join(dir, "workspace"), "generate")

	require.Error(t, err, "generate: %s", output)
	assert.Contains(t, output, "workspace.go:14:10: first argument to ParseFS must be an identifier or an exported variable of an imported package (got fragments.FS())", "generate output")
}

// TestGenerateReportsSharedFSFromOtherModule states that a ParseFS argument
// naming a package of another module fails with muxt's message at the
// argument's position. The other module is a replace target in the temp
// directory, so the test needs no network.
func TestGenerateReportsSharedFSFromOtherModule(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"go.mod":       "module example.com/m\n\ngo 1.24\n\nrequire example.org/other v0.0.0\n\nreplace example.org/other => ./other\n",
		"other/go.mod": "module example.org/other\n\ngo 1.24\n",
		"other/fragments/fragments.go": `package fragments

import "embed"

//go:embed *.gohtml
var TemplateSource embed.FS
`,
		"other/fragments/layout.gohtml": `{{define "other"}}other{{end}}`,
		"workspace/workspace.go":        workspaceReadingFS("example.org/other/fragments", "fragments.TemplateSource"),
	})

	output, err := runMuxt(t, filepath.Join(dir, "workspace"), "generate")

	require.Error(t, err, "generate: %s", output)
	assert.Contains(t, output, "workspace.go:14:10: package example.org/other/fragments is in module example.org/other, not in this module (example.com/m); a shared embed.FS must come from a package of this module", "generate output")
}

// overriddenFooter is a module where the shared package defines "footer" with
// a field the page's result does not have, and the package's own file, parsed
// later in the chain, defines "footer" again without the error. With
// sharedLast the chain parses the shared files after the package's own, so the
// shared definition is the one kept.
func overriddenFooter(t *testing.T, sharedLast bool) string {
	t.Helper()
	files := map[string]string{
		"fragments/footer.gohtml": `{{define "footer"}}<footer>{{.Nope}}</footer>{{end}}`,
		"workspace/page.gohtml": `{{define "GET /{$} Home()"}}{{template "layout" .}}{{template "footer" .}}{{end}}
{{define "footer"}}<footer>{{.Result}}</footer>{{end}}`,
	}
	if sharedLast {
		files["workspace/workspace.go"] = `package workspace

import (
	"embed"
	"html/template"

	"example.com/m/fragments"
)

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.Must(template.New("").
	ParseFS(templateSource, "*.gohtml")).
	ParseFS(fragments.TemplateSource, "*.gohtml"))

type T struct{}

func (T) Home() string { return "home" }
`
	}
	return fragmentsModule(t, files)
}

// TestCheckTypeChecksTheOverridingDefinition states that a name defined in the
// other package's file and again in the package's own file, parsed later, is
// checked against the package's own definition: the shared one would fail.
func TestCheckTypeChecksTheOverridingDefinition(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := overriddenFooter(t, false)
	workspace := filepath.Join(dir, "workspace")
	output, err := runMuxt(t, workspace, "generate", "--use-receiver-type=T")
	require.NoError(t, err, "generate: %s", output)

	output, err = runMuxt(t, workspace, "check")

	require.NoError(t, err, "check: %s", output)
}

// TestRoutesListsOverriddenSharedNames states that the bare listing names, after
// the Configuration section and before the routes, each name defined in a
// shared file and again later in the chain, with both sites by absolute path
// on a line each, and that it says which one the chain kept. With nothing
// overridden the section is absent.
func TestRoutesListsOverriddenSharedNames(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	for _, tt := range []struct {
		name       string
		sharedLast bool
		want       string
	}{
		{
			name: "own file after the shared files",
			want: "Overrides:\n  - footer\n      replaced: %[1]s/fragments/footer.gohtml:1:1\n      kept:     %[1]s/workspace/page.gohtml:2:1\n\nTemplate Routes:",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := resolvedDir(t, overriddenFooter(t, tt.sharedLast))

			output, err := runMuxt(t, filepath.Join(dir, "workspace"))

			require.NoError(t, err, "muxt: %s", output)
			assert.Contains(t, output, fmt.Sprintf(tt.want, filepath.ToSlash(dir)), "muxt output")
			assert.Less(t, strings.Index(output, "Configuration:"), strings.Index(output, "Overrides:"), "Overrides come after Configuration")
		})
	}

	t.Run("json", func(t *testing.T) {
		dir := resolvedDir(t, overriddenFooter(t, false))

		output, err := runMuxt(t, filepath.Join(dir, "workspace"), "--format", "json")

		require.NoError(t, err, "muxt: %s", output)
		var listing struct {
			Overrides []struct{ Name, Replaced, Kept string }
		}
		require.NoError(t, json.Unmarshal([]byte(output), &listing), "muxt output: %s", output)
		require.Len(t, listing.Overrides, 1, "Overrides")
		assert.Equal(t, "footer", listing.Overrides[0].Name, "Name")
		assert.Equal(t, filepath.Join(dir, "fragments", "footer.gohtml")+":1:1", listing.Overrides[0].Replaced, "Replaced")
		assert.Equal(t, filepath.Join(dir, "workspace", "page.gohtml")+":2:1", listing.Overrides[0].Kept, "Kept")
	})

	t.Run("none", func(t *testing.T) {
		dir := fragmentsModule(t, nil)

		output, err := runMuxt(t, filepath.Join(dir, "workspace"))

		require.NoError(t, err, "muxt: %s", output)
		assert.NotContains(t, output, "Overrides", "muxt output")
	})
}

// TestSharedFileReplacingOwnTemplateFailsEveryCommand states that a shared file
// parsed after the package's own files, defining a name the package's own file
// defined, fails loading the templates variable in every command, with the own
// definition first and both sites by absolute path, and that generate writes
// nothing.
func TestSharedFileReplacingOwnTemplateFailsEveryCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := resolvedDir(t, overriddenFooter(t, true))
	workspace := filepath.Join(dir, "workspace")
	want := fmt.Sprintf("%[1]s/workspace/page.gohtml:2:1: template \"footer\" is replaced by %[1]s/fragments/footer.gohtml:1:1; parse the shared embed.FS before this package's own files", filepath.ToSlash(dir))
	before, err := os.ReadDir(workspace)
	require.NoError(t, err, "read workspace")

	for _, args := range [][]string{
		{"generate", "--use-receiver-type=T"},
		{"check"},
		nil,
	} {
		output, err := runMuxt(t, workspace, args...)

		require.Error(t, err, "muxt %v: %s", args, output)
		assert.Contains(t, output+err.Error(), want, "muxt %v output", args)
	}

	after, err := os.ReadDir(workspace)
	require.NoError(t, err, "read workspace")
	assert.Equal(t, len(before), len(after), "files in workspace")
}

// TestCheckNamesSharedFileByAbsolutePath states that an error in a shared
// file's template names the file by its absolute path, with line and column,
// and that a template of the package's own file in the same run is named the
// same way, so an editor can open either from the output.
func TestCheckNamesSharedFileByAbsolutePath(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/layout.gohtml": `{{define "layout"}}<main>{{.Nope}}</main>{{end}}`,
		"workspace/own.gohtml":    `{{define "orphan"}}<i></i>{{end}}`,
	})
	dir = resolvedDir(t, dir)
	workspace := filepath.Join(dir, "workspace")
	output, err := runMuxt(t, workspace, "generate", "--use-receiver-type=T")
	require.NoError(t, err, "generate: %s", output)

	output, err = runMuxt(t, workspace, "check")

	require.Error(t, err, "check: %s", output)
	assert.Contains(t, output, filepath.Join(dir, "fragments", "layout.gohtml")+":1:28: executing \"layout\" at <.Nope>", "check output")
	assert.Contains(t, output, "  - "+filepath.Join(dir, "workspace", "own.gohtml")+`:1:20: "orphan"`, "check output")
}

// TestCheckDoesNotReportUnusedSharedTemplate states that a template defined in
// the other package's file that the package never calls is not reported as
// unused: the package that owns the file is the one that can remove it. The
// rule is stated on a hand-built package in internal/analysis; this is the one
// test that the real loader marks the definition as shared.
func TestCheckDoesNotReportUnusedSharedTemplate(t *testing.T) {
	if testing.Short() {
		t.Skip("loads packages with the go command")
	}
	dir := fragmentsModule(t, map[string]string{
		"fragments/footer.gohtml": `{{define "footer"}}<footer></footer>{{end}}`,
	})
	workspace := filepath.Join(dir, "workspace")
	output, err := runMuxt(t, workspace, "generate", "--use-receiver-type=T")
	require.NoError(t, err, "generate: %s", output)

	output, err = runMuxt(t, workspace, "check")

	require.NoError(t, err, "check: %s", output)
	assert.NotContains(t, output, "Unused templates", "check output")
}
