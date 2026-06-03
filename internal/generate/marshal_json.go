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

// appendMarshalIntoBuffer appends the statements that marshal valueIdent into the
// *bytes.Buffer named bufIdent as JSON, returning err on failure. With
// --output-jsonv2 it uses encoding/json/v2 MarshalWrite; otherwise it uses
// encoding/json Marshal followed by buf.Write.
func appendMarshalIntoBuffer(file *File, config RoutesFileConfiguration, bufIdent, valueIdent string) []ast.Stmt {
	const bIdent = "b"
	if config.JSONV2 {
		// if err := json.MarshalWrite(buf, value); err != nil { return err }
		return []ast.Stmt{&ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
				astgen.Call(file, "json", "encoding/json/v2", "MarshalWrite", ast.NewIdent(bufIdent), ast.NewIdent(valueIdent)),
			}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}}}},
		}}
	}
	return []ast.Stmt{
		// b, err := json.Marshal(value)
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(bIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
			astgen.Call(file, "", "encoding/json", "Marshal", ast.NewIdent(valueIdent)),
		}},
		// if err != nil { return err }
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}}}},
		},
		// _, _ = buf.Write(b)
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{
			&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Write")}, Args: []ast.Expr{ast.NewIdent(bIdent)}},
		}},
	}
}

// marshalSendClosure builds the callback passed to the receiver method for a
// marshalJSON(sendX) wrapped send argument. It mirrors sseClosure but, instead of
// executing a template, marshals the result value as JSON into the pooled buffer.
// The JSON bytes still flow through SSETemplateData.WriteTo so they are framed as
// SSE data: lines:
//
//	func(result T) error {
//		if err := request.Context().Err(); err != nil { return err }
//		buf := bytesBufferPool.Get().(*bytes.Buffer); buf.Reset(); defer bytesBufferPool.Put(buf)
//		td := SSETemplateData[Recv, T]{receiver: receiver, request: request, pathsPrefix: pathsPrefix, result: result}
//		// marshal result into buf
//		td.data = buf
//		mut.Lock()
//		defer mut.Unlock()
//		if _, err := td.WriteTo(response); err != nil { return err }
//		flusher.Flush()
//		return nil
//	}
func marshalSendClosure(file *File, config RoutesFileConfiguration, resultType types.Type, receiverInterfaceName, flusherIdent, mutexIdent string) (*ast.FuncLit, error) {
	const (
		bufIdent    = "buf"
		tdIdent     = "td"
		resultIdent = "result"
	)
	response := muxt.TemplateNameScopeIdentifierHTTPResponse
	request := muxt.TemplateNameScopeIdentifierHTTPRequest

	resultTypeExpr, err := file.TypeASTExpression(resultType)
	if err != nil {
		return nil, err
	}

	params := []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(resultIdent)}, Type: resultTypeExpr}}
	tdElts := []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierReceiver), Value: ast.NewIdent(receiverIdent)},
		&ast.KeyValueExpr{Key: ast.NewIdent(request), Value: ast.NewIdent(request)},
		&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: ast.NewIdent(pathPrefixPathsStructFieldName)},
		&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierResult), Value: ast.NewIdent(resultIdent)},
	}

	body := []ast.Stmt{
		// if err := request.Context().Err(); err != nil { return err }
		&ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
				&ast.CallExpr{Fun: &ast.SelectorExpr{
					X:   &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(request), Sel: ast.NewIdent(httpRequestContextMethod)}},
					Sel: ast.NewIdent("Err"),
				}},
			}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}}}},
		},
	}
	// buf := bytesBufferPool.Get().(*bytes.Buffer); buf.Reset(); defer bytesBufferPool.Put(buf)
	body = append(body, astgen.GetBufferFromPool(file, bufferPoolIdent, bufIdent)...)
	// td := SSETemplateData[Recv, T]{...}
	body = append(body, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(tdIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CompositeLit{
			Type: &ast.IndexListExpr{X: ast.NewIdent(config.SSETemplateDataType), Indices: []ast.Expr{ast.NewIdent(receiverInterfaceName), resultTypeExpr}},
			Elts: tdElts,
		}},
	})
	// marshal result into buf
	body = append(body, appendMarshalIntoBuffer(file, config, bufIdent, resultIdent)...)
	body = append(body,
		// td.data = buf
		&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(tdIdent), Sel: ast.NewIdent(sseTemplateDataFieldData)}}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(bufIdent)}},
		// mut.Lock()
		&ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(mutexIdent), Sel: ast.NewIdent("Lock")}}},
		// defer mut.Unlock()
		&ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(mutexIdent), Sel: ast.NewIdent("Unlock")}}},
		// if _, err := td.WriteTo(response); err != nil { return err }
		&ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: ast.NewIdent(tdIdent), Sel: ast.NewIdent("WriteTo")},
				Args: []ast.Expr{ast.NewIdent(response)},
			}}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}}}},
		},
		// flusher.Flush()
		&ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(flusherIdent), Sel: ast.NewIdent("Flush")}}},
		// return nil
		&ast.ReturnStmt{Results: []ast.Expr{astgen.Nil()}},
	)

	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: params},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}, nil
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
