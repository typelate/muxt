package astgen_test

import (
	"go/ast"
	"go/parser"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
)

func TestIterateFieldTypes(t *testing.T) {
	t.Run("multiple names per param", func(t *testing.T) {
		exp, err := parser.ParseExpr(`func (a, b, c int, x, y, z float64) {}`)
		require.NoError(t, err)
		expIndex := 0
		for gotIndex, tp := range astgen.IterateFieldTypes(exp.(*ast.FuncLit).Type.Params.List) {
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
		for gotIndex, tp := range astgen.IterateFieldTypes(exp.(*ast.FuncLit).Type.Params.List) {
			assert.NotNil(t, tp)
			assert.Equal(t, expIndex, gotIndex)
			expIndex++
		}
		assert.Equal(t, 2, expIndex)
	})
}
