package overlay_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/overlay"
)

func TestEngineImportPaths(t *testing.T) {
	engine := overlay.Engine()
	var goFiles int
	err := fs.WalkDir(engine, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(name) != ".go" {
			return err
		}
		goFiles++
		src, err := fs.ReadFile(engine, name)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, src, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range f.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			first, _, _ := strings.Cut(p, "/")
			isStd := !strings.Contains(first, ".")
			isEngine := strings.HasPrefix(p, "html/template/internal/")
			isSelf := p == "html/template" || strings.HasPrefix(p, "html/template/")
			if !isEngine && (!isStd || isSelf) {
				t.Errorf("%s imports %q, want a standard library path or html/template/internal/...", name, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if goFiles == 0 {
		t.Fatal("the embedded engine has no Go files")
	}
}
