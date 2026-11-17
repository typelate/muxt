package generate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
)

// TestRoutesFileUsesTheHubsPathTypes generates a routes file with a hub and
// reads the result as Go: the file names the hub's path types and declares
// none of its own.
func TestRoutesFileUsesTheHubsPathTypes(t *testing.T) {
	config := testConfig()
	config.ReceiverType = "T"
	pkg, receiver := testSource(t, `package server

type T struct{}

func (T) Index() string { return "" }
`, "T", `{{define "GET /{$} Index()"}}{{end}}`)
	defs, err := muxt.ResolveDefinitions(pkg, receiver, fake.NewChecker().Fake())
	require.NoError(t, err)
	hub := &Hub{ImportPath: "example.com/hub", PackageName: "hub", Namespace: "Server"}

	files, err := TemplateRoutesFiles(t.TempDir(), config, hub, pkg, defs, Loggers{})
	require.NoError(t, err)
	require.Len(t, files, 1)
	parsed, err := parser.ParseFile(token.NewFileSet(), "template_routes.go", files[0].Content, 0)
	require.NoError(t, err)

	var imports []string
	for _, spec := range parsed.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		require.NoError(t, err)
		imports = append(imports, importPath)
	}
	assert.Contains(t, imports, "example.com/hub", "imports")

	var declared []string
	var routesResult string
	for _, decl := range parsed.Decls {
		switch decl := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				if spec, ok := spec.(*ast.TypeSpec); ok {
					declared = append(declared, spec.Name.Name)
				}
			}
		case *ast.FuncDecl:
			if decl.Recv == nil && decl.Name.Name == config.RoutesFunction && decl.Type.Results != nil {
				routesResult = astgen.Format(decl.Type.Results.List[0].Type)
			}
		}
	}
	for _, name := range []string{"TemplateRoutePaths", "TemplateRouteBuilder", "TemplateRoute"} {
		assert.NotContains(t, declared, name, "declared types")
	}
	assert.Equal(t, "hub.TemplateRoutePaths", routesResult, "routes function result")
}
