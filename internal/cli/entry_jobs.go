package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
)

// packageJob is one package directory and what to run a command with. Key is
// the packages key the configuration came from, and File the absolute path of
// the muxt.json holding it; both are empty when flags gave the configuration.
// Given is the setting flags that did, so a report can name them.
type packageJob struct {
	File   string
	Key    string
	Dir    string
	Config generate.RoutesFileConfiguration
	Given  []string
}

// entryLookup is where a directory stands in the nearest muxt.json.
type entryLookup struct {
	// File is the muxt.json at or above the directory, nil when there is none.
	File *composite.File
	// Key is the directory's packages key in File.
	Key string
	// HasEntry is false when File has no entry under Key, which is not an
	// error here.
	HasEntry bool
}

// lookupMuxtJSONEntry finds the nearest muxt.json at or above wd and the key
// for wd in it.
func lookupMuxtJSONEntry(wd string) (entryLookup, error) {
	f, found, err := composite.Find(wd, deprecatedMuxtJSONKeys())
	if err != nil {
		// Find's errors already name the file, and a *composite.ParseError
		// must reach the caller as it is.
		return entryLookup{}, err
	}
	if !found {
		return entryLookup{}, nil
	}
	key, err := f.KeyFor(wd)
	if err != nil {
		return entryLookup{}, fmt.Errorf("muxt.json entry for %s: %w", wd, err)
	}
	_, hasEntry := f.Packages[key]
	return entryLookup{File: f, Key: key, HasEntry: hasEntry}, nil
}

// rejectConfiguredDirectory returns a *composite.ConfiguredDirectoryError when
// the muxt.json for wd has an entry for it, since that entry configures the
// directory and flags would be a second source for the same fields.
func rejectConfiguredDirectory(command, wd string, flagNames []string) error {
	lookup, err := lookupMuxtJSONEntry(wd)
	if err != nil || !lookup.HasEntry {
		return err
	}
	return &composite.ConfiguredDirectoryError{Path: lookup.File.Name, Command: command, Flags: flagNames}
}

// entryJobs returns the jobs the nearest muxt.json gives a flagless run in wd.
// In the file's own directory that is one job per entry, and every entry is
// validated before any job is returned. In a sub-directory it is that
// directory's entry alone. found is false when the flags decide the run, and
// when the file has no entry for wd (see requireEntry). file
// is the muxt.json the jobs came from, for a caller that says where each
// setting came from.
func entryJobs(wd string, getEnv func(string) string, version func() (string, bool)) (file *composite.File, jobs []packageJob, found bool, err error) {
	lookup, err := lookupMuxtJSONEntry(wd)
	if err != nil || lookup.File == nil {
		return nil, nil, false, err
	}
	f, key := lookup.File, lookup.Key
	ownKey, err := f.KeyFor(f.Dir)
	if err != nil {
		return nil, nil, false, fmt.Errorf("muxt.json entry for %s: %w", f.Dir, err)
	}
	keys := []string{key}
	if key != ownKey {
		if !lookup.HasEntry {
			return nil, nil, false, nil
		}
	} else {
		if len(f.Packages) == 0 {
			return nil, nil, false, nil
		}
		keys = f.Keys()
	}
	for _, k := range keys {
		dir := f.DirOf(k)
		if err := requireEntryDirectory(f, k, dir); err != nil {
			return nil, nil, false, err
		}
		entry, _ := f.Resolve(k)
		config, err := entryConfiguration(entry, dir, getEnv, version)
		if err != nil {
			return nil, nil, false, &composite.InvalidEntryError{Path: f.Name, Where: fmt.Sprintf("packages[%q]", k), Err: err}
		}
		jobs = append(jobs, packageJob{File: f.Name, Key: k, Dir: dir, Config: config})
	}
	return f, jobs, true, nil
}

// entryJobFor returns the job the muxt.json entry for dir gives a command run
// in dir, and found is false when the command's own settings decide instead.
// flagNames are the entry's flags the command was given: with any, the entry
// for dir would be a second source for the same settings, so it is rejected by
// rejectConfiguredDirectory. Without them the entry is read the way generate
// reads it. A directory below a muxt.json with no entry for it is not found,
// so the command's own flags and defaults apply. The command takes only the
// settings it has flags for from the job.
func entryJobFor(command, dir string, flagNames []string, getEnv func(string) string) (packageJob, bool, error) {
	// A path that is not a directory has no entry. The command's runner is
	// what says so, and a search for muxt.json under a file would fail first
	// with a message about muxt.json.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return packageJob{}, false, nil
	}
	if len(flagNames) > 0 {
		return packageJob{}, false, rejectConfiguredDirectory(command, dir, flagNames)
	}
	lookup, err := lookupMuxtJSONEntry(dir)
	if err != nil || lookup.File == nil {
		return packageJob{}, false, err
	}
	_, jobs, found, err := entryJobs(dir, getEnv, noVersion)
	if err != nil || !found {
		return packageJob{}, false, err
	}
	// entryJobs returns every entry when dir is the file's own directory, so
	// the job for dir is the one whose key is dir's.
	for _, job := range jobs {
		if job.Key == lookup.Key {
			return job, true, nil
		}
	}
	return packageJob{}, false, nil
}

// requireEntryDirectory returns a *composite.MissingDirectoryError unless dir,
// the directory of key in f, is a directory, and a
// *composite.OutsideDirectoryError when it is one that a symbolic link resolves
// outside f's directory. The errors name dir by its absolute path.
func requireEntryDirectory(f *composite.File, key, dir string) error {
	info, statErr := os.Stat(dir)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("muxt.json entry %q: %w", key, statErr)
	}
	if statErr != nil || !info.IsDir() {
		return &composite.MissingDirectoryError{Path: f.Name, Key: key, Dir: dir}
	}
	inside, err := resolvesInside(f.Dir, dir)
	if err != nil {
		return fmt.Errorf("muxt.json entry %q: %w", key, err)
	}
	if !inside {
		return &composite.OutsideDirectoryError{Path: f.Name, Key: key, Dir: dir}
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

// noVersion is the version func of a command that writes no header, so it has
// no use for the version a header would record.
func noVersion() (string, bool) { return "", false }

// defaultConfiguration is what generate's defaults and the environment give a
// package with no muxt.json entry, for a command that reads a package and sets
// only the few fields it has flags for.
func defaultConfiguration(dir string, getEnv func(string) string) (generate.RoutesFileConfiguration, error) {
	return entryConfiguration(composite.Entry{}, dir, getEnv, noVersion)
}
