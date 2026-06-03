package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"slices"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

// marshalJSONResponseFuncName is the generated package-level helper that
// marshals a value as JSON and writes it to the response.
const marshalJSONResponseFuncName = "marshalJSONResponse"

// definitionUsesBytesBufferPool reports whether a route's generated handler uses
// the shared bytesBufferPool. marshalJSON handlers marshal into a local buffer, so
// a file containing only marshalJSON routes must not emit the (then unused) pool.
func definitionUsesBytesBufferPool(def muxt.Definition) bool {
	switch def.Representation() {
	case muxt.RepresentationNone, muxt.RepresentationSSE:
		return true
	default:
		return false
	}
}

// marshalJSONResponseDecls returns the marshalJSONResponse helper. It is
// emitted when at least one route uses the marshalJSON representation wrapper.
func marshalJSONResponseDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	return []ast.Decl{marshalJSONResponseFunc(file, config)}
}

// marshalJSONResponseFunc builds:
//
//	func marshalJSONResponse(response http.ResponseWriter, v any) {
//		// with --output-jsonv2 (encoding/json/v2):
//		var buf bytes.Buffer
//		if err := json.MarshalWrite(&buf, v); err != nil {
//			http.Error(response, err.Error(), http.StatusInternalServerError)
//			return
//		}
//		response.Header().Set("Content-Type", "application/json")
//		_, _ = response.Write(buf.Bytes())
//
//		// default (encoding/json):
//		b, err := json.Marshal(v)
//		if err != nil {
//			http.Error(response, err.Error(), http.StatusInternalServerError)
//			return
//		}
//		response.Header().Set("Content-Type", "application/json")
//		_, _ = response.Write(b)
//	}
//
// Marshalling happens before any header/body write so a marshal error cleanly
// produces a 500 via http.Error (which sets its own header).
func marshalJSONResponseFunc(file *File, config RoutesFileConfiguration) *ast.FuncDecl {
	const (
		responseIdent = "response"
		vIdent        = "v"
		bIdent        = "b"
		bufIdent      = "buf"
	)

	responseExpr := ast.NewIdent(responseIdent)

	// http.Error(response, err.Error(), http.StatusInternalServerError)
	httpErrorStmt := func() ast.Stmt {
		return &ast.ExprStmt{X: astgen.HTTPErrorCall(file, responseExpr, astgen.CallError(errIdent), http.StatusInternalServerError)}
	}

	// response.Header().Set("Content-Type", "application/json")
	setContentTypeStmt := &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: responseExpr, Sel: ast.NewIdent("Header")},
				Args: []ast.Expr{},
			},
			Sel: ast.NewIdent("Set"),
		},
		Args: []ast.Expr{astgen.String("Content-Type"), astgen.String("application/json")},
	}}

	var body []ast.Stmt
	if config.JSONV2 {
		// var buf bytes.Buffer
		bufType := astgen.ExportedIdentifier(file, "", "bytes", "Buffer")
		body = []ast.Stmt{
			&ast.DeclStmt{Decl: &ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(bufIdent)},
					Type:  bufType,
				}},
			}},
			// if err := json.MarshalWrite(&buf, v); err != nil { http.Error(...); return }
			&ast.IfStmt{
				Init: &ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(errIdent)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{
						astgen.Call(file, "json", "encoding/json/v2", "MarshalWrite",
							&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(bufIdent)},
							ast.NewIdent(vIdent),
						),
					},
				},
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					httpErrorStmt(),
					&ast.ReturnStmt{},
				}},
			},
			// response.Header().Set("Content-Type", "application/json")
			setContentTypeStmt,
			// _, _ = response.Write(buf.Bytes())
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CallExpr{
					Fun:  &ast.SelectorExpr{X: responseExpr, Sel: ast.NewIdent("Write")},
					Args: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Bytes")}}},
				}},
			},
		}
	} else {
		body = []ast.Stmt{
			// b, err := json.Marshal(v)
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(bIdent), ast.NewIdent(errIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{
					astgen.Call(file, "", "encoding/json", "Marshal", ast.NewIdent(vIdent)),
				},
			},
			// if err != nil { http.Error(...); return }
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					httpErrorStmt(),
					&ast.ReturnStmt{},
				}},
			},
			// response.Header().Set("Content-Type", "application/json")
			setContentTypeStmt,
			// _, _ = response.Write(b)
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CallExpr{
					Fun:  &ast.SelectorExpr{X: responseExpr, Sel: ast.NewIdent("Write")},
					Args: []ast.Expr{ast.NewIdent(bIdent)},
				}},
			},
		}
	}

	return &ast.FuncDecl{
		Name: ast.NewIdent(marshalJSONResponseFuncName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(responseIdent)}, Type: astgen.HTTPResponseWriter(file)},
				{Names: []*ast.Ident{ast.NewIdent(vIdent)}, Type: ast.NewIdent("any")},
			}},
		},
		Body: &ast.BlockStmt{List: body},
	}
}

// marshalJSONHandlerFunc builds the http.HandlerFunc for a route wrapped in
// marshalJSON(...). The inner method returns (T) or (T, error). T is marshaled
// to the response as application/json with status 200. A non-nil method error,
// or a marshal error, responds 500. Ctx/path/unmarshalX(body) args are bound
// using the same appendParseArgumentStatements machinery as other handlers.
func marshalJSONHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sigs map[string]*types.Signature, receiver *types.Named, sig *types.Signature, receiverInterfaceName string) (*ast.FuncLit, error) {
	const (
		vIdent = "v"
	)

	response := muxt.TemplateNameScopeIdentifierHTTPResponse
	request := muxt.TemplateNameScopeIdentifierHTTPRequest

	// Validate the result signature: the method must return a value to marshal,
	// optionally followed by an error.
	errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	methodName := def.FunctionIdentifier().Name
	results := sig.Results()
	var hasErrResult bool
	switch results.Len() {
	case 0:
		return nil, fmt.Errorf("marshalJSON method %s must return a value to marshal", methodName)
	case 1:
		if types.Implements(results.At(0).Type(), errIface) {
			return nil, fmt.Errorf("marshalJSON method %s must return a value to marshal, not only error", methodName)
		}
	case 2:
		if !types.Implements(results.At(1).Type(), errIface) {
			return nil, fmt.Errorf("marshalJSON method %s second result must be error", methodName)
		}
		hasErrResult = true
	default:
		return nil, fmt.Errorf("marshalJSON method %s must return one value or a value and an error", methodName)
	}

	// Determine the callFun expression (selector or bare ident).
	var callFun ast.Expr
	if obj, _, _ := types.LookupFieldOrMethod(receiver, true, receiver.Obj().Pkg(), def.FunctionIdentifier().Name); obj != nil {
		callFun = &ast.SelectorExpr{X: ast.NewIdent(receiverIdent), Sel: ast.NewIdent(def.FunctionIdentifier().Name)}
	} else {
		callFun = ast.NewIdent(def.FunctionIdentifier().Name)
	}

	handlerFunc := &ast.FuncLit{
		Type: astgen.HTTPHandlerFuncType(file, response, request),
		Body: &ast.BlockStmt{},
	}

	// defer func() { _ = request.Body.Close() }()
	body := []ast.Stmt{
		&ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.FuncLit{
			Type: &ast.FuncType{Params: &ast.FieldList{}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{
					X:   &ast.SelectorExpr{X: ast.NewIdent(request), Sel: ast.NewIdent("Body")},
					Sel: ast.NewIdent("Close"),
				}}},
			}}},
		}}},
	}

	// Parse ctx/path/unmarshalX(body) args using existing machinery.
	// parseErrBlock responds 400 and returns (mirrors streamHandlerPrologue).
	parseErrBlock := func() *ast.BlockStmt {
		return &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: astgen.HTTPErrorCall(file, ast.NewIdent(response), astgen.CallError(errIdent), http.StatusBadRequest)},
			&ast.ReturnStmt{},
		}}
	}
	validationFailureBlock := func(string) *ast.BlockStmt { return parseErrBlock() }

	var err error
	body, err = appendParseArgumentStatements(body, def, file, types.NewStruct(nil, nil), sigs, nil, receiver, "", config, def.CallExpression(), validationFailureBlock, parseErrBlock)
	if err != nil {
		return nil, err
	}

	// Build call expression with parsed args.
	callArgs := slices.Clone(def.CallExpression().Args)
	callExpr := &ast.CallExpr{Fun: callFun, Args: callArgs}

	// Capture result(s) and handle method error.
	if hasErrResult {
		// v, err := receiver.Method(...)
		// if err != nil { http.Error(response, err.Error(), 500); return }
		body = append(body,
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(vIdent), ast.NewIdent(errIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{callExpr},
			},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.ExprStmt{X: astgen.HTTPErrorCall(file, ast.NewIdent(response), astgen.CallError(errIdent), http.StatusInternalServerError)},
					&ast.ReturnStmt{},
				}},
			},
		)
	} else {
		// v := receiver.Method(...)
		body = append(body, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(vIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{callExpr},
		})
	}

	// marshalJSONResponse(response, v)
	body = append(body, &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  ast.NewIdent(marshalJSONResponseFuncName),
		Args: []ast.Expr{ast.NewIdent(response), ast.NewIdent(vIdent)},
	}})

	handlerFunc.Body.List = body
	return handlerFunc, nil
}
