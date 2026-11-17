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

	"github.com/typelate/muxt/internal/overlay"
	"github.com/typelate/muxt/internal/templatecover"
)

// goTestFunc runs go with args in dir and env, writing to stdout and stderr.
type goTestFunc func(ctx context.Context, dir string, env, args []string, stdout, stderr io.Writer) error

// prepareFunc is templatecover.Prepare, replaced in tests.
type prepareFunc func(ctx context.Context, tc overlay.Toolchain, dir string) (templatecover.Engine, error)

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

// testRunner runs each job's tests on muxt's template engine. The go test
// of a job is separate from the others', so a profile line can be
// attributed to the files of the package it came from.
func testRunner(goTest goTestFunc, prepare prepareFunc) func(*cobra.Command, []packageJob, testConfiguration) error {
	return func(cmd *cobra.Command, jobs []packageJob, config testConfiguration) (err error) {
		// An interrupt stops the go test in flight and removes the files
		// the run wrote; a second one ends the process at once.
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		context.AfterFunc(ctx, stop)

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
		engines := make([]templatecover.Engine, len(jobs))
		for i, job := range jobs {
			engines[i], err = prepare(ctx, overlay.Toolchain{Go: "go", Dir: job.Dir}, filepath.Join(scratch, strconv.Itoa(i)))
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				return fmt.Errorf("cannot run the tests on muxt's template engine: %w", err)
			}
		}
		for i, job := range jobs {
			engine := engines[i]
			// -count=1 comes first so a caller's own -count wins, and a
			// replayed pass cannot leave a package out of the profile.
			args := append([]string{"test", "-count=1"}, engine.Flags()...)
			args = append(args, config.GoTestArgs...)
			if job.Keyed && !writesJSON(config.GoTestArgs) {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), job.Key); err != nil {
					return fmt.Errorf("writing the package key: %w", err)
				}
			}
			if err := goTest(ctx, job.Dir, engine.Environ(nil), args, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
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
		}
		return nil
	}
}
