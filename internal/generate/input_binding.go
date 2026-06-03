package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

// inputWrapperTargetType returns the parameter type bound to an input wrapper
// when the receiver method is not defined (so its real parameter type is
// unknown). The wrapper passes the request body through as a raw value.
func inputWrapperTargetType(file *File, config RoutesFileConfiguration, wrapper string) (types.Type, error) {
	switch wrapper {
	case muxt.InputWrapperUnmarshalForm:
		pkg, ok := file.Types("net/url")
		if !ok {
			return nil, fmt.Errorf(`the "net/url" package must be loaded`)
		}
		return pkg.Scope().Lookup("Values").Type(), nil
	default: // unmarshalJSON
		if config.JSONV2 {
			pkg, ok := file.Types("encoding/json/jsontext")
			if !ok {
				return nil, fmt.Errorf(`the "encoding/json/jsontext" package must be loaded`)
			}
			return types.NewPointer(pkg.Scope().Lookup("Decoder").Type()), nil
		}
		pkg, ok := file.Types("encoding/json")
		if !ok {
			return nil, fmt.Errorf(`the "encoding/json" package must be loaded`)
		}
		return pkg.Scope().Lookup("RawMessage").Type(), nil
	}
}

// isNamedType reports whether t is the named type pkgPath.name.
func isNamedType(t types.Type, pkgPath, name string) bool {
	// An alias (e.g. encoding/json.RawMessage under gotypesalias=1) carries its
	// own name/package on its Obj, distinct from the type it resolves to.
	if a, ok := t.(*types.Alias); ok {
		o := a.Obj()
		return o != nil && o.Pkg() != nil && o.Pkg().Path() == pkgPath && o.Name() == name
	}
	n, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	o := n.Obj()
	return o != nil && o.Pkg() != nil && o.Pkg().Path() == pkgPath && o.Name() == name
}

// unmarshalJSONStatements decodes request.Body (bodyExpr) into a new local
// `varIdent` of targetType. On error it runs parseErrBlock.
func unmarshalJSONStatements(file *File, config RoutesFileConfiguration, varIdent string, targetType types.Type, bodyExpr ast.Expr, parseErrBlock *ast.BlockStmt) ([]ast.Stmt, error) {
	typeExpr, err := file.TypeASTExpression(targetType)
	if err != nil {
		return nil, err
	}
	// Undefined-method pass-through: when the receiver method is not defined,
	// the wrapper binds a raw type. Bind the body directly instead of decoding.
	if config.JSONV2 {
		if ptr, ok := targetType.(*types.Pointer); ok && isNamedType(ptr.Elem(), "encoding/json/jsontext", "Decoder") {
			return []ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(varIdent)}, Tok: token.DEFINE,
				Rhs: []ast.Expr{astgen.Call(file, "json", "encoding/json/jsontext", "NewDecoder", bodyExpr)},
			}}, nil
		}
	}
	if isNamedType(targetType, "encoding/json", "RawMessage") {
		const bIdent = "bodyBytes"
		readAll := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(bIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE,
			Rhs: []ast.Expr{astgen.Call(file, "", "io", "ReadAll", bodyExpr)},
		}
		checkReadErr := &ast.IfStmt{
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: parseErrBlock,
		}
		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(varIdent)}, Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: typeExpr, Args: []ast.Expr{ast.NewIdent(bIdent)}}},
		}
		return []ast.Stmt{readAll, checkReadErr, assign}, nil
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
