package load_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/typelate/muxt/internal/load"
)

func TestEmbeddedFiles(t *testing.T) {
	write := func(t *testing.T, dir, name, text string) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("file directory and glob", func(t *testing.T) {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		write(t, dir, "go.mod", "module example.com/m\n\ngo 1.22\n")
		write(t, dir, "t.go", "package m\n\nimport \"embed\"\n\n//go:embed *.gohtml *.css\n//go:embed assets\n//go:embed one.txt\nvar f embed.FS\n")
		write(t, dir, "a.gohtml", "a")
		write(t, dir, "s.css", "s")
		write(t, dir, "x.md", "x")
		write(t, dir, "one.txt", "1")
		write(t, dir, "assets/b.css", "b")
		write(t, dir, "assets/sub/c.css", "c")
		got, ok, err := load.EmbeddedFiles(dir)
		if err != nil || !ok {
			t.Fatalf("EmbeddedFiles() = %v, %v, %v; want ok", got, ok, err)
		}
		var want []string
		for _, n := range []string{"a.gohtml", "assets/b.css", "assets/sub/c.css", "one.txt", "s.css"} {
			want = append(want, filepath.Join(dir, filepath.FromSlash(n)))
		}
		if !slices.Equal(got, want) {
			t.Errorf("EmbeddedFiles() = %v; want %v", got, want)
		}
	})
	t.Run("embed pattern matching nothing", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "go.mod", "module example.com/m\n\ngo 1.22\n")
		write(t, dir, "t.go", "package m\n\nimport \"embed\"\n\n//go:embed *.gohtml\nvar f embed.FS\n")
		got, ok, err := load.EmbeddedFiles(dir)
		want := filepath.Join(dir, "t.go") + ":5:12: pattern *.gohtml: no matching files found"
		if !ok || err == nil || err.Error() != want {
			t.Errorf("EmbeddedFiles(bad embed) = %v, %v, %v; want ok and error %q", got, ok, err, want)
		}
	})
	t.Run("outside any module", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "a.gohtml", "a")
		if _, ok, _ := load.EmbeddedFiles(dir); ok {
			t.Errorf("EmbeddedFiles(outside a module) is ok; want no package")
		}
	})
	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "go.mod", "module example.com/m\n\ngo 1.22\n")
		got, ok, err := load.EmbeddedFiles(dir)
		if err != nil || ok || len(got) != 0 {
			t.Errorf("EmbeddedFiles(empty) = %v, %v, %v; want none, false, nil", got, ok, err)
		}
	})
}
