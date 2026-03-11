package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMarkdownLinks validates that all relative links in markdown files
// point to files that exist within the repository.
func TestMarkdownLinks(t *testing.T) {
	// Pattern matches markdown links: [text](path)
	// Captures the path part, excluding anchors (#section)
	linkPattern := regexp.MustCompile(`\[([^\]]+)\]\(([^)#]+)`)

	var failures []string

	absRepo, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("Failed to resolve repo root: %v", err)
	}

	require.NoError(t, filepath.WalkDir(absRepo, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Only check markdown files
		if d.IsDir() {
			return nil
		}
		switch ext := filepath.Ext(p); ext {
		case ".md", ".txt":
		default:
			return nil
		}

		content, err := os.ReadFile(p)
		if err != nil {
			t.Logf("Failed to read %s: %v", p, err)
			return nil
		}

		matches := linkPattern.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}

			linkText := match[1]
			linkPath := match[2]

			// Skip external links (http://, https://, mailto:, etc.)
			if strings.Contains(linkPath, "://") || strings.HasPrefix(linkPath, "mailto:") {
				continue
			}

			// Resolve relative p from the markdown file's location
			mdDir := filepath.Dir(p)
			targetPath := filepath.Join(mdDir, filepath.FromSlash(linkPath))

			// Clean the p (resolve .. and .)
			targetPath = filepath.Clean(targetPath)

			// Ensure the p doesn't escape the repository
			absTarget, err := filepath.Abs(targetPath)
			if err != nil {
				t.Logf("%s: failed to resolve absolute p for %q: %v", p, linkPath, err)
				failures = append(failures, p+" -> "+linkPath)
				continue
			}

			if !strings.HasPrefix(absTarget, absRepo) {
				t.Logf("%s: link %q escapes repository bounds", p, linkPath)
				failures = append(failures, p+" -> "+linkPath)
				continue
			}

			// Check if the file exists
			if _, err := os.Stat(targetPath); os.IsNotExist(err) {
				t.Logf("%s: broken link [%s](%s) -> target %s does not exist",
					p, linkText, linkPath, targetPath)
				failures = append(failures, p+" -> "+linkPath)
			}
		}

		return nil
	}))

	if len(failures) > 0 {
		t.Errorf("Found %d broken links (see log output above for details)", len(failures))
	}
}
