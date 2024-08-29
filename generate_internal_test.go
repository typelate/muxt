package muxt

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateName_HandlerFuncLit_err(t *testing.T) {
	for _, tt := range []struct {
		Name   string
		In     string
		ErrSub string
		Method *ast.FuncType
	}{
		{
			Name: "missing arguments",
			In:   "GET / F()",
			Method: &ast.FuncType{
				Params:  &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			ErrSub: "handler func F(string) any expects 1 arguments but call F() has 0",
		},
		{
			Name: "extra arguments",
			In:   "GET /{name} F(ctx, name)",
			Method: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Type: &ast.SelectorExpr{X: ast.NewIdent(contextPackageIdent), Sel: ast.NewIdent(contextContextTypeIdent)}},
				}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			ErrSub: "handler func F(context.Context) any expects 1 arguments but call F(ctx, name) has 2",
		},
		{
			Name: "wrong argument type request",
			In:   "GET / F(request)",
			Method: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Type: ast.NewIdent("string")},
				}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			ErrSub: "method expects type string but request is *http.Request",
		},
		{
			Name: "wrong argument type ctx",
			In:   "GET / F(ctx)",
			Method: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Type: ast.NewIdent("string")},
				}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			ErrSub: "method expects type string but ctx is context.Context",
		},
		{
			Name: "wrong argument type response",
			In:   "GET / F(response)",
			Method: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Type: ast.NewIdent("string")},
				}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			ErrSub: "method expects type string but response is http.ResponseWriter",
		},
		{
			Name: "wrong argument type path value",
			In:   "GET /{name} F(name)",
			Method: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Type: ast.NewIdent("float64")},
				}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			ErrSub: "method param type float64 not supported",
		},
		{
			Name: "wrong argument type request ptr",
			In:   "GET / F(request)",
			Method: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Type: &ast.StarExpr{X: ast.NewIdent("T")}},
				}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			ErrSub: "method expects type *T but request is *http.Request",
		},
		{
			Name: "wrong argument type in field list",
			In:   "GET /post/{postID}/comment/{commentID} F(ctx, request, commentID)",
			Method: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Type: contextContextField().Type, Names: []*ast.Ident{{Name: "ctx"}}},
					{Names: []*ast.Ident{ast.NewIdent("postID"), ast.NewIdent("commentID")}, Type: ast.NewIdent("string")},
				}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			ErrSub: "method expects type string but request is *http.Request",
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			pat, err, ok := NewTemplateName(tt.In)
			require.True(t, ok)
			require.NoError(t, err)
			_, _, err = pat.funcLit(tt.Method, nil)
			assert.ErrorContains(t, err, tt.ErrSub)
		})
	}
}
