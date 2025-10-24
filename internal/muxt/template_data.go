package muxt

import (
	"go/ast"
	"go/token"

	"github.com/typelate/muxt/internal/astgen"
)

const (
	templateDataReceiverName = "data"
)

func templateDataType(file *File, templateTypeIdent string, receiverType ast.Expr) *ast.GenDecl {
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
							{Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)}, Type: astgen.HTTPResponseWriter(file)},
							{Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest)}, Type: astgen.HTTPRequestPtr(file)},
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

func templateDataMethodReceiver(templateDataTypeIdent string) *ast.FieldList {
	return &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(templateDataReceiverName)}, Type: &ast.StarExpr{X: &ast.IndexListExpr{
		X:       ast.NewIdent(templateDataTypeIdent),
		Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
	}}}}}
}

func templateDataOkay(templateDataTypeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Ok"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("bool")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("okay")}},
				},
			},
		},
	}
}

func templateDataError(file *File, templateDataTypeIdent string) *ast.FuncDecl {
	join := astgen.ErrorsJoin(file, &ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent(TemplateDataFieldIdentifierError)})
	join.Ellipsis = 1
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Err"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{join},
				},
			},
		},
	}
}

func templateDataReceiver(receiverType ast.Expr, templateDataTypeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Receiver"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("R")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("receiver")}},
				},
			},
		},
	}
}

func templateRedirect(file *File, config RoutesFileConfiguration) *ast.FuncDecl {
	const (
		codeParamIdent = "code"
		urlParamIdent  = "url"
	)
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(config.TemplateDataType),
		Name: ast.NewIdent("Redirect"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(urlParamIdent)}, Type: ast.NewIdent("string")},
				{Names: []*ast.Ident{ast.NewIdent(codeParamIdent)}, Type: ast.NewIdent("int")},
			}},
			Results: astgen.ResultsWithErr(&ast.StarExpr{X: &ast.IndexListExpr{X: ast.NewIdent(config.TemplateDataType), Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")}}}),
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{
						X: &ast.BinaryExpr{
							X:  ast.NewIdent(codeParamIdent),
							Op: token.LSS,
							Y:  astgen.Int(300),
						},
						Op: token.LOR,
						Y: &ast.BinaryExpr{
							X:  ast.NewIdent(codeParamIdent),
							Op: token.GEQ,
							Y:  astgen.Int(400),
						},
					},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.ReturnStmt{Results: []ast.Expr{
								ast.NewIdent(templateDataReceiverName),
								astgen.Call(file, "", "fmt", "Errorf", astgen.String("invalid status code %d for redirect"), ast.NewIdent("code")),
							}},
						},
					},
				},
				&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent(TemplateDataFieldIdentifierRedirectURL)}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{ast.NewIdent("url")},
				},
				&ast.ReturnStmt{Results: []ast.Expr{
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent(templateDataReceiverName),
							Sel: ast.NewIdent("StatusCode"),
						},
						Args: []ast.Expr{ast.NewIdent(codeParamIdent)},
					},
					astgen.Nil(),
				}},
			},
		},
	}
}

func templateDataMuxtVersionMethod(config RoutesFileConfiguration) *ast.FuncDecl {
	const versionIdent = "muxtVersion"
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(config.TemplateDataType),
		Name: ast.NewIdent("MuxtVersion"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("")}, Type: ast.NewIdent("string")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.DeclStmt{
					Decl: &ast.GenDecl{
						Tok: token.CONST,
						Specs: []ast.Spec{
							&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(versionIdent)}, Values: []ast.Expr{astgen.String(config.MuxtVersion)}},
						},
					},
				},
				&ast.ReturnStmt{
					Results: []ast.Expr{ast.NewIdent(versionIdent)},
				},
			},
		},
	}
}

func templateDataPathMethod(config RoutesFileConfiguration) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(config.TemplateDataType),
		Name: ast.NewIdent("Path"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(config.TemplateRoutePathsTypeName)}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.CompositeLit{Type: ast.NewIdent(config.TemplateRoutePathsTypeName), Elts: []ast.Expr{
						&ast.KeyValueExpr{
							Key:   ast.NewIdent(pathPrefixPathsStructFieldName),
							Value: &ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent(pathPrefixPathsStructFieldName)},
						},
					}}},
				},
			},
		},
	}
}

func templateDataResultMethod(templateDataTypeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Result"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("T")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("result")}},
				},
			},
		},
	}
}

func templateDataRequestMethod(file *File, templateDataTypeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Request"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: astgen.HTTPRequestPtr(file)}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("request")}},
				},
			},
		},
	}
}

func templateDataStatusCodeMethod(templateDataTypeIdent string) *ast.FuncDecl {
	const (
		scIdent = "statusCode"
	)
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("StatusCode"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent(scIdent)},
				Type:  ast.NewIdent("int"),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: &ast.StarExpr{X: &ast.IndexListExpr{
					X:       ast.NewIdent(templateDataTypeIdent),
					Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
				}},
			}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent(scIdent)}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{ast.NewIdent(scIdent)},
				},
				&ast.ReturnStmt{
					Results: []ast.Expr{ast.NewIdent(templateDataReceiverName)},
				},
			},
		},
	}
}

func templateDataHeaderMethod(templateDataTypeIdent string) *ast.FuncDecl {
	const (
		this       = "data"
		keyIdent   = "key"
		valueIdent = "value"
	)
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Header"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent("key"), ast.NewIdent("value")},
				Type:  ast.NewIdent("string"),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: &ast.StarExpr{X: &ast.IndexListExpr{
					X:       ast.NewIdent(templateDataTypeIdent),
					Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
				}},
			}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ExprStmt{X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   &ast.SelectorExpr{X: ast.NewIdent(this), Sel: ast.NewIdent("response")},
								Sel: ast.NewIdent("Header"),
							},
						},
						Sel: ast.NewIdent("Set"),
					},
					Args: []ast.Expr{ast.NewIdent(keyIdent), ast.NewIdent(valueIdent)},
				}},
				&ast.ReturnStmt{
					Results: []ast.Expr{ast.NewIdent(this)},
				},
			},
		},
	}
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
				Fun: &ast.SelectorExpr{
					X: &ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
							Sel: ast.NewIdent("Header"),
						},
					},
					Sel: ast.NewIdent("Get"),
				},
				Args: []ast.Expr{astgen.String(ctHeader)},
			}},
		},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(ctIdent), Op: token.EQL, Y: astgen.String("")},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("Header")}, Args: []ast.Expr{}}, Sel: ast.NewIdent("Set")},
				Args: []ast.Expr{astgen.String(ctHeader), astgen.String("text/html; charset=utf-8")},
			}}},
		},
	}
}
