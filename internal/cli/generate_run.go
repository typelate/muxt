package cli

import (
	"errors"
	"fmt"
	"go/token"
	"io"
	"io/fs"
	"log"
	"maps"
	"os"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/muxt"
)

// generateJobs chooses what a generate run writes. With no generate flag given
// and a muxt.json entry for wd, the entry configures the run; otherwise the
// flags do. Flags win outright so a command line never half-merges with a
// file. It reads the file itself, since nothing else needs it yet.
func generateJobs(wd string, flags *pflag.FlagSet, flagConfig *generate.RoutesFileConfiguration, deprecatedTemplatesVar string, getEnv func(string) string, version func() (string, bool)) ([]generateJob, error) {
	if flagNames := givenGenerateFlags(flags); len(flagNames) > 0 {
		if err := rejectConfiguredDirectory(generateCommandName, wd, flagNames); err != nil {
			return nil, err
		}
	} else {
		_, jobs, found, err := entryJobs(wd, getEnv, version)
		if err != nil {
			return nil, err
		}
		if found {
			// Verbose is a run setting, never a muxt.json key, so every job
			// takes it from the command line.
			for i := range jobs {
				jobs[i].Config.Verbose = flagConfig.Verbose
			}
			return jobs, nil
		}
		if err := requireEntry(wd); err != nil {
			return nil, err
		}
	}
	if err := finalizeGenerate(flagConfig, flags, deprecatedTemplatesVar, wd, getEnv, version); err != nil {
		return nil, err
	}
	return []generateJob{{Dir: wd, Config: *flagConfig}}, nil
}

// packageLoader is the only way a run asks for packages, so one go command
// run can serve several package directories and a test can answer without one.
// root is where the go command runs, dirs the package directories, patterns
// extra imports.
type packageLoader func(root string, dirs, patterns []string) (*token.FileSet, []*packages.Package, error)

// loadFromGoCommand is the packageLoader that runs the go command.
func loadFromGoCommand(root string, dirs, patterns []string) (*token.FileSet, []*packages.Package, error) {
	return load.PackagesInDirectories(root, nil, dirs, patterns...)
}

// generateRunner generates jobs from one load, one directory at a time, a
// package after the packages it imports. A job's files are all written before
// the next job is resolved, so a failure leaves the earlier jobs written.
func generateRunner(loadPackages packageLoader) func(*cobra.Command, []generateJob) error {
	return func(cmd *cobra.Command, jobs []generateJob) error {
		if len(jobs) == 0 {
			return nil
		}
		dirs := make([]string, 0, len(jobs))
		var patterns []string
		for _, job := range jobs {
			if err := requireDirectory(cmd.Name(), job.Dir); err != nil {
				return err
			}
			dirs = append(dirs, job.Dir)
			patterns = append(patterns, job.Config.ReceiverPackage)
		}
		// What was written is for -v; a run that has nothing to say is silent.
		stdout := cmd.OutOrStdout()
		verbose := jobs[0].Config.Verbose
		wroteTo := io.Discard
		if verbose {
			wroteTo = stdout
		}
		stderr := log.New(cmd.ErrOrStderr(), "", 0)
		_, pl, err := loadPackages(jobs[0].Dir, dirs, patterns)
		if err != nil {
			return err
		}
		warnPartialAST(stderr, pl)
		// The first load already holds a package that did not load, so it
		// is refused before an earlier entry is written.
		for _, job := range jobs {
			if err := load.CheckLoaded(job.Config.OutputDirectory(job.Dir), pl); err != nil {
				if job.File == "" {
					return err
				}
				return &composite.InvalidEntryError{Path: job.File, Where: fmt.Sprintf("packages[%q]", job.Key), Err: err}
			}
		}
		// changed holds the directories written since the last load. A load
		// that came before a write saw its package without the new files, so
		// a job importing one of them needs a fresh load; one that imports
		// none can use the load it has.
		var changed []string
		outputDirs := make([]string, len(jobs))
		for i, job := range jobs {
			outputDirs[i] = job.Config.OutputDirectory(job.Dir)
		}
		for _, i := range importOrder(pl, outputDirs) {
			job := jobs[i]
			// With several directories a failure names its package, since
			// nothing else says which one it was.
			failed := func(err error) error { return err }
			if len(jobs) > 1 {
				failed = func(err error) error { return fmt.Errorf("%s: %w", job.Key, err) }
				if verbose {
					// Several directories write to one stream, so each
					// says whose lines follow.
					_, _ = fmt.Fprintln(stdout, job.Key)
				}
			}
			dir := job.Config.OutputDirectory(job.Dir)
			if slices.ContainsFunc(changed, func(written string) bool { return load.Imports(pl, dir, written) }) {
				if _, pl, err = loadPackages(jobs[0].Dir, dirs, patterns); err != nil {
					return failed(err)
				}
				changed = nil
			}
			files, err := resolveJob(cmd, pl, job, len(jobs) > 1)
			if err != nil {
				return failed(err)
			}
			jobStderr := stderr
			if len(jobs) > 1 {
				jobStderr = log.New(cmd.ErrOrStderr(), job.Key+": ", 0)
			}
			wrote, err := writeJob(wroteTo, jobStderr, job, files)
			if err != nil {
				return failed(err)
			}
			if wrote {
				changed = append(changed, dir)
			}
		}
		return nil
	}
}

// importOrder returns the indexes of dirs, the package directory of each job,
// so that a directory follows every directory it imports, and directories with
// no import between them keep their order. The go command rejects import
// cycles, so there is always a directory to take next. It takes directories,
// not jobs, so a run that lists packages orders them the way generate does.
func importOrder(pl []*packages.Package, dirs []string) []int {
	remaining := make([]int, len(dirs))
	for i := range remaining {
		remaining[i] = i
	}
	ordered := make([]int, 0, len(dirs))
	for len(remaining) > 0 {
		next := slices.IndexFunc(remaining, func(i int) bool {
			return !slices.ContainsFunc(remaining, func(dependency int) bool {
				return load.Imports(pl, dirs[i], dirs[dependency])
			})
		})
		next = max(next, 0)
		ordered = append(ordered, remaining[next])
		remaining = slices.Delete(remaining, next, next+1)
	}
	return ordered
}

// resolveJob returns the files job generates from the loaded packages pl. A
// failure that explains itself over several lines is printed before it is
// returned, after job's key when several jobs run, since that printed text
// replaces the returned error.
func resolveJob(cmd *cobra.Command, pl []*packages.Package, job generateJob, several bool) ([]generate.GeneratedFile, error) {
	report := func(err error) {
		if _, ok := errors.AsType[muxt.MultiLineError](err); ok && several {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s:\n", job.Key)
		}
		printMultiLineError(cmd, err)
	}
	config := job.Config
	pkg, receiver, err := load.PackageWithReceiver(config.OutputDirectory(job.Dir), pl, config.ReceiverPackage, config.ReceiverType, config.TemplatesVariables)
	if err != nil {
		report(err)
		return nil, err
	}
	defs, err := muxt.ResolveDefinitions(pkg, receiver, load.StandardLibrary(pl))
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
func writeJob(stdout io.Writer, stderr *log.Logger, job generateJob, files []generate.GeneratedFile) (changed bool, err error) {
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
