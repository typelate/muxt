package overlay_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/overlay"
)

// targetModule is a test target's go.mod and go.sum, which Write copies.
const (
	targetGoMod = "module example.com/target\n\ngo 1.27\n"
	targetGoSum = "example.com/dep v1.0.0/go.mod h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n"
)

// writeTarget writes a test target module and returns its directory.
func writeTarget(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeFile(t, filepath.Join(dir, name), content)
	}
	return dir
}

// TestWriteShimOverlay states the overlay Write writes: html/template's
// own files deleted and the shim added in their place, backed by a copy
// of the shim under the scratch directory.
func TestWriteShimOverlay(t *testing.T) {
	goroot := realGOROOT(t)
	stdDir := filepath.Join(goroot, "src", "html", "template")
	target := writeTarget(t, map[string]string{"go.mod": targetGoMod})
	scratch := t.TempDir()

	files, err := overlay.Write(context.Background(), overlay.Toolchain{Dir: target, ModuleDir: muxtDir(t)}, scratch)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if want := filepath.Join(scratch, "overlay.json"); files.Overlay != want {
		t.Errorf("Overlay = %q, want %q", files.Overlay, want)
	}
	var got struct{ Replace map[string]string }
	if err := json.Unmarshal([]byte(readFile(t, files.Overlay)), &got); err != nil {
		t.Fatalf("overlay JSON %s: %v", files.Overlay, err)
	}

	shim := filepath.Join(scratch, "html-template", "muxt_shim.go")
	want := map[string]string{filepath.Join(stdDir, "muxt_shim.go"): shim}
	for _, name := range stdSourceFiles(t, stdDir) {
		want[filepath.Join(stdDir, name)] = "" // deleted
	}
	for key, backing := range got.Replace {
		if w, ok := want[key]; !ok {
			t.Errorf("overlay replaces %s, which is neither html/template's nor the shim", key)
		} else if backing != w {
			t.Errorf("overlay backs %s with %q, want %q", key, backing, w)
		}
	}
	for key := range want {
		if _, ok := got.Replace[key]; !ok {
			t.Errorf("overlay leaves out %s", key)
		}
	}
	if b := readFile(t, shim); b != string(overlay.Shim) {
		t.Errorf("the shim at %s is not the embedded one:\n%s", shim, b)
	}
}

// TestWriteCopiesGoSum states that the modfile copy has the test
// target's go.sum beside it, and that neither of the target's files
// changes.
func TestWriteCopiesGoSum(t *testing.T) {
	realGOROOT(t)
	for _, tt := range []struct {
		name  string
		goSum bool
	}{
		{name: "with a go.sum", goSum: true},
		{name: "without a go.sum"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			targetFiles := map[string]string{"go.mod": targetGoMod}
			if tt.goSum {
				targetFiles["go.sum"] = targetGoSum
			}
			target := writeTarget(t, targetFiles)
			scratch := t.TempDir()

			files, err := overlay.Write(context.Background(), overlay.Toolchain{Dir: target, ModuleDir: muxtDir(t)}, scratch)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if want := filepath.Join(scratch, "go.mod"); files.ModFile != want {
				t.Errorf("ModFile = %q, want %q", files.ModFile, want)
			}
			sum, err := os.ReadFile(filepath.Join(scratch, "go.sum"))
			switch {
			case tt.goSum && err != nil:
				t.Errorf("the target's go.sum is not copied: %v", err)
			case tt.goSum && string(sum) != targetGoSum:
				t.Errorf("go.sum = %q, want the target's %q", sum, targetGoSum)
			case !tt.goSum && err == nil && len(sum) > 0:
				t.Errorf("go.sum = %q, want none or empty", sum)
			}
			if b := readFile(t, filepath.Join(target, "go.mod")); b != targetGoMod {
				t.Errorf("the target's go.mod changed:\n%s", b)
			}
		})
	}
}

// TestWriteModuleDir states where the module directory that replaces
// muxt comes from: the toolchain's field, else MUXT_MODULE_DIR in its
// environment, a relative one resolved against the toolchain's directory.
func TestWriteModuleDir(t *testing.T) {
	realGOROOT(t)
	target := writeTarget(t, map[string]string{"go.mod": targetGoMod})
	for _, tt := range []struct {
		name string
		tc   overlay.Toolchain
		want string
	}{
		{name: "the field", tc: overlay.Toolchain{Dir: target, ModuleDir: "/src/muxt", Env: append(os.Environ(), "MUXT_MODULE_DIR=/elsewhere")}, want: "/src/muxt"},
		{name: "the environment", tc: overlay.Toolchain{Dir: target, Env: append(os.Environ(), "MUXT_MODULE_DIR=/src/muxt")}, want: "/src/muxt"},
		{name: "a relative directory", tc: overlay.Toolchain{Dir: target, ModuleDir: "muxt"}, want: filepath.Join(target, "muxt")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files, err := overlay.Write(context.Background(), tt.tc, t.TempDir())
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if files.ModuleDir != tt.want {
				t.Errorf("ModuleDir = %q, want %q", files.ModuleDir, tt.want)
			}
			if !strings.Contains(readFile(t, files.ModFile), "replace github.com/typelate/muxt => "+tt.want) {
				t.Errorf("modfile does not replace muxt with %s:\n%s", tt.want, readFile(t, files.ModFile))
			}
		})
	}
}

// TestWriteNeedsAVersion states that a muxt without a version to require,
// as a test binary is, cannot write the modfile without a module
// directory, and says how to give it one.
func TestWriteNeedsAVersion(t *testing.T) {
	realGOROOT(t)
	target := writeTarget(t, map[string]string{"go.mod": targetGoMod})
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "MUXT_MODULE_DIR=") })
	_, err := overlay.Write(context.Background(), overlay.Toolchain{Dir: target, Env: env}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "set MUXT_MODULE_DIR to its source directory") {
		t.Errorf("Write: error = %v, want one saying to set MUXT_MODULE_DIR", err)
	}
	if err := overlay.Probe(context.Background(), overlay.Toolchain{Dir: target, Env: env}); err == nil || !strings.Contains(err.Error(), "MUXT_MODULE_DIR") {
		t.Errorf("Probe: error = %v, want one saying to set MUXT_MODULE_DIR", err)
	}
}

// TestWriteNeedsAModule states that a target outside any module is an
// error: there is no go.mod to add the engine to.
func TestWriteNeedsAModule(t *testing.T) {
	realGOROOT(t)
	dir := t.TempDir()
	tc := overlay.Toolchain{Dir: dir, Env: append(os.Environ(), "GO111MODULE=on", "GOWORK=off")}
	if _, err := overlay.Write(context.Background(), tc, t.TempDir()); err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("Write outside a module: error = %v, want one naming go.mod", err)
	}
}

// TestOverlayBuildsFixture runs the fixture module's test, which passes
// only when html/template is the shim and the engine behind it is
// github.com/typelate/muxt/template, which the modfile requires and
// replaces with this repository: it fails without the overlay, and passes
// with it, vet on and -cover set, leaving the fixture's go.mod and go.sum
// as they were. The fixture is at muxt's go line, so the modfile copy
// keeps it.
func TestOverlayBuildsFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("builds html/template from the overlaid engine with the go command")
	}
	realGOROOT(t)
	fixture := t.TempDir()
	if err := os.CopyFS(fixture, os.DirFS(filepath.Join("testdata", "fixture"))); err != nil {
		t.Fatal(err)
	}
	goMod := readFile(t, filepath.Join(fixture, "go.mod"))
	goSum := readFile(t, filepath.Join(fixture, "go.sum"))

	if out, err := goCommand(fixture, "test", "-count=1", "."); err == nil {
		t.Fatalf("the fixture's test passes with the real html/template; it must not:\n%s", out)
	} else if !strings.Contains(out, "html/template is not the overlaid engine") {
		t.Fatalf("the fixture's test fails without the overlay, but not for the engine's absence: %v\n%s", err, out)
	}

	files, err := overlay.Write(context.Background(), overlay.Toolchain{Dir: fixture, ModuleDir: muxtDir(t)}, t.TempDir())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	out, err := goCommand(fixture, "test", "-count=1", "-cover", "-mod=mod", "-modfile="+files.ModFile, "-overlay="+files.Overlay, ".")
	if err != nil {
		t.Fatalf("go test with the overlay: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(fixture, "go.mod")); got != goMod {
		t.Errorf("the fixture's go.mod changed:\n%s", got)
	}
	if got := readFile(t, filepath.Join(fixture, "go.sum")); got != goSum {
		t.Errorf("the fixture's go.sum changed:\n%s", got)
	}
	target, used, err := files.GoLines()
	if err != nil {
		t.Fatalf("GoLines: %v", err)
	}
	if target != "1.27" || used != target {
		t.Errorf("GoLines = %q, %q; want the fixture's go 1.27 kept in the modfile", target, used)
	}
}

// TestGoLinesSeesARaise states that GoLines reads the test target's go
// line and the modfile copy's as the go command left it: a target below
// muxt's go line is raised to it in the copy.
func TestGoLinesSeesARaise(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list with the modfile copy")
	}
	realGOROOT(t)
	target := writeTarget(t, map[string]string{"go.mod": "module example.com/target\n\ngo 1.22\n", "a.go": "package a\n"})
	files, err := overlay.Write(context.Background(), overlay.Toolchain{Dir: target, ModuleDir: muxtDir(t)}, t.TempDir())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if out, err := goCommand(target, "list", "-mod=mod", "-modfile="+files.ModFile, "-overlay="+files.Overlay, "html/template"); err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	before, after, err := files.GoLines()
	if err != nil {
		t.Fatalf("GoLines: %v", err)
	}
	if before != "1.22" || after == "1.22" {
		t.Errorf("GoLines = %q, %q; want 1.22 raised in the copy", before, after)
	}
}

// TestUpgradesSeesARaise states that Upgrades reads what the go command
// did to the modfile copy: a target requiring an older golang.org/x/net
// than muxt does has it raised to muxt's in the copy once go test has
// built a package that imports html/template. go list does not write the
// raise; go test does.
func TestUpgradesSeesARaise(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test with the modfile copy")
	}
	realGOROOT(t)
	target := writeTarget(t, map[string]string{
		"go.mod":    "module example.com/target\n\ngo 1.27\n\nrequire golang.org/x/net v0.30.0\n",
		"a.go":      "package a\n\nimport \"html/template\"\n\nvar T = template.New(\"a\")\n",
		"a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) { _ = T }\n",
	})
	files, err := overlay.Write(context.Background(), overlay.Toolchain{Dir: target, ModuleDir: muxtDir(t)}, t.TempDir())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got, err := files.Upgrades(); err != nil || len(got) != 0 {
		t.Fatalf("Upgrades before go runs = %v, %v; want none", got, err)
	}
	if out, err := goCommand(target, "test", "-count=1", "-mod=mod", "-modfile="+files.ModFile, "-overlay="+files.Overlay, "./..."); err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
	got, err := files.Upgrades()
	if err != nil {
		t.Fatalf("Upgrades: %v", err)
	}
	if len(got) != 1 || got[0].Path != "golang.org/x/net" || got[0].From != "v0.30.0" || got[0].To == "v0.30.0" {
		t.Errorf("Upgrades = %+v; want golang.org/x/net raised from v0.30.0", got)
	}
}

// TestUpgrades states which requirements Upgrades reports: the ones both
// go.mod files have, raised in the copy, in the target's order, and never
// muxt's own, which the copy sets.
func TestUpgrades(t *testing.T) {
	for _, tt := range []struct {
		name         string
		target, copy string
		want         []overlay.Upgrade
	}{
		{
			name:   "none raised",
			target: "module m\n\ngo 1.27\n\nrequire example.com/a v1.0.0\n",
			copy:   "module m\n\ngo 1.27\n\nrequire example.com/a v1.0.0\n",
		},
		{
			name:   "raised, in the target's order",
			target: "module m\n\ngo 1.27\n\nrequire (\n\texample.com/b v1.0.0\n\texample.com/a v1.0.0\n\texample.com/c v1.2.0\n)\n",
			copy:   "module m\n\ngo 1.27\n\nrequire (\n\texample.com/a v1.1.0\n\texample.com/b v2.0.0+incompatible\n\texample.com/c v1.2.0\n)\n",
			want: []overlay.Upgrade{
				{Path: "example.com/b", From: "v1.0.0", To: "v2.0.0+incompatible"},
				{Path: "example.com/a", From: "v1.0.0", To: "v1.1.0"},
			},
		},
		{
			name:   "an indirect requirement",
			target: "module m\n\ngo 1.27\n\nrequire example.com/a v1.0.0 // indirect\n",
			copy:   "module m\n\ngo 1.27\n\nrequire example.com/a v1.0.1 // indirect\n",
			want:   []overlay.Upgrade{{Path: "example.com/a", From: "v1.0.0", To: "v1.0.1"}},
		},
		{
			name:   "only in the copy",
			target: "module m\n\ngo 1.27\n",
			copy:   "module m\n\ngo 1.27\n\nrequire example.com/a v1.0.0\n",
		},
		{
			name:   "muxt's own requirement",
			target: "module m\n\ngo 1.27\n\nrequire github.com/typelate/muxt v0.18.0\n",
			copy:   "module m\n\ngo 1.27\n\nrequire github.com/typelate/muxt v0.19.0\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			files := overlay.Files{Target: filepath.Join(dir, "target.mod"), ModFile: filepath.Join(dir, "copy.mod")}
			writeFile(t, files.Target, tt.target)
			writeFile(t, files.ModFile, tt.copy)
			got, err := files.Upgrades()
			if err != nil {
				t.Fatalf("Upgrades: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Upgrades = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGOROOT(t *testing.T) {
	want := realGOROOT(t)
	got, err := overlay.GOROOT(context.Background(), overlay.Toolchain{})
	if err != nil {
		t.Fatalf("GOROOT: %v", err)
	}
	if got != want {
		t.Errorf("GOROOT = %q, want %q", got, want)
	}
}

// TestProbe probes a module that requires muxt, replaced with this
// repository, and muxt itself, which requires nothing.
func TestProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list with a GOROOT overlay")
	}
	realGOROOT(t)
	target := writeTarget(t, map[string]string{"go.mod": targetGoMod})
	for name, tc := range map[string]overlay.Toolchain{
		"a module":    {Dir: target, Env: append(os.Environ(), "GOWORK=off", "MUXT_MODULE_DIR="+muxtDir(t))},
		"muxt itself": {},
	} {
		t.Run(name, func(t *testing.T) {
			if err := overlay.Probe(context.Background(), tc); err != nil {
				t.Fatalf("Probe: %v", err)
			}
		})
	}
}

// TestProbeReportsARefusedOverlay states that a refusal carries the go
// command's own reason, with advice only where it fits that reason.
func TestProbeReportsARefusedOverlay(t *testing.T) {
	for _, tt := range []struct {
		name, stderr string
		want         []string
		not          []string
	}{
		{
			name:   "an overlay refused",
			stderr: "go: overlay contains a replacement for GOROOT",
			want:   []string{"refuses an overlay", "html/template", "go: overlay contains a replacement for GOROOT"},
			not:    []string{"GOTOOLCHAIN=local", "go.work"},
		},
		{
			name:   "a toolchain in the module cache",
			stderr: "go: overlay contains a replacement for /m/golang.org/toolchain@v0.0.1-go1.27.0/src/html/template/a.go; Files beneath GOMODCACHE must not be replaced",
			want:   []string{"refuses an overlay", "Files beneath GOMODCACHE must not be replaced", "(try GOTOOLCHAIN=local)"},
		},
		{
			name:   "a workspace",
			stderr: "go: -modfile cannot be used in workspace mode",
			want:   []string{"go.work is in use", "go: -modfile cannot be used in workspace mode"},
			not:    []string{"GOTOOLCHAIN=local"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			goBin := fakeGo(t, t.TempDir(), t.TempDir(), "echo '"+tt.stderr+"' >&2; exit 1")
			err := overlay.Probe(context.Background(), overlay.Toolchain{Go: goBin, ModuleDir: "/src/muxt"})
			if err == nil {
				t.Fatal("Probe succeeded against a go command that refuses the overlay")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Probe error %q does not say %q", err, want)
				}
			}
			for _, not := range tt.not {
				if strings.Contains(err.Error(), not) {
					t.Errorf("Probe error %q says %q", err, not)
				}
			}
		})
	}
}

func TestProbeReportsAnIgnoredOverlay(t *testing.T) {
	goroot := t.TempDir()
	goBin := fakeGo(t, goroot, t.TempDir(), `echo template.go`)

	if err := overlay.Probe(context.Background(), overlay.Toolchain{Go: goBin, ModuleDir: "/src/muxt"}); err == nil {
		t.Fatal("Probe succeeded though go list did not see the probe file")
	}
}

// TestToolchainRunsInItsDirAndEnv states that the go command runs where
// and how the toolchain says: the directory's go.mod and the environment
// both decide which toolchain, and so which GOROOT, is used.
func TestToolchainRunsInItsDirAndEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake go command is a shell script")
	}
	dir := t.TempDir()
	goroot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(goroot, "src", "html", "template"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
[ "$(pwd -P)" = "$(cd "$WANT_DIR" && pwd -P)" ] || { echo "ran in $(pwd), want $WANT_DIR" >&2; exit 3; }
[ "$PWD" = "$WANT_DIR" ] || { echo "PWD=$PWD, want $WANT_DIR" >&2; exit 3; }
printf '%s\n%s\n' "$FAKE_GOROOT" "$WANT_DIR/modcache"
`
	goBin := filepath.Join(t.TempDir(), "go")
	if err := os.WriteFile(goBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	tc := overlay.Toolchain{Go: goBin, Dir: dir, Env: []string{"WANT_DIR=" + dir, "FAKE_GOROOT=" + goroot}}
	got, err := overlay.GOROOT(context.Background(), tc)
	if err != nil {
		t.Fatalf("GOROOT: %v", err)
	}
	if got != goroot {
		t.Errorf("GOROOT = %q, want %q", got, goroot)
	}
}

func TestGOROOTUnderGOMODCACHE(t *testing.T) {
	modcache := t.TempDir()
	goroot := filepath.Join(modcache, "golang.org", "toolchain@v0.0.1-go1.27.0.darwin-arm64")
	goBin := fakeGo(t, goroot, modcache, `exit 1`)
	ctx := context.Background()

	if _, err := overlay.GOROOT(ctx, overlay.Toolchain{Go: goBin}); err == nil || !strings.Contains(err.Error(), "GOMODCACHE") {
		t.Errorf("GOROOT error = %v, want one naming GOMODCACHE", err)
	}
	if _, err := overlay.Write(ctx, overlay.Toolchain{Go: goBin}, t.TempDir()); err == nil {
		t.Error("Write succeeded with GOROOT under GOMODCACHE")
	}
	if err := overlay.Probe(ctx, overlay.Toolchain{Go: goBin}); err == nil {
		t.Error("Probe succeeded with GOROOT under GOMODCACHE")
	}
}

// realGOROOT is go env GOROOT, skipping the test when there is no
// html/template source to overlay.
func realGOROOT(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Skipf("go env GOROOT: %v", err)
	}
	goroot := strings.TrimSpace(string(out))
	if goroot == "" {
		t.Skip("go env GOROOT is empty")
	}
	if _, err := os.Stat(filepath.Join(goroot, "src", "html", "template")); err != nil {
		t.Skipf("no html/template source under GOROOT: %v", err)
	}
	return goroot
}

// fakeGo writes a go command that reports goroot, modcache and a go.mod
// for go env and runs list for go list.
func fakeGo(t *testing.T, goroot, modcache, list string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake go command is a shell script")
	}
	writeFile(t, filepath.Join(goroot, "src", "html", "template", "template.go"), "package template\n")
	goMod := filepath.Join(t.TempDir(), "go.mod")
	writeFile(t, goMod, targetGoMod)
	script := `#!/bin/sh
case "$1" in
env) if [ "$2" = GOMOD ]; then echo '` + goMod + `'; else printf '%s\n%s\n' '` + goroot + `' '` + modcache + `'; fi ;;
list) ` + list + ` ;;
*) exit 2 ;;
esac
`
	goBin := filepath.Join(t.TempDir(), "go")
	if err := os.WriteFile(goBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return goBin
}

// goCommand runs the go command in dir with the local toolchain and no
// workspace, and returns what it printed.
func goCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "PWD="+dir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

// stdSourceFiles lists the non-test Go files in dir on disk.
func stdSourceFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if name := e.Name(); !e.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			names = append(names, name)
		}
	}
	return names
}

// muxtDir is this repository's directory, which replaces
// github.com/typelate/muxt in a test's modfile so that nothing is fetched.
func muxtDir(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/typelate/muxt").Output()
	if err != nil {
		t.Fatalf("go list -m github.com/typelate/muxt: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
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
