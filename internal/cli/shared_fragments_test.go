package cli

import (
	"bytes"
	"os"
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
