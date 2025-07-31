package source_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/source"
)

func TestIterateFieldTypes(t *testing.T) {
	t.Run("multiple names per param", func(t *testing.T) {
		exp, err := parser.ParseExpr(`func (a, b, c int, x, y, z float64) {}`)
		require.NoError(t, err)
		expIndex := 0
		for gotIndex, tp := range source.IterateFieldTypes(exp.(*ast.FuncLit).Type.Params.List) {
			assert.NotNil(t, tp)
			assert.Equal(t, expIndex, gotIndex)
			expIndex++
		}
		assert.Equal(t, 6, expIndex)
	})
	t.Run("just types", func(t *testing.T) {
		exp, err := parser.ParseExpr(`func (int, float64) {}`)
		require.NoError(t, err)
		expIndex := 0
		for gotIndex, tp := range source.IterateFieldTypes(exp.(*ast.FuncLit).Type.Params.List) {
			assert.NotNil(t, tp)
			assert.Equal(t, expIndex, gotIndex)
			expIndex++
		}
		assert.Equal(t, 2, expIndex)
	})
}

func TestHTTPStatusCode(t *testing.T) {
	fSet := fileSet()
	wd, err := workingDir()
	require.NoError(t, err)
	pl, err := loadPackages(wd, []string{wd})

	file, err := source.NewFile(filepath.Join(wd, "tr.go"), fSet, pl)
	require.NoError(t, err)

	exp := file.HTTPStatusCode(600)
	require.NotNil(t, exp)
	lit, ok := exp.(*ast.BasicLit)
	require.True(t, ok)
	require.Equal(t, token.INT, lit.Kind)
	require.Equal(t, "600", lit.Value)
	require.Empty(t, file.ImportSpecs(), "it should not add the import if it is not needed")
}
