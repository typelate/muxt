package docs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMarkdownFilesSkipsDrafts states that docs/drafts, which is gitignored and
// holds review drafts linking to pages at their future paths, is not checked,
// while the same directory name elsewhere and every other page are.
func TestMarkdownFilesSkipsDrafts(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"docs/drafts/x.md",
		"docs/how-to/y.md",
		"docs/how-to/drafts/z.md",
	} {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}

	got, err := markdownFiles(root)
	require.NoError(t, err)

	want := []string{
		filepath.Join(root, "docs", "how-to", "drafts", "z.md"),
		filepath.Join(root, "docs", "how-to", "y.md"),
	}
	require.Equal(t, want, got)
}
