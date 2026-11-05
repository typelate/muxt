package html_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/cover"

	"github.com/typelate/muxt/internal/overlay"
)

// TestCoverExportUnderOverlay runs three packages' tests with the engine laid
// over html/template and MUXT_TEMPLATE_COVER set, and reads back the
// profile each test binary appended.
func TestCoverExportUnderOverlay(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test binaries with html/template overlaid")
	}
	scratch := t.TempDir()
	module := filepath.Join(scratch, "module")
	writeFile(t, filepath.Join(module, "go.mod"), "module example.com/scratch\n\ngo 1.27\n")
	writeFile(t, filepath.Join(module, "a", "a_test.go"), `package a

import (
	"html/template"
	"strings"
	"testing"
)

func TestA(t *testing.T) {
	ts := template.Must(template.New("a").Parse(`+"`"+`{{if .}}<p>{{.}}</p>{{end}}`+"`"+`))
	for range 2 {
		var out strings.Builder
		if err := ts.Execute(&out, "x"); err != nil {
			t.Fatal(err)
		}
		if got, want := out.String(), "<p>x</p>"; got != want {
			t.Errorf("Execute wrote %s, want %s", got, want)
		}
	}
}
`)
	writeFile(t, filepath.Join(module, "b", "b_test.go"), `package b

import (
	"html/template"
	"strings"
	"testing"
	texttemplate "text/template"
)

func TestB(t *testing.T) {
	var funcs texttemplate.FuncMap = texttemplate.FuncMap{"up": strings.ToUpper}
	ts := template.Must(template.New("b").Funcs(funcs).Parse(`+"`"+`{{define "item"}}<i>{{up .}}</i>{{end}}`+"`"+`))
	var out strings.Builder
	if err := ts.ExecuteTemplate(&out, "item", "y"); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "<i>Y</i>"; got != want {
		t.Errorf("ExecuteTemplate wrote %s, want %s", got, want)
	}
}
`)

	// c links html/template and renders nothing: its profile is the mode
	// line alone, written as the test binary starts.
	writeFile(t, filepath.Join(module, "c", "c_test.go"), `package c

import (
	"html/template"
	"testing"
)

func TestC(t *testing.T) {
	if template.New("c") == nil {
		t.Fatal("no template")
	}
}
`)

	coverDir := filepath.Join(scratch, "cover")
	if err := os.Mkdir(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target, err := overlay.Resolve(t.Context(), overlay.Toolchain{Dir: module, Env: goEnv(), ModuleDir: muxtDir(t)})
	if err != nil {
		t.Fatalf("overlay.Resolve: %v", err)
	}
	engine, err := overlay.Write(target, filepath.Join(scratch, "engine"))
	if err != nil {
		t.Fatalf("overlay.Write: %v", err)
	}
	runGo(t, module, []string{"MUXT_TEMPLATE_COVER=" + coverDir},
		"test", "-count=1", "-mod=mod", "-modfile="+engine.ModFile, "-overlay="+engine.Overlay, "./...")

	files, err := filepath.Glob(filepath.Join(coverDir, "*.cov"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("%s has cover files %v, want one per test binary", coverDir, files)
	}
	got := make(map[string][]string)
	for _, name := range files {
		profiles, err := cover.ParseProfiles(name)
		if err != nil {
			t.Fatalf("cover.ParseProfiles(%s): %v", name, err)
		}
		for _, p := range profiles {
			for _, b := range p.Blocks {
				if b.Count != 1 {
					t.Errorf("%s: block %+v has count %d, want 1", name, b, b.Count)
				}
				got[p.FileName] = append(got[p.FileName], blockRange(b))
			}
		}
	}
	want := map[string][]string{
		// {{if .}}, <p, >, {{.}}, </p>, {{end}}; each once although executed twice.
		"a": {"1.1,1.9", "1.9,1.11", "1.11,1.12", "1.12,1.17", "1.17,1.21", "1.21,1.28"},
		// The define's <i, >, {{up .}}, </i>.
		"b": {"1.18,1.20", "1.20,1.21", "1.21,1.29", "1.29,1.33"},
	}
	for name, w := range want {
		if !slices.Equal(got[name], w) {
			t.Errorf("blocks for %s = %v, want %v", name, got[name], w)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("unexpected blocks for %s: %v", name, got[name])
		}
	}
}

func blockRange(b cover.ProfileBlock) string {
	return strconv.Itoa(b.StartLine) + "." + strconv.Itoa(b.StartCol) + "," + strconv.Itoa(b.EndLine) + "." + strconv.Itoa(b.EndCol)
}

// goEnv is the environment the test's go commands run in: the local
// toolchain, whose GOROOT the overlay is keyed on, and no workspace.
func goEnv() []string {
	return append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local")
}

// muxtDir is this repository's directory, which replaces
// github.com/typelate/muxt in the modfile so that nothing is fetched.
func muxtDir(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/typelate/muxt").Output()
	if err != nil {
		t.Fatalf("go list -m github.com/typelate/muxt: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func runGo(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(goEnv(), env...)
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
