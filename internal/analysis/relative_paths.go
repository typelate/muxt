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
	root = cmp.Or(root, dir)
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
		inside, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(inside, "..") {
			continue
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			continue
		}
		pairs = append(pairs, path, rel)
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
