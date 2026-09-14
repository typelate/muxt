package generate

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/typelate/muxt/internal/astgen"
)

// TestFormatFileImports states how a generated file lays out its imports:
// the standard library first, then every path whose first element has a
// dot, each group sorted by path and set apart by a blank line -- the layout
// goimports gives a file.
func TestFormatFileImports(t *testing.T) {
	spec := func(name, path string) *ast.ImportSpec {
		s := &ast.ImportSpec{Path: astgen.String(path)}
		if name != "" {
			s.Name = ast.NewIdent(name)
		}
		return s
	}
	for _, tt := range []struct {
		name  string
		specs []*ast.ImportSpec
		want  string
	}{
		{
			name:  "one import",
			specs: []*ast.ImportSpec{spec("", "net/http")},
			want:  "package server\n\nimport \"net/http\"\n",
		},
		{
			name: "the standard library before the rest",
			specs: []*ast.ImportSpec{
				spec("", "github.com/example/app/models"),
				spec("", "net/http"),
				spec("v2", "example.com/lib/v2"),
				spec("", "bytes"),
				spec("", "server/internal/data"),
				spec("", "server/v1.2/data"),
			},
			want: "package server\n\nimport (\n\t\"bytes\"\n\t\"net/http\"\n\t\"server/internal/data\"\n\t\"server/v1.2/data\"\n\n\tv2 \"example.com/lib/v2\"\n\t\"github.com/example/app/models\"\n)\n",
		},
		{
			name:  "a repeated import once",
			specs: []*ast.ImportSpec{spec("", "net/http"), spec("", "net/http")},
			want:  "package server\n\nimport \"net/http\"\n",
		},
		{
			name: "no imports",
			want: "package server\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &ast.File{Name: ast.NewIdent("server")}
			if len(tt.specs) > 0 {
				decl := &ast.GenDecl{Tok: token.IMPORT}
				for _, s := range tt.specs {
					decl.Specs = append(decl.Specs, s)
				}
				f.Decls = []ast.Decl{decl}
			}
			got, err := formatFile("routes.go", f)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("formatFile =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}
