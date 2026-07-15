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

func executeHTMLTemplateHandler(file *File, config RoutesFileConfiguration, def muxt.Definition, sig *types.Signature, resultDataIdent string, receiverInterfaceName string, bufIdent string, statusCodeIdent string) (*ast.FuncLit, error) {
	var callFun ast.Expr
	isMethodCall := sig.Recv() != nil
	if isMethodCall {
		callFun = &ast.SelectorExpr{
			X:   ast.NewIdent(receiverIdent),
			Sel: ast.NewIdent(def.FunctionIdentifier().Name),
		}
	} else {
		callFun = ast.NewIdent(def.FunctionIdentifier().Name)
	}

	execIdx, hasExecute := -1, false
	var resultType types.Type
	var execHasArg bool
	for i, arg := range def.Arguments {
		if arg.Type == muxt.ArgumentTypeExecute && arg.Identifier == muxt.TemplateNameScopeIdentifierExecute {
			// The callback contract (func() error or func(T) error) is
			// validated by muxt.ResolveCall, which records T and whether the
			// callback takes the data argument.
			execIdx, hasExecute = i, true
			resultType, execHasArg = arg.CallbackResultType(), arg.CallbackHasArg()
			break
		}
	}
	if !hasExecute {
		resultType = sig.Results().At(0).Type()
	}
	typeExpr, err := file.TypeASTExpression(resultType)
	if err != nil {
		return nil, err
	}

	handlerFunc := &ast.FuncLit{
		Type: astgen.HTTPHandlerFuncType(file, muxt.TemplateNameScopeIdentifierHTTPResponse, muxt.TemplateNameScopeIdentifierHTTPRequest),
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.DeclStmt{
					Decl: &ast.GenDecl{
						Tok: token.VAR,
						Specs: []ast.Spec{&ast.ValueSpec{
							Names: []*ast.Ident{ast.NewIdent(resultDataIdent)},
							Values: []ast.Expr{&ast.CompositeLit{Type: &ast.IndexListExpr{
								X:       ast.NewIdent(config.TemplateDataType),
								Indices: []ast.Expr{ast.NewIdent(receiverInterfaceName), typeExpr},
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

	if handlerFunc.Body.List, err = appendParseArgumentStatements(handlerFunc.Body.List, def, file, resultType, sig, def.Arguments, nil, resultDataIdent, config, def.CallExpression(), func(s string) *ast.BlockStmt {
		errBlock := appendTemplateDataError(file, resultDataIdent, astgen.ErrorsNew(file, astgen.String(s)))
		errBlock.List = append(errBlock.List, assignTemplateDataErrStatusCode(file, resultDataIdent, http.StatusBadRequest))
		return errBlock
	}, nil); err != nil {
		return nil, err
	}

	handlerFunc.Body.List = append(handlerFunc.Body.List, astgen.GetBufferFromPool(file, bufferPoolIdent, bufIdent)...)

	if hasExecute {
		const guardIdent = "executed"
		closure, err := executeClosure(file, def, resultDataIdent, bufIdent, guardIdent, resultType, execHasArg)
		if err != nil {
			return nil, err
		}
		// The render callback may be invoked more than once (possibly from
		// another goroutine); guard with an atomic.Bool so it renders at most
		// once (see executeClosure).
		handlerFunc.Body.List = append(handlerFunc.Body.List, &ast.DeclStmt{Decl: &ast.GenDecl{
			Tok:   token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(guardIdent)}, Type: astgen.ExportedIdentifier(file, "", "sync/atomic", "Bool")}},
		}})
		callArgs := slices.Clone(def.CallExpression().Args)
		callArgs[execIdx] = closure
		if config.Logger {
			handlerFunc.Body.List = append(handlerFunc.Body.List, logDebugStatement(file, "handling request", def.RawPattern()))
		}
		renderCheck := checkExecuteTemplateError(file, config.Logger, def.RawPattern())
		renderCheck.Init = &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(errIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: callFun, Args: callArgs}},
		}
		setOkay := &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierOkay)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{astgen.Bool(true)},
		}
		handlerFunc.Body.List = append(handlerFunc.Body.List, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  astgen.CallBuiltinLen(&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierError)}),
				Op: token.EQL,
				Y:  astgen.Int(0),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{renderCheck, setOkay}},
		})
	} else {
		errBody := appendTemplateDataError(file, resultDataIdent, ast.NewIdent(errIdent))
		errBody.List = append(errBody.List, assignTemplateDataErrStatusCode(file, resultDataIdent, http.StatusInternalServerError))
		receiverCall, err := callReceiverMethod(resultDataIdent, &ast.SelectorExpr{
			X:   ast.NewIdent(resultDataIdent),
			Sel: ast.NewIdent(TemplateDataFieldIdentifierResult),
		}, sig, def.FunctionIdentifier().Name, &ast.CallExpr{
			Fun:  callFun,
			Args: slices.Clone(def.CallExpression().Args),
		}, errBody)
		if err != nil {
			return nil, err
		}
		handlerFunc.Body.List = append(handlerFunc.Body.List, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: astgen.CallBuiltinLen(&ast.SelectorExpr{
					X:   ast.NewIdent(resultDataIdent),
					Sel: ast.NewIdent(TemplateDataFieldIdentifierError),
				}),
				Op: token.EQL,
				Y:  astgen.Int(0),
			},
			Body: &ast.BlockStmt{
				List: receiverCall.Stmts(),
			},
		})

		callExecuteTemplate(file, config, def, handlerFunc, bufIdent, resultDataIdent)
	}

	if !def.HasResponseWriterArg() {
		handlerFunc.Body.List = append(handlerFunc.Body.List, writeStatusAndHeaders(file, def, resultType, def.DefaultStatusCode(), statusCodeIdent, bufIdent, resultDataIdent, func() ast.Expr {
			return &ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierResult)}
		})...)
	} else {
		handlerFunc.Body.List = append(handlerFunc.Body.List, callWriteOnResponse(bufIdent))
	}
	return handlerFunc, nil
}

func callExecuteTemplate(file *File, config RoutesFileConfiguration, def muxt.Definition, handlerFunc *ast.FuncLit, bufIdent string, dataIdent string) {
	if config.Logger {
		handlerFunc.Body.List = append(handlerFunc.Body.List, logDebugStatement(file, "handling request", def.RawPattern()))
	}

	execTemplates := checkExecuteTemplateError(file, config.Logger, def.RawPattern())
	execTemplates.Init = &ast.AssignStmt{
		Lhs: []ast.Expr{
			ast.NewIdent(errIdent),
		},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(def.TemplatesVariable()), Sel: ast.NewIdent("ExecuteTemplate")},
			Args: []ast.Expr{ast.NewIdent(bufIdent), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(def.Name())}, &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(dataIdent)}},
		}},
	}

	handlerFunc.Body.List = append(handlerFunc.Body.List, execTemplates)
}

// executeClosure builds:
//
//	func(data T) error {
//		if !executed.CompareAndSwap(false, true) {
//			return errors.New("execute callback called more than once")
//		}
//		td.result = data
//		return templates.ExecuteTemplate(buf, name, &td)
//	}
//
// For the zero-arg form it omits the parameter and the td.result assignment. The
// guard renders at most once: ExecuteTemplate mutates the shared template data
// (status code, response headers), so a method that invokes the callback more
// than once gets an error on the later calls rather than a second render. The
// guard is an atomic.Bool compared-and-swapped so a callback invoked from
// another goroutine still renders exactly once.
func executeClosure(file *File, def muxt.Definition, tdIdent, bufIdent, guardIdent string, resultType types.Type, hasArg bool) (*ast.FuncLit, error) {
	const dataIdent = "data"
	var params []*ast.Field
	body := []ast.Stmt{
		&ast.IfStmt{
			Cond: &ast.UnaryExpr{Op: token.NOT, X: &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: ast.NewIdent(guardIdent), Sel: ast.NewIdent("CompareAndSwap")},
				Args: []ast.Expr{astgen.Bool(false), astgen.Bool(true)},
			}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
				astgen.ErrorsNew(file, astgen.String("execute callback called more than once")),
			}}}},
		},
	}
	if hasArg {
		tExpr, err := file.TypeASTExpression(resultType)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{Names: []*ast.Ident{ast.NewIdent(dataIdent)}, Type: tExpr})
		body = append(body, &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(tdIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierResult)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{ast.NewIdent(dataIdent)},
		})
	}
	body = append(body, &ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(def.TemplatesVariable()), Sel: ast.NewIdent("ExecuteTemplate")},
		Args: []ast.Expr{ast.NewIdent(bufIdent), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(def.Name())}, &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(tdIdent)}},
	}}})
	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: params},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}, nil
}
