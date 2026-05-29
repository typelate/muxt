package astgen

import (
	"go/ast"
	"go/token"
)

// BytesNewBuffer creates a bytes.NewBuffer call expression
func BytesNewBuffer(im ImportManager, expr ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(im.Import("", "bytes")),
			Sel: ast.NewIdent("NewBuffer"),
		},
		Args: []ast.Expr{expr},
	}
}

// SyncPoolBytesBuffer creates a sync.Pool composite literal whose New function
// returns a fresh *bytes.Buffer:
//
//	sync.Pool{New: func() any { return bytes.NewBuffer(nil) }}
func SyncPoolBytesBuffer(im ImportManager) *ast.CompositeLit {
	return &ast.CompositeLit{
		Type: ExportedIdentifier(im, "", "sync", "Pool"),
		Elts: []ast.Expr{
			&ast.KeyValueExpr{
				Key: ast.NewIdent("New"),
				Value: &ast.FuncLit{
					Type: &ast.FuncType{
						Params:  &ast.FieldList{},
						Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
					},
					Body: &ast.BlockStmt{List: []ast.Stmt{
						&ast.ReturnStmt{Results: []ast.Expr{BytesNewBuffer(im, Nil())}},
					}},
				},
			},
		},
	}
}

// GetBufferFromPool returns the statements that acquire a *bytes.Buffer from a
// sync.Pool, reset it for use, and defer returning it to the pool. The deferred
// reset is stacked after the deferred Put so that, running LIFO, the buffer is
// reset before it goes back into the pool:
//
//	buf := builderPool.Get().(*bytes.Buffer)
//	buf.Reset()
//	defer builderPool.Put(buf)
//	defer buf.Reset()
func GetBufferFromPool(im ImportManager, poolIdent, bufIdent string) []ast.Stmt {
	resetCall := func() *ast.CallExpr {
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Reset")}}
	}
	return []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(bufIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.TypeAssertExpr{
				X:    &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(poolIdent), Sel: ast.NewIdent("Get")}},
				Type: &ast.StarExpr{X: ExportedIdentifier(im, "", "bytes", "Buffer")},
			}},
		},
		&ast.ExprStmt{X: resetCall()},
		&ast.DeferStmt{Call: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(poolIdent), Sel: ast.NewIdent("Put")},
			Args: []ast.Expr{ast.NewIdent(bufIdent)},
		}},
		&ast.DeferStmt{Call: resetCall()},
	}
}
