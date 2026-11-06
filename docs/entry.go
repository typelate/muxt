package docs

import (
	"cmp"
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

// NotFoundError reports that a documentation path names no embedded file or
// directory. Near holds up to five repo-relative paths, closest first.
type NotFoundError struct {
	// Arg is the path argument as given.
	Arg string
	// Near holds the repo-relative paths closest to Arg, closest first.
	Near []string
}

// Error names Arg and the closest paths, or says how to list every path when
// no path is close.
func (e *NotFoundError) Error() string {
	if len(e.Near) == 0 {
		return fmt.Sprintf("unknown documentation path %q; run muxt documentation to list every path", e.Arg)
	}
	return fmt.Sprintf("unknown documentation path %q; did you mean: %s", e.Arg, strings.Join(e.Near, ", "))
}

// Resolve returns the embedded file or directory named by arg. arg may carry a
// #fragment, a leading "./", a trailing "/", or a leading "docs/". The empty
// argument and "docs" name the root directory, whose Path is "docs". Any other
// returned Path starts with "docs/". A path that names nothing returns a
// *NotFoundError.
func Resolve(arg string) (Entry, error) {
	rel, _, _ := strings.Cut(arg, "#")
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimSuffix(rel, "/")
	rel = strings.TrimPrefix(rel, "docs/")
	if rel == "" || rel == "docs" {
		return Entry{Path: "docs", Dir: true}, nil
	}
	info, err := fs.Stat(files, rel)
	if err != nil {
		near, nearErr := nearPaths(rel)
		if nearErr != nil {
			return Entry{}, fmt.Errorf("finding paths near %q: %w", arg, nearErr)
		}
		return Entry{}, &NotFoundError{Arg: arg, Near: near}
	}
	return Entry{Path: "docs/" + rel, Dir: info.IsDir()}, nil
}

// nearPaths returns up to five paths from the root directory closest to rel,
// closest first. A path is close when its edit distance to rel, measured
// against its path without the "docs/" prefix or its base name without
// extension, is at most a third of rel's length (minimum 2). A path whose base
// name without extension equals rel's is always included.
func nearPaths(rel string) ([]string, error) {
	all, err := Files(Entry{Path: "docs", Dir: true})
	if err != nil {
		return nil, err
	}
	limit := max(len(rel)/3, 2)
	argName := baseName(rel)
	var near []candidate
	for _, p := range all {
		dist := min(editDistance(rel, strings.TrimPrefix(p, "docs/")), editDistance(rel, baseName(p)))
		if dist <= limit || baseName(p) == argName {
			near = append(near, candidate{path: p, dist: dist})
		}
	}
	slices.SortFunc(near, func(a, b candidate) int {
		if c := cmp.Compare(a.dist, b.dist); c != 0 {
			return c
		}
		return strings.Compare(a.path, b.path)
	})
	near = near[:min(len(near), 5)]
	paths := make([]string, len(near))
	for i, c := range near {
		paths[i] = c.path
	}
	return paths, nil
}

// candidate is a file path with its edit distance to a documentation path
// argument.
type candidate struct {
	path string
	dist int
}

// baseName returns the last element of p without its extension.
func baseName(p string) string {
	return strings.TrimSuffix(path.Base(p), path.Ext(p))
}

// editDistance returns the Levenshtein distance between a and b: the fewest
// single-rune insertions, deletions, and substitutions that turn a into b.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// embedName returns the name of p within the embedded file system, which is
// rooted at docs/. The root directory is ".".
func embedName(p string) string {
	if p == "docs" {
		return "."
	}
	return strings.TrimPrefix(p, "docs/")
}

// Files returns the repo-relative path of every file under dir, sorted.
func Files(dir Entry) ([]string, error) {
	var paths []string
	err := fs.WalkDir(files, embedName(dir.Path), func(name string, d fs.DirEntry, err error) error {
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
