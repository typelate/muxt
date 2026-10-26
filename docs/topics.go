// Package docs embeds the reference, how-to, tutorial and explanation pages so
// the muxt binary can print them in a project that has no checkout of muxt.
package docs

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

//go:embed reference how-to tutorials explanation
var files embed.FS

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

// matches reports whether id contains want, or want contains id, or id contains
// any 3+ letter word of want.
func matches(id, want string) bool {
	if want == "" {
		return false
	}
	if strings.Contains(id, want) || strings.Contains(want, id) {
		return true
	}
	for _, word := range strings.FieldsFunc(want, func(r rune) bool { return r == '-' || r == '/' || r == '_' }) {
		if len(word) >= 3 && strings.Contains(id, word) {
			return true
		}
	}
	return false
}

// Find returns the topic named id. A "reference/" prefix and ".md" suffix are
// accepted. For an unknown id the error names the closest topics.
func Find(id string) (Topic, []Topic, error) {
	topics := Topics()
	want := strings.TrimSuffix(strings.TrimPrefix(id, "docs/"), ".md")
	want = strings.TrimPrefix(want, "reference/")
	for _, t := range topics {
		if t.ID == want || t.ID == "reference/"+want {
			return t, nil, nil
		}
	}
	var near []Topic
	for _, t := range topics {
		if matches(t.ID, want) {
			near = append(near, t)
		}
	}
	slices.SortStableFunc(near, func(a, b Topic) int { return len(a.ID) - len(b.ID) })
	near = near[:min(len(near), 5)]
	if len(near) == 0 {
		return Topic{}, nil, fmt.Errorf("unknown documentation topic %q; run `muxt documentation` to list topics", id)
	}
	ids := make([]string, len(near))
	for i, t := range near {
		ids[i] = t.ID
	}
	return Topic{}, near, fmt.Errorf("unknown documentation topic %q; did you mean: %s", id, strings.Join(ids, ", "))
}
