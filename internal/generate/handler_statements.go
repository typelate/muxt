package generate

import (
	"fmt"
	"go/ast"
	"go/token"
	"net/http"
	"strings"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

// bytesBufferPoolDeclaration returns the statement declaring the per-function
// sync.Pool of *bytes.Buffer that handler closures draw their render buffers
// from: bytesBufferPool := sync.Pool{New: func() any { return bytes.NewBuffer(nil) }}.
func bytesBufferPoolDeclaration(file *File) ast.Stmt {
	return &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{ast.NewIdent(bufferPoolIdent)},
		Rhs: []ast.Expr{astgen.SyncPoolBytesBuffer(file)},
	}
}

// middlewareNilGuard emits the statement that makes a nil middleware argument
// a no-op:
//
//	if middleware == nil {
//		middleware = func(next http.Handler) http.Handler { return next }
//	}
func middlewareNilGuard(file *File) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(middlewareParamName), Op: token.EQL, Y: astgen.Nil()},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(middlewareParamName)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.FuncLit{
					Type: astgen.HTTPMiddlewareFuncType(file),
					Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("next")}}}},
				}},
			},
		}},
	}
}

func callHandleFunc(file *File, def muxt.Definition, handlerFuncLit *ast.FuncLit, config RoutesFileConfiguration) *ast.ExprStmt {
	normalized := def.Pattern()
	pattern := ast.Expr(astgen.String(normalized))
	if config.PathPrefix {
		i := strings.Index(normalized, "/")
		pattern = &ast.BinaryExpr{
			X:  astgen.String(normalized[:i]),
			Op: token.ADD,
			Y:  astgen.Call(file, "path", "path", "Join", ast.NewIdent(pathPrefixPathsStructFieldName), astgen.String(normalized[i:])),
		}
	}
	method, handler := httpHandleFuncIdent, ast.Expr(handlerFuncLit)
	if config.Middleware {
		method = httpHandleIdent
		handler = &ast.CallExpr{
			Fun: ast.NewIdent(middlewareParamName),
			Args: []ast.Expr{&ast.CallExpr{
				Fun:  astgen.ExportedIdentifier(file, "http", "net/http", "HandlerFunc"),
				Args: []ast.Expr{handlerFuncLit},
			}},
		}
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(muxVarIdent),
			Sel: ast.NewIdent(method),
		},
		Args: []ast.Expr{pattern, handler},
	}}
}

func noReceiverMethodCall(file *File, def muxt.Definition, config RoutesFileConfiguration, receiverInterfaceName string) *ast.FuncLit {
	const (
		bufIdent             = "buf"
		statusCodeIdent      = "statusCode"
		templateDataVarIdent = "td"
	)
	handlerFunc := &ast.FuncLit{
		Type: astgen.HTTPHandlerFuncType(file, muxt.TemplateNameScopeIdentifierHTTPResponse, muxt.TemplateNameScopeIdentifierHTTPRequest),
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.DeclStmt{
					Decl: &ast.GenDecl{
						Tok: token.VAR,
						Specs: []ast.Spec{&ast.ValueSpec{
							Names: []*ast.Ident{ast.NewIdent(templateDataVarIdent)},
							Values: []ast.Expr{&ast.CompositeLit{Type: &ast.IndexListExpr{
								X:       ast.NewIdent(config.TemplateDataType),
								Indices: []ast.Expr{ast.NewIdent(receiverInterfaceName), astgen.EmptyStructType()},
							}, Elts: []ast.Expr{
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierReceiver), Value: ast.NewIdent(TemplateDataFieldIdentifierReceiver)},
								&ast.KeyValueExpr{Key: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), Value: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse)},
								&ast.KeyValueExpr{Key: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Value: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest)},
								&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: ast.NewIdent(pathPrefixPathsStructFieldName)},
							}}},
						}},
					},
				},
			},
		},
	}

	handlerFunc.Body.List = append(handlerFunc.Body.List, astgen.GetBufferFromPool(file, bufferPoolIdent, bufIdent)...)

	callExecuteTemplate(file, config, def, handlerFunc, bufIdent, templateDataVarIdent)

	handlerFunc.Body.List = append(handlerFunc.Body.List, writeStatusAndHeaders(file, def, def.DefaultStatusCode(), statusCodeIdent, bufIdent, templateDataVarIdent, func() ast.Expr {
		panic("when no receiver method is called, then the result variable should not be needed")
	})...)
	return handlerFunc
}

func callHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, receiverInterfaceName string) (*ast.FuncLit, error) {
	const (
		bufIdent        = "buf"
		statusCodeIdent = "statusCode"
		resultDataIdent = "td"
	)

	if def.Signature().IsZero() {
		return nil, fmt.Errorf("call for pattern %s was not resolved", def.Pattern())
	}
	switch def.Representation {
	case muxt.RepresentationSSE:
		return sseMethodHandlerFunc(file, config, def, receiverInterfaceName)
	case muxt.RepresentationMarshalJSON:
		return marshalJSONHandlerFunc(file, config, def, resultDataIdent, receiverInterfaceName, bufIdent, statusCodeIdent)
	default:
		return newHTMLTemplateHandler(file, config, def, resultDataIdent, receiverInterfaceName, bufIdent, statusCodeIdent)
	}
}

func appendTemplateDataError(_ *File, tdIdent string, err ast.Expr) *ast.BlockStmt {
	return &ast.BlockStmt{
		List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(tdIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierError)}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{astgen.CallBuiltinAppend(&ast.SelectorExpr{X: ast.NewIdent(tdIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierError)}, err)},
			},
		},
	}
}

func writeBodyAndWriteHeadersFunc(file *File, bufIdent, statusCodeIdent string) []ast.Stmt {
	return []ast.Stmt{
		setContentTypeHeaderSetOnTemplateData(),
		&ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("Header")}, Args: []ast.Expr{}}, Sel: ast.NewIdent("Set")},
			Args: []ast.Expr{astgen.String("content-length"), astgen.StrconvItoaCall(file, &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Len")}, Args: []ast.Expr{}})},
		}},
		callWriteHeader(ast.NewIdent(statusCodeIdent)),
		callWriteOnResponse(bufIdent),
	}
}

func callWriteHeader(statusCode ast.Expr) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("WriteHeader")},
		Args: []ast.Expr{statusCode},
	}}
}

func checkExecuteTemplateError(file *File, withLogger bool, pattern string) *ast.IfStmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				logErrorStatement(file, withLogger, executeTemplateErrorMessage, pattern),
				&ast.ExprStmt{X: astgen.HTTPErrorCall(file, ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), astgen.String(executeTemplateErrorMessage), http.StatusInternalServerError)},
				&ast.ReturnStmt{},
			},
		},
	}
}

// logErrorStatement logs message and err: with the logger the routes
// function is passed when withLogger is set, otherwise with log/slog's
// default logger.
func logErrorStatement(file *File, withLogger bool, message, pattern string) ast.Stmt {
	if withLogger {
		return &ast.ExprStmt{X: loggerErrorCall(file, message, pattern, errIdent)}
	}
	return &ast.ExprStmt{X: executeTemplateFailedLogLine(file, message, errIdent)}
}

func callWriteOnResponse(bufferIdent string) *ast.AssignStmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("_")},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(bufferIdent),
				Sel: ast.NewIdent("WriteTo"),
			},
			Args: []ast.Expr{ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse)},
		}},
	}
}

func writeStatusAndHeaders(file *File, def muxt.Definition, fallbackStatusCode int, statusCode, bufIdent, resultDataIdent string, resultVar func() ast.Expr) []ast.Stmt {
	statusCodePriorityList := []ast.Expr{
		&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(templateDataFieldStatusCode)},
		&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierErrStatusCode)},
	}
	var list []ast.Stmt
	// The result offers a status code only when the call succeeded: after
	// an error it may be a nil pointer whose StatusCode panics.
	var resultStatusCode ast.Expr
	switch def.ResultStatusCode() {
	case muxt.ResultStatusCodeMethod:
		// The result is a field of the template data variable, so it is
		// addressable and a pointer receiver method is called on it as
		// well as a value receiver one.
		resultStatusCode = &ast.CallExpr{Fun: &ast.SelectorExpr{X: resultVar(), Sel: ast.NewIdent("StatusCode")}}
	case muxt.ResultStatusCodeField:
		resultStatusCode = &ast.SelectorExpr{X: resultVar(), Sel: ast.NewIdent("StatusCode")}
	}
	if resultStatusCode != nil {
		list = append(list, resultStatusCodeStatements(resultDataIdent, resultStatusCode)...)
		statusCodePriorityList = append(statusCodePriorityList, ast.NewIdent(resultStatusCodeIdent))
	}
	if fallbackStatusCode == http.StatusOK {
		const defaultStatusIdent = "defaultStatusCode"
		list = append(list,
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(defaultStatusIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{astgen.HTTPStatusCode(file, http.StatusOK)},
			},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X:  &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Len")}},
					Op: token.EQL,
					Y:  astgen.Int(0),
				},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{ast.NewIdent(defaultStatusIdent)},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{astgen.HTTPStatusCode(file, http.StatusNoContent)},
					},
				}},
			},
		)
		statusCodePriorityList = append(statusCodePriorityList, ast.NewIdent(defaultStatusIdent))
	} else {
		statusCodePriorityList = append(statusCodePriorityList, astgen.HTTPStatusCode(file, fallbackStatusCode))
	}
	list = append(list, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(statusCode)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{astgen.CmpOr(file, statusCodePriorityList...)},
	})

	if def.MayRedirect() {
		list = append(list, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent(resultDataIdent),
					Sel: ast.NewIdent(TemplateDataFieldIdentifierRedirectURL),
				},
				Op: token.NEQ,
				Y:  astgen.String(""),
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{
						X: astgen.Call(file, "", "net/http", "Redirect",
							ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse),
							ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
							&ast.SelectorExpr{
								X:   ast.NewIdent(resultDataIdent),
								Sel: ast.NewIdent(TemplateDataFieldIdentifierRedirectURL),
							},
							ast.NewIdent(statusCode),
						),
					},
					&ast.ReturnStmt{},
				},
			},
		})
	}

	return append(list, writeBodyAndWriteHeadersFunc(file, bufIdent, statusCode)...)
}

// resultStatusCodeStatements declares the status code the result offers,
// read only when the template data has no errors:
//
//	var resultStatusCode int
//	if len(td.errList) == 0 {
//		resultStatusCode = td.result.StatusCode()
//	}
func resultStatusCodeStatements(resultDataIdent string, statusCode ast.Expr) []ast.Stmt {
	return []ast.Stmt{
		varDecl(resultStatusCodeIdent, ast.NewIdent("int"), nil),
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.CallExpr{Fun: ast.NewIdent("len"), Args: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierError)}}},
				Op: token.EQL,
				Y:  astgen.Int(0),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(resultStatusCodeIdent)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{statusCode},
			}}},
		},
	}
}

func executeTemplateFailedLogLine(file *File, message, errIdent string) *ast.CallExpr {
	args := []ast.Expr{
		&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Context")}},
		astgen.String(message),

		astgen.SlogString(file, "path", &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("URL")},
			Sel: ast.NewIdent("Path"),
		}),
		astgen.SlogString(file, "pattern", &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Pattern")}),
		astgen.SlogString(file, "error", astgen.CallError(errIdent)),
	}
	return astgen.Call(file, "", "log/slog", "ErrorContext", args...)
}

func loggerErrorCall(file *File, message, pattern, errIdent string) *ast.CallExpr {
	args := []ast.Expr{
		&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Context")}},
		astgen.String(message),
		astgen.SlogString(file, "pattern", astgen.String(pattern)),
		astgen.SlogString(file, "path", &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("URL")},
			Sel: ast.NewIdent("Path"),
		}),
		astgen.SlogString(file, "error", astgen.CallError(errIdent)),
	}
	return &ast.CallExpr{
		Fun:  selector(loggerIdent, "ErrorContext"),
		Args: args,
	}
}

func logDebugStatement(file *File, message, pattern string) *ast.ExprStmt {
	args := []ast.Expr{
		&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Context")}},
		astgen.String(message),
		astgen.SlogString(file, "pattern", astgen.String(pattern)),
		astgen.SlogString(file, "path", &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("URL")},
			Sel: ast.NewIdent("Path"),
		}),
		astgen.SlogString(file, "method", &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Method")}),
	}
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun:  selector(loggerIdent, "DebugContext"),
			Args: args,
		},
	}
}

func assignTemplateDataErrStatusCode(file *File, rdIdent string, code int) *ast.AssignStmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.SelectorExpr{
			X:   ast.NewIdent(rdIdent),
			Sel: ast.NewIdent(TemplateDataFieldIdentifierErrStatusCode),
		}},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{
			astgen.HTTPStatusCode(file, code),
		},
	}
}
