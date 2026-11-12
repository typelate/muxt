package cli

import (
	"errors"
	"fmt"
	"go/token"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/load"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/muxtjson"
)

// generateJob is one package directory and what to generate it with. Key is
// the packages key the configuration came from; it is empty when flags gave it.
type generateJob struct {
	Key    string
	Dir    string
	Config generate.RoutesFileConfiguration
}

// generateJobs chooses what a generate run writes. With no generate flag given
// and a muxt.json entry for wd, the entry configures the run; otherwise the
// flags do. Flags win outright so a command line never half-merges with a
// file. It reads the file itself, since nothing else needs it yet.
func generateJobs(wd string, flags *pflag.FlagSet, flagConfig *generate.RoutesFileConfiguration, deprecatedTemplatesVar string, getEnv func(string) string, version func() (string, bool)) ([]generateJob, error) {
	if flagNames := givenGenerateFlags(flags); len(flagNames) > 0 {
		if err := rejectConfiguredDirectory(wd, flagNames); err != nil {
			return nil, err
		}
	} else {
		jobs, found, err := entryJobs(wd, getEnv, version)
		if err != nil {
			return nil, err
		}
		if found {
			return jobs, nil
		}
	}
	if err := finalizeGenerate(flagConfig, flags, deprecatedTemplatesVar, wd, getEnv, version); err != nil {
		return nil, err
	}
	return []generateJob{{Dir: wd, Config: *flagConfig}}, nil
}

// entryJobs returns the jobs the nearest muxt.json gives a flagless run in wd.
// In the file's own directory that is one job per entry, and every entry is
// validated before any job is returned. In a sub-directory it is that
// directory's entry alone. found is false when the flags decide the run.
func entryJobs(wd string, getEnv func(string) string, version func() (string, bool)) (jobs []generateJob, found bool, err error) {
	f, key, _, found, err := lookupMuxtJSONEntry(wd)
	if err != nil || f == nil {
		return nil, false, err
	}
	keys := []string{key}
	if key != "." {
		if !found {
			return nil, false, &muxtjson.MissingEntryError{Path: f.Name, Key: key}
		}
	} else {
		if len(f.Packages) == 0 {
			return nil, false, nil
		}
		keys = f.Keys()
	}
	for _, k := range keys {
		dir := f.DirOf(k)
		if err := requireEntryDirectory(f, k, wd, dir); err != nil {
			return nil, false, err
		}
		config, err := entryConfiguration(f.Packages[k], dir, getEnv, version)
		if err != nil {
			return nil, false, &muxtjson.InvalidEntryError{Path: f.Name, Where: fmt.Sprintf("packages[%q]", k), Err: err}
		}
		jobs = append(jobs, generateJob{Key: k, Dir: dir, Config: config})
	}
	return jobs, true, nil
}

// requireEntryDirectory returns a *muxtjson.MissingDirectoryError unless dir,
// the directory of key in f, is a directory. wd is the working directory the
// error names dir relative to.
func requireEntryDirectory(f *muxtjson.File, key, wd, dir string) error {
	info, err := os.Stat(dir)
	if err == nil && info.IsDir() {
		return nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("muxt.json entry %q: %w", key, err)
	}
	rel, err := filepath.Rel(wd, dir)
	if err != nil {
		return fmt.Errorf("directory of muxt.json entry %q from %s: %w", key, wd, err)
	}
	return &muxtjson.MissingDirectoryError{Path: f.Name, Key: key, Dir: rel}
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
		stdout := cmd.OutOrStdout()
		stderr := log.New(cmd.ErrOrStderr(), "", 0)
		_, pl, err := loadPackages(jobs[0].Dir, dirs, patterns)
		if err != nil {
			return err
		}
		warnPartialAST(stderr, pl)
		for _, job := range importOrder(pl, jobs) {
			if len(jobs) > 1 {
				// Several directories write to one stream, so each says
				// whose lines follow.
				_, _ = fmt.Fprintln(stdout, job.Key)
			}
			files, err := resolveJob(cmd, pl, job)
			if err != nil {
				return err
			}
			if err := writeJob(stdout, stderr, job, files); err != nil {
				return err
			}
		}
		return nil
	}
}

// importOrder returns jobs so that a job follows every job whose package it
// imports, and jobs with no import between them keep their order. The go
// command rejects import cycles, so there is always a job to take next.
func importOrder(pl []*packages.Package, jobs []generateJob) []generateJob {
	remaining := slices.Clone(jobs)
	ordered := make([]generateJob, 0, len(jobs))
	for len(remaining) > 0 {
		next := slices.IndexFunc(remaining, func(job generateJob) bool {
			return !slices.ContainsFunc(remaining, func(dependency generateJob) bool {
				return load.Imports(pl, job.Config.OutputDirectory(job.Dir), dependency.Config.OutputDirectory(dependency.Dir))
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
// returned.
func resolveJob(cmd *cobra.Command, pl []*packages.Package, job generateJob) ([]generate.GeneratedFile, error) {
	config := job.Config
	pkg, receiver, err := load.PackageWithReceiver(config.OutputDirectory(job.Dir), pl, config.ReceiverPackage, config.ReceiverType, config.TemplatesVariables)
	if err != nil {
		printMultiLineError(cmd, err)
		return nil, err
	}
	defs, err := muxt.ResolveDefinitions(pkg, receiver, load.StandardLibrary(pl))
	if err != nil {
		printMultiLineError(cmd, err)
		return nil, err
	}
	files, err := generate.TemplateRoutesFiles(job.Dir, config, pkg, defs, log.New(cmd.OutOrStdout(), "", 0))
	if err != nil {
		printMultiLineError(cmd, err)
		return nil, err
	}
	return files, nil
}

// writeJob writes files to job's directory and removes the generated files of
// an earlier run that files no longer holds.
func writeJob(stdout io.Writer, stderr *log.Logger, job generateJob, files []generate.GeneratedFile) error {
	owned, err := ownedGeneratedFiles(job.Dir, job.Config.RoutesFunction, stderr)
	if err != nil {
		return err
	}
	written, err := writeGeneratedFiles(stdout, files, job.Config)
	if err != nil {
		return err
	}
	return removeOrphans(owned, written)
}
