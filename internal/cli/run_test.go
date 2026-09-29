package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestRunGenerate(t *testing.T) {
	wd := newModule(t)

	stdout, _, err := execute(t, wd, "generate", "--output-multiple-files")
	if err != nil {
		t.Fatalf("generate error = %v", err)
	}
	for _, want := range []string{"wrote old_template_routes_gen.go: 1 route\n", "wrote template_routes.go: 0 routes\n"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("generate output = %q, want containing %q", stdout, want)
		}
	}
	generated, err := header.Scan(wd)
	if err != nil {
		t.Fatal(err)
	}
	if h, ok := generated[filepath.Join(wd, "template_routes.go")]; !ok || h.Args()[0] != "--output-multiple-files" {
		t.Errorf("template_routes.go header = %+v (found %v), want it to record --output-multiple-files", h, ok)
	}

	unreadable := writeTestFile(t, wd, "unreadable.go", header.Format([]string{"--no-such-flag"}, "")+"package main\n")
	otherRoutes := writeTestFile(t, wd, "other.go", header.Format([]string{"--output-routes-func=AdminRoutes"}, "")+"package main\n")

	if err := os.Rename(filepath.Join(wd, "old.gohtml"), filepath.Join(wd, "new.gohtml")); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := execute(t, wd, "generate", "--output-multiple-files")
	if err != nil {
		t.Fatalf("second generate error = %v", err)
	}
	if exists(filepath.Join(wd, "old_template_routes_gen.go")) {
		t.Error("old_template_routes_gen.go survived the template rename")
	}
	if !exists(filepath.Join(wd, "new_template_routes_gen.go")) {
		t.Error("new_template_routes_gen.go was not written")
	}
	for _, kept := range []string{unreadable, otherRoutes} {
		if !exists(kept) {
			t.Errorf("%s was deleted, want it left alone", kept)
		}
	}
	if want := "WARNING: ignored generated file " + unreadable; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want containing %q", stderr, want)
	}
}

func TestRunCheck(t *testing.T) {
	wd := newModule(t)
	if _, _, err := execute(t, wd, "generate"); err != nil {
		t.Fatalf("generate error = %v", err)
	}
	stdout, _, err := execute(t, wd, "check")
	if err != nil {
		t.Fatalf("check error = %v", err)
	}
	if want := "ok: 2 templates\n"; stdout != want {
		t.Errorf("check output = %q, want %q", stdout, want)
	}
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
			if err != nil {
				t.Fatalf("muxt %v error = %v", tt.args, err)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Errorf("muxt %v output = %q, want containing %q", tt.args, stdout, tt.want)
			}
		})
	}
}

func TestRunListingsRejectAnUnknownFormat(t *testing.T) {
	wd := newModule(t)
	for _, args := range [][]string{{"--format=yaml"}, {"list-template-callers", "--format=yaml"}, {"list-template-calls", "--format=yaml"}} {
		if _, _, err := execute(t, wd, args...); err == nil || err.Error() != "unknown format: yaml" {
			t.Errorf("muxt %v error = %v, want unknown format: yaml", args, err)
		}
	}
}

func TestRunFailsOutsideAModule(t *testing.T) {
	for _, args := range [][]string{{"check"}, {"generate"}, {"list-template-callers"}} {
		if _, _, err := execute(t, t.TempDir(), args...); err == nil {
			t.Errorf("muxt %v outside a module = nil error, want one", args)
		}
	}
}
