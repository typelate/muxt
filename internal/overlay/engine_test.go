package overlay_test

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/overlay"
)

func TestEngineImportPaths(t *testing.T) {
	engine := overlay.Engine()
	var goFiles int
	err := fs.WalkDir(engine, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(name) != ".go" {
			return err
		}
		goFiles++
		src, err := fs.ReadFile(engine, name)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range f.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			first, _, _ := strings.Cut(p, "/")
			isStd := !strings.Contains(first, ".")
			isEngine := strings.HasPrefix(p, "html/template/internal/")
			isSelf := p == "html/template" || strings.HasPrefix(p, "html/template/")
			if !isEngine && (!isStd || isSelf) {
				t.Errorf("%s imports %q, want a standard library path or html/template/internal/...", name, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if goFiles == 0 {
		t.Fatal("the embedded engine has no Go files")
	}
}

func TestEngineBuildsAsHTMLTemplate(t *testing.T) {
	if testing.Short() {
		t.Skip("builds html/template from the overlaid engine with the go command")
	}
	goroot := goEnv(t, "GOROOT")
	stdDir := filepath.Join(goroot, "src", "html", "template")
	entries, err := os.ReadDir(stdDir)
	if err != nil {
		t.Skipf("the standard library's html/template source is not at %s: %v", stdDir, err)
	}

	scratch := t.TempDir()
	replace := make(map[string]string)
	for _, e := range entries {
		if name := e.Name(); !e.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			replace[filepath.Join(stdDir, name)] = ""
		}
	}
	engine := overlay.Engine()
	err = fs.WalkDir(engine, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(engine, name)
		if err != nil {
			return err
		}
		local := filepath.Join(scratch, "engine", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(local, b, 0o644); err != nil {
			return err
		}
		replace[filepath.Join(stdDir, filepath.FromSlash(name))] = local
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	overlayJSON, err := json.Marshal(struct{ Replace map[string]string }{replace})
	if err != nil {
		t.Fatal(err)
	}
	overlayPath := filepath.Join(scratch, "overlay.json")
	writeFile(t, overlayPath, string(overlayJSON))

	module := filepath.Join(scratch, "module")
	writeFile(t, filepath.Join(module, "go.mod"), "module example.com/scratch\n\ngo 1.27\n")
	writeFile(t, filepath.Join(module, "page_test.go"), `package scratch

import (
	"html/template"
	"reflect"
	"strings"
	"testing"
)

func TestExecute(t *testing.T) {
	ts := template.Must(template.New("page").Parse(`+"`"+`<p title="{{.Title}}">{{.Body}}</p>`+"`"+`))
	var out strings.Builder
	if err := ts.Execute(&out, struct{ Title, Body string }{"a\"b", "<i>"}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), `+"`"+`<p title="a&#34;b">&lt;i&gt;</p>`+"`"+`; got != want {
		t.Errorf("Execute wrote %s, want %s", got, want)
	}
	if got, want := reflect.TypeOf(ts.Tree).Elem().PkgPath(), "html/template/internal/engine"; got != want {
		t.Errorf("Template.Tree is from %q, want %q: html/template is not the overlaid engine", got, want)
	}
}
`)

	runGo(t, module, "build", "-overlay="+overlayPath, "html/template")
	runGo(t, module, "test", "-count=1", "-vet=off", "-overlay="+overlayPath, ".")
}

func goEnv(t *testing.T, key string) string {
	t.Helper()
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		t.Skipf("go env %s: %v", key, err)
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		t.Skipf("go env %s is empty", key)
	}
	return value
}

func runGo(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS="+os.Getenv("GOFLAGS"))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out.Bytes())
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
