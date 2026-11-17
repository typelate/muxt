package templatecover

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/typelate/htt/template"

	"github.com/typelate/muxt/internal/overlay"
)

// Engine is muxt's template engine written for one test target. CoverDir is
// absolute: each test binary writes <pid>.cov there.
type Engine struct {
	Files    overlay.Files
	CoverDir string
}

// UnavailableError is an engine that could not be put in place: the
// toolchain could not be resolved or probed, or the files could not be
// written. Error is Err's, byte for byte, so a caller that reports the
// reason as it always has keeps its message.
//
// Resolving marks Resolve and Probe, the steps the go command can be
// interrupted in; a caller that was cancelled reports the cancellation
// rather than the refusal it caused.
type UnavailableError struct {
	Err       error
	Resolving bool
}

func (e *UnavailableError) Error() string { return e.Err.Error() }

func (e *UnavailableError) Unwrap() error { return e.Err }

// Prepare resolves tc, probes it and writes the engine under dir, with
// CoverDir at dir/profiles. The toolchain is asked for its GOROOT and the
// target's go.mod once, for both the probe and the files.
//
// An error that is not an *UnavailableError is the file system's:
// filepath.Abs or os.MkdirAll.
func Prepare(ctx context.Context, tc overlay.Toolchain, dir string) (Engine, error) {
	target, err := overlay.Resolve(ctx, tc)
	if err == nil {
		err = overlay.Probe(ctx, target)
	}
	if err != nil {
		return Engine{}, &UnavailableError{Err: err, Resolving: true}
	}
	// The overlay names the shim by its path verbatim, so it is absolute.
	dir, err = filepath.Abs(dir)
	if err != nil {
		return Engine{}, err
	}
	coverDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(coverDir, 0o700); err != nil {
		return Engine{}, err
	}
	files, err := overlay.Write(target, dir)
	if err != nil {
		return Engine{}, &UnavailableError{Err: err}
	}
	return Engine{Files: files, CoverDir: coverDir}, nil
}

// Flags are the go test flags that put the engine in place: the modfile
// requiring github.com/typelate/htt, which the shim imports, with -mod=mod
// to let the go command add the sums its requirements need to the modfile's
// go.sum, never the test target's, and the overlay putting the shim in
// place of html/template.
func (e Engine) Flags() []string {
	return []string{"-mod=mod", "-modfile=" + e.Files.ModFile, "-overlay=" + e.Files.Overlay}
}

// Environ is env, or the process's own environment when env is nil, with
// the variable naming the directory the test binaries write their profiles
// to.
func (e Engine) Environ(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	return append(slices.Clip(env), template.CoverEnv+"="+e.CoverDir)
}
