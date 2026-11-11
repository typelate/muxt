// Package muxtjson reads a muxt.json file, which records the generate flags of
// package directories so a flagless muxt generate knows what to write.
package muxtjson

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// FileName is the name of the configuration file muxt looks for.
const FileName = "muxt.json"

// File is a muxt.json. Dir is the directory holding the file, set by Read.
// Packages is keyed by slash-separated directory, relative to Dir, such as ".".
type File struct {
	Dir      string           `json:"-"`
	Packages map[string]Entry `json:"packages"`
}

// Find reads the first muxt.json found walking up from wd, starting at wd
// itself. found is false when no directory up to the filesystem root has one.
func Find(wd string) (f *File, found bool, err error) {
	dir, err := filepath.Abs(wd)
	if err != nil {
		return nil, false, fmt.Errorf("absolute path of %s: %w", wd, err)
	}
	for {
		f, err := Read(filepath.Join(dir, FileName))
		if err == nil {
			return f, true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, false, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, false, nil
		}
		dir = parent
	}
}

// KeyFor returns the slash-separated path of dir relative to the directory
// holding f, the key its Packages entry uses. The directory itself is ".".
func (f *File) KeyFor(dir string) (string, error) {
	rel, err := filepath.Rel(f.Dir, dir)
	if err != nil {
		return "", fmt.Errorf("key for %s: %w", dir, err)
	}
	return filepath.ToSlash(rel), nil
}

// Entry holds the generate flags of one package directory. A nil field is
// absent from the file, so the flag's default applies.
type Entry struct {
	UseReceiverType  *string `json:"use-receiver-type,omitempty"`
	OutputRoutesFunc *string `json:"output-routes-func,omitempty"`
}

// Read reads the muxt.json at path. A missing file is an error that matches
// fs.ErrNotExist.
func Read(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	f.Dir = filepath.Dir(path)
	return &f, nil
}
