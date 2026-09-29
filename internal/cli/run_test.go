package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/header"
)

func newModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"go.mod": "module example.com\n\ngo 1.24\n",
		"template.go": `package main

import (
	"embed"
	"html/template"
	"io"
)

//go:embed *.gohtml
var source embed.FS

var templates = template.Must(template.ParseFS(source, "*.gohtml"))

type Server struct{}

func (s *Server) Home() any { return nil }

func render(w io.Writer) error { return templates.ExecuteTemplate(w, "GET / Home()", nil) }

func main() {}
`,
		"old.gohtml": `{{define "GET / Home()"}}{{template "heading"}}{{end}}{{define "heading"}}<h1>Home</h1>{{end}}`,
	} {
		writeTestFile(t, dir, name, content)
	}
	return dir
}

func execute(t *testing.T, wd string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = Commands(wd, args, func(string) string { return "" }, &out, &errOut)
	return out.String(), errOut.String(), err
}

// TestRunGenerate runs generate twice in one module: the second run sees
// the files the first wrote, and removes the ones a renamed template left
// behind.
func TestRunGenerate(t *testing.T) {
	wd := newModule(t)

	t.Run("the first run writes a file per template file", func(t *testing.T) {
		stdout, _, err := execute(t, wd, "generate", "--output-multiple-files")
		require.NoError(t, err, "generate")
		for _, want := range []string{"wrote old_template_routes_gen.go: 1 route\n", "wrote template_routes.go: 0 routes\n"} {
			assert.Contains(t, stdout, want, "generate output")
		}
		generated, err := header.Scan(wd)
		require.NoError(t, err)
		h, ok := generated[filepath.Join(wd, "template_routes.go")]
		if assert.True(t, ok, "template_routes.go has a header") {
			assert.Equal(t, "--output-multiple-files", h.Args()[0], "template_routes.go header = %+v, want it to record --output-multiple-files", h)
		}
	})

	unreadable := writeTestFile(t, wd, "unreadable.go", header.Format([]string{"--no-such-flag"}, "")+"package main\n")
	otherRoutes := writeTestFile(t, wd, "other.go", header.Format([]string{"--output-routes-func=AdminRoutes"}, "")+"package main\n")

	t.Run("the second run after a template rename", func(t *testing.T) {
		require.NoError(t, os.Rename(filepath.Join(wd, "old.gohtml"), filepath.Join(wd, "new.gohtml")))
		_, stderr, err := execute(t, wd, "generate", "--output-multiple-files")
		require.NoError(t, err, "second generate")
		assert.NoFileExists(t, filepath.Join(wd, "old_template_routes_gen.go"), "old_template_routes_gen.go survived the template rename")
		assert.FileExists(t, filepath.Join(wd, "new_template_routes_gen.go"), "new_template_routes_gen.go was not written")
		for _, kept := range []string{unreadable, otherRoutes} {
			assert.FileExists(t, kept, "%s was deleted, want it left alone", kept)
		}
		assert.Contains(t, stderr, "WARNING: ignored generated file "+unreadable, "stderr")
	})
}

func TestRunCheck(t *testing.T) {
	wd := newModule(t)
	_, _, err := execute(t, wd, "generate")
	require.NoError(t, err, "generate")
	stdout, _, err := execute(t, wd, "check")
	require.NoError(t, err, "check")
	assert.Equal(t, "ok: 2 templates\n", stdout, "check output")
}

func TestRunListings(t *testing.T) {
	wd := newModule(t)
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "routes", args: []string{"--format=json"}, want: `GET /`},
		{name: "callers", args: []string{"list-template-callers", "--format=json"}, want: `GET / Home()`},
		{name: "calls", args: []string{"list-template-calls", "--format=json"}, want: `heading`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stdout, _, err := execute(t, wd, tt.args...)
			require.NoError(t, err, "muxt %v", tt.args)
			assert.Contains(t, stdout, tt.want, "muxt %v output", tt.args)
		})
	}
}

func TestRunListingsRejectAnUnknownFormat(t *testing.T) {
	wd := newModule(t)
	for _, args := range [][]string{{"--format=yaml"}, {"list-template-callers", "--format=yaml"}, {"list-template-calls", "--format=yaml"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, err := execute(t, wd, args...)
			assert.EqualError(t, err, "unknown format: yaml", "muxt %v", args)
		})
	}
}

func TestRunFailsOutsideAModule(t *testing.T) {
	for _, args := range [][]string{{"check"}, {"generate"}, {"list-template-callers"}, {testTemplateMutationsName}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, err := execute(t, t.TempDir(), args...)
			assert.Error(t, err, "muxt %v outside a module", args)
		})
	}
}
