package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

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
	return func(cmd *cobra.Command, jobs []packageJob, config testConfiguration) error {
		scratch, err := os.MkdirTemp("", "muxt-test-")
		if err != nil {
			return fmt.Errorf("creating a scratch directory: %w", err)
		}
		for i, job := range jobs {
			engine, err := prepare(cmd.Context(), overlay.Toolchain{Go: "go", Dir: job.Dir}, filepath.Join(scratch, strconv.Itoa(i)))
			if err != nil {
				return fmt.Errorf("cannot run the tests on muxt's template engine: %w", err)
			}
			// -count=1 comes first so a caller's own -count wins, and a
			// replayed pass cannot leave a package out of the profile.
			args := append([]string{"test", "-count=1"}, engine.Flags()...)
			args = append(args, config.GoTestArgs...)
			if err := goTest(cmd.Context(), job.Dir, engine.Environ(nil), args, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
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
