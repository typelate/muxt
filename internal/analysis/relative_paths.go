package analysis

import (
	"cmp"
	"io"
	"log"
	"path/filepath"
	"slices"
	"strings"

	"github.com/typelate/muxt/internal/source"
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

// relativePaths returns a logger that writes what logger would, with the path
// of each template file in pkg shown relative to dir, so a run names own and
// shared files from one base. A path outside root stays absolute: a file in
// the module cache must not become a chain of "..".
//
// It replaces the exact paths the parsed templates carry rather than
// matching a path pattern in a message, so text that merely looks like a
// path is left alone. With no dir it returns logger.
func relativePaths(logger *log.Logger, dir, root string, pkg source.Package) *log.Logger {
	if dir == "" {
		return logger
	}
	base := PathBase{Dir: dir, ModuleRoot: root}
	seen := make(map[string]bool)
	var paths []string
	for _, variable := range pkg.Variables {
		for _, t := range variable.Set.Templates() {
			if t.Tree == nil || !filepath.IsAbs(t.Tree.Filepath) || seen[t.Tree.Filepath] {
				continue
			}
			seen[t.Tree.Filepath] = true
			paths = append(paths, t.Tree.Filepath)
		}
	}
	// A longer path first, so one that is the prefix of another does not
	// shadow it.
	slices.SortFunc(paths, func(a, b string) int { return cmp.Or(cmp.Compare(len(b), len(a)), strings.Compare(a, b)) })
	var pairs []string
	for _, path := range paths {
		pairs = append(pairs, path, base.Relative(path))
	}
	return log.New(&replacingWriter{w: logger.Writer(), replacer: strings.NewReplacer(pairs...)}, logger.Prefix(), logger.Flags())
}

// replacingWriter applies replacer to each write. A log.Logger makes one write
// per message, so a path is never split across two.
type replacingWriter struct {
	w        io.Writer
	replacer *strings.Replacer
}

func (r *replacingWriter) Write(p []byte) (int, error) {
	if _, err := r.replacer.WriteString(r.w, string(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}
