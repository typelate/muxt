package muxt

import (
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crhntr/muxt/internal/source"
)

func TestTemplateName_funcLit(t *testing.T) {
	for _, tt := range []struct {
		Name    string
		In      string
		Out     string
		Imports []string
		Method  *ast.FuncType
		Form    *ast.StructType
	}{
		{
			Name: "get",
			In:   "GET /",
			Out: `func(response http.ResponseWriter, request *http.Request) {
	execute(response, request, true, "GET /", http.StatusOK, request)
}`,
		},
		{
			Name: "call F",
			In:   "GET / F()",
			Out: `func(response http.ResponseWriter, request *http.Request) {
	data := receiver.F()
	execute(response, request, true, "GET / F()", http.StatusOK, data)
}`,
		},
		{
			Name: "call F with argument request",
			In:   "GET / F(request)",
			Method: &ast.FuncType{
				Params:  &ast.FieldList{List: []*ast.Field{{Type: httpRequestField().Type}}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			Out: `func(response http.ResponseWriter, request *http.Request) {
	data := receiver.F(request)
	execute(response, request, true, "GET / F(request)", http.StatusOK, data)
}`,
		},
		{
			Name: "call F with argument response",
			In:   "GET / F(response)",
			Method: &ast.FuncType{
				Params:  &ast.FieldList{List: []*ast.Field{{Type: httpResponseField().Type, Names: []*ast.Ident{{Name: "res"}}}}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			Out: `func(response http.ResponseWriter, request *http.Request) {
	data := receiver.F(response)
	execute(response, request, false, "GET / F(response)", http.StatusOK, data)
}`,
		},
		{
			Name: "call F with argument context",
			In:   "GET / F(ctx)",
			Method: &ast.FuncType{
				Params:  &ast.FieldList{List: []*ast.Field{{Type: contextContextField().Type, Names: []*ast.Ident{{Name: "reqCtx"}}}}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			Out: `func(response http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	data := receiver.F(ctx)
	execute(response, request, true, "GET / F(ctx)", http.StatusOK, data)
}`,
		},
		{
			Name: "call F with argument path param",
			In:   "GET /{param} F(param)",
			Method: &ast.FuncType{
				Params:  &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			Out: `func(response http.ResponseWriter, request *http.Request) {
	param := request.PathValue("param")
	data := receiver.F(param)
	execute(response, request, true, "GET /{param} F(param)", http.StatusOK, data)
}`,
		},
		{
			Name: "call F with multiple arguments",
			In:   "GET /{userName} F(ctx, userName)",
			Method: &ast.FuncType{
				Params: &ast.FieldList{List: []*ast.Field{
					{Type: contextContextField().Type, Names: []*ast.Ident{{Name: "ctx"}}},
					{Type: ast.NewIdent("string"), Names: []*ast.Ident{{Name: "n"}}},
				}},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
			},
			Out: `func(response http.ResponseWriter, request *http.Request) {
	ctx := request.Context()
	userName := request.PathValue("userName")
	data := receiver.F(ctx, userName)
	execute(response, request, true, "GET /{userName} F(ctx, userName)", http.StatusOK, data)
}`,
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			pat, err, ok := NewTemplateName(tt.In)
			require.True(t, ok)
			require.NoError(t, err)
			out, _, err := pat.funcLit(tt.Method, tt.Form)
			require.NoError(t, err)
			assert.Equal(t, tt.Out, source.Format(out))
		})
	}
}

func TestTemplateName_HandlerFuncLit_err(t *testing.T) {
	for _, tt := range []struct {
		Name   string
		In     string
		ErrSub string
		Method *ast.FuncType
		Form   *ast.StructType
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
			_, _, err = pat.funcLit(tt.Method, tt.Form)
			assert.ErrorContains(t, err, tt.ErrSub)
		})
	}
}
