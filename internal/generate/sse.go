package generate

import (
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"slices"
	"strconv"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

// sseMethodHandlerFunc builds the http.HandlerFunc for a route that streams
// Server-Sent Events. Unlike a normal handler it establishes an event stream
// (Content-Type text/event-stream, flush) and invokes the receiver method with
// a callback closure that renders and writes one SSE frame per call.
func sseMethodHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sig *types.Signature, receiverInterfaceName string) (*ast.FuncLit, error) {
	const (
		flusherIdent = "flusher"
		okIdent      = "ok"
		mutexIdent   = "mut"
		headerIdent  = "h"
	)
	response := muxt.TemplateNameScopeIdentifierHTTPResponse
	request := muxt.TemplateNameScopeIdentifierHTTPRequest

	methodReturnsErr := def.ResultShape() == muxt.ResultShapeError

	functionIdent := def.FunctionIdentifier().Name

	var callFun ast.Expr
	if def.IsMethod() {
		callFun = &ast.SelectorExpr{X: ast.NewIdent(receiverIdent), Sel: ast.NewIdent(functionIdent)}
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
		// flusher, ok := response.(http.Flusher)
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(flusherIdent), ast.NewIdent(okIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.TypeAssertExpr{X: ast.NewIdent(response), Type: astgen.ExportedIdentifier(file, "", "net/http", "Flusher")}},
		},
		// if !ok { http.Error(response, "streaming unsupported", 500); return }
		&ast.IfStmt{
			Cond: &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent(okIdent)},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.ExprStmt{X: astgen.HTTPErrorCall(file, ast.NewIdent(response), astgen.String("streaming unsupported"), http.StatusInternalServerError)},
				&ast.ReturnStmt{},
			}},
		},
	}

	// Parse ctx, lastEventID and any path params into locals. A typed parse
	// failure responds 400 and returns before the stream is established.
	parseErrBlock := func() *ast.BlockStmt {
		return &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: astgen.HTTPErrorCall(file, ast.NewIdent(response), astgen.CallError(errIdent), http.StatusBadRequest)},
			&ast.ReturnStmt{},
		}}
	}
	validationFailureBlock := func(string) *ast.BlockStmt { return parseErrBlock() }
	// The result type is per-callback; arg parsing only needs ctx/lastEventID/path
	// (it ignores the result type), so pass an empty struct here.
	body, err := appendParseArgumentStatements(body, def, file, types.NewStruct(nil, nil), sig, def.Arguments, nil, "", config, def.CallExpression(), validationFailureBlock, parseErrBlock)
	if err != nil {
		return nil, err
	}

	// h := response.Header(); set the SSE headers; WriteHeader(200); flush.
	headerSet := func(key, value string) ast.Stmt {
		return &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(headerIdent), Sel: ast.NewIdent("Set")},
			Args: []ast.Expr{astgen.String(key), astgen.String(value)},
		}}
	}
	body = append(body,
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(headerIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(response), Sel: ast.NewIdent("Header")}}},
		},
		headerSet("Content-Type", "text/event-stream"),
		headerSet("Connection", "keep-alive"),
		headerSet("Cache-Control", "no-cache"),
		&ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(response), Sel: ast.NewIdent("WriteHeader")},
			Args: []ast.Expr{astgen.HTTPStatusCode(file, http.StatusOK)},
		}},
		&ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(flusherIdent), Sel: ast.NewIdent("Flush")}}},
		// var mut sync.Mutex
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
			Names: []*ast.Ident{ast.NewIdent(mutexIdent)},
			Type:  astgen.ExportedIdentifier(file, "", "sync", "Mutex"),
		}}}},
	)

	callArgs := slices.Clone(def.CallExpression().Args)
	for i, arg := range def.Arguments {
		if arg.Type != muxt.ArgumentTypeExecute {
			continue
		}
		// The callback contract (func() error or func(T) error) and template
		// existence are validated by muxt.ResolveCall, which records T and
		// whether the callback takes the data argument.
		resultType, hasArg := arg.CallbackResultType(), arg.CallbackHasArg()
		closure, err := sseClosure(file, config, def, arg.Template().Name(), resultType, hasArg, receiverInterfaceName, flusherIdent, mutexIdent)
		if err != nil {
			return nil, err
		}
		callArgs[i] = closure
	}
	callExpr := &ast.CallExpr{Fun: callFun, Args: callArgs}

	if methodReturnsErr {
		body = append(body, &ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{callExpr}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: executeTemplateFailedLogLine(file, "sse handler returned an error", errIdent)}}},
		})
	} else {
		body = append(body, &ast.ExprStmt{X: callExpr})
	}

	handlerFunc.Body.List = body
	return handlerFunc, nil
}

// sseClosure builds the callback passed to the receiver method. Each call
// acquires a pooled buffer, renders the template into it, then writes one SSE
// frame to the response under a mutex and flushes:
//
//	func(result T) error {
//		if err := request.Context().Err(); err != nil { return err }
//		buf := bytesBufferPool.Get().(*bytes.Buffer)
//		buf.Reset()
//		defer bytesBufferPool.Put(buf)
//		td := SSETemplateData[Recv, T]{receiver: receiver, request: request, pathsPrefix: pathsPrefix, result: result}
//		if err := templates.ExecuteTemplate(buf, name, &td); err != nil { slog...; return err }
//		td.data = buf
//		mut.Lock()
//		defer mut.Unlock()
//		if _, err := td.WriteTo(response); err != nil { return err }
//		flusher.Flush()
//		return nil
//	}
//
// For the zero-arg form it omits the parameter and the result field.
func sseClosure(file *File, config RoutesFileConfiguration, def muxt.Definition, templateName string, resultType types.Type, hasArg bool, receiverInterfaceName, flusherIdent, mutexIdent string) (*ast.FuncLit, error) {
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
		&ast.KeyValueExpr{Key: ast.NewIdent(request), Value: ast.NewIdent(request)},
		&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: ast.NewIdent(pathPrefixPathsStructFieldName)},
	}
	if hasArg {
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(resultIdent)}, Type: resultTypeExpr})
		tdElts = append(tdElts, &ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierResult), Value: ast.NewIdent(resultIdent)})
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
	// if err := templates.ExecuteTemplate(buf, name, &td); err != nil { slog...; return err }
	body = append(body, &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(def.TemplatesVariable()), Sel: ast.NewIdent("ExecuteTemplate")},
			Args: []ast.Expr{ast.NewIdent(bufIdent), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(templateName)}, &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(tdIdent)}},
		}}},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: executeTemplateFailedLogLine(file, executeTemplateErrorMessage, errIdent)},
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}},
		}},
	})
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
