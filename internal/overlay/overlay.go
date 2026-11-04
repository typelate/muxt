package overlay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GOROOT is go env GOROOT of goBin, exactly as it prints it. Overlay keys
// must be the paths the go command itself uses, so symbolic links are not
// resolved.
//
// A GOROOT under GOMODCACHE is an error: that is where a toolchain the go
// command downloaded lives, and it refuses to overlay files there.
func GOROOT(ctx context.Context, goBin string) (string, error) {
	out, err := goOutput(ctx, goBin, "env", "GOROOT", "GOMODCACHE")
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	if len(lines) != 2 {
		return "", fmt.Errorf("go env GOROOT GOMODCACHE printed %q, want two lines", out)
	}
	goroot, modcache := strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	if goroot == "" {
		return "", errors.New("go env GOROOT is empty")
	}
	if modcache != "" {
		if rel, err := filepath.Rel(modcache, goroot); err == nil && filepath.IsLocal(rel) {
			return "", fmt.Errorf("GOROOT %s is under GOMODCACHE %s, where the go command refuses overlays; set GOTOOLCHAIN=local or install the toolchain outside the module cache", goroot, modcache)
		}
	}
	return goroot, nil
}

// Write copies the engine into scratchDir and writes, beside it, the
// overlay JSON that makes goBin build the engine as html/template. It
// returns the JSON's path, for go build -overlay.
//
// The overlay deletes every non-test Go file of $GOROOT/src/html/template,
// as listed on disk now, and adds each engine file at its path under that
// directory. Packages the engine adds under GOROOT have no directory on
// disk, so go test must run with -vet=off.
func Write(ctx context.Context, goBin, scratchDir string) (string, error) {
	goroot, err := GOROOT(ctx, goBin)
	if err != nil {
		return "", err
	}
	stdDir := filepath.Join(goroot, "src", "html", "template")
	entries, err := os.ReadDir(stdDir)
	if err != nil {
		return "", fmt.Errorf("reading the html/template source: %w", err)
	}
	replace := make(map[string]string)
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			replace[filepath.Join(stdDir, name)] = "" // an empty backing path deletes the file
		}
	}

	engineDir := filepath.Join(scratchDir, "engine")
	engine := Engine()
	err = fs.WalkDir(engine, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		src, err := fs.ReadFile(engine, name)
		if err != nil {
			return err
		}
		local := filepath.Join(engineDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(local, src, 0o644); err != nil {
			return err
		}
		replace[filepath.Join(stdDir, filepath.FromSlash(name))] = local
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("writing the engine to %s: %w", engineDir, err)
	}

	jsonPath := filepath.Join(scratchDir, "overlay.json")
	if err := writeOverlay(jsonPath, replace); err != nil {
		return "", err
	}
	return jsonPath, nil
}

// probeFile is the file Probe adds to html/template.
const probeFile = "muxt_overlay_probe.go"

// Probe checks that goBin honors an overlay of GOROOT's html/template: it
// adds a one-line file to the package and asks go list whether the package
// has it. An error means the engine cannot replace html/template with
// this toolchain.
func Probe(ctx context.Context, goBin string) (err error) {
	goroot, err := GOROOT(ctx, goBin)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "muxt-overlay-probe-")
	if err != nil {
		return fmt.Errorf("making the overlay probe's directory: %w", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("removing the overlay probe's directory: %w", rmErr))
		}
	}()

	local := filepath.Join(dir, probeFile)
	if err := os.WriteFile(local, []byte("package template // muxt overlay probe\n"), 0o644); err != nil {
		return fmt.Errorf("writing the overlay probe: %w", err)
	}
	jsonPath := filepath.Join(dir, "overlay.json")
	key := filepath.Join(goroot, "src", "html", "template", probeFile)
	if err := writeOverlay(jsonPath, map[string]string{key: local}); err != nil {
		return err
	}
	out, err := goOutput(ctx, goBin, "list", "-overlay="+jsonPath, "-f", "{{range .GoFiles}}{{println .}}{{end}}", "html/template")
	if err != nil {
		return fmt.Errorf("the go command refuses an overlay of GOROOT's html/template: %w", err)
	}
	for _, name := range strings.Fields(string(out)) {
		if name == probeFile {
			return nil
		}
	}
	return fmt.Errorf("the go command ignores an overlay of GOROOT's html/template: go list html/template does not include %s", key)
}

func writeOverlay(jsonPath string, replace map[string]string) error {
	b, err := json.MarshalIndent(struct{ Replace map[string]string }{replace}, "", "\t")
	if err != nil {
		return fmt.Errorf("encoding the overlay: %w", err)
	}
	if err := os.WriteFile(jsonPath, b, 0o644); err != nil {
		return fmt.Errorf("writing the overlay: %w", err)
	}
	return nil
}

// goOutput runs goBin with args and returns its standard output. An error
// carries what it printed to standard error.
func goOutput(ctx context.Context, goBin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, goBin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", goBin, strings.Join(args, " "), err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out, nil
}
