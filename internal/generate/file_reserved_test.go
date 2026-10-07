package generate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/txtar"

	"github.com/typelate/muxt/internal/muxt"
)

// TestGeneratedLocalIdentifiersAreReserved reads the routes functions in
// every snapshot's generated files and checks that each name they declare
// is one an import may not have: otherwise a package with that name would
// be shadowed where a handler spells its types.
func TestGeneratedLocalIdentifiersAreReserved(t *testing.T) {
	archives, err := filepath.Glob(filepath.Join("testdata", "generate", "*.txtar"))
	require.NoError(t, err)
	require.NotEmpty(t, archives)

	// Locals named for a path parameter or a nested call's result.
	expectedPattern := regexp.MustCompile(`^(_|.+PathParam|.+Parsed|result[0-9]+)$`)
	// multipart is declared after the handler spells mime/multipart's
	// types (see generatedLocalIdentifiers), and next only in the no-op
	// middleware, whose body is return next.
	unreservedOnPurpose := []string{muxt.TemplateNameScopeIdentifierMultipart, "next"}
	reserved := newFile(outputFile().pkg).taken
	declared := make(map[string][]string)
	for _, archivePath := range archives {
		archive, err := txtar.ParseFile(archivePath)
		require.NoError(t, err)
		for _, file := range archive.Files {
			if !strings.HasPrefix(file.Name, "want/") || filepath.Ext(file.Name) != ".go" {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), file.Name, file.Data, 0)
			require.NoError(t, err, "%s %s", archivePath, file.Name)
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				// The routes functions have no receiver; the methods
				// of the generated types spell no imported types
				// from outside the standard library.
				if !ok || fn.Recv != nil {
					continue
				}
				for _, name := range declaredNames(fn) {
					if !expectedPattern.MatchString(name) && !reserved[name] && !slices.Contains(unreservedOnPurpose, name) {
						declared[name] = append(declared[name], filepath.Base(archivePath))
					}
				}
			}
		}
	}
	for name, where := range declared {
		assert.Fail(t, "an unreserved generated local", "%s is declared in %v but is not in generatedLocalIdentifiers", name, slices.Compact(where))
	}
}

// declaredNames are the names fn declares: parameters, including those of
// function literals, := and var declarations, and range variables.
func declaredNames(fn *ast.FuncDecl) []string {
	var names []string
	fields := func(list *ast.FieldList) {
		if list == nil {
			return
		}
		for _, field := range list.List {
			for _, name := range field.Names {
				names = append(names, name.Name)
			}
		}
	}
	idents := func(exprs ...ast.Expr) {
		for _, expr := range exprs {
			if ident, ok := expr.(*ast.Ident); ok {
				names = append(names, ident.Name)
			}
		}
	}
	fields(fn.Type.Params)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.FuncLit:
			fields(node.Type.Params)
		case *ast.AssignStmt:
			if node.Tok == token.DEFINE {
				idents(node.Lhs...)
			}
		case *ast.ValueSpec:
			for _, name := range node.Names {
				names = append(names, name.Name)
			}
		case *ast.RangeStmt:
			if node.Tok == token.DEFINE {
				idents(node.Key, node.Value)
			}
		}
		return true
	})
	return names
}
