package mutation

import (
	"context"
	"errors"
	"fmt"
	"go/version"
	"io"
	"path/filepath"
	"strings"

	"github.com/typelate/muxt/internal/diagnostic"
	"github.com/typelate/muxt/internal/overlay"
	"github.com/typelate/muxt/internal/templatecover"
)

// coverage is the template blocks the tests executed in the coverage
// pre-pass, as templatecover.Reached reads them. It is a defined type, not
// an alias, so that reaches can be its method.
type coverage templatecover.Reached

// coverBlock is a block's span in its template, as a cover profile gives
// it.
type coverBlock = templatecover.Block

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
// union of the blocks every test binary reports executing, and which htt
// module the engine came from, as engineNote says it, empty when no go
// test ran. It writes under scratch, the run's own directory, which the
// caller removes.
//
// Coverage is something a run can do without, so a pre-pass that cannot
// tell -- a toolchain refusing the overlay, tests that do not build with
// the engine, a test binary that stopped exporting -- writes one warning
// line to warn and returns nil coverage, and the run goes on. So do tests
// that fail under the engine: kill runs use the real html/template, and
// the baseline, which has passed by then, is what says the tests pass.
func templateCoverage(ctx context.Context, tc overlay.Toolchain, scratch string, run coverageRun, warn io.Writer) (coverage, string, error) {
	prepared, err := templatecover.Prepare(ctx, tc, filepath.Join(scratch, "coverage"))
	if err != nil {
		unavailable, ok := errors.AsType[*templatecover.UnavailableError](err)
		if !ok {
			return nil, "", err
		}
		if ctxErr := ctx.Err(); unavailable.Resolving && ctxErr != nil {
			return nil, "", ctxErr
		}
		return coverageUnknown(warn, unavailable.Error()), "", nil
	}
	engine, coverDir := prepared.Files, prepared.CoverDir

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
		// about the verdicts; the baseline, which passed, said the tests
		// pass.
		return coverageUnknown(warn, engineFailure(out, err)), engineNote(engine), nil
	case err != nil:
		return coverageUnknown(warn, "go test with muxt's template engine in place of html/template did not run: "+firstError(out.String(), err)), engineNote(engine), nil
	case out.coverDisabled != "":
		return coverageUnknown(warn, out.coverDisabled), engineNote(engine), nil
	}
	reached, err := templatecover.ReadProfiles(coverDir)
	if err != nil {
		return coverageUnknown(warn, err.Error()), engineNote(engine), nil
	}
	warnRaisedGoLine(warn, engine)
	warnUpgrades(warn, engine)
	return coverage(reached), engineNote(engine), nil
}

// engineNote says which github.com/typelate/htt the pre-pass built the
// engine from, for the verbose preamble: the version the modfile copy
// requires, what replaces it, and the test target's own requirement when
// the copy overrode it. A workspace build has no version to name.
func engineNote(engine overlay.Files) string {
	note := "engine " + overlay.Module
	if engine.Version != "" {
		note += "@" + engine.Version
	}
	if engine.Replace != "" {
		note += " => " + engine.Replace
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
		writeWarning(warn, warning(fmt.Sprintf("cannot tell the go line template coverage was computed at: %v", err)))
	case version.Lang("go"+used) != version.Lang("go"+target):
		writeWarning(warn, warning(fmt.Sprintf("template coverage was computed at go %s (your go.mod says go %s): branches that depend on the older language semantics may be reported unreached", used, target)))
	}
}

// warning is a diagnostic of severity Warning that says message. None of the
// warnings of a run is about one file: they are about the toolchain.
func warning(message string) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Severity: diagnostic.Warning, Message: message}
}

// writeWarning writes d as a warning line. A warning is not a failure, so a
// write error is not reported: the run goes on without it.
func writeWarning(warn io.Writer, d diagnostic.Diagnostic) {
	_, _ = fmt.Fprintf(warn, "warning: %s\n", d)
}

// warnUpgrades writes one warning when the go command selected, in the
// modfile copy, newer versions of modules the test target's go.mod
// requires, because the engine requires them too: the pre-pass then ran
// the tests against code they are not built with elsewhere, and a branch
// the older version takes would read as unreached. The coverage is still
// used.
func warnUpgrades(warn io.Writer, engine overlay.Files) {
	if warn == nil {
		return
	}
	upgrades, err := engine.Upgrades()
	if err != nil {
		writeWarning(warn, warning(fmt.Sprintf("cannot tell which module versions template coverage was computed with: %v", err)))
		return
	}
	if len(upgrades) == 0 {
		return
	}
	raised := make([]string, len(upgrades))
	for i, u := range upgrades {
		raised[i] = u.Path + " " + u.From + " → " + u.To
	}
	writeWarning(warn, warning(fmt.Sprintf("the coverage pre-pass selected newer versions of modules your go.mod requires: %s; tests exercising them may take other paths", strings.Join(raised, ", "))))
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
		writeWarning(warn, warning("template coverage unknown: "+why))
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
