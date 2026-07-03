package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"slices"
	"strconv"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

// signalCallbackSignature returns the func(any, bool) error type synthesized for
// a signal argument when the receiver method is not already defined. The bool is
// the patch-signals onlyIfMissing flag.
func signalCallbackSignature() *types.Signature {
	anyType := types.Universe.Lookup("any").Type()
	boolType := types.Typ[types.Bool]
	errType := types.Universe.Lookup("error").Type()
	return types.NewSignatureType(nil, nil, nil,
		types.NewTuple(
			types.NewVar(0, nil, "", anyType),
			types.NewVar(0, nil, "", boolType),
		),
		types.NewTuple(types.NewVar(0, nil, "", errType)),
		false)
}

// validateSignalCallbackShape checks that a signal callback parameter is
// func(T, bool) error and returns the marshaled result type T. The second
// parameter carries the patch-signals onlyIfMissing flag.
func validateSignalCallbackShape(methodName string, callback *types.Signature) (types.Type, error) {
	errIface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	boolType := types.Typ[types.Bool]
	if callback == nil || callback.Results().Len() != 1 || !types.Implements(callback.Results().At(0).Type(), errIface) {
		return nil, fmt.Errorf("signal argument for %s must be a func(data, onlyIfMissing bool) error", methodName)
	}
	if callback.Params().Len() != 2 || !types.Identical(callback.Params().At(1).Type(), boolType) {
		return nil, fmt.Errorf("signal callback for %s must take a data value and an onlyIfMissing bool", methodName)
	}
	return callback.Params().At(0).Type(), nil
}

// signalEventClosure builds the callback for a signal argument on a streaming
// route. Each call marshals the result as JSON and writes one
// datastar-patch-signals frame under the stream mutex:
//
//	func(result T, onlyIfMissing bool) error {
//		if err := request.Context().Err(); err != nil { return err }
//		buf := bytesBufferPool.Get().(*bytes.Buffer); buf.Reset(); defer bytesBufferPool.Put(buf)
//		if err := datastarMarshalSignals(buf, result); err != nil { return err }
//		mut.Lock()
//		defer mut.Unlock()
//		if _, err := io.WriteString(response, "event: datastar-patch-signals\ndata: signals "); err != nil { return err }
//		if _, err := response.Write(buf.Bytes()); err != nil { return err }
//		if _, err := io.WriteString(response, "\n"); err != nil { return err }
//		if onlyIfMissing { if _, err := io.WriteString(response, "data: onlyIfMissing true\n"); err != nil { return err } }
//		if _, err := io.WriteString(response, "\n"); err != nil { return err }
//		flusher.Flush()
//		return nil
//	}
func signalEventClosure(file *File, resultType types.Type) (*ast.FuncLit, error) {
	const (
		bufIdent           = "buf"
		resultIdent        = "result"
		onlyIfMissingIdent = "onlyIfMissing"
	)
	response := muxt.TemplateNameScopeIdentifierHTTPResponse

	resultTypeExpr, err := file.TypeASTExpression(resultType)
	if err != nil {
		return nil, err
	}

	writeStringStmt := func(s string) ast.Stmt {
		return returnErrIfWriteFails(astgen.Call(file, "", "io", "WriteString", ast.NewIdent(response), astgen.String(s)))
	}

	body := []ast.Stmt{
		contextCancelledGuard(file),
	}
	body = append(body, astgen.GetBufferFromPool(file, bufferPoolIdent, bufIdent)...)
	body = append(body,
		// if err := datastarMarshalSignals(buf, result); err != nil { return err }
		&ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
				&ast.CallExpr{Fun: ast.NewIdent(datastarMarshalSignalsFuncName), Args: []ast.Expr{ast.NewIdent(bufIdent), ast.NewIdent(resultIdent)}},
			}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}}}},
		},
		&ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(streamMutexIdent), Sel: ast.NewIdent("Lock")}}},
		&ast.DeferStmt{Call: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(streamMutexIdent), Sel: ast.NewIdent("Unlock")}}},
		writeStringStmt("event: datastar-patch-signals\ndata: signals "),
		// if _, err := response.Write(buf.Bytes()); err != nil { return err }
		returnErrIfWriteFails(&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(response), Sel: ast.NewIdent("Write")},
			Args: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Bytes")}}},
		}),
		writeStringStmt("\n"),
		// if onlyIfMissing { ... }
		&ast.IfStmt{
			Cond: ast.NewIdent(onlyIfMissingIdent),
			Body: &ast.BlockStmt{List: []ast.Stmt{writeStringStmt("data: onlyIfMissing true\n")}},
		},
		writeStringStmt("\n"),
		&ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(streamFlusherIdent), Sel: ast.NewIdent("Flush")}}},
		&ast.ReturnStmt{Results: []ast.Expr{astgen.Nil()}},
	)

	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(resultIdent)}, Type: resultTypeExpr},
				{Names: []*ast.Ident{ast.NewIdent(onlyIfMissingIdent)}, Type: ast.NewIdent("bool")},
			}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}, nil
}

// datastarSignalsHandlerFunc builds the http.HandlerFunc for a route wrapped in
// datastar(marshalJSON(...)). It mirrors marshalJSONHandlerFunc: the inner method
// returns (T) or (T, error); T is marshaled to the response as application/json
// with status 200; a non-nil method error or a marshal error responds 500.
//
// Before writing the JSON body it evaluates the route's define body once against
// a DatastarSignalsTemplateData value (rendered output discarded into a local
// buffer) so the template can call .OnlyIfMissing to set the
// datastar-only-if-missing response header. Header writes from the template
// execution must occur before the body is written, so the template runs first,
// then marshalJSONResponse sets Content-Type and writes the body.
func datastarSignalsHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sigs map[string]*types.Signature, receiver *types.Named, sig *types.Signature, receiverInterfaceName string) (*ast.FuncLit, error) {
	const (
		vIdent   = "v"
		tdIdent  = "td"
		bufIdent = "buf"
	)

	response := muxt.TemplateNameScopeIdentifierHTTPResponse
	request := muxt.TemplateNameScopeIdentifierHTTPRequest

	// Validate the result signature: the method must return a value to marshal,
	// optionally followed by an error. Mirrors marshalJSONHandlerFunc.
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

	resultType := results.At(0).Type()
	resultTypeExpr, err := file.TypeASTExpression(resultType)
	if err != nil {
		return nil, err
	}

	// Determine the callFun expression (selector or bare ident).
	var callFun ast.Expr
	if obj, _, _ := types.LookupFieldOrMethod(receiver, true, receiver.Obj().Pkg(), methodName); obj != nil {
		callFun = &ast.SelectorExpr{X: ast.NewIdent(receiverIdent), Sel: ast.NewIdent(methodName)}
	} else {
		callFun = ast.NewIdent(methodName)
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

	// Parse ctx/path/unmarshalX(body) args (400 on failure), mirroring marshalJSONHandlerFunc.
	parseErrBlock := func() *ast.BlockStmt {
		return &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: astgen.HTTPErrorCall(file, ast.NewIdent(response), astgen.CallError(errIdent), http.StatusBadRequest)},
			&ast.ReturnStmt{},
		}}
	}
	validationFailureBlock := func(string) *ast.BlockStmt { return parseErrBlock() }

	body, err = appendParseArgumentStatements(body, def, file, types.NewStruct(nil, nil), sigs, nil, receiver, "", config, def.CallExpression(), validationFailureBlock, parseErrBlock)
	if err != nil {
		return nil, err
	}

	// Build call expression with parsed args.
	callArgs := slices.Clone(def.CallExpression().Args)
	callExpr := &ast.CallExpr{Fun: callFun, Args: callArgs}

	// Capture result(s) and handle method error (mirrors marshalJSONHandlerFunc).
	if hasErrResult {
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
		body = append(body, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(vIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{callExpr},
		})
	}

	// Evaluate the route's define body against a DatastarSignalsTemplateData value
	// so .OnlyIfMissing can set the response header. The rendered output is
	// discarded into a local buffer; only the header side effects matter.
	//
	//	td := DatastarSignalsTemplateData[Recv, T]{receiver: receiver, response: response, request: request, pathsPrefix: pathsPrefix, result: v}
	tdElts := []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierReceiver), Value: ast.NewIdent(receiverIdent)},
		&ast.KeyValueExpr{Key: ast.NewIdent(response), Value: ast.NewIdent(response)},
		&ast.KeyValueExpr{Key: ast.NewIdent(request), Value: ast.NewIdent(request)},
		&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: ast.NewIdent(pathPrefixPathsStructFieldName)},
		&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierResult), Value: ast.NewIdent(vIdent)},
	}
	body = append(body,
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tdIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CompositeLit{
				Type: &ast.IndexListExpr{X: ast.NewIdent(config.DatastarSignalsTemplateDataType), Indices: []ast.Expr{ast.NewIdent(receiverInterfaceName), resultTypeExpr}},
				Elts: tdElts,
			}},
		},
		// var buf bytes.Buffer
		&ast.DeclStmt{Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{
				Names: []*ast.Ident{ast.NewIdent(bufIdent)},
				Type:  astgen.ExportedIdentifier(file, "", "bytes", "Buffer"),
			}},
		}},
		// if err := templates.ExecuteTemplate(&buf, name, &td); err != nil { slog...; http.Error(500); return }
	)
	execErr := checkExecuteTemplateError(file, config.Logger, def.RawPattern())
	execErr.Init = &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(errIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(def.TemplatesVariable()), Sel: ast.NewIdent("ExecuteTemplate")},
			Args: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(bufIdent)}, &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(def.Name())}, &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(tdIdent)}},
		}},
	}
	body = append(body, execErr)

	// marshalJSONResponse(response, v) — sets Content-Type and writes the JSON body.
	body = append(body, &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  ast.NewIdent(marshalJSONResponseFuncName),
		Args: []ast.Expr{ast.NewIdent(response), ast.NewIdent(vIdent)},
	}})

	handlerFunc.Body.List = body
	return handlerFunc, nil
}

// contextCancelledGuard returns the statement:
//
//	if err := request.Context().Err(); err != nil { return err }
func contextCancelledGuard(file *File) ast.Stmt {
	request := muxt.TemplateNameScopeIdentifierHTTPRequest
	return &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
			&ast.CallExpr{Fun: &ast.SelectorExpr{
				X:   &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(request), Sel: ast.NewIdent(httpRequestContextMethod)}},
				Sel: ast.NewIdent("Err"),
			}},
		}},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}}}},
	}
}

// returnErrIfWriteFails wraps a write call (returning (n, error)) in:
//
//	if _, err := <call>; err != nil { return err }
func returnErrIfWriteFails(call ast.Expr) ast.Stmt {
	return &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}}}},
	}
}
