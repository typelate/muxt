package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

// generateParseValueFromStringStatements emits the statements that parse str
// into valueType and pass the result to assignment. On a parse failure it runs
// errBlock, which callers supply so the failure can be handled differently per
// context (normal handlers accumulate into the template data; SSE handlers
// respond 400 before establishing the stream).
func generateParseValueFromStringStatements(file *File, tmp string, str ast.Expr, valueType source.Type, method muxt.UnmarshalMethod, validations []ast.Stmt, assignment func(ast.Expr) ast.Stmt, errBlock *ast.BlockStmt) ([]ast.Stmt, error) {
	typeExpr, err := file.TypeExpr(valueType)
	if err != nil {
		return nil, err
	}
	// convert wraps the parsed value in a conversion to the target basic type
	// for the strconv functions that return a wider type (ParseInt/ParseUint).
	convert := func(exp ast.Expr) ast.Stmt {
		return assignment(&ast.CallExpr{
			Fun:  typeExpr,
			Args: []ast.Expr{exp},
		})
	}
	switch method {
	case muxt.UnmarshalBool:
		return parseBlock(tmp, astgen.StrconvParseBoolCall(file, str), validations, errBlock, assignment), nil
	case muxt.UnmarshalInt:
		return parseBlock(tmp, astgen.StrconvAtoiCall(file, str), validations, errBlock, assignment), nil
	case muxt.UnmarshalInt8:
		return parseBlock(tmp, astgen.StrconvParseInt8Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalInt16:
		return parseBlock(tmp, astgen.StrconvParseInt16Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalInt32:
		return parseBlock(tmp, astgen.StrconvParseInt32Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalInt64:
		return parseBlock(tmp, astgen.StrconvParseInt64Call(file, str), validations, errBlock, assignment), nil
	case muxt.UnmarshalUint:
		return parseBlock(tmp, astgen.StrconvParseUint0Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalUint8:
		return parseBlock(tmp, astgen.StrconvParseUint8Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalUint16:
		return parseBlock(tmp, astgen.StrconvParseUint16Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalUint32:
		return parseBlock(tmp, astgen.StrconvParseUint32Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalUint64:
		return parseBlock(tmp, astgen.StrconvParseUint64Call(file, str), validations, errBlock, assignment), nil
	case muxt.UnmarshalFloat32:
		return parseBlock(tmp, astgen.StrconvParseFloatCall(file, str, 32), validations, errBlock, convert), nil
	case muxt.UnmarshalFloat64:
		return parseBlock(tmp, astgen.StrconvParseFloatCall(file, str, 64), validations, errBlock, assignment), nil
	case muxt.UnmarshalString:
		if len(validations) == 0 {
			assign := assignment(str)
			statements := slices.Concat(validations, []ast.Stmt{assign})
			return statements, nil
		}
		statements := slices.Concat([]ast.Stmt{&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{str},
		}}, validations, []ast.Stmt{assignment(ast.NewIdent(tmp))})
		return statements, nil
	case muxt.UnmarshalTextUnmarshaler:
		return []ast.Stmt{
			&ast.DeclStmt{
				Decl: &ast.GenDecl{
					Tok: token.VAR,
					Specs: []ast.Spec{
						&ast.ValueSpec{
							Names: []*ast.Ident{ast.NewIdent(tmp)},
							Type:  typeExpr,
						},
					},
				},
			},
			&ast.IfStmt{
				Init: &ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(errIdent)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent(tmp),
							Sel: ast.NewIdent("UnmarshalText"),
						},
						Args: []ast.Expr{&ast.CallExpr{
							Fun: &ast.ArrayType{
								Elt: ast.NewIdent("byte"),
							},
							Args: []ast.Expr{str},
						}},
					}},
				},
				Cond: &ast.BinaryExpr{
					X:  ast.NewIdent(errIdent),
					Op: token.NEQ,
					Y:  ast.NewIdent("nil"),
				},
				Body: errBlock,
			},
			assignment(ast.NewIdent(tmp)),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported type: %s", astgen.Format(typeExpr))
	}
}

func parseBlock(tmpIdent string, parseCall ast.Expr, validations []ast.Stmt, errBlock *ast.BlockStmt, handleResult func(out ast.Expr) ast.Stmt) []ast.Stmt {
	const errIdent = "err"
	callParse := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(tmpIdent), ast.NewIdent(errIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{parseCall},
	}
	errCheckStmt := &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: errBlock,
	}
	if len(validations) > 0 {
		errCheckStmt.Else = &ast.BlockStmt{List: validations}
	}
	block := &ast.BlockStmt{List: []ast.Stmt{callParse, errCheckStmt}}
	block.List = append(block.List, handleResult(ast.NewIdent(tmpIdent)))
	return block.List
}
