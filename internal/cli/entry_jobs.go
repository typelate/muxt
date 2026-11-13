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
