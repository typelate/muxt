package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"maps"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/muxt"
)

// packageLoader is the only way a run asks for packages, so one go command
// run can serve several package directories and a test can answer without one.
// root is where the go command runs, dirs the package directories, patterns
// extra imports.
type packageLoader func(root string, dirs, patterns []string) (*load.Loaded, error)

// generateRunner generates jobs from one load, one directory at a time, a
// package after the packages it imports. A job's files are all written before
// the next job is resolved, so a failure leaves the earlier jobs written.
func generateRunner(loadPackages packageLoader) func(*cobra.Command, []packageJob) error {
	return func(cmd *cobra.Command, jobs []packageJob) error {
		if len(jobs) == 0 {
			return nil
		}
		several := jobs[0].Keyed
		// What was written is for -v; a run that has nothing to say is silent.
		stdout := cmd.OutOrStdout()
		verbose := jobs[0].Config.Verbose
		wroteTo := io.Discard
		if verbose {
			wroteTo = stdout
		}
		stderr := log.New(cmd.ErrOrStderr(), "", 0)
		// With several directories a failure names its package, since
		// nothing else says which one it was.
		failed := func(job packageJob, err error) error {
			if entryErr, ok := entryVariableError(job, err); ok {
				// It names the entry already.
				return entryErr
			}
			if several {
				return fmt.Errorf("%s: %w", job.Key, err)
			}
			return err
		}
		return runJobs(cmd, loadPackages, jobs, jobVisitor{
			// The first load already holds a package that did not load, so it
			// is refused before an earlier entry is written.
			Ready: func(_ int, job packageJob, l *load.Loaded) error {
				err := l.CheckLoaded(job.Config.OutputDirectory(job.Dir))
				if err == nil {
					return nil
				}
				// One package has no other entry to name, so it explains
				// itself the way every later failure of it does.
				if len(jobs) == 1 {
					printMultiLineError(cmd, err)
					return err
				}
				if job.File == "" {
					return err
				}
				return &composite.InvalidEntryError{Path: job.File, Where: fmt.Sprintf("packages[%q]", job.Key), Err: err}
			},
			Begin: func(_ int, job packageJob) {
				if several && verbose {
					// Several directories write to one stream, so each
					// says whose lines follow.
					_, _ = fmt.Fprintln(stdout, job.Key)
				}
			},
			Visit: func(_ int, job packageJob, l *load.Loaded, several bool) (bool, error) {
				files, err := resolveJob(cmd, l, job, several)
				if err != nil {
					return false, failed(job, err)
				}
				jobStderr := stderr
				if several {
					jobStderr = log.New(cmd.ErrOrStderr(), job.Key+": ", 0)
				}
				wrote, err := writeJob(wroteTo, jobStderr, job, files)
				if err != nil {
					return false, failed(job, err)
				}
				return wrote, nil
			},
		})
	}
}

// resolveJob returns the files job generates from the loaded packages l. A
// failure that explains itself over several lines is printed before it is
// returned, after job's key when several jobs run, since that printed text
// replaces the returned error.
func resolveJob(cmd *cobra.Command, l *load.Loaded, job packageJob, several bool) ([]generate.GeneratedFile, error) {
	report := func(err error) {
		if _, ok := errors.AsType[muxt.MultiLineError](err); ok && several {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s:\n", job.Key)
		}
		printMultiLineError(cmd, err)
	}
	config := job.Config
	pkg, receiver, err := l.PackageWithReceiver(config.OutputDirectory(job.Dir), config.ReceiverPackage, config.ReceiverType, config.TemplatesVariables)
	if err != nil {
		report(err)
		return nil, err
	}
	defs, err := muxt.ResolveDefinitions(pkg, receiver, l.StandardLibrary())
	if err != nil {
		report(err)
		return nil, err
	}
	// Several jobs share one stream, so each note says whose it is.
	logs := generate.Loggers{Progress: log.New(cmd.OutOrStdout(), "", 0), Notes: log.New(cmd.OutOrStdout(), "", 0)}
	if several {
		logs.Notes.SetPrefix(job.Key + ": ")
	}
	files, err := generate.TemplateRoutesFiles(job.Dir, config, pkg, defs, logs)
	if err != nil {
		report(err)
		return nil, err
	}
	return files, nil
}

// writeJob writes files to job's directory and removes the generated files of
// an earlier run that files no longer holds. changed is whether any file it
// wrote or removed differs from what was on disk before.
func writeJob(stdout io.Writer, stderr *log.Logger, job packageJob, files []generate.GeneratedFile) (changed bool, err error) {
	owned, err := ownedGeneratedFiles(job.Dir, job.Config.RoutesFunction, stderr)
	if err != nil {
		return false, err
	}
	paths := slices.Collect(maps.Keys(owned))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	before, err := readFiles(paths)
	if err != nil {
		return false, err
	}
	written, err := writeGeneratedFiles(stdout, files, job.Config)
	if err != nil {
		return false, err
	}
	if err := removeOrphans(owned, written); err != nil {
		return false, err
	}
	after, err := readFiles(paths)
	if err != nil {
		return false, err
	}
	return !maps.Equal(before, after), nil
}

// readFiles returns the content of each of paths that exists.
func readFiles(paths []string) (map[string]string, error) {
	contents := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		contents[path] = string(data)
	}
	return contents, nil
}
