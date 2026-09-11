package mutation

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/asteval"
)

// This file holds what more than one test needs: a module on disk, a git
// repository, the types a template is rendered with, and the readers that
// say what a plan decided.
//
// Adding a mutation operator? Add a row to TestMutantsInScope in
// enumerate_test.go, and one to TestConstructDropKeepsTheElseBranch if it
// interacts with {{else}}. A golden under cmd/muxt/testdata is for the
// command line surface -- flags, streams, exit codes -- not for what a
// template is mutated into.

// goEnv is the environment a test's go command runs in: the process's own,
// without any workspace the caller happens to be in, since a module a test
// wrote is not part of it.
//
// It is passed to the code under test rather than set on the process, so
// that tests writing modules can still run in parallel.
func goEnv() []string { return append(os.Environ(), "GOWORK=off") }

// module writes a Go module into a temporary directory and returns it.
func module(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, files)
	return dir
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

const goMod = "module server\n\ngo 1.24\n"

// greetingGo holds its template as a raw string literal.
const greetingGo = `package server

import (
	"html/template"
	"io"
)

var templates = template.Must(template.New("greeting").Parse(` + "`" + `Hello, {{.Name}}!{{if .Loud}} !!!{{end}}` + "`" + `))

type Greeting struct {
	Name string
	Loud bool
}

func Render(w io.Writer, greeting Greeting) error {
	return templates.ExecuteTemplate(w, "greeting", greeting)
}
`

// greetingTest checks the name and never renders with Loud set, so it
// catches the mutants that change the name or force the exclamation, and
// misses the one that removes it.
const greetingTest = `package server

import (
	"strings"
	"testing"
)

func TestGreeting(t *testing.T) {
	var buf strings.Builder
	if err := Render(&buf, Greeting{Name: "World"}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "Hello, World!"; got != want {
		t.Errorf("greeting = %q, want %q", got, want)
	}
}
`

// diffTemplates has "page" render "name" twice with the same dot, so the
// second call is trimmed, and "count" once.
const diffTemplates = `{{define "page"}}{{template "name" .Name}}{{template "name" .Name}}{{template "count" .Count}}{{end}}
{{define "name"}}<b>{{.}}</b>{{end}}
{{define "count"}}<i>{{.}}</i>{{end}}
`

// diffGoFile renders "page" with a struct named by the first argument,
// whose Count has the type the second names.
const diffGoFile = `package server

import (
	"embed"
	"html/template"
	"io"
)

//go:embed *.gohtml
var templateFiles embed.FS

var templates = template.Must(template.ParseFS(templateFiles, "*.gohtml"))

type %[1]s struct {
	Name  string
	Count %[2]s
}

func Render(w io.Writer, data %[1]s) error {
	return templates.ExecuteTemplate(w, "page", data)
}
`

// repo is a git repository in a temporary directory.
//
// git runs with its own environment rather than the process's, so that a
// test using one can run beside the others, and so that no setting of the
// invoking user's -- commit signing, say -- changes what its commits do.
type repo struct {
	t   *testing.T
	dir string
	env []string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	r := &repo{t: t, dir: t.TempDir(), env: append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+filepath.Join(t.TempDir(), "gitconfig"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=muxt",
		"GIT_AUTHOR_EMAIL=muxt@example.com",
		"GIT_COMMITTER_NAME=muxt",
		"GIT_COMMITTER_EMAIL=muxt@example.com",
		// git translates its messages, and one test reads one.
		"LC_ALL=C",
	)}
	r.git("init", "-q")
	return r
}

func (r *repo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = r.env
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func (r *repo) write(files map[string]string) {
	r.t.Helper()
	writeFiles(r.t, r.dir, files)
}

func (r *repo) commit(message string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "-m", message)
}

// exitStatusOne is the error a command that ran and failed gives. Only a
// real process carries one, and it is what tells a suite that failed from
// a command that could not run at all.
func exitStatusOne(t *testing.T) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("false is not a command on Windows")
	}
	err := exec.Command("false").Run()
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("false = %v, want exit status 1", err)
	}
	return err
}

// scopeOf builds the scope a traversal reports for one template: its text
// parsed, the file it was read from, and the type of dot it renders with.
//
// Everything the selection and the revision decide is decided from these,
// so a test of either needs no module and no loader.
func scopeOf(t *testing.T, name, text string, dot types.Type) scope {
	t.Helper()
	trees, err := asteval.ParseTrees(name, text, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	tree, ok := trees[name]
	if !ok {
		t.Fatalf("the text does not hold %q", name)
	}
	file := name + ".gohtml"
	return scope{
		template:     name,
		dataType:     dot,
		treeLocation: treeLocation{src: newFileSource(file, file, text, "", ""), tree: tree},
	}
}

// mutatedTemplates names each template a plan mutates, with its dot.
func mutatedTemplates(p *plan) []string {
	var names []string
	for _, group := range p.groups {
		for _, template := range group.Templates {
			names = append(names, template.Template+" "+template.DataType)
		}
	}
	return names
}

func unchangedTemplates(p *plan) []string {
	var names []string
	for _, u := range p.unchanged {
		names = append(names, u.Template+" "+u.DataType)
	}
	return names
}

// reportText renders a plan's report as a verbose dry run would.
func reportText(t *testing.T, p *plan) string {
	t.Helper()
	report := p.report()
	report.DryRun, report.Verbose = true, true
	var out strings.Builder
	if _, err := report.WriteTo(&out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// pageSource declares the types the typed tests render against.
const pageSource = `
type Page struct {
	Name  string
	Count int
	Price float64
	Flag  bool
	Items []Item
	Tags  map[string]Item
	Owner *User
}

type Item struct{ ID int }

type User struct{ Email string }

func (*User) Display() string { return "" }

func (Page) Title() string { return "" }

func (Page) Load() (int, error) { return 0, nil }

func (Page) Lookup(key string) string { return key }

func (Page) Reset() {}

func (Page) secret() string { return "" }
`

// dataType type checks src as package example.com/data and returns the
// type it declares under name.
//
// It is a real type check of real source, without the cost of loading a
// module: what dot resolution reads is the same either way.
func dataType(t *testing.T, src, name string) types.Type {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "data.go", "package data\n"+src, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := new(types.Config).Check("example.com/data", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return pkg.Scope().Lookup(name).Type()
}

func safeHTML() types.Type {
	pkg := types.NewPackage("html/template", "template")
	return types.NewNamed(types.NewTypeName(token.NoPos, pkg, "HTML", nil), types.Typ[types.String], nil)
}
