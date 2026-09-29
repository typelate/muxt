package generate

import (
	"go/ast"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/source"
)

func TestCallParseMultipartFormMaxMemory(t *testing.T) {
	for _, tt := range []struct {
		name      string
		maxMemory int64
		want      string
	}{
		{name: "unset uses the default", maxMemory: 0, want: "request.ParseMultipartForm(33554432)"},
		{name: "negative uses the default", maxMemory: -1, want: "request.ParseMultipartForm(33554432)"},
		{name: "smallest override", maxMemory: 1, want: "request.ParseMultipartForm(1)"},
		{name: "override", maxMemory: 1 << 10, want: "request.ParseMultipartForm(1024)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := testConfig()
			config.MultipartMaxMemory = tt.maxMemory
			got := astgen.Format(callParseMultipartForm(scalarTestFile(t), config, &ast.BlockStmt{}))
			assert.Contains(t, got, tt.want, "callParseMultipartForm(%d)", tt.maxMemory)
		})
	}
}

func TestTypedVar(t *testing.T) {
	tp := source.NewType(types.NewMap(types.Typ[types.String], types.NewSlice(types.Typ[types.String])))
	for _, tt := range []struct {
		name  string
		value ast.Expr
		want  string
	}{
		{name: "without a value", want: "var form map[string][]string"},
		{name: "form", value: requestField("Form"), want: "var form map[string][]string = request.Form"},
		{name: "multipart form", value: requestField("MultipartForm"), want: "var form map[string][]string = request.MultipartForm"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			decl, err := typedVar(scalarTestFile(t), "form", tp, tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, astgen.Format(decl), "typedVar(form, %s)", tt.name)
		})
	}
}
