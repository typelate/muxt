package docs_test

import (
	"os"
	"strings"
	"testing"

	"github.com/typelate/muxt/docs"
)

func TestSearch(t *testing.T) {
	root, err := docs.Resolve("")
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v", "", err)
	}
	for _, tt := range []struct {
		query      string
		wantSuffix string
	}{
		{query: "ExecuteTemplate"},
		{query: "executetemplate"},
		{query: "package main", wantSuffix: ".go"},
	} {
		matches, err := docs.Search(root, tt.query)
		if err != nil {
			t.Fatalf("Search(%q) error = %v", tt.query, err)
		}
		if len(matches) == 0 {
			t.Fatalf("Search(%q) returned no matches, want at least one", tt.query)
		}
		if tt.wantSuffix != "" {
			var found bool
			for _, m := range matches {
				found = found || strings.HasSuffix(m.Path, tt.wantSuffix)
			}
			if !found {
				t.Errorf("Search(%q) has no match with path suffix %q", tt.query, tt.wantSuffix)
			}
		}
		for i, m := range matches {
			if !strings.HasPrefix(m.Path, "docs/") {
				t.Errorf("Search(%q)[%d].Path = %q, want a path starting with %q", tt.query, i, m.Path, "docs/")
			}
			if !strings.Contains(strings.ToLower(m.Text), strings.ToLower(tt.query)) {
				t.Errorf("Search(%q)[%d].Text = %q, want it to contain the query ignoring case", tt.query, i, m.Text)
			}
			content, err := os.ReadFile(strings.TrimPrefix(m.Path, "docs/"))
			if err != nil {
				t.Fatalf("reading %s: %v", m.Path, err)
			}
			lines := strings.Split(string(content), "\n")
			if m.Line < 1 || m.Line > len(lines) {
				t.Fatalf("Search(%q)[%d].Line = %d, want a line of %s with %d lines", tt.query, i, m.Line, m.Path, len(lines))
			}
			if want := strings.TrimSuffix(lines[m.Line-1], "\r"); m.Text != want {
				t.Errorf("Search(%q)[%d] = %s:%d %q, want the file's line %q", tt.query, i, m.Path, m.Line, m.Text, want)
			}
			if i > 0 {
				prev := matches[i-1]
				inOrder := prev.Path < m.Path || (prev.Path == m.Path && prev.Line < m.Line)
				if !inOrder {
					t.Errorf("Search(%q)[%d] = %s:%d follows %s:%d, want path then line order", tt.query, i, m.Path, m.Line, prev.Path, prev.Line)
				}
			}
		}
	}
}

func TestSearchUnder(t *testing.T) {
	for _, tt := range []struct {
		arg        string
		query      string
		wantPrefix string
		wantPath   string
	}{
		{arg: "docs/how-to", query: "htmx", wantPrefix: "docs/how-to/"},
		{arg: "docs/reference/commands", query: "muxt check", wantPrefix: "docs/reference/commands/"},
		{arg: "docs/reference/commands/check.md", query: "muxt check", wantPath: "docs/reference/commands/check.md"},
	} {
		entry, err := docs.Resolve(tt.arg)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", tt.arg, err)
		}
		matches, err := docs.Search(entry, tt.query)
		if err != nil {
			t.Fatalf("Search(%q, %q) error = %v", tt.arg, tt.query, err)
		}
		if len(matches) == 0 {
			t.Fatalf("Search(%q, %q) returned no matches, want at least one", tt.arg, tt.query)
		}
		for i, m := range matches {
			if tt.wantPrefix != "" && !strings.HasPrefix(m.Path, tt.wantPrefix) {
				t.Errorf("Search(%q, %q)[%d].Path = %q, want a path starting with %q", tt.arg, tt.query, i, m.Path, tt.wantPrefix)
			}
			if tt.wantPath != "" && m.Path != tt.wantPath {
				t.Errorf("Search(%q, %q)[%d].Path = %q, want %q", tt.arg, tt.query, i, m.Path, tt.wantPath)
			}
		}
	}
}

func TestSearchUnderRootHasMoreMatches(t *testing.T) {
	const query = "htmx"
	root, err := docs.Resolve("")
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v", "", err)
	}
	how, err := docs.Resolve("docs/how-to")
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v", "docs/how-to", err)
	}
	all, err := docs.Search(root, query)
	if err != nil {
		t.Fatalf("Search(root, %q) error = %v", query, err)
	}
	under, err := docs.Search(how, query)
	if err != nil {
		t.Fatalf("Search(%q, %q) error = %v", "docs/how-to", query, err)
	}
	if len(all) <= len(under) {
		t.Errorf("Search(root, %q) has %d matches, want more than the %d under docs/how-to", query, len(all), len(under))
	}
}

func TestSearchIgnoresCase(t *testing.T) {
	root, err := docs.Resolve("")
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v", "", err)
	}
	upper, err := docs.Search(root, "ExecuteTemplate")
	if err != nil {
		t.Fatalf("Search(%q) error = %v", "ExecuteTemplate", err)
	}
	lower, err := docs.Search(root, "executetemplate")
	if err != nil {
		t.Fatalf("Search(%q) error = %v", "executetemplate", err)
	}
	if len(lower) != len(upper) {
		t.Errorf("Search(%q) has %d matches, want %d as for %q", "executetemplate", len(lower), len(upper), "ExecuteTemplate")
	}
}
