package generate

import (
	"go/ast"
	"go/token"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

const (
	datastarEventFieldSelector          = "selector"
	datastarEventFieldMode              = "mode"
	datastarEventFieldNamespace         = "namespace"
	datastarEventFieldUseViewTransition = "useViewTransition"

	datastarPatchElementsEvent = "event: datastar-patch-elements\n"
	datastarElementsLinePrefix = "data: elements "
	datastarDefaultPatchMode   = "outer"
)

// datastarEventTemplateDataDecls returns the DatastarEventTemplateData type
// declaration and all of its methods. It is emitted in Datastar mode when at
// least one route uses an elements render callback. The type mirrors
// SSETemplateData but renders a datastar-patch-elements SSE frame: methods set
// the selector/mode/namespace/useViewTransition metadata and WriteTo serializes
// the buffered template output as `data: elements` lines. typeIdent names the
// generated type: the old --use-datastar arg path passes config.SSETemplateDataType;
// the datastar(sse(...)) framing path passes config.DatastarEventTemplateDataType.
func datastarEventTemplateDataDecls(file *File, config RoutesFileConfiguration, typeIdent string) []ast.Decl {
	return []ast.Decl{
		datastarEventTemplateDataType(file, typeIdent),
		sseTemplateDataStringMethod(typeIdent),
		sseTemplateDataReceiverMethod(typeIdent),
		sseTemplateDataRequestMethod(file, typeIdent),
		sseTemplateDataResultMethod(typeIdent),
		sseTemplateDataErrMethod(file, typeIdent),
		sseTemplateDataPointerSetterMethod(typeIdent, "Selector", datastarEventFieldSelector, "string", datastarEventFieldSelector),
		sseTemplateDataPointerSetterMethod(typeIdent, "Mode", datastarEventFieldMode, "string", datastarEventFieldMode),
		sseTemplateDataPointerSetterMethod(typeIdent, "Namespace", datastarEventFieldNamespace, "string", datastarEventFieldNamespace),
		sseTemplateDataPointerSetterMethod(typeIdent, "UseViewTransition", datastarEventFieldUseViewTransition, "bool", datastarEventFieldUseViewTransition),
		sseTemplateDataPathMethod(config, typeIdent),
		datastarEventTemplateDataWriteToMethod(file, typeIdent),
	}
}

func datastarEventTemplateDataType(file *File, typeIdent string) *ast.GenDecl {
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
						{Names: []*ast.Ident{ast.NewIdent(datastarEventFieldSelector), ast.NewIdent(datastarEventFieldMode), ast.NewIdent(datastarEventFieldNamespace)}, Type: ptrString},
						{Names: []*ast.Ident{ast.NewIdent(datastarEventFieldUseViewTransition)}, Type: &ast.StarExpr{X: ast.NewIdent("bool")}},
						{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierError)}, Type: &ast.ArrayType{Elt: ast.NewIdent("error")}},
						{Names: []*ast.Ident{ast.NewIdent(sseTemplateDataFieldData)}, Type: &ast.StarExpr{X: astgen.ExportedIdentifier(file, "", "bytes", "Buffer")}},
					}},
				},
			},
		},
	}
}

// datastarEventTemplateDataWriteToMethod builds the WriteTo method that
// serializes a datastar-patch-elements frame: the event line, the
// selector/mode/namespace/useViewTransition metadata lines (omitting defaults),
// then each buffered template output line as a `data: elements` line.
func datastarEventTemplateDataWriteToMethod(file *File, typeIdent string) *ast.FuncDecl {
	const (
		writerIdent  = "w"
		countIdent   = "bytesWritten"
		nIdent       = "n"
		dataVarIdent = "data"
		lineIdent    = "line"
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
	// metadataLine emits a "data: <token> <value>\n" frame line when the pointer
	// field is non-nil:
	//
	//	if m.<field> != nil {
	//		<write "data: <token> ">; <write *m.<field>>; <write '\n'>
	//	}
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
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(countIdent)}, Type: ast.NewIdent("int")}}}},
		writeAndCount(ioWriteString(astgen.String(datastarPatchElementsEvent))),
		metadataLine(datastarEventFieldSelector, "data: selector "),
		// mode is written only when set and not the default "outer".
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.BinaryExpr{X: mSel(datastarEventFieldMode), Op: token.NEQ, Y: astgen.Nil()},
				Op: token.LAND,
				Y:  &ast.BinaryExpr{X: deref(datastarEventFieldMode), Op: token.NEQ, Y: astgen.String(datastarDefaultPatchMode)},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				writeAndCount(ioWriteString(astgen.String("data: mode "))),
				writeAndCount(ioWriteString(deref(datastarEventFieldMode))),
				writeAndCount(wWrite(newline())),
			}},
		},
		metadataLine(datastarEventFieldNamespace, "data: namespace "),
		// useViewTransition is written only when set to true (default false).
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.BinaryExpr{X: mSel(datastarEventFieldUseViewTransition), Op: token.NEQ, Y: astgen.Nil()},
				Op: token.LAND,
				Y:  deref(datastarEventFieldUseViewTransition),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				writeAndCount(ioWriteString(astgen.String("data: useViewTransition true\n"))),
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
		// for line := range bytes.SplitSeq(data, []byte{'\n'}) { data: elements <line>\n }
		&ast.RangeStmt{
			Key: ast.NewIdent(lineIdent),
			Tok: token.DEFINE,
			X:   astgen.Call(file, "", "bytes", "SplitSeq", ast.NewIdent(dataVarIdent), newline()),
			Body: &ast.BlockStmt{List: []ast.Stmt{
				writeAndCount(ioWriteString(astgen.String(datastarElementsLinePrefix))),
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
