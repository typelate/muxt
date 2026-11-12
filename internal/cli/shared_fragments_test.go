package cli

import (
	"bytes"
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
	assert.Contains(t, output, "workspace.go:14:10: package example.org/other/fragments is not in this module (example.com/m); a shared embed.FS must come from a package of this module", "generate output")
}
