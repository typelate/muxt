package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runFormat runs muxt fmt with args in wd, reading stdin, and returns what
// it printed.
func runFormat(t *testing.T, wd, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = commands(wd, append([]string{fmtCommandName}, args...), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, strings.NewReader(stdin), &out, &errOut, runners{format: runFormatCommand})
	return out.String(), errOut.String(), err
}

func TestFormatRun(t *testing.T) {
	const (
		unformatted = "<div>\n<p>a</p>\n</div>\n"
		formatted   = "<div>\n\t<p>a</p>\n</div>\n"
	)

	t.Run("standard input", func(t *testing.T) {
		stdout, stderr, err := runFormat(t, t.TempDir(), unformatted)
		require.NoError(t, err, "muxt fmt < %q", unformatted)
		assert.Equal(t, formatted, stdout, "muxt fmt < %q", unformatted)
		assert.Empty(t, stderr, "muxt fmt < %q", unformatted)
	})

	t.Run("list standard input with a difference", func(t *testing.T) {
		stdout, _, err := runFormat(t, t.TempDir(), unformatted, "-l")
		require.Error(t, err, "muxt fmt -l < %q", unformatted)
		assert.Equal(t, "<standard input>\n", stdout, "muxt fmt -l < %q", unformatted)
	})

	t.Run("list standard input without a difference", func(t *testing.T) {
		stdout, _, err := runFormat(t, t.TempDir(), formatted, "-l")
		require.NoError(t, err, "muxt fmt -l < %q", formatted)
		assert.Empty(t, stdout, "muxt fmt -l < %q", formatted)
	})

	t.Run("walk", func(t *testing.T) {
		wd := t.TempDir()
		for _, name := range []string{"a.gohtml", "b.html", "sub/c.gohtml", ".git/d.gohtml", "_old/e.gohtml", "testdata/f.gohtml", "vendor/g.gohtml"} {
			path := filepath.Join(wd, filepath.FromSlash(name))
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(unformatted), 0o644))
		}

		stdout, _, err := runFormat(t, wd, "", "-l", ".")
		require.Error(t, err, "muxt fmt -l .")
		assert.Equal(t, "a.gohtml\n"+filepath.Join("sub", "c.gohtml")+"\n", stdout, "muxt fmt -l .")

		stdout, _, err = runFormat(t, wd, "", "-l", "testdata")
		require.Error(t, err, "muxt fmt -l testdata")
		assert.Equal(t, filepath.Join("testdata", "f.gohtml")+"\n", stdout, "muxt fmt -l testdata")
	})

	t.Run("write leaves a formatted file alone", func(t *testing.T) {
		wd := t.TempDir()
		path := writeTestFile(t, wd, "page.gohtml", formatted)
		past := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
		require.NoError(t, os.Chtimes(path, past, past))

		_, _, err := runFormat(t, wd, "", "-w", "page.gohtml")
		require.NoError(t, err, "muxt fmt -w page.gohtml")
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.True(t, info.ModTime().Equal(past), "muxt fmt -w page.gohtml changed the modification time of a formatted file to %s", info.ModTime())
	})

	t.Run("write keeps permissions", func(t *testing.T) {
		wd := t.TempDir()
		path := writeTestFile(t, wd, "page.gohtml", unformatted)
		require.NoError(t, os.Chmod(path, 0o600))

		_, _, err := runFormat(t, wd, "", "-w", "page.gohtml")
		require.NoError(t, err, "muxt fmt -w page.gohtml")
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, formatted, string(got), "muxt fmt -w page.gohtml")
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "muxt fmt -w page.gohtml permissions")
		entries, err := os.ReadDir(wd)
		require.NoError(t, err)
		assert.Len(t, entries, 1, "muxt fmt -w left a temporary file")
	})

	t.Run("a bad file does not stop a good one", func(t *testing.T) {
		wd := t.TempDir()
		writeTestFile(t, wd, "bad.gohtml", "{{if .X}}\n")
		good := writeTestFile(t, wd, "good.gohtml", unformatted)

		stdout, stderr, err := runFormat(t, wd, "", "-w", "bad.gohtml", "good.gohtml")
		require.Error(t, err, "muxt fmt -w bad.gohtml good.gohtml")
		assert.Empty(t, stdout, "muxt fmt -w bad.gohtml good.gohtml")
		assert.Equal(t, 1, strings.Count(stderr, "\n"), "muxt fmt -w bad.gohtml good.gohtml stderr = %q, want one line", stderr)
		assert.True(t, strings.HasPrefix(stderr, "bad.gohtml:"), "muxt fmt -w bad.gohtml good.gohtml stderr = %q, want it to start with the file", stderr)
		got, err := os.ReadFile(good)
		require.NoError(t, err)
		assert.Equal(t, formatted, string(got), "muxt fmt -w bad.gohtml good.gohtml wrote good.gohtml")
	})
}
