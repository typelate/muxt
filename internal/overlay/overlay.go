// Package overlay writes the files that make the go command build a test
// target with muxt's template engine in place of the standard library's
// html/template: a shim of html/template that forwards to
// github.com/typelate/muxt/template/html, an overlay putting the shim in
// html/template's place, and a copy of the target's go.mod that requires
// github.com/typelate/muxt.
package overlay

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// Module is muxt's module path, which the modfile copy requires.
const Module = "github.com/typelate/muxt"

// ModuleDirEnv names the environment variable holding a directory of
// muxt's module source, which the modfile copy then replaces Module with.
// A development build of muxt has no published version to require, so it
// needs one.
const ModuleDirEnv = "MUXT_MODULE_DIR"

// PlaceholderVersion is the version Module is required at when a module
// directory replaces it and muxt's own version is unknown.
const PlaceholderVersion = "v0.0.0-00010101000000-000000000000"

// shim is html/template's API forwarded to Module's template/html, which
// the overlay puts in place of html/template's own files.
//
//go:embed shim/shim.go
var shim []byte

// shimFile is the shim's name in html/template.
const shimFile = "muxt_shim.go"

// Toolchain is the go command as a caller runs it. The directory matters
// because its go.mod can select another toolchain, and the environment
// because GOTOOLCHAIN and GOROOT can; either changes which GOROOT an
// overlay must be keyed on. A caller passes the ones its go test uses.
type Toolchain struct {
	// Go is the go command, "go" when empty.
	Go string

	// Dir is the directory the go command runs in, the process's own
	// when empty.
	Dir string

	// Env is the go command's environment, nil for the process's own.
	Env []string

	// ModuleDir is a directory of muxt's module source to replace Module
	// with. When empty, ModuleDirEnv in Env says, and when that is
	// empty too, nothing replaces Module. A relative directory is
	// relative to Dir.
	ModuleDir string
}

func (tc Toolchain) goBin() string {
	if tc.Go == "" {
		return "go"
	}
	return tc.Go
}

// GOROOT is go env GOROOT of the toolchain, exactly as it prints it. Overlay keys
// must be the paths the go command itself uses, so symbolic links are not
// resolved.
//
// A GOROOT under GOMODCACHE is an error: that is where a toolchain the go
// command downloaded lives, and it refuses to overlay files there.
func GOROOT(ctx context.Context, tc Toolchain) (string, error) {
	out, err := goOutput(ctx, tc, "env", "GOROOT", "GOMODCACHE")
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

// Files are the paths Write writes that go test needs, and what the
// modfile copy says about Module.
type Files struct {
	// Overlay is the overlay JSON, for go test -overlay. It replaces
	// $GOROOT/src/html/template with the shim.
	Overlay string

	// ModFile is the test target's go.mod with Module required, for go
	// test -modfile. Its go.sum is beside it.
	ModFile string

	// Target is the test target's go.mod, which ModFile copies.
	Target string

	// Version is the version of Module that ModFile requires, "" when
	// the test target is Module itself and requires nothing.
	Version string

	// ModuleDir is the directory ModFile replaces Module with, "" for
	// none.
	ModuleDir string

	// TargetVersion is the version of Module the test target's go.mod
	// requires, when it requires one other than Version, which ModFile
	// requires in its place; "" otherwise.
	TargetVersion string
}

// GoLines returns the go line of the test target's go.mod and that of the
// modfile copy, as they are now. The go command raises the copy's when a
// requirement, muxt among them, needs a newer one, and the
// pre-pass then compiles the target at that language version. A go.mod
// without a go line gives "".
func (f Files) GoLines() (target, used string, err error) {
	if target, err = goLine(f.Target); err != nil {
		return "", "", err
	}
	if used, err = goLine(f.ModFile); err != nil {
		return "", "", err
	}
	return target, used, nil
}

// Upgrade is a module the test target's go.mod requires that the modfile
// copy requires at a newer version.
type Upgrade struct {
	Path, From, To string
}

// Upgrades returns the modules the test target's go.mod requires that the
// modfile copy, as it is now, requires at a newer version, in the
// target's order. Module requirements meet in the copy: the go command
// raises the target's to the versions Module needs, and the pre-pass then
// builds the target's tests against those. Module itself is left out;
// the copy requires it at the version Write chose.
func (f Files) Upgrades() ([]Upgrade, error) {
	target, err := requires(f.Target)
	if err != nil {
		return nil, err
	}
	used, err := requires(f.ModFile)
	if err != nil {
		return nil, err
	}
	versions := make(map[string]string, len(used))
	for _, r := range used {
		versions[r.Path] = r.Version
	}
	var upgrades []Upgrade
	for _, r := range target {
		to, ok := versions[r.Path]
		if r.Path == Module || !ok || semver.Compare(to, r.Version) <= 0 {
			continue
		}
		upgrades = append(upgrades, Upgrade{Path: r.Path, From: r.Version, To: to})
	}
	return upgrades, nil
}

// requires is the modules a go.mod requires, in its order.
func requires(goModPath string) ([]module.Version, error) {
	b, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("reading the requirements: %w", err)
	}
	f, err := modfile.ParseLax(goModPath, b, nil)
	if err != nil {
		return nil, fmt.Errorf("reading the requirements: %w", err)
	}
	mods := make([]module.Version, len(f.Require))
	for i, r := range f.Require {
		mods[i] = r.Mod
	}
	return mods, nil
}

// goLine is the version of a go.mod's go directive, "" when it has none.
func goLine(goModPath string) (string, error) {
	b, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("reading the go line: %w", err)
	}
	for line := range strings.Lines(string(b)) {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "go" {
			return fields[1], nil
		}
	}
	return "", nil
}

// Target is a test target as its toolchain resolves it: the GOROOT whose
// html/template the overlay replaces, the go.mod the modfile copies, and
// how the copy requires Module. Resolve makes one, and Probe and Write
// share it, so the go command is asked once.
type Target struct {
	tc     Toolchain
	goroot string
	goMod  string
	req    requirement
}

// Resolve asks the toolchain for its GOROOT and the test target's go.mod,
// and decides how the modfile copy requires Module.
//
// The test target's go.mod is the one go env GOMOD names in the
// toolchain's directory. When it is Module's own, the copy requires
// nothing. Otherwise, a muxt whose version is unknown, or a build with
// uncommitted changes, is an error unless a module directory is named.
func Resolve(ctx context.Context, tc Toolchain) (Target, error) {
	goroot, err := GOROOT(ctx, tc)
	if err != nil {
		return Target{}, err
	}
	targetMod, err := goMod(ctx, tc)
	if err != nil {
		return Target{}, err
	}
	req, err := requirementFor(tc, targetMod)
	if err != nil {
		return Target{}, err
	}
	return Target{tc: tc, goroot: goroot, goMod: targetMod, req: req}, nil
}

// Write writes under scratchDir the files go test needs to build the test
// target with muxt's template engine in place of html/template:
//
//   - html-template/muxt_shim.go: the shim, which forwards html/template's
//     API to Module's template/html;
//   - overlay.json: deletes every non-test Go file of
//     $GOROOT/src/html/template, as listed on disk now, and adds the shim
//     there;
//   - go.mod and go.sum: copies of the test target's, with Module required
//     as target says.
//
// The overlay names files by their paths under scratchDir as given, so
// scratchDir should be absolute; the caller removes it.
func Write(target Target, scratchDir string) (Files, error) {
	goroot, targetMod, req := target.goroot, target.goMod, target.req
	stdDir := filepath.Join(goroot, "src", "html", "template")
	replace, err := deleteStd(stdDir)
	if err != nil {
		return Files{}, err
	}

	files := Files{
		Overlay:   filepath.Join(scratchDir, "overlay.json"),
		ModFile:   filepath.Join(scratchDir, "go.mod"),
		Target:    targetMod,
		Version:   req.Version,
		ModuleDir: req.Dir,
	}
	local := filepath.Join(scratchDir, "html-template", shimFile)
	if err := writeFile(local, shim); err != nil {
		return Files{}, fmt.Errorf("writing the html/template shim: %w", err)
	}
	replace[filepath.Join(stdDir, shimFile)] = local
	if files.TargetVersion, err = writeModFile(targetMod, files.ModFile, req); err != nil {
		return Files{}, err
	}
	if err := writeOverlay(files.Overlay, replace); err != nil {
		return Files{}, err
	}
	return files, nil
}

// deleteStd returns an overlay deleting every non-test Go file in stdDir.
func deleteStd(stdDir string) (map[string]string, error) {
	entries, err := os.ReadDir(stdDir)
	if err != nil {
		return nil, fmt.Errorf("reading the html/template source: %w", err)
	}
	replace := make(map[string]string)
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			replace[filepath.Join(stdDir, name)] = "" // an empty backing path deletes the file
		}
	}
	return replace, nil
}

// goMod is the path of the test target's go.mod.
func goMod(ctx context.Context, tc Toolchain) (string, error) {
	out, err := goOutput(ctx, tc, "env", "GOMOD")
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(out))
	if name == "" || name == os.DevNull {
		return "", errors.New("the tests are not in a module: muxt's template engine is required in a copy of the test target's go.mod, and there is none")
	}
	return name, nil
}

// requirement is how the modfile copy requires Module.
type requirement struct {
	// Version is the version required; "" for none.
	Version string

	// Dir is the directory that replaces Module, absolute; "" for none.
	Dir string
}

// requirementFor is how the modfile copy of targetMod requires Module:
// at this muxt's version, replaced by the toolchain's module directory if
// it names one, and not at all if targetMod is Module's own.
func requirementFor(tc Toolchain, targetMod string) (requirement, error) {
	if self, err := isModule(targetMod); err != nil || self {
		return requirement{}, err
	}
	dir, err := tc.moduleDir()
	if err != nil {
		return requirement{}, err
	}
	info, ok := debug.ReadBuildInfo()
	version, err := moduleVersion(info, ok)
	if dir == "" {
		return requirement{Version: version}, err
	}
	if err != nil {
		version = PlaceholderVersion
	}
	return requirement{Version: version, Dir: dir}, nil
}

// moduleDir is the absolute module directory the toolchain names, "" for
// none.
func (tc Toolchain) moduleDir() (string, error) {
	dir := tc.ModuleDir
	if dir == "" {
		dir = tc.getenv(ModuleDirEnv)
	}
	if dir == "" {
		return "", nil
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(tc.Dir, dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("%s: %w", ModuleDirEnv, err)
	}
	return abs, nil
}

// getenv is the value of key in the go command's environment.
func (tc Toolchain) getenv(key string) string {
	if tc.Env == nil {
		return os.Getenv(key)
	}
	value := ""
	for _, kv := range tc.Env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value = v // the last one wins, as in os/exec
		}
	}
	return value
}

// errModified is a muxt that cannot be required by version, since its
// source is not that of any commit.
var errModified = errors.New("muxt is a modified development build; set " + ModuleDirEnv + " to its source directory")

// moduleVersion is the version of Module this muxt was built from, given
// its build information, as debug.ReadBuildInfo returns it: muxt's own
// version when it is the main module, its version as a dependency when it
// is another module's tool, and when it is a development build of a
// commit, a pseudo-version of that commit. A build with uncommitted
// changes, or without version control information, has none.
//
// A dependency replaced by a directory reports the version required; the
// replace in the test target's go.mod, if it is that module, applies to
// the copy too.
func moduleVersion(info *debug.BuildInfo, ok bool) (string, error) {
	if !ok || info == nil {
		return "", errors.New("muxt has no build information; set " + ModuleDirEnv + " to its source directory")
	}
	m := info.Main
	if m.Path != Module {
		for _, dep := range info.Deps {
			if dep.Path == Module {
				m = *dep
				break
			}
		}
	}
	if m.Path != Module {
		return "", errors.New("muxt's build information does not name " + Module + "; set " + ModuleDirEnv + " to its source directory")
	}
	switch {
	case strings.HasSuffix(m.Version, "+dirty"):
		return "", errModified
	case semver.IsValid(m.Version):
		return m.Version, nil
	}

	settings := make(map[string]string)
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	revision, stamp := settings["vcs.revision"], settings["vcs.time"]
	switch {
	case settings["vcs.modified"] == "true":
		return "", errModified
	case revision == "" || stamp == "" || settings["vcs.modified"] != "false":
		return "", errors.New("muxt is a development build without version control information; set " + ModuleDirEnv + " to its source directory")
	}
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return "", fmt.Errorf("muxt's vcs.time %q: %w; set %s to its source directory", stamp, err, ModuleDirEnv)
	}
	return module.PseudoVersion("", "", t.UTC(), revision[:min(12, len(revision))]), nil
}

// isModule reports whether the go.mod at goModPath is Module's own.
func isModule(goModPath string) (bool, error) {
	b, err := os.ReadFile(goModPath)
	if err != nil {
		return false, fmt.Errorf("reading the test target's go.mod: %w", err)
	}
	return modfile.ModulePath(b) == Module, nil
}

// writeModFile writes to modFile the test target's go.mod, targetMod,
// with Module required as req says, and beside it a copy of the target's
// go.sum, if it has one. The target's own requirement of Module, if any,
// gives way to req's version; it returns that version when it differs,
// and "" otherwise. A targetMod that is Module's own is copied as it is.
//
// The go command resolves a relative replace against the target's
// directory, not the modfile's, so req.Dir must be absolute.
func writeModFile(targetMod, modFile string, req requirement) (string, error) {
	mod, err := os.ReadFile(targetMod)
	if err != nil {
		return "", fmt.Errorf("reading the test target's go.mod: %w", err)
	}
	targetVersion := ""
	if modfile.ModulePath(mod) != Module {
		f, err := modfile.Parse(targetMod, mod, nil)
		if err != nil {
			return "", fmt.Errorf("reading the test target's go.mod: %w", err)
		}
		for _, r := range f.Require {
			if r.Mod.Path == Module && r.Mod.Version != req.Version {
				targetVersion = r.Mod.Version
			}
		}
		if err := f.AddRequire(Module, req.Version); err != nil {
			return "", fmt.Errorf("requiring %s: %w", Module, err)
		}
		if req.Dir != "" {
			if err := f.AddReplace(Module, "", req.Dir, ""); err != nil {
				return "", fmt.Errorf("replacing %s: %w", Module, err)
			}
		}
		f.Cleanup()
		if mod, err = f.Format(); err != nil {
			return "", fmt.Errorf("writing the modfile: %w", err)
		}
	}
	if err := writeFile(modFile, mod); err != nil {
		return "", err
	}
	sum, err := os.ReadFile(strings.TrimSuffix(targetMod, ".mod") + ".sum")
	if errors.Is(err, fs.ErrNotExist) {
		return targetVersion, nil
	}
	if err != nil {
		return "", fmt.Errorf("reading the test target's go.sum: %w", err)
	}
	return targetVersion, writeFile(strings.TrimSuffix(modFile, ".mod")+".sum", sum)
}

func writeFile(name string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(name, content, 0o644)
}

// probeFile is the file Probe adds to html/template.
const probeFile = "muxt_overlay_probe.go"

// Probe checks that the toolchain honors an overlay of GOROOT's
// html/template that imports Module's template/html, as the shim does, in
// the test target's module with Module required as Write requires it: it
// adds a one-line file importing that package to html/template and asks
// go list whether the package has it. The go command fetches Module if it
// must. An error means the engine cannot replace html/template with this
// toolchain, in this module, or with this muxt.
func Probe(ctx context.Context, target Target) (err error) {
	tc, goroot, targetMod, req := target.tc, target.goroot, target.goMod, target.req
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
	if err := writeFile(local, []byte("package template // muxt overlay probe\n\nimport _ \""+Module+"/template/html\"\n")); err != nil {
		return fmt.Errorf("writing the overlay probe: %w", err)
	}
	modFile := filepath.Join(dir, "go.mod")
	if _, err := writeModFile(targetMod, modFile, req); err != nil {
		return err
	}
	jsonPath := filepath.Join(dir, "overlay.json")
	key := filepath.Join(goroot, "src", "html", "template", probeFile)
	if err := writeOverlay(jsonPath, map[string]string{key: local}); err != nil {
		return err
	}
	out, err := goOutput(ctx, tc, "list", "-mod=mod", "-modfile="+modFile, "-overlay="+jsonPath, "-f", "{{range .GoFiles}}{{println .}}{{end}}", "html/template")
	if err != nil {
		return probeRefusal(err, req)
	}
	for _, name := range strings.Fields(string(out)) {
		if name == probeFile {
			return nil
		}
	}
	return fmt.Errorf("the go command ignores an overlay of GOROOT's html/template: go list html/template does not include %s", key)
}

// probeRefusal says why the probe's go list failed: the go command's own
// reason, which is what it printed to standard error, with the advice that
// fits it. The go command says what it refuses only in that text.
func probeRefusal(err error, req requirement) error {
	reason := err.Error()
	var goErr *goCommandError
	if errors.As(err, &goErr) && goErr.stderr != "" {
		reason = goErr.stderr
	}
	switch {
	case strings.Contains(reason, "workspace mode"):
		return fmt.Errorf("go.work is in use, and the go command refuses the coverage pre-pass's -modfile in workspace mode: %s", reason)
	case req.Version != "" && req.Dir == "" && strings.Contains(reason, Module+"@"):
		return fmt.Errorf("the go command cannot get %s@%s, which the coverage pre-pass requires (it must be in the module cache or on the module proxy): %s", Module, req.Version, reason)
	case strings.Contains(strings.ToLower(reason), "toolchain"), strings.Contains(reason, "GOMODCACHE"):
		return fmt.Errorf("the go command refuses an overlay of GOROOT's html/template (try GOTOOLCHAIN=local): %s", reason)
	default:
		return fmt.Errorf("the go command refuses an overlay of GOROOT's html/template: %s", reason)
	}
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

// goOutput runs the toolchain's go command with args and returns its
// standard output. An error carries what it printed to standard error.
func goOutput(ctx context.Context, tc Toolchain, args ...string) ([]byte, error) {
	goBin := tc.goBin()
	cmd := exec.CommandContext(ctx, goBin, args...)
	cmd.Dir = tc.Dir
	if tc.Env != nil {
		cmd.Env = tc.Env
		if tc.Dir != "" {
			// os/exec points PWD at Dir only when it supplies the
			// environment itself, and the go command reads paths the
			// way PWD says.
			cmd.Env = append(slices.Clip(tc.Env), "PWD="+tc.Dir)
		}
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, &goCommandError{command: goBin + " " + strings.Join(args, " "), err: err, stderr: string(bytes.TrimSpace(stderr.Bytes()))}
	}
	return out, nil
}

// goCommandError is a go command that failed, with what it printed to
// standard error.
type goCommandError struct {
	command string
	err     error
	stderr  string
}

func (e *goCommandError) Error() string {
	return e.command + ": " + e.err.Error() + ": " + e.stderr
}

func (e *goCommandError) Unwrap() error { return e.err }
