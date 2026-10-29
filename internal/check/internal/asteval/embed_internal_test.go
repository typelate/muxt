package asteval

import "testing"

func TestEmbedPatternMatches(t *testing.T) {
	for _, tt := range []struct {
		Name    string
		Pattern string
		File    string
		Match   bool
	}{
		{Name: "glob on file", Pattern: "*.gohtml", File: "index.gohtml", Match: true},
		{Name: "glob wrong extension", Pattern: "*.gohtml", File: "main.go", Match: false},
		{Name: "directory embeds subtree", Pattern: "assets", File: "assets/pages/index.gohtml", Match: true},
		{Name: "directory excludes underscore element", Pattern: "assets", File: "assets/_hidden/index.gohtml", Match: false},
		{Name: "directory excludes dot element", Pattern: "assets", File: "assets/.hidden.gohtml", Match: false},
		{Name: "all prefix includes underscore element", Pattern: "all:assets", File: "assets/_hidden/index.gohtml", Match: true},
		{Name: "all prefix includes dot element", Pattern: "all:assets", File: "assets/.hidden.gohtml", Match: true},
		{Name: "glob on directory embeds subtree", Pattern: "assets/*", File: "assets/pages/index.gohtml", Match: true},
		{Name: "unrelated directory", Pattern: "assets", File: "static/site.css", Match: false},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			matched, err := embedPatternMatches(tt.Pattern, tt.File)
			if err != nil {
				t.Fatal(err)
			}
			if matched != tt.Match {
				t.Errorf("embedPatternMatches(%q, %q) = %v, want %v", tt.Pattern, tt.File, matched, tt.Match)
			}
		})
	}
}
