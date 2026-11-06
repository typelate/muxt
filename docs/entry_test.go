package docs_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/typelate/muxt/docs"
)

func TestResolve(t *testing.T) {
	for _, tt := range []struct {
		arg         string
		wantPath    string
		wantHeading string
	}{
		{
			arg:         "docs/reference/template-names.md",
			wantPath:    "docs/reference/template-names.md",
			wantHeading: "# Template Name Syntax\n",
		},
		{
			arg:         "./docs/reference/template-names.md",
			wantPath:    "docs/reference/template-names.md",
			wantHeading: "# Template Name Syntax\n",
		},
		{
			arg:         "reference/template-names.md",
			wantPath:    "docs/reference/template-names.md",
			wantHeading: "# Template Name Syntax\n",
		},
		{
			arg:         "docs/reference/template-names.md#route-names",
			wantPath:    "docs/reference/template-names.md",
			wantHeading: "# Template Name Syntax\n",
		},
		{
			arg:         "docs/reference/template-names.md/",
			wantPath:    "docs/reference/template-names.md",
			wantHeading: "# Template Name Syntax\n",
		},
		{
			arg:         "docs/README.md",
			wantPath:    "docs/README.md",
			wantHeading: "# Muxt Documentation\n",
		},
	} {
		entry, err := docs.Resolve(tt.arg)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", tt.arg, err)
		}
		if entry.Path != tt.wantPath {
			t.Errorf("Resolve(%q).Path = %q, want %q", tt.arg, entry.Path, tt.wantPath)
		}
		content, err := entry.Render("main")
		if err != nil {
			t.Fatalf("Resolve(%q).Render() error = %v", tt.arg, err)
		}
		if !strings.HasPrefix(string(content), tt.wantHeading) {
			t.Errorf("Resolve(%q).Render() = %q, want prefix %q", tt.arg, firstLine(content), tt.wantHeading)
		}
	}
}

func TestRenderFile(t *testing.T) {
	for _, tt := range []struct {
		path         string
		wantPrefix   string
		wantContains string
	}{
		{
			path:         "docs/examples/simple/main.go",
			wantPrefix:   "docs/examples/simple/main.go\n\n```go\n",
			wantContains: "package main",
		},
		{
			path:       "docs/examples/simple/index.gohtml",
			wantPrefix: "docs/examples/simple/index.gohtml\n\n```html\n",
		},
		{
			path:       "docs/examples/datastar-counter/styles.css",
			wantPrefix: "docs/examples/datastar-counter/styles.css\n\n```css\n",
		},
		{
			path:       "docs/examples/htmx-todo/.gitignore",
			wantPrefix: "docs/examples/htmx-todo/.gitignore\n\n```\n",
		},
	} {
		entry, err := docs.Resolve(tt.path)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", tt.path, err)
		}
		content, err := entry.Render("main")
		if err != nil {
			t.Fatalf("Resolve(%q).Render() error = %v", tt.path, err)
		}
		got := string(content)
		if !strings.HasPrefix(got, tt.wantPrefix) {
			t.Errorf("Resolve(%q).Render() = %q, want prefix %q", tt.path, firstLine(content), tt.wantPrefix)
		}
		if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
			t.Errorf("Resolve(%q).Render() does not contain %q", tt.path, tt.wantContains)
		}
		if !strings.HasSuffix(got, "```\n") {
			t.Errorf("Resolve(%q).Render() does not end with the closing fence \"```\\n\"", tt.path)
		}
	}
}

func TestRenderDirectory(t *testing.T) {
	for _, tt := range []struct {
		arg      string
		wantDir  string
		wantLine string
	}{
		{
			arg:      "docs/how-to",
			wantDir:  "docs/how-to",
			wantLine: "docs/how-to/template-functions.md",
		},
		{
			arg:      "how-to/",
			wantDir:  "docs/how-to",
			wantLine: "docs/how-to/template-functions.md",
		},
		{
			arg:      "docs/examples/simple",
			wantDir:  "docs/examples/simple",
			wantLine: "docs/examples/simple/main.go",
		},
		{
			arg:      "docs/reference",
			wantDir:  "docs/reference",
			wantLine: "docs/reference/commands/check.md",
		},
	} {
		entry, err := docs.Resolve(tt.arg)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", tt.arg, err)
		}
		content, err := entry.Render("main")
		if err != nil {
			t.Fatalf("Resolve(%q).Render() error = %v", tt.arg, err)
		}
		got := string(content)
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("Resolve(%q).Render() = %q, want a trailing newline", tt.arg, got)
		}
		lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		for _, line := range lines {
			if !strings.HasPrefix(line, tt.wantDir+"/") {
				t.Errorf("Resolve(%q).Render() line %q, want a path under %q", tt.arg, line, tt.wantDir+"/")
			}
		}
		if !slices.IsSorted(lines) {
			t.Errorf("Resolve(%q).Render() lines are not sorted: %q", tt.arg, lines)
		}
		if !slices.Contains(lines, tt.wantLine) {
			t.Errorf("Resolve(%q).Render() does not contain line %q", tt.arg, tt.wantLine)
		}
	}
}

func firstLine(b []byte) string {
	line, _, _ := strings.Cut(string(b), "\n")
	return line
}
