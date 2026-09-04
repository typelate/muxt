package generate

import (
	"go/ast"
	"go/token"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

const (
	sseTemplateDataReceiverName = "m"

	sseTemplateDataFieldEvent = "event"
	sseTemplateDataFieldID    = "id"
	sseTemplateDataFieldRetry = "retryMilliseconds"
	sseTemplateDataFieldData  = "data"

	sseTemplateDataFieldSelector          = "selector"
	sseTemplateDataFieldMode              = "mode"
	sseTemplateDataFieldUseViewTransition = "useViewTransition"

	datastarPatchElementsEvent = "datastar-patch-elements"
)

// sseTemplateDataDecls returns the SSETemplateData type declaration and all of
// its methods. It is emitted only when at least one route uses the sse
// argument. The generated type mirrors TemplateData but renders Server-Sent
// Event frames: methods set the id/event/retry metadata and WriteTo serializes
// the buffered template output as one or more `data:` lines.
func sseTemplateDataDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	if config.OutputDatastar {
		return datastarSSETemplateDataDecls(file, config)
	}
	typeIdent := config.SSETemplateDataType
	decls := []ast.Decl{
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
	}
	if !config.WireTypelateSSE {
		// Under --wire-typelate-sse the closure maps the setter fields to
		// MessageOption values and sse.Response writes the frames, so the
		// generated WriteTo wire writer is not emitted.
		decls = append(decls, sseTemplateDataWriteToMethod(file, typeIdent))
	}
	return decls
}

// datastarSSETemplateDataDecls emits SSETemplateData for a --output-datastar
// package: the shared surface plus the patch option setters, with WriteTo
// framing every event as datastar-patch-elements. The event name is fixed by
// the protocol, so there is no Event setter.
func datastarSSETemplateDataDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	typeIdent := config.SSETemplateDataType
	return []ast.Decl{
		datastarSSETemplateDataType(file, typeIdent),
		sseTemplateDataStringMethod(typeIdent),
		sseTemplateDataReceiverMethod(typeIdent),
		sseTemplateDataRequestMethod(file, typeIdent),
		sseTemplateDataResultMethod(typeIdent),
		sseTemplateDataErrMethod(file, typeIdent),
		sseTemplateDataIDMethod(typeIdent),
		sseTemplateDataRetryMethod(typeIdent),
		sseTemplateDataPathMethod(config),
		sseTemplateDataPointerSetterMethod(typeIdent, "Selector", "selector", "string", sseTemplateDataFieldSelector),
		sseTemplateDataPointerSetterMethod(typeIdent, "Mode", "mode", "string", sseTemplateDataFieldMode),
		sseTemplateDataBoolSetterMethod(typeIdent, "UseViewTransition", sseTemplateDataFieldUseViewTransition),
		datastarWriteToMethod(file, typeIdent),
	}
}

func datastarSSETemplateDataType(file *File, typeIdent string) *ast.GenDecl {
	decl := sseTemplateDataType(file, typeIdent)
	st := decl.Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	st.Fields.List = append(st.Fields.List,
		&ast.Field{
			Names: []*ast.Ident{ast.NewIdent(sseTemplateDataFieldSelector), ast.NewIdent(sseTemplateDataFieldMode)},
			Type:  &ast.StarExpr{X: ast.NewIdent("string")},
		},
		&ast.Field{
			Names: []*ast.Ident{ast.NewIdent(sseTemplateDataFieldUseViewTransition)},
			Type:  ast.NewIdent("bool"),
		},
	)
	return decl
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

// sseTemplateDataBoolSetterMethod builds a chainable setter of the form
//
//	func (m *SSETemplateData[R, T]) Name(value bool) *SSETemplateData[R, T] {
//		m.field = value
//		return m
//	}
func sseTemplateDataBoolSetterMethod(typeIdent, methodName, field string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent(methodName),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("value")}, Type: ast.NewIdent("bool")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sseTemplateDataSelfType(typeIdent)}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(field)}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{ast.NewIdent("value")},
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

// Idents used inside the generated WriteTo method bodies.
const (
	sseWriterIdent   = "w"
	sseCountIdent    = "bytesWritten"
	sseNIdent        = "n"
	sseDataVarIdent  = "data"
	sseLineIdent     = "line"
	sseRetryBufIdent = "retryBuf"
)

func sseField(field string) ast.Expr {
	return &ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(field)}
}

func sseFieldDeref(field string) ast.Expr { return &ast.StarExpr{X: sseField(field)} }

func sseByteSlice(elts ...ast.Expr) ast.Expr {
	return &ast.CompositeLit{Type: &ast.ArrayType{Elt: ast.NewIdent("byte")}, Elts: elts}
}

func sseNewline() ast.Expr {
	return sseByteSlice(&ast.BasicLit{Kind: token.CHAR, Value: `'\n'`})
}

func sseByteConv(s string) ast.Expr {
	return &ast.CallExpr{Fun: &ast.ArrayType{Elt: ast.NewIdent("byte")}, Args: []ast.Expr{astgen.String(s)}}
}

func sseWriteString(file *File, x ast.Expr) ast.Expr {
	return astgen.Call(file, "", "io", "WriteString", ast.NewIdent(sseWriterIdent), x)
}

func sseWrite(x ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(sseWriterIdent), Sel: ast.NewIdent("Write")}, Args: []ast.Expr{x}}
}

// sseWriteAndCount emits:
//
//	if n, err := <call>; err != nil {
//		return int64(bytesWritten + n), err
//	} else {
//		bytesWritten += n
//	}
func sseWriteAndCount(call ast.Expr) ast.Stmt {
	return &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(sseNIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{&ast.BinaryExpr{X: ast.NewIdent(sseCountIdent), Op: token.ADD, Y: ast.NewIdent(sseNIdent)}}},
			ast.NewIdent(errIdent),
		}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(sseCountIdent)}, Tok: token.ADD_ASSIGN, Rhs: []ast.Expr{ast.NewIdent(sseNIdent)}}}},
	}
}

func sseForbiddenCharCheck(file *File, field, forbiddenChars, message string) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  &ast.BinaryExpr{X: sseField(field), Op: token.NEQ, Y: astgen.Nil()},
			Op: token.LAND,
			Y:  astgen.Call(file, "", "strings", "ContainsAny", sseFieldDeref(field), astgen.String(forbiddenChars)),
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			astgen.Int(0), astgen.Call(file, "", "errors", "New", astgen.String(message)),
		}}}},
	}
}

func sseMetadataLine(file *File, field, prefix string) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: sseField(field), Op: token.NEQ, Y: astgen.Nil()},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			sseWriteAndCount(sseWriteString(file, astgen.String(prefix))),
			sseWriteAndCount(sseWriteString(file, sseFieldDeref(field))),
			sseWriteAndCount(sseWrite(sseNewline())),
		}},
	}
}

func sseRetryLine(file *File) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: sseField(sseTemplateDataFieldRetry), Op: token.NEQ, Y: astgen.Nil()},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			sseWriteAndCount(sseWriteString(file, astgen.String("retry: "))),
			&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
				Names: []*ast.Ident{ast.NewIdent(sseRetryBufIdent)},
				Type:  &ast.ArrayType{Len: astgen.Int(20), Elt: ast.NewIdent("byte")},
			}}}},
			sseWriteAndCount(sseWrite(astgen.Call(file, "", "strconv", "AppendInt",
				&ast.SliceExpr{X: ast.NewIdent(sseRetryBufIdent), High: astgen.Int(0)},
				&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{sseFieldDeref(sseTemplateDataFieldRetry)}},
				astgen.Int(10),
			))),
			sseWriteAndCount(sseWrite(sseNewline())),
		}},
	}
}

func sseDeclareCount() ast.Stmt {
	return &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(sseCountIdent)}, Type: ast.NewIdent("int")}}}}
}

func sseReturnCount() ast.Stmt {
	return &ast.ReturnStmt{Results: []ast.Expr{
		&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{ast.NewIdent(sseCountIdent)}},
		astgen.Nil(),
	}}
}

// sseDataLineLoop normalizes the buffered template output (CRLF to LF, no
// trailing newline) and emits rangeBody once per line.
func sseDataLineLoop(file *File, rangeBody ...ast.Stmt) []ast.Stmt {
	return []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(sseDataVarIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
			&ast.CallExpr{Fun: &ast.SelectorExpr{X: sseField(sseTemplateDataFieldData), Sel: ast.NewIdent("Bytes")}},
		}},
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  astgen.Call(file, "", "bytes", "IndexByte", ast.NewIdent(sseDataVarIdent), &ast.BasicLit{Kind: token.CHAR, Value: `'\r'`}),
				Op: token.GEQ,
				Y:  astgen.Int(0),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(sseDataVarIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{
					astgen.Call(file, "", "bytes", "ReplaceAll", ast.NewIdent(sseDataVarIdent), sseByteConv("\r\n"), sseByteConv("\n")),
				}},
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(sseDataVarIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{
					astgen.Call(file, "", "bytes", "ReplaceAll", ast.NewIdent(sseDataVarIdent), sseByteConv("\r"), sseByteConv("\n")),
				}},
			}},
		},
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(sseDataVarIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{
			astgen.Call(file, "", "bytes", "TrimSuffix", ast.NewIdent(sseDataVarIdent), sseNewline()),
		}},
		&ast.RangeStmt{
			Key:  ast.NewIdent(sseLineIdent),
			Tok:  token.DEFINE,
			X:    astgen.Call(file, "", "bytes", "SplitSeq", ast.NewIdent(sseDataVarIdent), sseNewline()),
			Body: &ast.BlockStmt{List: rangeBody},
		},
	}
}

func sseWriteToFuncDecl(file *File, typeIdent string, body []ast.Stmt) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent("WriteTo"),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(sseWriterIdent)}, Type: astgen.ExportedIdentifier(file, "", "io", "Writer")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("int64")}, {Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
}

// sseTemplateDataWriteToMethod builds the WriteTo method that serializes the
// event metadata and buffered template output into the SSE wire format.
func sseTemplateDataWriteToMethod(file *File, typeIdent string) *ast.FuncDecl {
	body := []ast.Stmt{
		sseForbiddenCharCheck(file, sseTemplateDataFieldID, "\r\n\x00", "sse: id contains a forbidden character"),
		sseForbiddenCharCheck(file, sseTemplateDataFieldEvent, "\r\n", "sse: event contains a forbidden character"),
		sseDeclareCount(),
		sseMetadataLine(file, sseTemplateDataFieldID, "id: "),
		sseMetadataLine(file, sseTemplateDataFieldEvent, "event: "),
		sseRetryLine(file),
	}
	body = append(body, sseDataLineLoop(file,
		sseWriteAndCount(sseWriteString(file, astgen.String("data: "))),
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: astgen.CallBuiltinLen(ast.NewIdent(sseLineIdent)), Op: token.GTR, Y: astgen.Int(0)},
			Body: &ast.BlockStmt{List: []ast.Stmt{sseWriteAndCount(sseWrite(ast.NewIdent(sseLineIdent)))}},
		},
		sseWriteAndCount(sseWrite(sseNewline())),
	)...)
	body = append(body,
		sseWriteAndCount(sseWrite(sseNewline())),
		sseReturnCount(),
	)
	return sseWriteToFuncDecl(file, typeIdent, body)
}

func sseBoolLine(file *File, field, line string) ast.Stmt {
	return &ast.IfStmt{
		Cond: sseField(field),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			sseWriteAndCount(sseWriteString(file, astgen.String(line))),
		}},
	}
}

// datastarWriteToMethod builds the WriteTo method that frames one event with
// the Datastar patch-elements protocol: a fixed event name, the SSE id and
// retry metadata, the patch option lines, and each rendered line as a
// "data: elements" line.
func datastarWriteToMethod(file *File, typeIdent string) *ast.FuncDecl {
	body := []ast.Stmt{
		sseForbiddenCharCheck(file, sseTemplateDataFieldID, "\r\n\x00", "sse: id contains a forbidden character"),
		sseForbiddenCharCheck(file, sseTemplateDataFieldSelector, "\r\n", "sse: selector contains a forbidden character"),
		sseForbiddenCharCheck(file, sseTemplateDataFieldMode, "\r\n", "sse: mode contains a forbidden character"),
		sseDeclareCount(),
		sseWriteAndCount(sseWriteString(file, astgen.String("event: "+datastarPatchElementsEvent+"\n"))),
		sseMetadataLine(file, sseTemplateDataFieldID, "id: "),
		sseRetryLine(file),
		sseMetadataLine(file, sseTemplateDataFieldSelector, "data: selector "),
		sseMetadataLine(file, sseTemplateDataFieldMode, "data: mode "),
		sseBoolLine(file, sseTemplateDataFieldUseViewTransition, "data: useViewTransition true\n"),
	}
	body = append(body, sseDataLineLoop(file,
		sseWriteAndCount(sseWriteString(file, astgen.String("data: elements "))),
		sseWriteAndCount(sseWrite(ast.NewIdent(sseLineIdent))),
		sseWriteAndCount(sseWrite(sseNewline())),
	)...)
	body = append(body,
		sseWriteAndCount(sseWrite(sseNewline())),
		sseReturnCount(),
	)
	return sseWriteToFuncDecl(file, typeIdent, body)
}
