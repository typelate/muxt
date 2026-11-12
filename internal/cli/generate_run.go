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
	}
	if err := finalizeGenerate(flagConfig, flags, deprecatedTemplatesVar, wd, getEnv, version); err != nil {
		return nil, err
	}
	return []generateJob{{Dir: wd, Config: *flagConfig}}, nil
}

// entryJobs returns the jobs the nearest muxt.json gives a flagless run in wd.
// In the file's own directory that is one job per entry, and every entry is
// validated before any job is returned. In a sub-directory it is that
// directory's entry alone. found is false when the flags decide the run. file
// is the muxt.json the jobs came from, for a caller that says where each
// setting came from.
func entryJobs(wd string, getEnv func(string) string, version func() (string, bool)) (file *muxtjson.File, jobs []generateJob, found bool, err error) {
	f, key, _, found, err := lookupMuxtJSONEntry(wd)
	if err != nil || f == nil {
		return nil, nil, false, err
	}
	ownKey, err := f.KeyFor(f.Dir)
	if err != nil {
		return nil, nil, false, fmt.Errorf("muxt.json entry for %s: %w", f.Dir, err)
	}
	keys := []string{key}
	if key != ownKey {
		if !found {
			return nil, nil, false, &muxtjson.MissingEntryError{Path: f.Name, Key: key}
		}
	} else {
		if len(f.Packages) == 0 {
			return nil, nil, false, nil
		}
		keys = f.Keys()
	}
	for _, k := range keys {
		dir := f.DirOf(k)
		if err := requireEntryDirectory(f, k, wd, dir); err != nil {
			return nil, nil, false, err
		}
		entry, _ := f.Resolve(k)
		config, err := entryConfiguration(entry, dir, getEnv, version)
		if err != nil {
			return nil, nil, false, &muxtjson.InvalidEntryError{Path: f.Name, Where: fmt.Sprintf("packages[%q]", k), Err: err}
		}
		jobs = append(jobs, generateJob{Key: k, Dir: dir, Config: config})
	}
	return f, jobs, true, nil
}

// requireEntryDirectory returns a *muxtjson.MissingDirectoryError unless dir,
// the directory of key in f, is a directory, and a
// *muxtjson.OutsideDirectoryError when it is one that a symbolic link resolves
// outside f's directory. wd is the working directory the errors name dir
// relative to.
func requireEntryDirectory(f *muxtjson.File, key, wd, dir string) error {
	info, statErr := os.Stat(dir)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("muxt.json entry %q: %w", key, statErr)
	}
	rel, err := filepath.Rel(wd, dir)
	if err != nil {
		return fmt.Errorf("directory of muxt.json entry %q from %s: %w", key, wd, err)
	}
	if statErr != nil || !info.IsDir() {
		return &muxtjson.MissingDirectoryError{Path: f.Name, Key: key, Dir: rel}
	}
	inside, err := resolvesInside(f.Dir, dir)
	if err != nil {
		return fmt.Errorf("muxt.json entry %q: %w", key, err)
	}
	if !inside {
		return &muxtjson.OutsideDirectoryError{Path: f.Name, Key: key, Dir: rel}
	}
	return nil
}

// resolvesInside reports whether dir, with its symbolic links followed, is
// base or below it, with base's links followed too. Both sides resolve because
// the temporary directory of a test may itself sit behind a link.
func resolvesInside(base, dir string) (bool, error) {
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return false, fmt.Errorf("resolving %s: %w", base, err)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false, fmt.Errorf("resolving %s: %w", dir, err)
	}
	rel, err := filepath.Rel(realBase, realDir)
	if err != nil {
		return false, fmt.Errorf("directory %s from %s: %w", realDir, realBase, err)
	}
	return filepath.IsLocal(rel), nil
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
			if len(jobs) > 1 {
				// Several directories write to one stream, so each says
				// whose lines follow.
				_, _ = fmt.Fprintln(stdout, job.Key)
			}
			dir := job.Config.OutputDirectory(job.Dir)
			if slices.ContainsFunc(changed, func(written string) bool { return load.Imports(pl, dir, written) }) {
				if _, pl, err = loadPackages(jobs[0].Dir, dirs, patterns); err != nil {
					return err
				}
				changed = nil
			}
			files, err := resolveJob(cmd, pl, job)
			if err != nil {
				return err
			}
			wrote, err := writeJob(stdout, stderr, job, files)
			if err != nil {
				return err
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
