package scaffold

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPackageDeclaresPackageTypeAndTemplateVariables states that the file for
// a new package declares its package name, its receiver type, and the two
// variables that embed and parse the package's templates.
func TestPackageDeclaresPackageTypeAndTemplateVariables(t *testing.T) {
	filename, src, err := Package(PackageSpec{Name: "workspace", ReceiverType: "T"})

	require.NoError(t, err)
	assert.Equal(t, "workspace.go", filename, "file name")
	file, err := parser.ParseFile(token.NewFileSet(), filename, src, parser.AllErrors)
	require.NoError(t, err, "source parses")
	assert.Equal(t, "workspace", file.Name.Name, "package name")
	assert.Contains(t, declaredNames(file, token.TYPE), "T", "receiver type")
	assert.Contains(t, declaredNames(file, token.VAR), "templateSource", "embedded files variable")
	assert.Contains(t, declaredNames(file, token.VAR), "templates", "parsed templates variable")
}

// TestPackageSourceIsGolden states the exact formatted file for a receiver
// type named T.
func TestPackageSourceIsGolden(t *testing.T) {
	_, src, err := Package(PackageSpec{Name: "workspace", ReceiverType: "T"})

	require.NoError(t, err)
	const want = `package workspace

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.New("").ParseFS(templateSource, "*.gohtml"))

type T struct{}
`
	assert.Equal(t, want, string(src))
}

// declaredNames returns the names declared by the GenDecls of kind tok in file.
func declaredNames(file *ast.File, tok token.Token) []string {
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != tok {
			continue
		}
		for _, spec := range gen.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				names = append(names, s.Name.Name)
			case *ast.ValueSpec:
				for _, name := range s.Names {
					names = append(names, name.Name)
				}
			}
		}
	}
	return names
}
