package generate

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

// sseCallbackSignature returns the func(any) error type synthesized for an sse
// argument when the receiver method is not already defined.
func sseCallbackSignature() *types.Signature {
	anyType := types.Universe.Lookup("any").Type()
	errType := types.Universe.Lookup("error").Type()
	return types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewVar(0, nil, "", anyType)),
		types.NewTuple(types.NewVar(0, nil, "", errType)),
		false)
}

const (
	sseTemplateDataReceiverName = "m"

	sseTemplateDataFieldEvent = "event"
	sseTemplateDataFieldID    = "id"
	sseTemplateDataFieldRetry = "retryMilliseconds"
	sseTemplateDataFieldData  = "data"
)

// sseTemplateDataDecls returns the SSETemplateData type declaration and all of
// its methods. It is emitted only when at least one route uses the sse
// argument. The generated type mirrors TemplateData but renders Server-Sent
// Event frames: methods set the id/event/retry metadata and WriteTo serializes
// the buffered template output as one or more `data:` lines.
func sseTemplateDataDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	typeIdent := config.SSETemplateDataType
	return []ast.Decl{
		sseTemplateDataType(file, typeIdent),
		sseTemplateDataStringMethod(typeIdent),
		sseTemplateDataReceiverMethod(typeIdent),
		sseTemplateDataRequestMethod(file, typeIdent),
		sseTemplateDataResultMethod(typeIdent),
		sseTemplateDataErrMethod(file, typeIdent),
		sseTemplateDataEventMethod(typeIdent),
		sseTemplateDataIDMethod(typeIdent),
		sseTemplateDataRetryMethod(typeIdent),
		sseTemplateDataPathMethod(config),
		sseTemplateDataWriteToMethod(file, typeIdent),
	}
}

func sseTemplateDataTypeParams() *ast.FieldList {
	return &ast.FieldList{List: []*ast.Field{
		{Names: []*ast.Ident{ast.NewIdent("R"), ast.NewIdent("T")}, Type: ast.NewIdent("any")},
	}}
}

func sseTemplateDataMethodReceiver(typeIdent string) *ast.FieldList {
	return &ast.FieldList{List: []*ast.Field{{
		Names: []*ast.Ident{ast.NewIdent(sseTemplateDataReceiverName)},
		Type: &ast.StarExpr{X: &ast.IndexListExpr{
			X:       ast.NewIdent(typeIdent),
			Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
		}},
	}}}
}

// sseTemplateDataSelfType returns the *SSETemplateData[R, T] expression used as
// the return type of the chainable setter methods.
func sseTemplateDataSelfType(typeIdent string) ast.Expr {
	return &ast.StarExpr{X: &ast.IndexListExpr{
		X:       ast.NewIdent(typeIdent),
		Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
	}}
}

func sseTemplateDataType(file *File, typeIdent string) *ast.GenDecl {
	ptrString := &ast.StarExpr{X: ast.NewIdent("string")}
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name:       ast.NewIdent(typeIdent),
				TypeParams: sseTemplateDataTypeParams(),
				Type: &ast.StructType{
					Fields: &ast.FieldList{List: []*ast.Field{
						{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierReceiver)}, Type: ast.NewIdent("R")},
						{Names: []*ast.Ident{ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest)}, Type: astgen.HTTPRequestPtr(file)},
						{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierResult)}, Type: ast.NewIdent("T")},
						{Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string")},
						{Names: []*ast.Ident{ast.NewIdent(sseTemplateDataFieldEvent), ast.NewIdent(sseTemplateDataFieldID)}, Type: ptrString},
						{Names: []*ast.Ident{ast.NewIdent(sseTemplateDataFieldRetry)}, Type: &ast.StarExpr{X: ast.NewIdent("int")}},
						{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierError)}, Type: &ast.ArrayType{Elt: ast.NewIdent("error")}},
						{Names: []*ast.Ident{ast.NewIdent(sseTemplateDataFieldData)}, Type: &ast.StarExpr{X: astgen.ExportedIdentifier(file, "", "bytes", "Buffer")}},
					}},
				},
			},
		},
	}
}

func sseTemplateDataStringMethod(typeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent("String"),
		Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{astgen.String("")}}}},
	}
}

func sseTemplateDataReceiverMethod(typeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent("Receiver"),
		Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("R")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			&ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(TemplateDataFieldIdentifierReceiver)},
		}}}},
	}
}

func sseTemplateDataRequestMethod(file *File, typeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent("Request"),
		Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: astgen.HTTPRequestPtr(file)}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			&ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest)},
		}}}},
	}
}

func sseTemplateDataResultMethod(typeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent("Result"),
		Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("T")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			&ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(TemplateDataFieldIdentifierResult)},
		}}}},
	}
}

func sseTemplateDataErrMethod(file *File, typeIdent string) *ast.FuncDecl {
	join := astgen.ErrorsJoin(file, &ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(TemplateDataFieldIdentifierError)})
	join.Ellipsis = 1
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent("Err"),
		Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{join}}}},
	}
}

// sseTemplateDataPointerSetterMethod builds a chainable setter of the form
//
//	func (m *SSETemplateData[R, T]) Name(param paramType) *SSETemplateData[R, T] {
//		m.field = &param
//		return m
//	}
func sseTemplateDataPointerSetterMethod(typeIdent, methodName, paramName, paramType, field string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent(methodName),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(paramName)}, Type: ast.NewIdent(paramType)}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sseTemplateDataSelfType(typeIdent)}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(field)}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(paramName)}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(sseTemplateDataReceiverName)}},
		}},
	}
}

func sseTemplateDataEventMethod(typeIdent string) *ast.FuncDecl {
	return sseTemplateDataPointerSetterMethod(typeIdent, "Event", "event", "string", sseTemplateDataFieldEvent)
}

func sseTemplateDataIDMethod(typeIdent string) *ast.FuncDecl {
	// ID always takes a string; callers convert their id type to a string.
	return sseTemplateDataPointerSetterMethod(typeIdent, "ID", "id", "string", sseTemplateDataFieldID)
}

func sseTemplateDataRetryMethod(typeIdent string) *ast.FuncDecl {
	return sseTemplateDataPointerSetterMethod(typeIdent, "Retry", "retryMilliseconds", "int", sseTemplateDataFieldRetry)
}

func sseTemplateDataPathMethod(config RoutesFileConfiguration) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(config.SSETemplateDataType),
		Name: ast.NewIdent("Path"),
		Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent(config.TemplateRoutePathsTypeName)}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			&ast.CompositeLit{Type: ast.NewIdent(config.TemplateRoutePathsTypeName), Elts: []ast.Expr{
				&ast.KeyValueExpr{
					Key:   ast.NewIdent(pathPrefixPathsStructFieldName),
					Value: &ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(pathPrefixPathsStructFieldName)},
				},
			}},
		}}}},
	}
}

// sseTemplateDataWriteToMethod builds the WriteTo method that serializes the
// event metadata and buffered template output into the SSE wire format.
func sseTemplateDataWriteToMethod(file *File, typeIdent string) *ast.FuncDecl {
	const (
		writerIdent  = "w"
		countIdent   = "bytesWritten"
		nIdent       = "n"
		dataVarIdent = "data"
		lineIdent    = "line"
		retryBuf     = "retryBuf"
	)
	mSel := func(field string) ast.Expr {
		return &ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(field)}
	}
	deref := func(field string) ast.Expr { return &ast.StarExpr{X: mSel(field)} }
	byteSlice := func(elts ...ast.Expr) ast.Expr {
		return &ast.CompositeLit{Type: &ast.ArrayType{Elt: ast.NewIdent("byte")}, Elts: elts}
	}
	newline := func() ast.Expr {
		return byteSlice(&ast.BasicLit{Kind: token.CHAR, Value: `'\n'`})
	}
	byteConv := func(s string) ast.Expr {
		return &ast.CallExpr{Fun: &ast.ArrayType{Elt: ast.NewIdent("byte")}, Args: []ast.Expr{astgen.String(s)}}
	}
	ioWriteString := func(x ast.Expr) ast.Expr {
		return astgen.Call(file, "", "io", "WriteString", ast.NewIdent(writerIdent), x)
	}
	wWrite := func(x ast.Expr) ast.Expr {
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(writerIdent), Sel: ast.NewIdent("Write")}, Args: []ast.Expr{x}}
	}
	// writeAndCount emits:
	//
	//	if n, err := <call>; err != nil {
	//		return int64(bytesWritten + n), err
	//	} else {
	//		bytesWritten += n
	//	}
	writeAndCount := func(call ast.Expr) ast.Stmt {
		return &ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(nIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
				&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{&ast.BinaryExpr{X: ast.NewIdent(countIdent), Op: token.ADD, Y: ast.NewIdent(nIdent)}}},
				ast.NewIdent(errIdent),
			}}}},
			Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(countIdent)}, Tok: token.ADD_ASSIGN, Rhs: []ast.Expr{ast.NewIdent(nIdent)}}}},
		}
	}
	forbidden := func(field, forbiddenChars, message string) ast.Stmt {
		return &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.BinaryExpr{X: mSel(field), Op: token.NEQ, Y: astgen.Nil()},
				Op: token.LAND,
				Y:  astgen.Call(file, "", "strings", "ContainsAny", deref(field), astgen.String(forbiddenChars)),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
				astgen.Int(0), astgen.Call(file, "", "errors", "New", astgen.String(message)),
			}}}},
		}
	}
	metadataLine := func(field, prefix string) ast.Stmt {
		return &ast.IfStmt{
			Cond: &ast.BinaryExpr{X: mSel(field), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				writeAndCount(ioWriteString(astgen.String(prefix))),
				writeAndCount(ioWriteString(deref(field))),
				writeAndCount(wWrite(newline())),
			}},
		}
	}

	body := []ast.Stmt{
		forbidden(sseTemplateDataFieldID, "\r\n\x00", "sse: id contains a forbidden character"),
		forbidden(sseTemplateDataFieldEvent, "\r\n", "sse: event contains a forbidden character"),
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(countIdent)}, Type: ast.NewIdent("int")}}}},
		metadataLine(sseTemplateDataFieldID, "id: "),
		metadataLine(sseTemplateDataFieldEvent, "event: "),
		// retry block
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: mSel(sseTemplateDataFieldRetry), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				writeAndCount(ioWriteString(astgen.String("retry: "))),
				&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(retryBuf)},
					Type:  &ast.ArrayType{Len: astgen.Int(20), Elt: ast.NewIdent("byte")},
				}}}},
				writeAndCount(wWrite(astgen.Call(file, "", "strconv", "AppendInt",
					&ast.SliceExpr{X: ast.NewIdent(retryBuf), High: astgen.Int(0)},
					&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{deref(sseTemplateDataFieldRetry)}},
					astgen.Int(10),
				))),
				writeAndCount(wWrite(newline())),
			}},
		},
		// data := m.data.Bytes()
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
			&ast.CallExpr{Fun: &ast.SelectorExpr{X: mSel(sseTemplateDataFieldData), Sel: ast.NewIdent("Bytes")}},
		}},
		// if bytes.IndexByte(data, '\r') >= 0 { normalize CRLF -> LF }
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  astgen.Call(file, "", "bytes", "IndexByte", ast.NewIdent(dataVarIdent), &ast.BasicLit{Kind: token.CHAR, Value: `'\r'`}),
				Op: token.GEQ,
				Y:  astgen.Int(0),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{
					astgen.Call(file, "", "bytes", "ReplaceAll", ast.NewIdent(dataVarIdent), byteConv("\r\n"), byteConv("\n")),
				}},
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{
					astgen.Call(file, "", "bytes", "ReplaceAll", ast.NewIdent(dataVarIdent), byteConv("\r"), byteConv("\n")),
				}},
			}},
		},
		// data = bytes.TrimSuffix(data, []byte{'\n'})
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{
			astgen.Call(file, "", "bytes", "TrimSuffix", ast.NewIdent(dataVarIdent), newline()),
		}},
		// for line := range bytes.SplitSeq(data, []byte{'\n'}) { ... }
		&ast.RangeStmt{
			Key: ast.NewIdent(lineIdent),
			Tok: token.DEFINE,
			X:   astgen.Call(file, "", "bytes", "SplitSeq", ast.NewIdent(dataVarIdent), newline()),
			Body: &ast.BlockStmt{List: []ast.Stmt{
				writeAndCount(ioWriteString(astgen.String("data: "))),
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{X: astgen.CallBuiltinLen(ast.NewIdent(lineIdent)), Op: token.GTR, Y: astgen.Int(0)},
					Body: &ast.BlockStmt{List: []ast.Stmt{writeAndCount(wWrite(ast.NewIdent(lineIdent)))}},
				},
				writeAndCount(wWrite(newline())),
			}},
		},
		writeAndCount(wWrite(newline())),
		&ast.ReturnStmt{Results: []ast.Expr{
			&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{ast.NewIdent(countIdent)}},
			astgen.Nil(),
		}},
	}

	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent("WriteTo"),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(writerIdent)}, Type: astgen.ExportedIdentifier(file, "", "io", "Writer")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("int64")}, {Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
}
