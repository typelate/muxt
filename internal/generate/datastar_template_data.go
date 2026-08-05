package generate

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

const (
	datastarEventFieldSelector          = "selector"
	datastarEventFieldMode              = "mode"
	datastarEventFieldNamespace         = "namespace"
	datastarEventFieldUseViewTransition = "useViewTransition"
	datastarEventFieldOnlyIfMissing     = "onlyIfMissing"
	datastarEventFieldIsSignals         = "isSignals"

	datastarPatchElementsEvent = "datastar-patch-elements"
	datastarPatchSignalsEvent  = "datastar-patch-signals"
)

// datastarTemplateDataDecls emits the render template-data type for
// non-streaming datastar routes: the base helper surface under the datastar
// type name (no HX* helpers, no datastar-specific methods).
func datastarTemplateDataDecls(file *File, config RoutesFileConfiguration, receiverInterface ast.Expr) []ast.Decl {
	dsConfig := config
	dsConfig.TemplateDataType = config.DatastarTemplateDataType
	return defaultTemplateDataDecls(file, dsConfig, receiverInterface)
}

// datastarSignalsTemplateDataDecls emits the template data for standalone
// signals responses (datastar(marshalJSON(...)) on a non-streaming route):
// the base helper surface plus a chainable OnlyIfMissing setter. A plain JSON
// body cannot carry onlyIfMissing, so the setter is a documented no-op on
// standalone responses; on streams the send option carries it.
func datastarSignalsTemplateDataDecls(file *File, config RoutesFileConfiguration, receiverInterface ast.Expr) []ast.Decl {
	dsConfig := config
	dsConfig.TemplateDataType = config.DatastarSignalsTemplateDataType
	decls := defaultTemplateDataDecls(file, dsConfig, receiverInterface)
	return append(decls, datastarSignalsOnlyIfMissingMethod(dsConfig.TemplateDataType))
}

// datastarSignalsOnlyIfMissingMethod builds the chainable no-op:
//
//	func (data *DatastarSignalsTemplateData[R, T]) OnlyIfMissing(bool) *DatastarSignalsTemplateData[R, T] {
//		return data
//	}
func datastarSignalsOnlyIfMissingMethod(typeIdent string) *ast.FuncDecl {
	selfType := &ast.StarExpr{X: &ast.IndexListExpr{
		X:       ast.NewIdent(typeIdent),
		Indices: []ast.Expr{ast.NewIdent("R"), ast.NewIdent("T")},
	}}
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent("OnlyIfMissing"),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("bool")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: selfType}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(templateDataReceiverName)}}}},
	}
}

// datastarEventTemplateDataDecls emits the stream-event template data for
// datastar routes. It reuses the generic SSE event builders (the field and
// method builders are parameterized by type name) and adds the Datastar patch
// fields, chainable setters, and the patch-protocol WriteTo marshaler that
// frames each event as datastar-patch-elements or datastar-patch-signals.
func datastarEventTemplateDataDecls(file *File, config RoutesFileConfiguration) []ast.Decl {
	dsConfig := config
	dsConfig.SSETemplateDataType = config.DatastarEventTemplateDataType
	typeIdent := dsConfig.SSETemplateDataType

	typeDecl := sseTemplateDataType(file, dsConfig, typeIdent)
	st := typeDecl.Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	ptrString := &ast.StarExpr{X: ast.NewIdent("string")}
	st.Fields.List = append(st.Fields.List,
		&ast.Field{Names: []*ast.Ident{
			ast.NewIdent(datastarEventFieldSelector),
			ast.NewIdent(datastarEventFieldMode),
			ast.NewIdent(datastarEventFieldNamespace),
		}, Type: ptrString},
		&ast.Field{Names: []*ast.Ident{
			ast.NewIdent(datastarEventFieldUseViewTransition),
			ast.NewIdent(datastarEventFieldOnlyIfMissing),
			ast.NewIdent(datastarEventFieldIsSignals),
		}, Type: ast.NewIdent("bool")},
	)

	return []ast.Decl{
		typeDecl,
		sseTemplateDataStringMethod(typeIdent),
		sseTemplateDataReceiverMethod(typeIdent),
		sseTemplateDataRequestMethod(file, typeIdent),
		sseTemplateDataResultMethod(typeIdent),
		sseTemplateDataErrMethod(file, typeIdent),
		sseTemplateDataIDMethod(typeIdent),
		sseTemplateDataRetryMethod(typeIdent),
		sseTemplateDataPathMethod(dsConfig),
		// The event name is fixed by the patch protocol, so there is no Event
		// setter; these chainable setters each become a wire line.
		sseTemplateDataPointerSetterMethod(typeIdent, "Selector", "selector", "string", datastarEventFieldSelector),
		sseTemplateDataPointerSetterMethod(typeIdent, "Mode", "mode", "string", datastarEventFieldMode),
		sseTemplateDataPointerSetterMethod(typeIdent, "Namespace", "namespace", "string", datastarEventFieldNamespace),
		datastarEventBoolSetterMethod(typeIdent, "UseViewTransition", datastarEventFieldUseViewTransition),
		datastarEventBoolSetterMethod(typeIdent, "OnlyIfMissing", datastarEventFieldOnlyIfMissing),
		datastarEventWriteToMethod(file, typeIdent),
	}
}

// datastarEventBoolSetterMethod builds a chainable bool setter:
//
//	func (m *DatastarEventTemplateData[R, T]) Name(value bool) *DatastarEventTemplateData[R, T] {
//		m.field = value
//		return m
//	}
func datastarEventBoolSetterMethod(typeIdent, methodName, field string) *ast.FuncDecl {
	const paramName = "value"
	return &ast.FuncDecl{
		Recv: sseTemplateDataMethodReceiver(typeIdent),
		Name: ast.NewIdent(methodName),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(paramName)}, Type: ast.NewIdent("bool")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sseTemplateDataSelfType(typeIdent)}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(sseTemplateDataReceiverName), Sel: ast.NewIdent(field)}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{ast.NewIdent(paramName)},
			},
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(sseTemplateDataReceiverName)}},
		}},
	}
}

const signalsValueIdent = "signalsValue"

// signalsBindingStatements emits the statements decoding Datastar's
// client-sent signals into the method parameter's type. Following Datastar's
// convention, GET and DELETE requests carry the signals JSON in the datastar
// query parameter and other methods carry a JSON body; the branch is decided
// statically from the route's method. Absent signals are not an error — the
// value stays zero — while malformed JSON runs the caller's 400 block.
func signalsBindingStatements(file *File, def muxt.Definition, config RoutesFileConfiguration, paramType types.Type, parseErrBlock func() *ast.BlockStmt) ([]ast.Stmt, error) {
	const rawIdent = "signalsRaw"
	typeExpr, err := file.TypeASTExpression(paramType)
	if err != nil {
		return nil, err
	}
	stmts := []ast.Stmt{&ast.DeclStmt{Decl: &ast.GenDecl{
		Tok:   token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(signalsValueIdent)}, Type: typeExpr}},
	}}}
	jsonPkgPath := "encoding/json"
	if config.JSONV2 {
		jsonPkgPath = "encoding/json/v2"
	}
	switch def.HTTPMethod() {
	case "GET", "DELETE":
		// if signalsRaw := request.URL.Query().Get("datastar"); signalsRaw != "" {
		//     if err := json.Unmarshal([]byte(signalsRaw), &signalsValue); err != nil { <400>; return }
		// }
		unmarshal := &ast.CallExpr{
			Fun: astgen.ExportedIdentifier(file, "json", jsonPkgPath, "Unmarshal"),
			Args: []ast.Expr{
				&ast.CallExpr{Fun: &ast.ArrayType{Elt: ast.NewIdent("byte")}, Args: []ast.Expr{ast.NewIdent(rawIdent)}},
				&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(signalsValueIdent)},
			},
		}
		stmts = append(stmts, &ast.IfStmt{
			Init: &ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(rawIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X: &ast.CallExpr{Fun: &ast.SelectorExpr{
							X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("URL")},
							Sel: ast.NewIdent("Query"),
						}},
						Sel: ast.NewIdent("Get"),
					},
					Args: []ast.Expr{astgen.String("datastar")},
				}},
			},
			Cond: &ast.BinaryExpr{X: ast.NewIdent(rawIdent), Op: token.NEQ, Y: astgen.String("")},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.IfStmt{
				Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{unmarshal}},
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: parseErrBlock(),
			}}},
		})
	default:
		// if err := json.NewDecoder(request.Body).Decode(&signalsValue); err != nil && !errors.Is(err, io.EOF) { <400>; return }
		// An empty body is absent signals, not malformed JSON.
		requestBody := &ast.SelectorExpr{
			X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
			Sel: ast.NewIdent("Body"),
		}
		var decode ast.Expr
		if config.JSONV2 {
			decode = &ast.CallExpr{
				Fun:  astgen.ExportedIdentifier(file, "json", "encoding/json/v2", "UnmarshalRead"),
				Args: []ast.Expr{requestBody, &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(signalsValueIdent)}},
			}
		} else {
			decode = &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X: &ast.CallExpr{
						Fun:  astgen.ExportedIdentifier(file, "json", "encoding/json", "NewDecoder"),
						Args: []ast.Expr{requestBody},
					},
					Sel: ast.NewIdent("Decode"),
				},
				Args: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(signalsValueIdent)}},
			}
		}
		stmts = append(stmts, &ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{decode}},
			Cond: &ast.BinaryExpr{
				X:  &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Op: token.LAND,
				Y: &ast.UnaryExpr{Op: token.NOT, X: astgen.Call(file, "", "errors", "Is",
					ast.NewIdent(errIdent), astgen.ExportedIdentifier(file, "", "io", "EOF"))},
			},
			Body: parseErrBlock(),
		})
	}
	return stmts, nil
}

// datastarEventWriteToMethod builds the Datastar patch-protocol event
// marshaler. The wire order matches datastar-go v1: the event line, then id
// and retry, then the option-derived data lines, then one prefixed data line
// per payload line, then the terminating blank line.
func datastarEventWriteToMethod(file *File, typeIdent string) *ast.FuncDecl {
	const (
		writerIdent     = "w"
		countIdent      = "bytesWritten"
		nIdent          = "n"
		dataVarIdent    = "data"
		lineIdent       = "line"
		retryBuf        = "retryBuf"
		eventTypeIdent  = "eventType"
		dataPrefixIdent = "dataPrefix"
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
	// stringOptionLine emits, for a *string field:
	//
	//	if m.<field> != nil {
	//		<write "data: <name> ">; <write *m.<field>>; <write '\n'>
	//	}
	stringOptionLine := func(field, name string) ast.Stmt {
		return &ast.IfStmt{
			Cond: &ast.BinaryExpr{X: mSel(field), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				writeAndCount(ioWriteString(astgen.String("data: " + name + " "))),
				writeAndCount(ioWriteString(deref(field))),
				writeAndCount(wWrite(newline())),
			}},
		}
	}
	boolOptionLine := func(field, name string) ast.Stmt {
		return &ast.IfStmt{
			Cond: mSel(field),
			Body: &ast.BlockStmt{List: []ast.Stmt{
				writeAndCount(ioWriteString(astgen.String("data: " + name + " true\n"))),
			}},
		}
	}

	body := []ast.Stmt{
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.BinaryExpr{X: mSel(sseTemplateDataFieldID), Op: token.NEQ, Y: astgen.Nil()},
				Op: token.LAND,
				Y:  astgen.Call(file, "", "strings", "ContainsAny", deref(sseTemplateDataFieldID), astgen.String("\r\n\x00")),
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
				astgen.Int(0), astgen.Call(file, "", "errors", "New", astgen.String("sse: id contains a forbidden character")),
			}}}},
		},
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(countIdent)}, Type: ast.NewIdent("int")}}}},
		// eventType, dataPrefix := "datastar-patch-elements", "data: elements "
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(eventTypeIdent), ast.NewIdent(dataPrefixIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{astgen.String(datastarPatchElementsEvent), astgen.String("data: elements ")},
		},
		&ast.IfStmt{
			Cond: mSel(datastarEventFieldIsSignals),
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(eventTypeIdent), ast.NewIdent(dataPrefixIdent)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{astgen.String(datastarPatchSignalsEvent), astgen.String("data: signals ")},
			}}},
		},
		// event: <type>
		writeAndCount(ioWriteString(astgen.String("event: "))),
		writeAndCount(ioWriteString(ast.NewIdent(eventTypeIdent))),
		writeAndCount(wWrite(newline())),
		// id: <v>
		&ast.IfStmt{
			Cond: &ast.BinaryExpr{X: mSel(sseTemplateDataFieldID), Op: token.NEQ, Y: astgen.Nil()},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				writeAndCount(ioWriteString(astgen.String("id: "))),
				writeAndCount(ioWriteString(deref(sseTemplateDataFieldID))),
				writeAndCount(wWrite(newline())),
			}},
		},
		// retry: <ms>
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
		// element-patch option lines, or the signals-only onlyIfMissing line
		&ast.IfStmt{
			Cond: &ast.UnaryExpr{Op: token.NOT, X: mSel(datastarEventFieldIsSignals)},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				stringOptionLine(datastarEventFieldSelector, "selector"),
				stringOptionLine(datastarEventFieldMode, "mode"),
				stringOptionLine(datastarEventFieldNamespace, "namespace"),
				boolOptionLine(datastarEventFieldUseViewTransition, "useViewTransition"),
			}},
			Else: &ast.BlockStmt{List: []ast.Stmt{
				boolOptionLine(datastarEventFieldOnlyIfMissing, "onlyIfMissing"),
			}},
		},
		// data := m.data.Bytes(); normalize CRLF; trim trailing newline
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{
			&ast.CallExpr{Fun: &ast.SelectorExpr{X: mSel(sseTemplateDataFieldData), Sel: ast.NewIdent("Bytes")}},
		}},
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
		&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{
			astgen.Call(file, "", "bytes", "TrimSuffix", ast.NewIdent(dataVarIdent), newline()),
		}},
		// for line := range bytes.SplitSeq(data, []byte{'\n'}) { dataPrefix + line + '\n' }
		&ast.RangeStmt{
			Key: ast.NewIdent(lineIdent),
			Tok: token.DEFINE,
			X:   astgen.Call(file, "", "bytes", "SplitSeq", ast.NewIdent(dataVarIdent), newline()),
			Body: &ast.BlockStmt{List: []ast.Stmt{
				writeAndCount(ioWriteString(ast.NewIdent(dataPrefixIdent))),
				writeAndCount(wWrite(ast.NewIdent(lineIdent))),
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
