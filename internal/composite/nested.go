package composite

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// exists reports whether path is there. Any error other than the path being
// missing is returned, so a directory the process cannot read is not taken for
// one with no muxt.json.
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("checking %s: %w", path, err)
}

// requireNoneAbove returns a *NestedError for the nearest muxt.json in a
// directory above f's, up to and including the module root.
func (f *File) requireNoneAbove() error {
	for dir := f.Dir; dir != f.moduleDir; {
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
		outer := filepath.Join(dir, FileName)
		found, err := exists(outer)
		if err != nil {
			return err
		}
		if found {
			return &NestedError{Inner: f.Name, Outer: outer}
		}
	}
	return nil
}

// CheckNotNested returns a *NestedError when a muxt.json sits in dir or in a
// directory between dir and f's own, the file's directory excluded. dir is
// the absolute directory of one of f's entries, so a run that reads the entry
// does not meet a file that would configure the same package differently.
// A dir that is not below f's directory has nothing between it and f. The walk
// stops at a directory holding go.mod: a muxt.json there or below belongs to
// that nested module, not f's.
func (f *File) CheckNotNested(dir string) error {
	rel, err := filepath.Rel(f.Dir, dir)
	if err != nil {
		return fmt.Errorf("checking %s against %s: %w", dir, f.Name, err)
	}
	if !filepath.IsLocal(rel) {
		return nil
	}
	for ; dir != f.Dir; dir = filepath.Dir(dir) {
		hasModule, err := exists(filepath.Join(dir, "go.mod"))
		if err != nil {
			return err
		}
		if hasModule {
			return nil
		}
		inner := filepath.Join(dir, FileName)
		found, err := exists(inner)
		if err != nil {
			return err
		}
		if found {
			return &NestedError{Inner: inner, Outer: f.Name}
		}
	}
	return nil
}

// requireNoneBelow returns a *NestedError for the first muxt.json, in the
// order filepath.WalkDir visits, below dir in dir's module. It skips a
// directory with its own go.mod, which is another module, and vendor, testdata
// and names starting with . or _, which the go command ignores. outer is the
// file that is to be written in dir.
func requireNoneBelow(dir, outer string) error {
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if d.Name() == FileName && filepath.Dir(path) != dir {
				return &NestedError{Inner: path, Outer: outer}
			}
			return nil
		}
		if path == dir {
			return nil
		}
		name := d.Name()
		if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			return filepath.SkipDir
		}
		hasModule, err := exists(filepath.Join(path, "go.mod"))
		if err != nil {
			return err
		}
		if hasModule {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		if _, ok := errors.AsType[*NestedError](err); ok {
			return err
		}
		return fmt.Errorf("looking for muxt.json below %s: %w", dir, err)
	}
	return nil
}
