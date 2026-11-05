package mutation

import (
	"context"
	"errors"
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

// coverageRun runs the tests once with the engine's files and the
// directory each test binary writes its profile to, as goTest.coverage.
type coverageRun func(ctx context.Context, engine overlay.Files, coverDir string) (*goTestOutput, error)

// toolchain is the go command as the run's go test uses it, which is what
// decides the GOROOT the engine overlay replaces html/template under.
func (c Configuration) toolchain(dir string) overlay.Toolchain {
	return overlay.Toolchain{Go: "go", Dir: dir, Env: c.env}
}

// templateCoverage runs the coverage pre-pass: the tests once, unmutated,
// with muxt's template engine laid over html/template, and returns the
// union of the blocks every test binary reports executing, and which muxt
// module the engine came from, as engineNote says it, empty when no go
// test ran. It writes under scratch, the run's own directory, which the
// caller removes.
//
// Coverage is something a run can do without, so a pre-pass that cannot
// tell -- a toolchain refusing the overlay, tests that do not build with
// the engine, a test binary that stopped exporting -- writes one warning
// line to warn and returns nil coverage, and the run goes on. So do tests
// that fail under the engine: kill runs use the real html/template, and
// the baseline after this is what says whether the tests pass.
func templateCoverage(ctx context.Context, tc overlay.Toolchain, scratch string, run coverageRun, warn io.Writer) (coverage, string, error) {
	if err := overlay.Probe(ctx, tc); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		return coverageUnknown(warn, err.Error()), "", nil
	}
	dir, err := filepath.Abs(filepath.Join(scratch, "coverage"))
	if err != nil {
		return nil, "", err
	}
	// The overlay names the shim by its path verbatim, so it is
	// absolute.
	coverDir := filepath.Join(dir, "profiles")
	if err := os.MkdirAll(coverDir, 0o700); err != nil {
		return nil, "", err
	}
	engine, err := overlay.Write(ctx, tc, dir)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		return coverageUnknown(warn, err.Error()), "", nil
	}

	out, err := run(ctx, engine, coverDir)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, "", ctxErr
	}
	switch {
	case err != nil && out.setupFailed:
		// The go command never got as far as the engine's code, so its
		// gaps are not to blame.
		return coverageUnknown(warn, "go test with muxt's template engine in place of html/template did not run: "+firstError(out.String(), err)), engineNote(engine), nil
	case err != nil && isTestFailure(err) && (out.testFailed || out.couldNotRun):
		// Kill runs use the real html/template, so this says nothing
		// about the verdicts; whether the tests pass is the baseline's
		// to say.
		return coverageUnknown(warn, engineFailure(out, err)), engineNote(engine), nil
	case err != nil:
		return coverageUnknown(warn, "go test with muxt's template engine in place of html/template did not run: "+firstError(out.String(), err)), engineNote(engine), nil
	case out.coverDisabled != "":
		return coverageUnknown(warn, out.coverDisabled), engineNote(engine), nil
	}
	cov, err := mergeProfiles(coverDir)
	if err != nil {
		return coverageUnknown(warn, err.Error()), engineNote(engine), nil
	}
	warnRaisedGoLine(warn, engine)
	return cov, engineNote(engine), nil
}

// engineNote says which github.com/typelate/muxt the pre-pass built the
// engine from, for the verbose preamble: the version the modfile copy
// requires, the directory replacing it, and the test target's own
// requirement when the copy overrode it.
func engineNote(engine overlay.Files) string {
	if engine.Version == "" {
		return "engine from this module"
	}
	note := "engine " + overlay.Module + "@" + engine.Version
	if engine.ModuleDir != "" {
		note += " => " + engine.ModuleDir
	}
	if engine.TargetVersion != "" {
		note += ", not your go.mod's " + engine.TargetVersion
	}
	return note
}

// warnRaisedGoLine writes one warning when the go command raised the
// modfile copy's go line above the test target's: the pre-pass then
// compiled the target at a newer language version than it is written
// for, and a branch only the older semantics reach would read as
// unreached. The coverage is still used.
func warnRaisedGoLine(warn io.Writer, engine overlay.Files) {
	if warn == nil {
		return
	}
	target, used, err := engine.GoLines()
	switch {
	case err != nil:
		_, _ = fmt.Fprintf(warn, "warning: cannot tell the go line template coverage was computed at: %v\n", err)
	case used != target:
		_, _ = fmt.Fprintf(warn, "warning: template coverage was computed at go %s (your go.mod says go %s): branches that depend on the older language semantics may be reported unreached\n", used, target)
	}
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
func coverageUnknown(warn io.Writer, why string) coverage {
	if warn != nil {
		why = strings.Join(strings.Fields(why), " ")
		_, _ = fmt.Fprintf(warn, "warning: template coverage unknown: %s\n", why)
	}
	return nil
}

// firstError is the first line of go test's output that says what went
// wrong, or err when no line does.
func firstError(out string, err error) string {
	for line := range strings.Lines(out) {
		line = strings.TrimSpace(line)
		switch {
		case line == "", strings.HasPrefix(line, "#"), strings.HasPrefix(line, "=== "),
			strings.HasPrefix(line, "go: downloading "),
			strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "ok "),
			strings.HasPrefix(line, "FAIL"), strings.HasPrefix(line, "PASS"):
			continue
		}
		return line
	}
	return err.Error()
}

// errNoProfile is a pre-pass in which no test binary wrote a profile.
var errNoProfile = errors.New("no test binary ran the templates (no coverage profile was written)")

// mergeProfiles unions every <pid>.cov in dir by block. A block is in the
// union when any test binary executed it.
//
// Every test binary that runs the engine writes its profile as it starts,
// so a profile with no blocks is a binary that executed no template.
// No profile at all is errNoProfile: nothing says the engine ran, as when
// a package has no test files, -exec runs the tests elsewhere, or they
// render through a server binary of their own, and reading it as nothing
// reached would skip every mutant.
func mergeProfiles(dir string) (coverage, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.cov"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errNoProfile
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
