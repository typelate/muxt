package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/check"
	"github.com/typelate/muxt/internal/composite"
	"github.com/typelate/muxt/internal/generate"
)

// scope is what a flagless run in the muxt.json's own directory acts on.
type scope uint8

const (
	// eachEntry is every entry, each validated before any is returned.
	eachEntry scope = iota + 1
	// itsEntry is the directory's entry, else the spec's Fallback.
	itsEntry
	// itsEntryOnly is itsEntry, except that in the muxt.json's own directory
	// without an entry it is an error when the file lists packages: the
	// command acts on one, and the directory is not one of them.
	itsEntryOnly
)

// jobSpec is what a command tells selectJobs about itself, so one function
// answers which packages and settings a directory means for every command.
type jobSpec struct {
	// Name is the command in errors: "generate", "muxt", "check", ...
	Name string
	// Settings are what the command registers that configures a package. A
	// flag that is not a spelling of one, such as --verbose or --format, never
	// decides between a muxt.json entry and the command line.
	Settings []setting
	Scope    scope
	// OnePackage words the error of an itsEntryOnly run in a muxt.json's own
	// directory that is not one of its packages.
	OnePackage onePackageWording
	// RequireEntry makes a flagless run without an entry for its directory an
	// error, for the command that writes from the file.
	RequireEntry bool
	// Verbose is a run setting, never a muxt.json key, so it is copied onto
	// every job here, once.
	Verbose bool
	// Version is what a generated header records; nil means no header is
	// written and there is none to record.
	Version func() (string, bool)
	// Fallback is the configuration the command's own flags and defaults give
	// dir.
	Fallback func(dir string) (generate.RoutesFileConfiguration, error)
}

// onePackageWording is how a command that acts on one package says so, and how
// a person picks the package: Task follows the command name ("tests one
// package") and Where leads the directories ("pass one of").
type onePackageWording struct {
	Task, Where string
}

// packageJob is one package directory and what to run a command with. Key is
// the packages key the configuration came from, and File the absolute path of
// the muxt.json holding it; both are empty when flags gave the configuration.
// Given is the settings that did, by the spelling typed, so a report can name
// them.
type packageJob struct {
	File   string
	Key    string
	Dir    string
	Config generate.RoutesFileConfiguration
	Given  givenSettings
	// Keyed is set on every job of a run that read every entry of the muxt.json
	// in its own directory, however many entries it holds. Such a run names each
	// package by its key; a run in one package directory, or one that flags
	// configured, does not. Choosing by the entry count instead would change
	// the output shape of a run when an entry is added.
	Keyed bool
}

// selection is what selectJobs chose.
type selection struct {
	// File is the muxt.json read, nil when there is none.
	File *composite.File
	// Jobs is never empty on success, and is in key order.
	Jobs []packageJob
}

// EntryFile is the muxt.json job's configuration came from, nil when the
// command's flags and defaults configured it, so its report names no file.
func (s selection) EntryFile(job packageJob) *composite.File {
	if job.File == "" {
		return nil
	}
	return s.File
}

// selectJobs chooses the package directories and configurations a command
// acts on in dir, from the nearest muxt.json and the setting flags given.
// Flags win outright so a command line never half-merges with a file: a
// setting flag in a directory with an entry is an error, and in any other
// directory the Fallback decides. It validates every job's directory and entry
// before returning any. flags may be nil when nothing was given.
func selectJobs(spec jobSpec, dir string, flags *pflag.FlagSet, getEnv func(string) string) (selection, error) {
	var given givenSettings
	if flags != nil {
		given = givenFlags(flags, spec.Settings)
	}
	// A path that is not a directory is read from no muxt.json: the runner
	// says what is wrong with it, which no entry's message can.
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fallbackSelection(spec, dir, nil, given)
	}
	lookup, err := lookupMuxtJSONEntry(dir)
	if err != nil {
		return selection{}, err
	}
	f := lookup.File
	if len(given) > 0 {
		if lookup.HasEntry {
			return selection{}, given.configuredDirectory(f.Name, spec.Name)
		}
		return fallbackSelection(spec, dir, f, given)
	}
	if f == nil {
		return fallbackSelection(spec, dir, nil, nil)
	}
	ownKey, err := f.KeyFor(f.Dir)
	if err != nil {
		return selection{}, fmt.Errorf("muxt.json entry for %s: %w", f.Dir, err)
	}
	keys := []string{lookup.Key}
	keyed := false
	if lookup.Key != ownKey {
		if !lookup.HasEntry {
			if spec.RequireEntry {
				return selection{}, &composite.MissingEntryError{Path: f.Name, Key: lookup.Key}
			}
			return fallbackSelection(spec, dir, f, nil)
		}
	} else {
		if len(f.Packages) == 0 {
			if spec.RequireEntry {
				return selection{}, &composite.EmptyPackagesError{Path: f.Name}
			}
			return fallbackSelection(spec, dir, f, nil)
		}
		keys = f.Keys()
		if spec.Scope == itsEntry || spec.Scope == itsEntryOnly {
			// Only the directory's own entry is read, so another entry's
			// mistake cannot fail a command that acts on one package.
			if !lookup.HasEntry {
				if spec.Scope == itsEntryOnly {
					return selection{}, severalPackages(spec, f)
				}
				return fallbackSelection(spec, dir, f, nil)
			}
			keys = []string{lookup.Key}
		} else {
			keyed = true
		}
	}
	version := spec.Version
	if version == nil {
		version = noVersion
	}
	var jobs []packageJob
	keyOf := map[string]string{} // a real directory to the first key that reached it
	for _, key := range keys {
		entryDir := f.DirOf(key)
		realDir, err := requireEntryDirectory(f, key, entryDir)
		if err != nil {
			return selection{}, err
		}
		if other, repeated := keyOf[realDir]; repeated {
			return selection{}, &composite.SameDirectoryError{Path: f.Name, Key: key, Other: other, Dir: realDir}
		}
		keyOf[realDir] = key
		entry, _ := f.Resolve(key)
		config, err := entryConfiguration(entry, fromMuxtJSON, generateEnv{Dir: entryDir, GetEnv: getEnv, Version: version})
		if err != nil {
			return selection{}, &composite.InvalidEntryError{Path: f.Name, Where: fmt.Sprintf("packages[%q]", key), Err: err}
		}
		config.Verbose = spec.Verbose
		jobs = append(jobs, packageJob{File: f.Name, Key: key, Dir: entryDir, Config: config, Keyed: keyed})
	}
	return selection{File: f, Jobs: jobs}, nil
}

// severalPackages names the directories of f's entries, relative to f, for a
// command that tests one package.
func severalPackages(spec jobSpec, f *composite.File) error {
	var dirs []string
	for _, key := range f.Keys() {
		rel, err := filepath.Rel(f.Dir, f.DirOf(key))
		if err != nil {
			return fmt.Errorf("muxt.json entry %s: %w", key, err)
		}
		dirs = append(dirs, filepath.ToSlash(rel))
	}
	return &composite.SeveralPackagesError{Path: f.Name, Command: spec.Name, Task: spec.OnePackage.Task, Where: spec.OnePackage.Where, Dirs: dirs}
}

// fallbackSelection is the one job the command's own flags and defaults give
// dir. given is the setting flags that did.
func fallbackSelection(spec jobSpec, dir string, f *composite.File, given givenSettings) (selection, error) {
	config, err := spec.Fallback(dir)
	if err != nil {
		return selection{}, err
	}
	config.Verbose = spec.Verbose
	return selection{File: f, Jobs: []packageJob{{Dir: dir, Config: config, Given: given}}}, nil
}

// entryVariableError returns the error naming job's muxt.json entry when err
// says the package lacks the templates variable, so the fix is in the file.
// ok is false for any other err, and for a job the command line configured.
func entryVariableError(job packageJob, err error) (entryErr error, ok bool) {
	missing, ok := errors.AsType[*check.VariableNotFoundError](err)
	if !ok || job.File == "" {
		return nil, false
	}
	return &composite.NoVariableError{Path: job.File, Key: job.Key, Package: missing.Package, Variable: missing.Variable}, true
}

// configuredHook is analysis.NewModule's answer to whether a muxt.json entry
// configures a package directory. It asks selectJobs, as every other command
// does, so an entry that breaks a rule fails here too instead of passing as
// configured.
func configuredHook(getEnv func(string) string) func(dir string) (bool, error) {
	spec := jobSpec{
		Name:  exploreModuleCommandName,
		Scope: itsEntry,
		Fallback: func(dir string) (generate.RoutesFileConfiguration, error) {
			return defaultConfiguration(dir, getEnv)
		},
	}
	return func(dir string) (bool, error) {
		sel, err := selectJobs(spec, dir, nil, getEnv)
		if err != nil {
			return false, err
		}
		return sel.Jobs[0].File != "", nil
	}
}
