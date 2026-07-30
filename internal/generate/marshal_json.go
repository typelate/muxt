package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

// marshalJSONHandlerFunc assembles the handler for a marshalJSON(...) route.
// It reuses the rendered-route assembly — template data, argument parsing,
// method call, template execution into the buffer — so the define body runs
// for its side effects (headers, status), and swaps the response tail: when
// the method succeeded the rendered output is discarded and the marshaled
// result is written as application/json; on any recorded error the rendered
// output is sent as the usual text/html fallback.
func marshalJSONHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sig *types.Signature, resultDataIdent, receiverInterfaceName, bufIdent, statusCodeIdent string) (*ast.FuncLit, error) {
	for _, arg := range def.Arguments {
		if arg.Type == muxt.ArgumentTypeExecute && arg.Identifier == muxt.TemplateNameScopeIdentifierExecute {
			return nil, fmt.Errorf("marshalJSON does not support the execute callback")
		}
	}
	return executeHTMLTemplateHandler(file, config, def, sig, resultDataIdent, receiverInterfaceName, bufIdent, statusCodeIdent, marshalJSONRespondStmts(file, config, resultDataIdent, bufIdent)...)
}

// marshalJSONRespondStmts builds:
//
//	if len(td.errList) == 0 {
//		buf.Reset()
//		jsonBody, err := json.Marshal(td.result)
//		if err != nil {
//			http.Error(response, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
//			return
//		}
//		_, _ = buf.Write(jsonBody)
//		response.Header().Set("content-type", "application/json")
//	}
//
// Marshaling into the buffer before any status write keeps the 500 fallback
// possible on a marshal failure, and the shared write tail then reports the
// correct content-length. Under --output-jsonv2 the Marshal import switches
// to encoding/json/v2.
func marshalJSONRespondStmts(file *File, config RoutesFileConfiguration, resultDataIdent, bufIdent string) []ast.Stmt {
	const jsonBodyIdent = "jsonBody"
	jsonPkgPath := "encoding/json"
	if config.JSONV2 {
		jsonPkgPath = "encoding/json/v2"
	}
	// Success means no recorded errors: parse failures and method errors both
	// append to the template data's error list (td.okay is only set for
	// methods without an error result, so it cannot be the predicate here).
	return []ast.Stmt{&ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X: astgen.CallBuiltinLen(&ast.SelectorExpr{
				X:   ast.NewIdent(resultDataIdent),
				Sel: ast.NewIdent(TemplateDataFieldIdentifierError),
			}),
			Op: token.EQL,
			Y:  astgen.Int(0),
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Reset")}}},
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(jsonBodyIdent), ast.NewIdent(errIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.CallExpr{
					Fun:  astgen.ExportedIdentifier(file, "json", jsonPkgPath, "Marshal"),
					Args: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierResult)}},
				}},
			},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.ExprStmt{X: astgen.HTTPErrorCall(file, ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse),
						&ast.CallExpr{
							Fun:  astgen.ExportedIdentifier(file, "", "net/http", "StatusText"),
							Args: []ast.Expr{astgen.HTTPStatusCode(file, http.StatusInternalServerError)},
						},
						http.StatusInternalServerError)},
					&ast.ReturnStmt{},
				}},
			},
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CallExpr{
					Fun:  &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Write")},
					Args: []ast.Expr{ast.NewIdent(jsonBodyIdent)},
				}},
			},
			&ast.ExprStmt{X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("Header")}},
					Sel: ast.NewIdent("Set"),
				},
				Args: []ast.Expr{astgen.String("content-type"), astgen.String("application/json")},
			}},
		}},
	}}
}
