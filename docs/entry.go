package docs

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed README.md reference how-to tutorials explanation
var files embed.FS

// Entry is one embedded file, named by its repo-relative path.
type Entry struct {
	// Path is the repo-relative path, such as "docs/reference/cli.md".
	Path string
}

// Resolve returns the embedded file named by arg. arg may carry a #fragment,
// a leading "./", a trailing "/", or a leading "docs/"; the returned Path
// always starts with "docs/".
func Resolve(arg string) (Entry, error) {
	rel, _, _ := strings.Cut(arg, "#")
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimSuffix(rel, "/")
	rel = strings.TrimPrefix(rel, "docs/")
	info, err := fs.Stat(files, rel)
	if err != nil {
		return Entry{}, fmt.Errorf("unknown documentation path %q: %w", arg, err)
	}
	if info.IsDir() {
		return Entry{}, fmt.Errorf("documentation path %q is a directory", arg)
	}
	return Entry{Path: "docs/" + rel}, nil
}

// Render returns the file's content verbatim. ref is reserved for rewriting
// links to the build's version and is not used yet.
func (e Entry) Render(ref string) ([]byte, error) {
	content, err := files.ReadFile(strings.TrimPrefix(e.Path, "docs/"))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", e.Path, err)
	}
	return content, nil
}
