package generate

import (
	"go/ast"
	"go/token"

	"github.com/typelate/muxt/internal/astgen"
)

// datastarMarshalSignalsFuncName is the generated package-level helper that
// marshals a signal result into a buffer as JSON for datastar-patch-signals.
const datastarMarshalSignalsFuncName = "datastarMarshalSignals"

// datastarSignalsDecls returns the datastarMarshalSignals helper. It is emitted
// in Datastar mode when at least one route uses a signal render callback.
func datastarSignalsDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	return []ast.Decl{datastarMarshalSignalsFunc(file, config)}
}

// datastarMarshalSignalsFunc builds:
//
//	func datastarMarshalSignals(buf *bytes.Buffer, v any) error {
//		// under GOEXPERIMENT=jsonv2:
//		return json.MarshalWrite(buf, v) // encoding/json/v2
//		// otherwise:
//		b, err := json.Marshal(v)
//		if err != nil {
//			return err
//		}
//		_, err = buf.Write(b)
//		return err
//	}
//
// The jsonv2 form streams directly into the pooled buffer with MarshalWrite; the
// fallback marshals to a slice and copies it in.
func datastarMarshalSignalsFunc(file *File, config RoutesFileConfiguration) *ast.FuncDecl {
	const (
		bufIdent = "buf"
		vIdent   = "v"
		bIdent   = "b"
	)
	bufType := &ast.StarExpr{X: astgen.ExportedIdentifier(file, "", "bytes", "Buffer")}

	var body []ast.Stmt
	if config.JSONV2 {
		body = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			astgen.Call(file, "json", "encoding/json/v2", "MarshalWrite", ast.NewIdent(bufIdent), ast.NewIdent(vIdent)),
		}}}
	} else {
		body = []ast.Stmt{
			// b, err := json.Marshal(v)
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(bIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
				astgen.Call(file, "", "encoding/json", "Marshal", ast.NewIdent(vIdent)),
			}},
			// if err != nil { return err }
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}}}},
			},
			// _, err = buf.Write(b)
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent(errIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{
				&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Write")}, Args: []ast.Expr{ast.NewIdent(bIdent)}},
			}},
			// return err
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}},
		}
	}

	return &ast.FuncDecl{
		Name: ast.NewIdent(datastarMarshalSignalsFuncName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(bufIdent)}, Type: bufType},
				{Names: []*ast.Ident{ast.NewIdent(vIdent)}, Type: ast.NewIdent("any")},
			}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
}
