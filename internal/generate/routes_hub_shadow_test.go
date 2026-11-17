package generate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
)

// TestHubImportIsNotShadowedByGeneratedNames generates a routes file with
// an sse route and a hub whose package name is also a name the generated
// methods declare. The file must refer to the hub through a name no
// receiver, parameter or local in it declares.
func TestHubImportIsNotShadowedByGeneratedNames(t *testing.T) {
	for _, hubName := range []string{
		"m",    // the SSETemplateData receiver
		"w",    // the SSETemplateData WriteTo writer
		"code", // the TemplateData redirect parameter
		"n",    // a WriteTo local
		"url",  // the TemplateData redirect parameter
	} {
		t.Run(hubName, func(t *testing.T) {
			config := testConfig()
			config.ReceiverType = "T"
			pkg, receiver := testSource(t, `package server

type T struct{}

func (T) Stream(func(string) error) {}
`, "T", `{{define "GET /x sse(Stream(execute))"}}{{end}}`)
			defs, err := muxt.ResolveDefinitions(pkg, receiver, fake.NewChecker().Fake())
			require.NoError(t, err)
			hub := &Hub{ImportPath: "example.com/" + hubName, PackageName: hubName, Namespace: "Admin"}

			files, err := TemplateRoutesFiles(t.TempDir(), config, hub, pkg, defs, Loggers{})
			require.NoError(t, err)
			require.Len(t, files, 1)
			parsed, err := parser.ParseFile(token.NewFileSet(), "template_routes.go", files[0].Content, 0)
			require.NoError(t, err)

			var hubIdent string
			for _, spec := range parsed.Imports {
				importPath, err := strconv.Unquote(spec.Path.Value)
				require.NoError(t, err)
				if importPath != hub.ImportPath {
					continue
				}
				hubIdent = path.Base(importPath)
				if spec.Name != nil {
					hubIdent = spec.Name.Name
				}
			}
			require.NotEmpty(t, hubIdent, "the file does not import the hub")

			for _, decl := range parsed.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				names := declaredNames(fn)
				if fn.Recv != nil {
					for _, field := range fn.Recv.List {
						for _, name := range field.Names {
							names = append(names, name.Name)
						}
					}
				}
				assert.NotContains(t, names, hubIdent, "names %s declares, the hub import is %s", fn.Name.Name, hubIdent)
			}
		})
	}
}

// TestOnlyTheHubImportAvoidsTheHubShadowedNames imports a package named like
// a name the template data methods declare: the hub's import gets an alias,
// another package's does not.
func TestOnlyTheHubImportAvoidsTheHubShadowedNames(t *testing.T) {
	pkg, _ := testSource(t, "package server\n", "", ``)
	file := newFile(pkg)
	file.hubImportPath = "example.com/hub/event"

	assert.Equal(t, "event", file.Import("event", "example.com/other/event"), "a package that is not the hub")
	assert.NotEqual(t, "event", file.Import("event", "example.com/hub/event"), "the hub")
}
