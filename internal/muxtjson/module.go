package muxtjson

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// moduleOf returns the module path from the nearest go.mod at or above dir and
// the directory holding that go.mod. found is false when no go.mod is on the
// way up. It reads the file itself, so a run that only reads muxt.json needs
// no go command.
func moduleOf(dir string) (modulePath, root string, found bool, err error) {
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			modulePath := modfile.ModulePath(data)
			if modulePath == "" {
				return "", "", false, fmt.Errorf("%s has no module line", filepath.Join(dir, "go.mod"))
			}
			return modulePath, dir, true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", false, fmt.Errorf("reading %s: %w", filepath.Join(dir, "go.mod"), err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", false, nil
		}
		dir = parent
	}
}
