package overlay_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/overlay"
)

func TestWriteOverlay(t *testing.T) {
	ctx := context.Background()
	goroot := realGOROOT(t)
	stdDir := filepath.Join(goroot, "src", "html", "template")
	scratch := t.TempDir()

	jsonPath, err := overlay.Write(ctx, overlay.Toolchain{}, scratch)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	b, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Replace map[string]string }
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("overlay JSON %s: %v", jsonPath, err)
	}

	for key := range got.Replace {
		if rel, err := filepath.Rel(stdDir, key); err != nil || !filepath.IsLocal(rel) {
			t.Errorf("overlay replaces %s, outside %s", key, stdDir)
		}
	}

	engine := overlay.Engine()
	entries, err := os.ReadDir(stdDir)
	if err != nil {
		t.Fatal(err)
	}
	var deletes int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		key := filepath.Join(stdDir, name)
		backing, ok := got.Replace[key]
		if !ok {
			t.Errorf("overlay leaves %s in place", key)
			continue
		}
		if _, err := fs.Stat(engine, name); err == nil {
			continue // the engine's file of the same name takes its place
		}
		if backing != "" {
			t.Errorf("overlay backs %s with %q, want \"\" (deleted)", key, backing)
		}
		deletes++
	}
	if deletes == 0 {
		t.Errorf("overlay deletes none of %s", stdDir)
	}

	var adds int
	err = fs.WalkDir(engine, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		adds++
		key := filepath.Join(stdDir, filepath.FromSlash(name))
		backing := got.Replace[key]
		if backing == "" {
			t.Errorf("overlay does not add %s", key)
			return nil
		}
		if rel, err := filepath.Rel(scratch, backing); err != nil || !filepath.IsLocal(rel) {
			t.Errorf("overlay backs %s with %s, outside the scratch dir %s", key, backing, scratch)
		}
		want, err := fs.ReadFile(engine, name)
		if err != nil {
			return err
		}
		have, err := os.ReadFile(backing)
		if err != nil {
			t.Errorf("backing file of %s: %v", key, err)
			return nil
		}
		if !bytes.Equal(have, want) {
			t.Errorf("backing file %s differs from the engine's %s", backing, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if adds == 0 {
		t.Fatal("the engine has no files")
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

func TestProbe(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list with a GOROOT overlay")
	}
	realGOROOT(t)
	if err := overlay.Probe(context.Background(), overlay.Toolchain{}); err != nil {
		t.Fatalf("Probe: %v", err)
	}
}

func TestProbeReportsARefusedOverlay(t *testing.T) {
	goroot := t.TempDir()
	goBin := fakeGo(t, goroot, t.TempDir(), `echo "go: overlay contains a replacement for GOROOT" >&2; exit 1`)

	err := overlay.Probe(context.Background(), overlay.Toolchain{Go: goBin})
	if err == nil {
		t.Fatal("Probe succeeded against a go command that refuses the overlay")
	}
	for _, want := range []string{"overlay", "html/template", "go: overlay contains a replacement for GOROOT", "GOTOOLCHAIN=local"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Probe error %q does not mention %q", err, want)
		}
	}
}

func TestProbeReportsAnIgnoredOverlay(t *testing.T) {
	goroot := t.TempDir()
	goBin := fakeGo(t, goroot, t.TempDir(), `echo template.go`)

	if err := overlay.Probe(context.Background(), overlay.Toolchain{Go: goBin}); err == nil {
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

func TestOverlayBuildsFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("builds html/template from the overlaid engine with the go command")
	}
	realGOROOT(t)
	jsonPath, err := overlay.Write(context.Background(), overlay.Toolchain{}, t.TempDir())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	fixture, err := filepath.Abs(filepath.Join("testdata", "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	runGo(t, fixture, "test", "-count=1", "-vet=off", "-overlay="+jsonPath, ".")
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

// fakeGo writes a go command that reports goroot and modcache for go env
// and runs list for go list.
func fakeGo(t *testing.T, goroot, modcache, list string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake go command is a shell script")
	}
	if err := os.MkdirAll(filepath.Join(goroot, "src", "html", "template"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goroot, "src", "html", "template", "template.go"), []byte("package template\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$1" in
env) printf '%s\n%s\n' '` + goroot + `' '` + modcache + `' ;;
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

func runGo(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS="+os.Getenv("GOFLAGS"), "PWD="+dir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out.Bytes())
	}
}
