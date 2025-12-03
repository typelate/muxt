package generate

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_packageImportName(t *testing.T) {
	type (
		given struct {
			importSpecs        []*ast.ImportSpec
			packageIdentifiers map[string]string
			outPkgPath         string
			pkgPath            string
			pkgIdent           string
		}
		// "when" occurs in the Run function
		then struct {
			importSpecs        []*ast.ImportSpec
			packageIdentifiers map[string]string
			ident              string
		}
	)

	tests := []struct {
		name  string
		given given
		then  func(*testing.T, then)
	}{
		{
			name: "no hint is given and path has exactly one segment",
			given: given{
				outPkgPath:         "example/cmd/cli",
				importSpecs:        []*ast.ImportSpec{},
				packageIdentifiers: map[string]string{},
				pkgPath:            "fmt",
				pkgIdent:           "",
			},
			then: func(t *testing.T, then then) {
				require.Equal(t, "fmt", then.ident, "it uses the package path as identifier")
			},
		},
		{
			name: "no hint is given and path has multiple segments",
			given: given{
				outPkgPath:         "example/cmd/cli",
				importSpecs:        []*ast.ImportSpec{},
				packageIdentifiers: map[string]string{},
				pkgPath:            "net/http",
				pkgIdent:           "",
			},
			then: func(t *testing.T, then then) {
				require.Equal(t, "http", then.ident, "it uses the package path as identifier")
			},
		},
		{
			name: "when no path is given and an ident is given it uses the hint",
			given: given{
				outPkgPath:         "example/cmd/cli",
				importSpecs:        []*ast.ImportSpec{},
				packageIdentifiers: map[string]string{},
				pkgPath:            "fmt",
				pkgIdent:           "format",
			},
			then: func(t *testing.T, then then) {
				require.Equal(t, "format", then.ident, "it uses the package path as identifier")
			},
		},
		{
			name: "package identifier found in map, returns it",
			given: given{
				outPkgPath: "example/cmd/cli",
				packageIdentifiers: map[string]string{
					"example.com/package": "abc",
				},
				pkgPath:  "example.com/package",
				pkgIdent: "",
			},
			then: func(t *testing.T, then then) {
				require.Equal(t, "abc", then.ident, "it uses the cached value")
			},
		},
		{
			name: "new package no previous value and ident is given",
			given: given{
				outPkgPath:         "example/cmd/cli",
				importSpecs:        []*ast.ImportSpec{},
				packageIdentifiers: map[string]string{},
				pkgPath:            "example.com/xyz",
				pkgIdent:           "abc",
			},

			then: func(t *testing.T, then then) {
				require.Equal(t, "abc", then.ident, "it uses the package ident")
				require.Len(t, then.importSpecs, 1)
				require.Equal(t, "abc", then.importSpecs[0].Name.Name, "it caches the package ident")
				require.Equal(t, `"example.com/xyz"`, then.importSpecs[0].Path.Value, "it caches the package ident")
				require.Len(t, then.packageIdentifiers, 1)
			},
		},
		{
			name: "existing import found by path, no name conflict",
			given: given{
				outPkgPath: "example/cmd/cli",
				importSpecs: []*ast.ImportSpec{
					{Name: ast.NewIdent("existing"), Path: &ast.BasicLit{Value: `"example.com/pkg/existing"`}},
				},
				packageIdentifiers: map[string]string{
					"example.com/pkg/existing": "existing",
				},
				pkgPath:  "example.com/pkg/existing",
				pkgIdent: "something",
			},
			then: func(t *testing.T, then then) {
				require.Equal(t, "existing", then.ident, "it uses the cached value ident")
			},
		},
		{
			name: "duplicate identifier found for a different package",
			given: given{
				outPkgPath: "example/cmd/cli",
				importSpecs: []*ast.ImportSpec{
					{Name: ast.NewIdent("existing"), Path: &ast.BasicLit{Value: `"example.com/pkg/existing"`}},
				},
				packageIdentifiers: map[string]string{
					"example.com/pkg/existing": "existing",
				},
				pkgPath:  "example.com/pkg/internal/existing",
				pkgIdent: "",
			},
			then: func(t *testing.T, then then) {
				require.Equal(t, "existing4075376d1c12", then.ident, "it uses the cached value ident")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := packageImportName(&tt.given.importSpecs, tt.given.packageIdentifiers, tt.given.pkgPath, tt.given.pkgIdent)
			if tt.then != nil {
				tt.then(t, then{
					importSpecs:        tt.given.importSpecs,
					packageIdentifiers: tt.given.packageIdentifiers,
					ident:              result,
				})
			}
		})
	}
}
