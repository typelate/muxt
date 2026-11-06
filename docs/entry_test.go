package docs_test

import (
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

func firstLine(b []byte) string {
	line, _, _ := strings.Cut(string(b), "\n")
	return line
}
