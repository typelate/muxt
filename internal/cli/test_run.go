package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/diagnostic"
	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/overlay"
	"github.com/typelate/muxt/internal/templatecover"
)

// goTestFunc runs go with args in dir and env, writing to stdout and stderr.
type goTestFunc func(ctx context.Context, dir string, env, args []string, stdout, stderr io.Writer) error

// prepareFunc is templatecover.Prepare, replaced in tests.
type prepareFunc func(ctx context.Context, tc overlay.Toolchain, dir string) (templatecover.Engine, error)

// attributeFunc reads the template files of a package, which is where
// a profile line's parse name becomes a file. The error says why they
// cannot be read.
type attributeFunc func(job packageRun) ([]templatecover.Source, error)

// loadSources is attributeFunc loading the package the way muxt check does:
// the package's directory, then the templates variables of all its sets.
func loadSources(job packageRun) ([]templatecover.Source, error) {
	_, pl, err := load.Packages(job.Dir)
	if err != nil {
		return nil, fmt.Errorf("loading the package: %w", err)
	}
	pkg, err := load.Package(job.Dir, pl, job.TemplatesVariables())
	if err != nil {
		return nil, fmt.Errorf("reading the templates: %w", err)
	}
	return templatecover.Sources(pkg), nil
}

// goTestFailure is go test exiting with a failure: a test failed or a
// package did not build. go test printed which. Key is the muxt.json key of
// the package in a keyed run, where several packages' reports are on screen
// and the last line must say whose failed; it is empty otherwise.
type goTestFailure struct {
	Key string
	Err *exec.ExitError
}

func (e *goTestFailure) Error() string {
	if e.Key == "" {
		return "go test failed"
	}
	return e.Key + ": go test failed"
}

func (e *goTestFailure) Unwrap() error { return e.Err }

// runGoTest is the go command, run as templatecover.Command sets it up.
func runGoTest(ctx context.Context, dir string, env, args []string, stdout, stderr io.Writer) error {
	cmd := templatecover.Command(ctx, dir, env, args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
}

// writesJSON reports whether go test is asked for its JSON event stream,
// which a line of muxt's would corrupt. A later -json=false wins.
func writesJSON(goTestArgs []string) bool {
	json := false
	for f := range templatecover.Flags(goTestArgs) {
		if f.Name != "json" {
			continue
		}
		on, err := strconv.ParseBool(f.Value)
		json = !f.HasValue || (err == nil && on)
	}
	return json
}

// testRunner runs each package's tests on muxt's template engine, once per
// package however many sets it has. The go test of a package is separate from
// the others', so a profile line can be attributed to the files of the package
// it came from.
func testRunner(goTest goTestFunc, prepare prepareFunc, attribute attributeFunc) func(*cobra.Command, []packageRun, testConfiguration) error {
	return func(cmd *cobra.Command, runs []packageRun, config testConfiguration) (err error) {
		// An interrupt stops the go test in flight and removes the files
		// the run wrote; a second one ends the process at once.
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		context.AfterFunc(ctx, stop)

		if config.CoverProfile != "" {
			// A profile that cannot be written is found out now, not
			// after the tests ran.
			dir := filepath.Dir(config.CoverProfile)
			info, err := os.Stat(dir)
			if err != nil {
				return fmt.Errorf("--%s: %w", outputCoverProfile, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("--%s: %s is not a directory", outputCoverProfile, dir)
			}
		}

		scratch, err := os.MkdirTemp("", "muxt-test-")
		if err != nil {
			return fmt.Errorf("creating a scratch directory: %w", err)
		}
		defer func() {
			if rmErr := os.RemoveAll(scratch); rmErr != nil {
				err = errors.Join(err, fmt.Errorf("removing the scratch directory: %w", rmErr))
			}
		}()
		// Every engine is in place before the first go test runs, so a
		// package that cannot have one costs no run of the packages before it.
		engines := make([]templatecover.Engine, len(runs))
		for i, job := range runs {
			engines[i], err = prepare(ctx, overlay.Toolchain{Go: "go", Dir: job.Dir, Purpose: "muxt " + testCommandName}, filepath.Join(scratch, strconv.Itoa(i)))
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				return fmt.Errorf("cannot run the tests on muxt's template engine: %w", err)
			}
		}
		runJob := func(job packageRun, engine templatecover.Engine) error {
			// -count=1 comes first so a caller's own -count wins, and a
			// replayed pass cannot leave a package out of the profile.
			args := append([]string{"test", "-count=1"}, engine.Flags()...)
			args = append(args, config.GoTestArgs...)
			if job.Keyed && !writesJSON(config.GoTestArgs) {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), job.Key); err != nil {
					return fmt.Errorf("writing the package key: %w", err)
				}
			}
			err := goTest(ctx, job.Dir, engine.Environ(nil), args, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if err == nil {
				return nil
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				// Whatever the interrupted run said, it was not a verdict.
				return ctxErr
			}
			exit, ok := errors.AsType[*exec.ExitError](err)
			if !ok {
				return fmt.Errorf("running go test in %s: %w", job.Dir, err)
			}
			// go test said why; cobra repeating that it failed would
			// only bury it, unless the run covered several packages and
			// the line must name the one that did.
			cmd.SilenceErrors = !job.Keyed
			return &goTestFailure{Key: job.Key, Err: exit}
		}
		var profile templatecover.Profile
		var failure error
		// go test printed why it failed but not why the profile is
		// missing, so the profile's error is printed even beside a
		// failure cobra was told to keep quiet. It comes first so a keyed
		// run's last line still names the failing package.
		profileFailed := func(err error) error {
			cmd.SilenceErrors = false
			return errors.Join(err, failure)
		}
		for i, job := range runs {
			failure = runJob(job, engines[i])
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			// A failing package still says what its tests reached.
			if config.CoverProfile != "" {
				if err := addToProfile(cmd.ErrOrStderr(), &profile, job, engines[i], attribute); err != nil {
					return profileFailed(err)
				}
			}
			if failure != nil {
				break
			}
		}
		if config.CoverProfile != "" {
			if err := writeCoverProfile(config.CoverProfile, &profile); err != nil {
				return profileFailed(err)
			}
		}
		return failure
	}
}

// addToProfile attributes the template blocks a package's tests reached to the
// template files of its package.
//
// What cannot be attributed is a warning on warn, not a failure: go test
// and the engine decide the exit status.
func addToProfile(warn io.Writer, profile *templatecover.Profile, job packageRun, engine templatecover.Engine, attribute attributeFunc) error {
	reached, err := templatecover.ReadProfiles(engine.CoverDir)
	if errors.Is(err, templatecover.ErrNoProfile) {
		return writeWarning(warn, diagnostic.Diagnostic{
			Severity: diagnostic.Warning,
			Message:  fmt.Sprintf("no test binary in %s ran muxt's template engine; the cover profile has nothing from that package", job.Dir),
		})
	}
	if err != nil {
		return fmt.Errorf("reading the template coverage of %s: %w", job.Dir, err)
	}
	sources, err := attribute(job)
	if err != nil {
		return writeWarning(warn, diagnostic.Diagnostic{
			Severity: diagnostic.Warning,
			Message:  fmt.Sprintf("cannot read the template files of %s (%v); the cover profile has nothing from that package", job.Dir, err),
		})
	}
	for _, ambiguous := range profile.Add(reached, sources) {
		if err := writeWarning(warn, diagnostic.Diagnostic{
			Severity: diagnostic.Warning,
			Message:  ambiguous.Error(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// writeCoverProfile writes p to the file at name.
func writeCoverProfile(name string, p *templatecover.Profile) (err error) {
	f, err := os.Create(name)
	if err != nil {
		return fmt.Errorf("writing the cover profile: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("writing the cover profile: %w", closeErr))
		}
	}()
	if _, err := p.WriteTo(f); err != nil {
		return fmt.Errorf("writing the cover profile: %w", err)
	}
	return nil
}
