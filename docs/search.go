package docs

import (
	"fmt"
	"strings"
)

// Match is a line of an embedded file that holds a search query.
type Match struct {
	// Path is the repo-relative path of the file, such as "docs/reference/cli.md".
	Path string
	// Line is the 1-based line number within the file.
	Line int
	// Text is the line without its trailing newline or carriage return.
	Text string
}

// Search returns every line of the files under dir that holds query, ignoring
// case, in path then line order. The line is matched as raw file content, so
// the query is a literal string, not a pattern.
func Search(dir Entry, query string) ([]Match, error) {
	paths, err := Files(dir)
	if err != nil {
		return nil, fmt.Errorf("searching %s: %w", dir.Path, err)
	}
	needle := strings.ToLower(query)
	var matches []Match
	for _, p := range paths {
		content, err := files.ReadFile(embedName(p))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", p, err)
		}
		for i, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSuffix(line, "\r")
			if strings.Contains(strings.ToLower(line), needle) {
				matches = append(matches, Match{Path: p, Line: i + 1, Text: line})
			}
		}
	}
	return matches, nil
}
