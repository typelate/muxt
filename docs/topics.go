// Package docs embeds the reference, how-to, tutorial and explanation pages so
// the muxt binary can print them in a project that has no checkout of muxt.
package docs

import (
	"bufio"
	"io/fs"
	"strings"
)

// Topic is one embedded markdown page.
type Topic struct {
	// ID is the path under docs/ without ".md"; pages under reference/ drop
	// that prefix, so reference/template-names is "template-names".
	ID      string `json:"id"`
	Title   string `json:"title"`
	Section string `json:"section"`
	path    string
}

// Sections lists the section names in the order an index prints them.
var Sections = []string{"reference", "how-to", "tutorials", "explanation"}

// Topics returns every topic, ordered by section then ID.
func Topics() []Topic {
	var topics []Topic
	for _, section := range Sections {
		_ = fs.WalkDir(files, section, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
				return nil
			}
			id := strings.TrimSuffix(p, ".md")
			if section == "reference" {
				id = strings.TrimPrefix(id, "reference/")
			}
			topics = append(topics, Topic{ID: id, Title: title(p), Section: section, path: p})
			return nil
		})
	}
	return topics
}

// Content returns the page verbatim.
func (t Topic) Content() ([]byte, error) {
	return files.ReadFile(t.path)
}

func title(p string) string {
	f, err := files.Open(p)
	if err != nil {
		return p
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	for s.Scan() {
		if t, ok := strings.CutPrefix(s.Text(), "# "); ok {
			return t
		}
	}
	return p
}
