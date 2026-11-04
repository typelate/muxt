package mutation

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/cover"

	"github.com/typelate/muxt/internal/overlay"
)

// coverage is the template blocks the tests executed in the coverage
// pre-pass, by the parse name of the template they are in, which is a
// file's base name: two files with one base name share a key.
//
// Nil means coverage is unknown, and nothing may be read from it; an
// empty coverage means the tests executed no template at all.
type coverage map[string]map[coverBlock]struct{}

// coverBlock is a block's span in its template, as a cover profile gives
// it.
type coverBlock struct {
	StartLine, StartCol, EndLine, EndCol int
}

// coverageRun runs the tests once with the engine overlay and the
// directory each test binary writes its profile to, as goTest.coverage.
type coverageRun func(ctx context.Context, overlay, coverDir string) (*goTestOutput, error)

// toolchain is the go command as the run's go test uses it, which is what
// decides the GOROOT the engine overlay replaces html/template under.
func (c Configuration) toolchain(dir string) overlay.Toolchain {
	return overlay.Toolchain{Go: "go", Dir: dir, Env: c.env}
}

// templateCoverage runs the coverage pre-pass: the tests once, unmutated,
// with muxt's template engine laid over html/template, and returns the
// union of the blocks every test binary reports executing. It writes under
// scratch, the run's own directory, which the caller removes.
//
// Coverage is something a run can do without, so a pre-pass that cannot
// tell -- a toolchain refusing the overlay, tests that do not build with
// the engine, a test binary that stopped exporting -- writes one warning
// line to warn and returns nil coverage, and the run goes on. So do tests
// that fail under the engine: kill runs use the real html/template, and
// the baseline after this is what says whether the tests pass.
func templateCoverage(ctx context.Context, tc overlay.Toolchain, scratch string, run coverageRun, warn io.Writer) (coverage, error) {
	if err := overlay.Probe(ctx, tc); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return coverageUnknown(warn, err.Error())
	}
	dir, err := filepath.Abs(filepath.Join(scratch, "coverage"))
	if err != nil {
		return nil, err
	}
	// The overlay names the engine's files by these paths verbatim, so
	// they are absolute.
	coverDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(coverDir, 0o700); err != nil {
		return nil, err
	}
	overlayPath, err := overlay.Write(ctx, tc, dir)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return coverageUnknown(warn, err.Error())
	}

	out, err := run(ctx, overlayPath, coverDir)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	switch {
	case err != nil && isTestFailure(err) && (out.testFailed || out.couldNotRun):
		// Kill runs use the real html/template, so this says nothing
		// about the verdicts; whether the tests pass is the baseline's
		// to say.
		return coverageUnknown(warn, engineFailure(out, err))
	case err != nil:
		return coverageUnknown(warn, "go test with muxt's template engine in place of html/template did not run: "+firstError(out.String(), err))
	case out.coverDisabled != "":
		return coverageUnknown(warn, out.coverDisabled)
	}
	cov, err := mergeProfiles(coverDir)
	if err != nil {
		return coverageUnknown(warn, err.Error())
	}
	return cov, nil
}

// engineFailure is why coverage is unknown when the tests fail, or do not
// build, with the engine in place: the first failure, and the engine's
// known gaps, which are the likely cause.
func engineFailure(out *goTestOutput, err error) string {
	first := firstError(out.String(), err)
	if !out.couldNotRun && len(out.failed) > 0 {
		first = "--- FAIL: " + out.failed[0]
	}
	return "tests fail under muxt's template engine (coverage pre-pass): " + first +
		"; known gaps: Template.Tree, AddParseTree, Error.Node"
}

// coverageUnknown writes why the run goes on without coverage, on one
// line, and returns no coverage.
func coverageUnknown(warn io.Writer, why string) (coverage, error) {
	if warn != nil {
		why = strings.Join(strings.Fields(why), " ")
		_, _ = fmt.Fprintf(warn, "warning: template coverage unknown: %s\n", why)
	}
	return nil, nil
}

// firstError is the first line of go test's output that says what went
// wrong, or err when no line does.
func firstError(out string, err error) string {
	for line := range strings.Lines(out) {
		line = strings.TrimSpace(line)
		switch {
		case line == "", strings.HasPrefix(line, "#"), strings.HasPrefix(line, "=== "),
			strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "ok "),
			strings.HasPrefix(line, "FAIL"), strings.HasPrefix(line, "PASS"):
			continue
		}
		return line
	}
	return err.Error()
}

// mergeProfiles unions every <pid>.cov in dir by block. A block is in the
// union when any test binary executed it.
func mergeProfiles(dir string) (coverage, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.cov"))
	if err != nil {
		return nil, err
	}
	cov := make(coverage)
	for _, name := range files {
		profiles, err := cover.ParseProfiles(name)
		if err != nil {
			return nil, fmt.Errorf("reading template coverage: %w", err)
		}
		for _, p := range profiles {
			for _, b := range p.Blocks {
				if b.Count == 0 {
					continue
				}
				blocks := cov[p.FileName]
				if blocks == nil {
					blocks = make(map[coverBlock]struct{})
					cov[p.FileName] = blocks
				}
				blocks[coverBlock{StartLine: b.StartLine, StartCol: b.StartCol, EndLine: b.EndLine, EndCol: b.EndCol}] = struct{}{}
			}
		}
	}
	return cov, nil
}
