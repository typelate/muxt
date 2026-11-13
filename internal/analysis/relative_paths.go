package analysis

import (
	"cmp"
	"path/filepath"
	"strings"
)

// PathBase is the directory a run shows template paths relative to: the
// working directory muxt started in, after -C.
type PathBase struct {
	// Dir is the working directory. Empty leaves every path as it is.
	Dir string

	// ModuleRoot bounds Dir: a path outside it stays absolute, so a file in
	// the module cache does not become a chain of "..". Empty means Dir.
	ModuleRoot string
}

// Relative returns path relative to the base, or path itself when it is not
// absolute, the base is empty, or path is outside the module root.
func (b PathBase) Relative(path string) string {
	if b.Dir == "" || !filepath.IsAbs(path) {
		return path
	}
	inside, err := filepath.Rel(cmp.Or(b.ModuleRoot, b.Dir), path)
	if err != nil || strings.HasPrefix(inside, "..") {
		return path
	}
	rel, err := filepath.Rel(b.Dir, path)
	if err != nil {
		return path
	}
	return rel
}
