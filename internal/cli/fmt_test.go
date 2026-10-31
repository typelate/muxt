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
		for _, name := range []string{"a.gohtml", "b.html", "sub/c.gohtml", ".git/d.gohtml", "_old/e.gohtml", "testdata/f.gohtml", "vendor/g.gohtml", ".h.gohtml", "sub/.i.gohtml"} {
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

	t.Run("an explicit hidden file is formatted", func(t *testing.T) {
		wd := t.TempDir()
		writeTestFile(t, wd, ".h.gohtml", unformatted)
		stdout, _, err := runFormat(t, wd, "", "-l", ".h.gohtml")
		require.Error(t, err, "muxt fmt -l .h.gohtml")
		assert.Equal(t, ".h.gohtml\n", stdout, "muxt fmt -l .h.gohtml")
	})

	t.Run("a walk does not follow a symbolic link", func(t *testing.T) {
		wd := t.TempDir()
		outside := t.TempDir()
		writeTestFile(t, outside, "real.gohtml", unformatted)
		require.NoError(t, os.Mkdir(filepath.Join(wd, "root"), 0o755))
		link := filepath.Join("root", "link.gohtml")
		require.NoError(t, os.Symlink(filepath.Join(outside, "real.gohtml"), filepath.Join(wd, link)))

		stdout, _, err := runFormat(t, wd, "", "-l", "root")
		require.NoError(t, err, "muxt fmt -l root")
		assert.Empty(t, stdout, "muxt fmt -l root visited a symbolic link")

		stdout, _, err = runFormat(t, wd, "", "-l", link)
		require.Error(t, err, "muxt fmt -l %s", link)
		assert.Equal(t, link+"\n", stdout, "muxt fmt -l %s", link)
	})

	t.Run("write through a symbolic link", func(t *testing.T) {
		wd := t.TempDir()
		real := writeTestFile(t, wd, "real.gohtml", unformatted)
		link := filepath.Join(wd, "link.gohtml")
		require.NoError(t, os.Symlink("real.gohtml", link))

		_, _, err := runFormat(t, wd, "", "-w", "link.gohtml")
		require.NoError(t, err, "muxt fmt -w link.gohtml")
		info, err := os.Lstat(link)
		require.NoError(t, err)
		assert.Equal(t, os.ModeSymlink, info.Mode().Type(), "muxt fmt -w link.gohtml replaced the link")
		got, err := os.ReadFile(real)
		require.NoError(t, err)
		assert.Equal(t, formatted, string(got), "muxt fmt -w link.gohtml wrote the link's target")
	})

	// The package in the working directory is not the package of what an
	// editor pipes in: standard input is formatted with no package, so a
	// class is unknown and left as written.
	t.Run("standard input has no package context", func(t *testing.T) {
		wd := t.TempDir()
		writeTestFile(t, wd, "go.mod", "module example.com/m\n\ngo 1.22\n")
		writeTestFile(t, wd, "t.go", "package m\n\nimport \"embed\"\n\n//go:embed *.gohtml\nvar f embed.FS\n")
		writeTestFile(t, wd, "layout.gohtml", "<style>.x{display:block}</style>\n")
		const input = `<p>a<span class="x">b</span> c</p>` + "\n"
		stdout, stderr, err := runFormat(t, wd, input)
		require.NoError(t, err, "muxt fmt < %q", input)
		assert.Equal(t, input, stdout, "muxt fmt < %q in a package whose layout makes .x block", input)
		assert.Empty(t, stderr, "muxt fmt < %q", input)
	})

	// A package that loads with errors may embed a template that links any
	// stylesheet: its templates are left as written, and the error is
	// reported once.
	t.Run("a package that loads with errors", func(t *testing.T) {
		wd := t.TempDir()
		writeTestFile(t, wd, "go.mod", "module example.com/m\n\ngo 1.22\n")
		writeTestFile(t, wd, "t.go", "package m\n\nimport \"embed\"\n\n//go:embed *.gohtml *.css\nvar f embed.FS\n")
		const input = "<section>\n<div class=\"code\">\n  keep   me\n</div>\n</section>\n"
		writeTestFile(t, wd, "a.gohtml", input)
		writeTestFile(t, wd, "b.gohtml", input)
		stdout, stderr, err := runFormat(t, wd, "", "a.gohtml", "b.gohtml")
		require.Error(t, err, "muxt fmt a.gohtml b.gohtml")
		assert.Equal(t, input+input, stdout, "muxt fmt a.gohtml b.gohtml")
		assert.Equal(t, "t.go:5:21: pattern *.css: no matching files found\n", stderr, "muxt fmt a.gohtml b.gohtml")
	})

	// A file in another directory is in another package: its <style>
	// elements do not apply.
	t.Run("a walk keeps context per directory", func(t *testing.T) {
		wd := t.TempDir()
		writeTestFile(t, wd, "go.mod", "module example.com/m\n\ngo 1.22\n")
		writeTestFile(t, wd, "t.go", "package m\n\nimport \"embed\"\n\n//go:embed *.gohtml\nvar f embed.FS\n")
		const page = `<p>a<span class="x">b</span> c</p>` + "\n"
		writeTestFile(t, wd, "page.gohtml", page)
		require.NoError(t, os.Mkdir(filepath.Join(wd, "b"), 0o755))
		writeTestFile(t, wd, filepath.Join("b", "t.go"), "package b\n\nimport \"embed\"\n\n//go:embed *.gohtml\nvar f embed.FS\n")
		writeTestFile(t, wd, filepath.Join("b", "style.gohtml"), "<style>.x{display:block}</style>\n")
		stdout, stderr, err := runFormat(t, wd, "", "-l", ".")
		require.NoError(t, err, "muxt fmt -l .")
		assert.Empty(t, stdout, "muxt fmt -l . listed a file")
		assert.Empty(t, stderr, "muxt fmt -l .")
	})

	// An embedded .html template is context like a .gohtml one: the layout
	// that links the sheet may be either.
	t.Run("embedded html templates are context", func(t *testing.T) {
		wd := t.TempDir()
		writeTestFile(t, wd, "go.mod", "module example.com/m\n\ngo 1.22\n")
		writeTestFile(t, wd, "t.go", "package m\n\nimport \"embed\"\n\n//go:embed *.gohtml *.html *.txt\nvar f embed.FS\n")
		writeTestFile(t, wd, "layout.html", "<style>.x{display:block}</style>\n")
		writeTestFile(t, wd, "notes.txt", "{{if}}<style>.y{display:block}</style>\n")
		const page = "<p>a<span class=\"x\">b</span> c</p>\n<p>a<span class=\"y\">b</span> c</p>\n"
		const want = "<p>\n\ta\n\t<span class=\"x\">b</span>\n\tc\n</p>\n<p>a<span class=\"y\">b</span> c</p>\n"
		writeTestFile(t, wd, "page.gohtml", page)
		stdout, stderr, err := runFormat(t, wd, "", "page.gohtml")
		require.NoError(t, err, "muxt fmt page.gohtml")
		assert.Equal(t, want, stdout, "muxt fmt page.gohtml")
		assert.Empty(t, stderr, "muxt fmt page.gohtml")
	})
}
