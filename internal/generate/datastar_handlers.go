package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"strconv"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

// datastarMethodHandlerFunc selects the Datastar handler shape for a route from
// its declared render-callback arguments: any elements argument streams a
// text/event-stream response (with inline signal frames); a lone signal returns
// application/json; a lone script returns text/javascript.
func datastarMethodHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sigs map[string]*types.Signature, receiver *types.Named, sig *types.Signature, receiverInterfaceName string) (*ast.FuncLit, error) {
	methodName := def.FunctionIdentifier().Name
	if _, _, hasExecute := executeArg(def.CallExpression(), sig); hasExecute {
		return nil, fmt.Errorf("call %s cannot use both a Datastar render callback and the %q argument", methodName, muxt.TemplateNameScopeIdentifierExecute)
	}
	if def.HasResponseWriterArg() {
		return nil, fmt.Errorf("call %s cannot use both a Datastar render callback and the %q argument", methodName, muxt.TemplateNameScopeIdentifierHTTPResponse)
	}
	switch {
	case def.UsesElements():
		return datastarStreamHandlerFunc(file, config, def, sigs, receiver, sig, receiverInterfaceName)
	case def.UsesSignal() && def.UsesScript():
		return nil, fmt.Errorf("call %s cannot combine the signal and script arguments without a streaming (elements) response", methodName)
	case def.UsesSignal():
		return datastarSignalHandlerFunc(file, config, def, sigs, receiver, sig)
	case def.UsesScript():
		return datastarScriptHandlerFunc(file, config, def, sigs, receiver, sig, receiverInterfaceName)
	default:
		return nil, fmt.Errorf("call %s has no Datastar render callback", methodName)
	}
}

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

// datastarStreamHandlerFunc builds the streaming handler for a Datastar route
// that uses an elements render callback. Each elements argument renders a
// same-named template into a DatastarEventTemplateData (patch-elements) frame;
// each signal argument emits an inline datastar-patch-signals frame.
func datastarStreamHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sigs map[string]*types.Signature, receiver *types.Named, sig *types.Signature, receiverInterfaceName string) (*ast.FuncLit, error) {
	methodName := def.FunctionIdentifier().Name
	return streamMethodHandlerFunc(file, config, def, sigs, receiver, sig, "datastar", "datastar handler returned an error",
		func(i int, id *ast.Ident, cb *types.Signature) (ast.Expr, bool, error) {
			switch {
			case muxt.IsElementsArgument(id.Name):
				resultType, hasArg, err := validateSSECallbackShape(methodName, cb)
				if err != nil {
					return nil, false, err
				}
				templateName := def.Name()
				if id.Name != muxt.TemplateNameScopeIdentifierElements {
					templateName = id.Name
					if def.Template() == nil || def.Template().Lookup(templateName) == nil {
						return nil, false, fmt.Errorf("no template %q for elements argument %s", templateName, id.Name)
					}
				}
				closure, err := sseClosure(file, config, def, templateName, resultType, hasArg, receiverInterfaceName, streamFlusherIdent, streamMutexIdent)
				return closure, true, err
			case muxt.IsSignalArgument(id.Name):
				resultType, err := validateSignalCallbackShape(methodName, cb)
				if err != nil {
					return nil, false, err
				}
				closure, err := signalEventClosure(file, resultType)
				return closure, true, err
			case muxt.IsScriptArgument(id.Name):
				return nil, false, fmt.Errorf("call %s cannot combine the script argument with a streaming (elements) response", methodName)
			default:
				return nil, false, nil
			}
		})
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

// datastarResponseHandlerFunc builds a non-streaming Datastar handler: it parses
// arguments, sets the response Content-Type, then invokes the receiver method
// with a single-shot render callback produced by buildClosure.
func datastarResponseHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sigs map[string]*types.Signature, receiver *types.Named, sig *types.Signature, contentType, callbackLabel, callbackErrorLogMessage string, buildClosure func(i int, id *ast.Ident, cb *types.Signature) (ast.Expr, bool, error)) (*ast.FuncLit, error) {
	response := muxt.TemplateNameScopeIdentifierHTTPResponse
	request := muxt.TemplateNameScopeIdentifierHTTPRequest

	methodReturnsErr, err := validateStreamMethodResults(def.FunctionIdentifier().Name, sig, callbackLabel)
	if err != nil {
		return nil, err
	}

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
	body := []ast.Stmt{
		// defer func() { _ = request.Body.Close() }()
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

	// response.Header().Set("Content-Type", contentType)
	body = append(body, &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(response), Sel: ast.NewIdent("Header")}},
			Sel: ast.NewIdent("Set"),
		},
		Args: []ast.Expr{astgen.String("Content-Type"), astgen.String(contentType)},
	}})

	callArgs := append([]ast.Expr(nil), def.CallExpression().Args...)
	for i, a := range def.CallExpression().Args {
		id, ok := a.(*ast.Ident)
		if !ok {
			continue
		}
		var cb *types.Signature
		if i < sig.Params().Len() {
			cb, _ = sig.Params().At(i).Type().Underlying().(*types.Signature)
		}
		replacement, matched, err := buildClosure(i, id, cb)
		if err != nil {
			return nil, err
		}
		if matched {
			callArgs[i] = replacement
		}
	}
	callExpr := &ast.CallExpr{Fun: callFun, Args: callArgs}

	if methodReturnsErr {
		body = append(body, &ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{callExpr}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: executeTemplateFailedLogLine(file, callbackErrorLogMessage, errIdent)}}},
		})
	} else {
		body = append(body, &ast.ExprStmt{X: callExpr})
	}

	handlerFunc.Body.List = body
	return handlerFunc, nil
}

// datastarSignalHandlerFunc builds the non-streaming handler for a Datastar
// route whose only render callback is a signal: it responds application/json
// with the marshaled result, which the Datastar frontend reads as a
// patch-signals update. onlyIfMissing has no representation in a plain JSON body
// and is therefore ignored here; use a streaming (elements) route to emit
// onlyIfMissing.
func datastarSignalHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sigs map[string]*types.Signature, receiver *types.Named, sig *types.Signature) (*ast.FuncLit, error) {
	methodName := def.FunctionIdentifier().Name
	return datastarResponseHandlerFunc(file, config, def, sigs, receiver, sig, "application/json", "signal", "datastar signal handler returned an error",
		func(i int, id *ast.Ident, cb *types.Signature) (ast.Expr, bool, error) {
			if !muxt.IsSignalArgument(id.Name) {
				return nil, false, nil
			}
			resultType, err := validateSignalCallbackShape(methodName, cb)
			if err != nil {
				return nil, false, err
			}
			closure, err := signalResponseClosure(file, resultType)
			return closure, true, err
		})
}

// signalResponseClosure builds the callback for a lone signal argument: it
// marshals the result as JSON and writes it as the (already content-typed)
// response body. onlyIfMissing is accepted to satisfy the callback signature but
// is not representable in a JSON body, so it is ignored.
func signalResponseClosure(file *File, resultType types.Type) (*ast.FuncLit, error) {
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

	body := astgen.GetBufferFromPool(file, bufferPoolIdent, bufIdent)
	body = append(body,
		// _ = onlyIfMissing
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ast.NewIdent(onlyIfMissingIdent)}},
		// if err := datastarMarshalSignals(buf, result); err != nil { return err }
		&ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
				&ast.CallExpr{Fun: ast.NewIdent(datastarMarshalSignalsFuncName), Args: []ast.Expr{ast.NewIdent(bufIdent), ast.NewIdent(resultIdent)}},
			}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}}}},
		},
		// _, err := response.Write(buf.Bytes()); return err
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
			&ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: ast.NewIdent(response), Sel: ast.NewIdent("Write")},
				Args: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Bytes")}}},
			},
		}},
		&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}},
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

// datastarScriptHandlerFunc builds the non-streaming handler for a Datastar
// route whose only render callback is a script: it responds text/javascript with
// the rendered same-named template.
func datastarScriptHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sigs map[string]*types.Signature, receiver *types.Named, sig *types.Signature, receiverInterfaceName string) (*ast.FuncLit, error) {
	methodName := def.FunctionIdentifier().Name
	return datastarResponseHandlerFunc(file, config, def, sigs, receiver, sig, "text/javascript", "script", "datastar script handler returned an error",
		func(i int, id *ast.Ident, cb *types.Signature) (ast.Expr, bool, error) {
			if !muxt.IsScriptArgument(id.Name) {
				return nil, false, nil
			}
			resultType, hasArg, err := validateSSECallbackShape(methodName, cb)
			if err != nil {
				return nil, false, err
			}
			templateName := def.Name()
			if id.Name != muxt.TemplateNameScopeIdentifierScript {
				templateName = id.Name
				if def.Template() == nil || def.Template().Lookup(templateName) == nil {
					return nil, false, fmt.Errorf("no template %q for script argument %s", templateName, id.Name)
				}
			}
			closure, err := scriptResponseClosure(file, config, def, templateName, resultType, hasArg, receiverInterfaceName)
			return closure, true, err
		})
}

// scriptResponseClosure builds the callback for a script argument: it renders
// the same-named template into a pooled buffer using the route's template data
// type, then writes it as the (already content-typed) response body.
func scriptResponseClosure(file *File, config RoutesFileConfiguration, def muxt.Definition, templateName string, resultType types.Type, hasArg bool, receiverInterfaceName string) (*ast.FuncLit, error) {
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

	var params []*ast.Field
	tdElts := []ast.Expr{
		&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierReceiver), Value: ast.NewIdent(receiverIdent)},
		&ast.KeyValueExpr{Key: ast.NewIdent(response), Value: ast.NewIdent(response)},
		&ast.KeyValueExpr{Key: ast.NewIdent(request), Value: ast.NewIdent(request)},
		&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: ast.NewIdent(pathPrefixPathsStructFieldName)},
	}
	if hasArg {
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(resultIdent)}, Type: resultTypeExpr})
		tdElts = append(tdElts, &ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierResult), Value: ast.NewIdent(resultIdent)})
	}

	body := astgen.GetBufferFromPool(file, bufferPoolIdent, bufIdent)
	body = append(body,
		// td := DatastarTemplateData[Recv, T]{...}
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(tdIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CompositeLit{
			Type: &ast.IndexListExpr{X: ast.NewIdent(config.TemplateDataType), Indices: []ast.Expr{ast.NewIdent(receiverInterfaceName), resultTypeExpr}},
			Elts: tdElts,
		}}},
		// if err := templates.ExecuteTemplate(buf, name, &td); err != nil { slog...; return err }
		&ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: ast.NewIdent(def.TemplatesVariable()), Sel: ast.NewIdent("ExecuteTemplate")},
				Args: []ast.Expr{ast.NewIdent(bufIdent), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(templateName)}, &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(tdIdent)}},
			}}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ExprStmt{X: executeTemplateFailedLogLine(file, executeTemplateErrorMessage, errIdent)},
				&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}},
			}},
		},
		// _, err := response.Write(buf.Bytes()); return err
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
			&ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: ast.NewIdent(response), Sel: ast.NewIdent("Write")},
				Args: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Bytes")}}},
			},
		}},
		&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}},
	)

	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: params},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}, nil
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
