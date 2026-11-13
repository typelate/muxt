package cli

import (
	"fmt"
	"os"

	"github.com/spf13/pflag"

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
)

// jobSpec is what a command tells selectJobs about itself, so one function
// answers which packages and settings a directory means for every command.
type jobSpec struct {
	// Name is the command in errors: "generate", "muxt", "check", ...
	Name string
	// Settings are the flag spellings that configure a package. A flag outside
	// it, such as --verbose or --format, never decides between a muxt.json
	// entry and the command line.
	Settings []string
	Scope    scope
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
	var given []string
	if flags != nil {
		given = givenSettingFlags(flags, spec.Settings)
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
			return selection{}, &composite.ConfiguredDirectoryError{Path: f.Name, Command: spec.Name, Flags: given}
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
		if spec.Scope == itsEntry {
			// Only the directory's own entry is read, so another entry's
			// mistake cannot fail a command that acts on one package.
			if !lookup.HasEntry {
				return fallbackSelection(spec, dir, f, nil)
			}
			keys = []string{lookup.Key}
		}
	}
	version := spec.Version
	if version == nil {
		version = noVersion
	}
	var jobs []packageJob
	for _, key := range keys {
		entryDir := f.DirOf(key)
		if err := requireEntryDirectory(f, key, entryDir); err != nil {
			return selection{}, err
		}
		entry, _ := f.Resolve(key)
		config, err := entryConfiguration(entry, entryDir, getEnv, version)
		if err != nil {
			return selection{}, &composite.InvalidEntryError{Path: f.Name, Where: fmt.Sprintf("packages[%q]", key), Err: err}
		}
		config.Verbose = spec.Verbose
		jobs = append(jobs, packageJob{File: f.Name, Key: key, Dir: entryDir, Config: config})
	}
	return selection{File: f, Jobs: jobs}, nil
}

// fallbackSelection is the one job the command's own flags and defaults give
// dir. given is the setting flags that did.
func fallbackSelection(spec jobSpec, dir string, f *composite.File, given []string) (selection, error) {
	config, err := spec.Fallback(dir)
	if err != nil {
		return selection{}, err
	}
	config.Verbose = spec.Verbose
	return selection{File: f, Jobs: []packageJob{{Dir: dir, Config: config, Given: given}}}, nil
}
