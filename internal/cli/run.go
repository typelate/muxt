package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/mutation"
)

// This file holds what each command does with its configuration: load the
// package, run the implementation, and write what it produced. What the
// flags decide happens before a runner is called, in the command
// constructors; only the --format a result is written in is read here.

// requireDirectory says why a package directory cannot be read before the go
// command is asked to load it, whose own message for a missing one is empty.
func requireDirectory(command, dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%s: package directory: %w", command, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: %s is not a package directory", command, dir)
	}
	return nil
}

func runTemplateMutations(cmd *cobra.Command, wd string, config mutation.Configuration) error {
	// An interrupt stops the go test runs in flight and removes the
	// files the run wrote; a second one ends the process at once.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	context.AfterFunc(ctx, stop)
	if err := requireDirectory(cmd.Name(), wd); err != nil {
		return err
	}
	// A person watching a terminal gets a line per template; a log gets
	// them only with -v. stdout is never written to, so --format json
	// stays machine-clean.
	stderr := cmd.ErrOrStderr()
	config.Progress = isTerminal(stderr)
	report, err := mutation.Run(ctx, config, wd, stderr)
	if err != nil {
		printMultiLineError(cmd, err)
		return err
	}
	if err := writeResult(cmd, cmd.OutOrStdout(), report); err != nil {
		return err
	}
	// Like go test, the command fails once the report is out when a
	// mutant was missed.
	return report.Err()
}

// isTerminal reports whether w is a character device such as a terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
