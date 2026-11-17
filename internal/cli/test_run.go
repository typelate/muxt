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
// package did not build. go test printed which.
type goTestFailure struct {
	Err *exec.ExitError
}

func (e *goTestFailure) Error() string { return "go test failed" }

func (e *goTestFailure) Unwrap() error { return e.Err }

// runGoTest is the go command, run as templatecover.Command sets it up.
func runGoTest(ctx context.Context, dir string, env, args []string, stdout, stderr io.Writer) error {
	cmd := templatecover.Command(ctx, dir, env, args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	return cmd.Run()
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
		for i, job := range jobs {
			engine, err := prepare(ctx, overlay.Toolchain{Go: "go", Dir: job.Dir}, filepath.Join(scratch, strconv.Itoa(i)))
			if err != nil {
				return fmt.Errorf("cannot run the tests on muxt's template engine: %w", err)
			}
			// -count=1 comes first so a caller's own -count wins, and a
			// replayed pass cannot leave a package out of the profile.
			args := append([]string{"test", "-count=1"}, engine.Flags()...)
			args = append(args, config.GoTestArgs...)
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
				// only bury it.
				cmd.SilenceErrors = true
				return &goTestFailure{Err: exit}
			}
		}
		return nil
	}
}
