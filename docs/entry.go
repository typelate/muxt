package docs

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed reference how-to tutorials explanation
var files embed.FS

// Entry is one embedded file, named by its repo-relative path.
type Entry struct {
	// Path is the repo-relative path, such as "docs/reference/cli.md".
	Path string
}

// Resolve returns the embedded file named by arg. For now arg must be the
// exact repo-relative path of a file under docs/.
func Resolve(arg string) (Entry, error) {
	rel, ok := strings.CutPrefix(arg, "docs/")
	if !ok {
		return Entry{}, fmt.Errorf("unknown documentation path %q", arg)
	}
	info, err := fs.Stat(files, rel)
	if err != nil {
		return Entry{}, fmt.Errorf("unknown documentation path %q: %w", arg, err)
	}
	if info.IsDir() {
		return Entry{}, fmt.Errorf("documentation path %q is a directory", arg)
	}
	return Entry{Path: arg}, nil
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
