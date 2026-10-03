package generate

import (
	"go/ast"
	"go/token"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

const (
	templateDataReceiverName = "data"

	TemplateDataFieldIdentifierResult        = "result"
	TemplateDataFieldIdentifierOkay          = "okay"
	TemplateDataFieldIdentifierRedirectURL   = "redirectURL"
	TemplateDataFieldIdentifierError         = "errList"
	TemplateDataFieldIdentifierReceiver      = "receiver"
	TemplateDataFieldIdentifierStatusCode    = "statusCode"
	TemplateDataFieldIdentifierErrStatusCode = "errStatusCode"
)

func templateDataType(file *File, templateTypeIdent string) *ast.GenDecl {
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name: ast.NewIdent(templateTypeIdent),
				TypeParams: &ast.FieldList{
					List: []*ast.Field{
						{Names: []*ast.Ident{ast.NewIdent("R")}, Type: ast.NewIdent("any")},
						{Names: []*ast.Ident{ast.NewIdent("T")}, Type: ast.NewIdent("any")},
					},
				},
				Type: &ast.StructType{
					Fields: &ast.FieldList{
						List: []*ast.Field{
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierReceiver)}, Type: ast.NewIdent("R")},
							{Names: []*ast.Ident{ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse)}, Type: astgen.HTTPResponseWriter(file)},
							{Names: []*ast.Ident{ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest)}, Type: astgen.HTTPRequestPtr(file)},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierResult)}, Type: ast.NewIdent("T")},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierStatusCode)}, Type: ast.NewIdent("int")},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierErrStatusCode)}, Type: ast.NewIdent("int")},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierOkay)}, Type: ast.NewIdent("bool")},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierError)}, Type: &ast.ArrayType{Elt: ast.NewIdent("error")}},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierRedirectURL)}, Type: ast.NewIdent("string")},
							{Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string")},
						},
					},
				},
			},
		},
	}
}

// genericMethod declares func (recv *typeIdent[R, T]) name(params) results {
// body }. The template data types share the type parameters R and T.
func genericMethod(recv, typeIdent, name string, params, results []*ast.Field, body ...ast.Stmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(recv)}, Type: genericPointer(typeIdent)}}},
		Name: ast.NewIdent(name),
		Type: &ast.FuncType{Params: fieldList(params), Results: fieldList(results)},
		Body: &ast.BlockStmt{List: body},
	}
}

func genericPointer(typeIdent string) ast.Expr {
	return &ast.StarExpr{X: &ast.IndexListExpr{
		X:       ast.NewIdent(typeIdent),
		Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
	}}
}

func fieldList(fields []*ast.Field) *ast.FieldList {
	if len(fields) == 0 {
		return nil
	}
	return &ast.FieldList{List: fields}
}

func param(typ ast.Expr, names ...string) *ast.Field {
	field := &ast.Field{Type: typ}
	for _, name := range names {
		field.Names = append(field.Names, ast.NewIdent(name))
	}
	return field
}

func results(types ...ast.Expr) []*ast.Field {
	fields := make([]*ast.Field, 0, len(types))
	for _, typ := range types {
		fields = append(fields, param(typ))
	}
	return fields
}

func selector(x, sel string) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: ast.NewIdent(x), Sel: ast.NewIdent(sel)}
}

func returnExprs(exprs ...ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{Results: exprs}
}

func templatePathsLiteral(config RoutesFileConfiguration, recv string) *ast.CompositeLit {
	return pathsLiteral(config, selector(recv, pathPrefixPathsStructFieldName))
}

func dataMethod(typeIdent, name string, params, results []*ast.Field, body ...ast.Stmt) *ast.FuncDecl {
	return genericMethod(templateDataReceiverName, typeIdent, name, params, results, body...)
}

func templateDataOkay(typeIdent string) *ast.FuncDecl {
	return dataMethod(typeIdent, "Ok", nil, results(ast.NewIdent("bool")),
		returnExprs(selector(templateDataReceiverName, "okay")))
}

func templateDataError(file *File, typeIdent string) *ast.FuncDecl {
	join := astgen.ErrorsJoin(file, selector(templateDataReceiverName, TemplateDataFieldIdentifierError))
	join.Ellipsis = 1
	return dataMethod(typeIdent, "Err", nil, results(ast.NewIdent("error")), returnExprs(join))
}

func templateDataReceiver(typeIdent string) *ast.FuncDecl {
	return dataMethod(typeIdent, "Receiver", nil, results(ast.NewIdent("R")),
		returnExprs(selector(templateDataReceiverName, "receiver")))
}

func templateRedirect(file *File, config RoutesFileConfiguration) *ast.FuncDecl {
	const (
		codeParamIdent = "code"
		urlParamIdent  = "url"
	)
	invalidCode := &ast.BinaryExpr{
		X:  &ast.BinaryExpr{X: ast.NewIdent(codeParamIdent), Op: token.LSS, Y: astgen.Int(300)},
		Op: token.LOR,
		Y:  &ast.BinaryExpr{X: ast.NewIdent(codeParamIdent), Op: token.GEQ, Y: astgen.Int(400)},
	}
	return dataMethod(config.TemplateDataType, "Redirect",
		[]*ast.Field{param(ast.NewIdent("string"), urlParamIdent), param(ast.NewIdent("int"), codeParamIdent)},
		results(genericPointer(config.TemplateDataType), ast.NewIdent("error")),
		&ast.IfStmt{
			Cond: invalidCode,
			Body: &ast.BlockStmt{List: []ast.Stmt{returnExprs(
				ast.NewIdent(templateDataReceiverName),
				astgen.Call(file, "", "fmt", "Errorf", astgen.String("invalid status code %d for redirect"), ast.NewIdent("code")),
			)}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{selector(templateDataReceiverName, TemplateDataFieldIdentifierRedirectURL)},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{ast.NewIdent("url")},
		},
		returnExprs(
			&ast.CallExpr{Fun: selector(templateDataReceiverName, "StatusCode"), Args: []ast.Expr{ast.NewIdent(codeParamIdent)}},
			astgen.Nil(),
		),
	)
}

func templateRedirectHelperMethod(file *File, config RoutesFileConfiguration, methodName string, statusCode int) *ast.FuncDecl {
	const urlParamIdent = "url"
	return dataMethod(config.TemplateDataType, methodName,
		[]*ast.Field{param(ast.NewIdent("string"), urlParamIdent)},
		results(genericPointer(config.TemplateDataType), ast.NewIdent("error")),
		returnExprs(&ast.CallExpr{
			Fun:  selector(templateDataReceiverName, "Redirect"),
			Args: []ast.Expr{ast.NewIdent(urlParamIdent), astgen.HTTPStatusCode(file, statusCode)},
		}),
	)
}

func templateRedirectHelperMethods(file *File, config RoutesFileConfiguration) []*ast.FuncDecl {
	return []*ast.FuncDecl{
		templateRedirectHelperMethod(file, config, "RedirectMultipleChoices", 300),
		templateRedirectHelperMethod(file, config, "RedirectMovedPermanently", 301),
		templateRedirectHelperMethod(file, config, "RedirectFound", 302),
		templateRedirectHelperMethod(file, config, "RedirectSeeOther", 303),
	}
}

func templateDataMuxtVersionMethod(config RoutesFileConfiguration) *ast.FuncDecl {
	const versionIdent = "muxtVersion"
	return dataMethod(config.TemplateDataType, "MuxtVersion", nil, results(ast.NewIdent("string")),
		&ast.DeclStmt{Decl: &ast.GenDecl{
			Tok:   token.CONST,
			Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(versionIdent)}, Values: []ast.Expr{astgen.String(config.MuxtVersion)}}},
		}},
		returnExprs(ast.NewIdent(versionIdent)),
	)
}

func templateDataPathMethod(config RoutesFileConfiguration) *ast.FuncDecl {
	return dataMethod(config.TemplateDataType, "Path", nil, results(ast.NewIdent(config.TemplateRoutePathsTypeName)),
		returnExprs(templatePathsLiteral(config, templateDataReceiverName)))
}

func templateDataResultMethod(typeIdent string) *ast.FuncDecl {
	return dataMethod(typeIdent, "Result", nil, results(ast.NewIdent("T")),
		returnExprs(selector(templateDataReceiverName, "result")))
}

func templateDataRequestMethod(file *File, typeIdent string) *ast.FuncDecl {
	return dataMethod(typeIdent, "Request", nil, results(astgen.HTTPRequestPtr(file)),
		returnExprs(selector(templateDataReceiverName, "request")))
}

func templateDataStatusCodeMethod(typeIdent string) *ast.FuncDecl {
	const scIdent = "statusCode"
	return dataMethod(typeIdent, "StatusCode",
		[]*ast.Field{param(ast.NewIdent("int"), scIdent)},
		results(genericPointer(typeIdent)),
		&ast.AssignStmt{
			Lhs: []ast.Expr{selector(templateDataReceiverName, scIdent)},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{ast.NewIdent(scIdent)},
		},
		returnExprs(ast.NewIdent(templateDataReceiverName)),
	)
}

func templateDataHeaderMethod(typeIdent string) *ast.FuncDecl {
	const (
		keyIdent   = "key"
		valueIdent = "value"
	)
	return dataMethod(typeIdent, "Header",
		[]*ast.Field{param(ast.NewIdent("string"), keyIdent, valueIdent)},
		results(genericPointer(typeIdent)),
		&ast.ExprStmt{X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   &ast.CallExpr{Fun: &ast.SelectorExpr{X: selector(templateDataReceiverName, "response"), Sel: ast.NewIdent("Header")}},
				Sel: ast.NewIdent("Set"),
			},
			Args: []ast.Expr{ast.NewIdent(keyIdent), ast.NewIdent(valueIdent)},
		}},
		returnExprs(ast.NewIdent(templateDataReceiverName)),
	)
}

func responseHeader() *ast.CallExpr {
	return &ast.CallExpr{Fun: selector(muxt.TemplateNameScopeIdentifierHTTPResponse, "Header")}
}

func setContentTypeHeaderSetOnTemplateData() *ast.IfStmt {
	const (
		ctIdent  = "contentType"
		ctHeader = "content-type"
	)
	return &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(ctIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: responseHeader(), Sel: ast.NewIdent("Get")},
				Args: []ast.Expr{astgen.String(ctHeader)},
			}},
		},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(ctIdent), Op: token.EQL, Y: astgen.String("")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: responseHeader(), Sel: ast.NewIdent("Set")},
			Args: []ast.Expr{astgen.String(ctHeader), astgen.String("text/html; charset=utf-8")},
		}}}},
	}
}

func dataHeaderCall(name string, value ast.Expr) *ast.CallExpr {
	return &ast.CallExpr{Fun: selector(templateDataReceiverName, "Header"), Args: []ast.Expr{astgen.String(name), value}}
}

func htmxHeaderSetterMethod(typeIdent, methodName, headerName, paramName string) *ast.FuncDecl {
	return dataMethod(typeIdent, methodName,
		[]*ast.Field{param(ast.NewIdent("string"), paramName)},
		results(genericPointer(typeIdent)),
		returnExprs(dataHeaderCall(headerName, ast.NewIdent(paramName))),
	)
}

func htmxRefreshMethod(typeIdent string) *ast.FuncDecl {
	return dataMethod(typeIdent, "HXRefresh", nil, results(genericPointer(typeIdent)),
		returnExprs(dataHeaderCall("HX-Refresh", astgen.String("true"))))
}

// requestHeaderGet is data.Request().Header.Get(name).
func requestHeaderGet(name string) *ast.CallExpr {
	request := &ast.CallExpr{Fun: selector(templateDataReceiverName, "Request")}
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: &ast.SelectorExpr{X: request, Sel: ast.NewIdent("Header")}, Sel: ast.NewIdent("Get")},
		Args: []ast.Expr{astgen.String(name)},
	}
}

func htmxRequestHeaderStringMethod(typeIdent, methodName, headerName string) *ast.FuncDecl {
	return dataMethod(typeIdent, methodName, nil, results(ast.NewIdent("string")),
		returnExprs(requestHeaderGet(headerName)))
}

func htmxRequestHeaderBoolMethod(typeIdent, methodName, headerName string, op token.Token, value string) *ast.FuncDecl {
	return dataMethod(typeIdent, methodName, nil, results(ast.NewIdent("bool")),
		returnExprs(&ast.BinaryExpr{X: requestHeaderGet(headerName), Op: op, Y: astgen.String(value)}))
}

func htmxRequestHeaderBoolNonEmptyMethod(typeIdent, methodName, headerName string) *ast.FuncDecl {
	return htmxRequestHeaderBoolMethod(typeIdent, methodName, headerName, token.NEQ, "")
}

func htmxRequestHeaderBoolTrueMethod(typeIdent, methodName, headerName string) *ast.FuncDecl {
	return htmxRequestHeaderBoolMethod(typeIdent, methodName, headerName, token.EQL, "true")
}

func templateDataStringMethod(typeIdent string) *ast.FuncDecl {
	return dataMethod(typeIdent, "String", nil, results(ast.NewIdent("string")), returnExprs(astgen.String("")))
}

func templateDataHTMXHelperMethods(typeIdent string) []*ast.FuncDecl {
	return []*ast.FuncDecl{
		htmxHeaderSetterMethod(typeIdent, "HXLocation", "HX-Location", "link"),
		htmxHeaderSetterMethod(typeIdent, "HXPushURL", "HX-Push-Url", "link"),
		htmxHeaderSetterMethod(typeIdent, "HXRedirect", "HX-Redirect", "link"),
		htmxRefreshMethod(typeIdent),
		htmxHeaderSetterMethod(typeIdent, "HXReplaceURL", "HX-Replace-Url", "link"),
		htmxHeaderSetterMethod(typeIdent, "HXReswap", "HX-Reswap", "swap"),
		htmxHeaderSetterMethod(typeIdent, "HXRetarget", "HX-Retarget", "target"),
		htmxHeaderSetterMethod(typeIdent, "HXReselect", "HX-Reselect", "selector"),
		htmxHeaderSetterMethod(typeIdent, "HXTrigger", "HX-Trigger", "eventName"),
		htmxHeaderSetterMethod(typeIdent, "HXTriggerAfterSettle", "HX-Trigger-After-Settle", "eventName"),
		htmxHeaderSetterMethod(typeIdent, "HXTriggerAfterSwap", "HX-Trigger-After-Swap", "eventName"),

		htmxRequestHeaderBoolNonEmptyMethod(typeIdent, "HXBoosted", "HX-Boosted"),
		htmxRequestHeaderStringMethod(typeIdent, "HXCurrentURL", "HX-Current-Url"),
		htmxRequestHeaderBoolTrueMethod(typeIdent, "HXHistoryRestoreRequest", "HX-History-Restore-Request"),
		htmxRequestHeaderStringMethod(typeIdent, "HXPrompt", "HX-Prompt"),
		htmxRequestHeaderBoolTrueMethod(typeIdent, "HXRequest", "HX-Request"),
		htmxRequestHeaderStringMethod(typeIdent, "HXTargetElementID", "HX-Target"),
		htmxRequestHeaderStringMethod(typeIdent, "HXTriggerName", "HX-Trigger-Name"),
		htmxRequestHeaderStringMethod(typeIdent, "HXTriggerElementID", "HX-Trigger"),
	}
}

func templateDataDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	decls := []ast.Decl{
		templateDataType(file, config.TemplateDataType),
	}
	if config.OutputMuxtVersion {
		decls = append(decls, templateDataMuxtVersionMethod(config))
	}
	decls = append(decls,
		templateDataPathMethod(config),
		templateDataResultMethod(config.TemplateDataType),
		templateDataRequestMethod(file, config.TemplateDataType),
		templateDataStatusCodeMethod(config.TemplateDataType),
		templateDataHeaderMethod(config.TemplateDataType),
		templateDataOkay(config.TemplateDataType),
		templateDataError(file, config.TemplateDataType),
		templateDataReceiver(config.TemplateDataType),
		templateRedirect(file, config),
	)
	for _, method := range templateRedirectHelperMethods(file, config) {
		decls = append(decls, method)
	}
	decls = append(decls, templateDataStringMethod(config.TemplateDataType))
	if config.OutputHTMX {
		for _, method := range templateDataHTMXHelperMethods(config.TemplateDataType) {
			decls = append(decls, method)
		}
	}
	return decls
}
