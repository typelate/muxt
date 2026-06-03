package generate

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/typelate/muxt/internal/astgen"
)

// unmarshalJSONStatements decodes request.Body (bodyExpr) into a new local
// `varIdent` of targetType. On error it runs parseErrBlock.
func unmarshalJSONStatements(file *File, config RoutesFileConfiguration, varIdent string, targetType types.Type, bodyExpr ast.Expr, parseErrBlock *ast.BlockStmt) ([]ast.Stmt, error) {
	typeExpr, err := file.TypeASTExpression(targetType)
	if err != nil {
		return nil, err
	}
	decl := &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{
		&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(varIdent)}, Type: typeExpr},
	}}}
	ifErr := func(call ast.Expr) ast.Stmt {
		return &ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: parseErrBlock,
		}
	}
	if config.JSONV2 {
		return []ast.Stmt{decl, ifErr(astgen.Call(file, "json", "encoding/json/v2", "UnmarshalRead",
			bodyExpr, &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(varIdent)}))}, nil
	}
	const bIdent = "bodyBytes"
	readAll := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(bIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE,
		Rhs: []ast.Expr{astgen.Call(file, "", "io", "ReadAll", bodyExpr)},
	}
	checkReadErr := &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: parseErrBlock,
	}
	return []ast.Stmt{decl, readAll, checkReadErr, ifErr(astgen.Call(file, "", "encoding/json", "Unmarshal",
		ast.NewIdent(bIdent), &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(varIdent)}))}, nil
}
