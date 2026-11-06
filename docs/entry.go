package docs

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

//go:embed README.md all:examples reference how-to tutorials explanation
var files embed.FS

// Entry is one embedded file or directory, named by its repo-relative path.
type Entry struct {
	// Path is the repo-relative path, such as "docs/reference/cli.md".
	Path string
	// Dir reports whether Path names a directory.
	Dir bool
}

// Resolve returns the embedded file or directory named by arg. arg may carry a
// #fragment, a leading "./", a trailing "/", or a leading "docs/"; the returned
// Path always starts with "docs/".
func Resolve(arg string) (Entry, error) {
	rel, _, _ := strings.Cut(arg, "#")
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimSuffix(rel, "/")
	rel = strings.TrimPrefix(rel, "docs/")
	info, err := fs.Stat(files, rel)
	if err != nil {
		return Entry{}, fmt.Errorf("unknown documentation path %q: %w", arg, err)
	}
	return Entry{Path: "docs/" + rel, Dir: info.IsDir()}, nil
}

// Files returns the repo-relative path of every file under dir, sorted.
func Files(dir Entry) ([]string, error) {
	var paths []string
	err := fs.WalkDir(files, strings.TrimPrefix(dir.Path, "docs/"), func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, "docs/"+name)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", dir.Path, err)
	}
	slices.Sort(paths)
	return paths, nil
}

// Render returns a markdown file's content verbatim. Any other file is
// returned under its path line in a code fence. A directory is returned as
// the path of each file under it, one per line. ref is reserved for rewriting
// links to the build's version and is not used yet.
func (e Entry) Render(ref string) ([]byte, error) {
	if e.Dir {
		paths, err := Files(e)
		if err != nil {
			return nil, err
		}
		var b strings.Builder
		for _, p := range paths {
			b.WriteString(p)
			b.WriteString("\n")
		}
		return []byte(b.String()), nil
	}
	content, err := files.ReadFile(strings.TrimPrefix(e.Path, "docs/"))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", e.Path, err)
	}
	if path.Ext(e.Path) == ".md" {
		return content, nil
	}
	return fenced(e.Path, content), nil
}

// fenced returns the path line, a blank line, and content inside a code fence
// tagged for the file's extension. A fence of four backticks is used when the
// content has a line that opens a three-backtick fence.
func fenced(filePath string, content []byte) []byte {
	mark := "```"
	for line := range strings.SplitSeq(string(content), "\n") {
		if strings.HasPrefix(line, "```") {
			mark = "````"
			break
		}
	}
	var b strings.Builder
	b.WriteString(filePath)
	b.WriteString("\n\n")
	b.WriteString(mark)
	b.WriteString(languageTag(filePath))
	b.WriteString("\n")
	b.Write(content)
	if len(content) == 0 || content[len(content)-1] != '\n' {
		b.WriteString("\n")
	}
	b.WriteString(mark)
	b.WriteString("\n")
	return []byte(b.String())
}

// languageTag returns the code fence language for filePath's extension, or
// the empty string when the extension has no known language.
func languageTag(filePath string) string {
	switch path.Ext(filePath) {
	case ".go":
		return "go"
	case ".gohtml":
		return "html"
	case ".css":
		return "css"
	default:
		return ""
	}
}
