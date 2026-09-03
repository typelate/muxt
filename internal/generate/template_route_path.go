package generate

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

const (
	routePathsReceiverName     = "routePaths"
	escapePathSegmentFuncName  = "escapePathSegment"
	escapePathSegmentsFuncName = "escapePathSegments"
)

func routePathTypeAndMethods(imports *File, config RoutesFileConfiguration, defs []muxt.Definition) ([]ast.Decl, error) {
	decls := []ast.Decl{
		&ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{
				&ast.TypeSpec{Name: ast.NewIdent(config.TemplateRoutePathsTypeName), Type: &ast.StructType{Fields: &ast.FieldList{
					List: []*ast.Field{
						{Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string")},
					},
				}}},
			},
		},
	}
	if err := muxt.CheckPathMethodCollisions(defs); err != nil {
		return nil, err
	}
	needsSegmentEscaper, needsSegmentsEscaper := false, false
	for _, t := range defs {
		decl, usesEscaper, usesSegmentsEscaper, err := routePathFunc(imports, config, &t)
		if err != nil {
			return nil, err
		}
		// escapePathSegments calls escapePathSegment, so needing the former
		// implies emitting both.
		needsSegmentEscaper = needsSegmentEscaper || usesEscaper || usesSegmentsEscaper
		needsSegmentsEscaper = needsSegmentsEscaper || usesSegmentsEscaper
		decls = append(decls, decl)
	}
	if needsSegmentEscaper {
		decls = append(decls, escapePathSegmentMethod(imports, config))
	}
	if needsSegmentsEscaper {
		decls = append(decls, escapePathSegmentsMethod(imports, config))
	}
	return decls, nil
}

func routePathFunc(file *File, config RoutesFileConfiguration, def *muxt.Definition) (_ *ast.FuncDecl, usesEscaper, usesSegmentsEscaper bool, _ error) {
	const methodReceiverName = routePathsReceiverName
	encodingPkg, ok := file.Types("encoding")
	if !ok {
		return nil, false, false, fmt.Errorf(`the "encoding" package must be loaded`)
	}
	scope := encodingPkg.Scope()
	textMarshalerObject := scope.Lookup("TextMarshaler")
	textMarshalerType := textMarshalerObject.Type()
	textMarshalerUnderlying := textMarshalerType.Underlying()
	textMarshalerInterface := textMarshalerUnderlying.(*types.Interface)

	ident, err := def.ExportedPathIdentifier()
	if err != nil {
		return nil, false, false, err
	}

	method := &ast.FuncDecl{
		Name: ast.NewIdent(ident),
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(methodReceiverName)}, Type: ast.NewIdent(config.TemplateRoutePathsTypeName)},
			},
		},
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: nil},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}},
		},
		Body: &ast.BlockStmt{
			List: nil,
		},
	}

	if def.Path() == "/" || def.Path() == "/{$}" {
		if config.PathPrefix {
			method.Body.List = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
				astgen.Call(file, "path", "path", "Join",
					astgen.Call(file, "cmp", "cmp", "Or",
						&ast.SelectorExpr{
							X:   ast.NewIdent(methodReceiverName),
							Sel: ast.NewIdent(pathPrefixPathsStructFieldName),
						},
						astgen.String("/"),
					),
				),
			}}}
		} else {
			method.Body.List = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{astgen.String("/")}}}
		}
		return method, usesEscaper, usesSegmentsEscaper, nil
	}

	templatePath, hasDollarSuffix := strings.CutSuffix(def.Path(), "{$}")
	segmentStrings := strings.Split(templatePath, "/")
	var (
		fields []*ast.Field
		last   types.Type

		identIndex = 0

		segmentIdentifiers = def.PathValueIdentifiers()
	)

	hasErrorResult := false
	segmentExpressions := []ast.Expr{
		astgen.Call(file, "cmp", "cmp", "Or",
			&ast.SelectorExpr{
				X:   ast.NewIdent(methodReceiverName),
				Sel: ast.NewIdent(pathPrefixPathsStructFieldName),
			},
			astgen.String("/"),
		),
	}
	for si, segment := range segmentStrings {
		if len(segment) < 1 {
			continue
		}
		if segment[0] != '{' || segment[len(segment)-1] != '}' {
			if len(segmentExpressions) > 0 {
				prev := segmentExpressions[len(segmentExpressions)-1]
				if prevBasic, ok := prev.(*ast.BasicLit); ok {
					prevVal, _ := strconv.Unquote(prevBasic.Value)
					prevBasic.Value = strconv.Quote(prevVal + "/" + segment)
					continue
				}
			}
			segmentExpressions = append(segmentExpressions, &ast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote(segment),
			})
			continue
		}

		name := segmentIdentifiers[identIndex]
		ident := pathParamIdent(name)
		wildcard := si == len(segmentStrings)-1 && isWildcardSegment(segment)
		pathValueType, ok := def.ArgumentType(name)
		identIndex++
		if !ok {
			pathValueType = types.Universe.Lookup("string").Type()
		}
		tpNode, err := file.TypeASTExpression(pathValueType)
		if err != nil {
			return nil, false, false, err
		}
		if last != nil && len(fields) > 0 && types.Identical(last, pathValueType) {
			fields[len(fields)-1].Names = append(fields[len(fields)-1].Names, ast.NewIdent(ident))
		} else {
			fields = append(fields, &ast.Field{
				Names: []*ast.Ident{ast.NewIdent(ident)},
				Type:  tpNode,
			})
			last = pathValueType
		}

		summer := sha1.New()
		summer.Write([]byte(def.Name()))
		pathHash := hex.EncodeToString(summer.Sum(nil))

		if types.Implements(pathValueType, textMarshalerInterface) {
			hasErrorResult = true
			if len(method.Type.Results.List) == 1 {
				method.Type.Results.List = append(method.Type.Results.List, &ast.Field{
					Type: ast.NewIdent("error"),
				})
			}
			segmentIdent := fmt.Sprintf("segment%d_%s", si, pathHash[:8])
			method.Body.List = append(method.Body.List, &ast.AssignStmt{
				Rhs: []ast.Expr{&ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent(ident),
						Sel: ast.NewIdent("MarshalText"),
					},
				}},
				Tok: token.DEFINE,
				Lhs: []ast.Expr{
					ast.NewIdent(segmentIdent),
					ast.NewIdent("err"),
				},
			}, &ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ReturnStmt{
							Results: []ast.Expr{
								&ast.BasicLit{Kind: token.STRING, Value: `""`},
								astgen.Call(file, "fmt", "fmt", "Errorf",
									astgen.String(fmt.Sprintf("failed to marshal path value {%s} (segment %d) in %s: %%w", name, si, def.Path())),
									ast.NewIdent("err"),
								),
							},
						},
					},
				},
			})
			var marshaled ast.Expr = &ast.CallExpr{
				Fun:  ast.NewIdent("string"),
				Args: []ast.Expr{ast.NewIdent(segmentIdent)},
			}
			if wildcard {
				usesSegmentsEscaper = true
				marshaled = escapedPathSegments(marshaled)
			} else {
				usesEscaper = true
				marshaled = escapedPathSegment(marshaled)
			}
			segmentExpressions = append(segmentExpressions, marshaled)
			continue
		}

		basicType, ok := pathValueType.Underlying().(*types.Basic)
		if !ok {
			return nil, false, false, fmt.Errorf("unsupported type %s for path parameters: %s", astgen.Format(tpNode), ident)
		}
		exp, err := astgen.ConvertToString(file, ast.NewIdent(ident), basicType.Kind())
		if err != nil {
			return nil, false, false, fmt.Errorf("failed to encode variable %s: %v", ident, err)
		}
		if basicType.Info()&types.IsString != 0 {
			if wildcard {
				usesSegmentsEscaper = true
				exp = escapedPathSegments(exp)
			} else {
				usesEscaper = true
				exp = escapedPathSegment(exp)
			}
		}
		segmentExpressions = append(segmentExpressions, exp)
	}

	returnStmt := ast.Expr(&ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(file.Import("path", "path")),
			Sel: ast.NewIdent("Join"),
		},
		Args: segmentExpressions,
	})
	if hasDollarSuffix {
		returnStmt = &ast.BinaryExpr{
			X:  returnStmt,
			Op: token.ADD,
			Y: &ast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote("/"),
			},
		}
	}

	if hasErrorResult {
		method.Body.List = append(method.Body.List, &ast.ReturnStmt{Results: []ast.Expr{returnStmt, astgen.Nil()}})
	} else {
		method.Body.List = append(method.Body.List, &ast.ReturnStmt{Results: []ast.Expr{returnStmt}})
	}

	method.Type.Params.List = fields

	return method, usesEscaper, usesSegmentsEscaper, nil
}

// isWildcardSegment reports whether segment is a {name...} pattern; only the
// trailing segment of a pattern may be one, and its value names a path suffix
// spliced without escaping.
func isWildcardSegment(segment string) bool {
	return strings.HasSuffix(strings.TrimSuffix(segment, "}"), "...")
}

// pathParamIdent names the generated local for a path parameter. The suffix
// keeps any wildcard name from colliding with an identifier the generated
// code references: an import, another local, or a captured variable.
func pathParamIdent(name string) string {
	return name + "PathParam"
}

// escapedPathSegment wraps value in a call to the generated escapePathSegment
// method; the caller must arrange for escapePathSegmentMethod to be emitted.
func escapedPathSegment(value ast.Expr) ast.Expr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(routePathsReceiverName),
			Sel: ast.NewIdent(escapePathSegmentFuncName),
		},
		Args: []ast.Expr{value},
	}
}

// escapedPathSegments wraps a trailing-wildcard value in a call to the
// generated escapePathSegments method; the caller must arrange for both
// escaper methods to be emitted.
func escapedPathSegments(value ast.Expr) ast.Expr {
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(routePathsReceiverName),
			Sel: ast.NewIdent(escapePathSegmentsFuncName),
		},
		Args: []ast.Expr{value},
	}
}

// escapePathSegmentsMethod emits:
//
//	func (routePaths TemplateRoutePaths) escapePathSegments(value string) string {
//		segments := strings.Split(value, "/")
//		for i, segment := range segments {
//			segments[i] = routePaths.escapePathSegment(segment)
//		}
//		return strings.Join(segments, "/")
//	}
//
// A trailing {name...} wildcard names a multi-segment path suffix, so its "/"
// separators are meaningful and each segment between them is escaped alone.
func escapePathSegmentsMethod(file *File, config RoutesFileConfiguration) *ast.FuncDecl {
	const (
		valueIdent    = "value"
		segmentsIdent = "segments"
		indexIdent    = "i"
		segmentIdent  = "segment"
	)
	return &ast.FuncDecl{
		Name: ast.NewIdent(escapePathSegmentsFuncName),
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(routePathsReceiverName)}, Type: ast.NewIdent(config.TemplateRoutePathsTypeName)},
			},
		},
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(valueIdent)}, Type: ast.NewIdent("string")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(segmentsIdent)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{astgen.Call(file, "strings", "strings", "Split", ast.NewIdent(valueIdent), astgen.String("/"))},
				},
				&ast.RangeStmt{
					Key:   ast.NewIdent(indexIdent),
					Value: ast.NewIdent(segmentIdent),
					Tok:   token.DEFINE,
					X:     ast.NewIdent(segmentsIdent),
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.AssignStmt{
								Lhs: []ast.Expr{&ast.IndexExpr{X: ast.NewIdent(segmentsIdent), Index: ast.NewIdent(indexIdent)}},
								Tok: token.ASSIGN,
								Rhs: []ast.Expr{escapedPathSegment(ast.NewIdent(segmentIdent))},
							},
						},
					},
				},
				&ast.ReturnStmt{Results: []ast.Expr{
					astgen.Call(file, "strings", "strings", "Join", ast.NewIdent(segmentsIdent), astgen.String("/")),
				}},
			},
		},
	}
}

// escapePathSegmentMethod emits:
//
//	func (routePaths TemplateRoutePaths) escapePathSegment(value string) string {
//		switch value {
//		case ".":
//			return "%2E"
//		case "..":
//			return "%2E%2E"
//		}
//		return url.PathEscape(value)
//	}
//
// url.PathEscape leaves "." and ".." unchanged ('.' is unreserved), but
// path.Join and request routing collapse dot segments, so a helper value of
// ".." would address a parent route. The percent-encoded forms survive both:
// http.ServeMux matches the escaped path and PathValue decodes them back.
func escapePathSegmentMethod(file *File, config RoutesFileConfiguration) *ast.FuncDecl {
	const valueIdent = "value"
	caseReturn := func(match, encoded string) *ast.CaseClause {
		return &ast.CaseClause{
			List: []ast.Expr{astgen.String(match)},
			Body: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{astgen.String(encoded)}}},
		}
	}
	return &ast.FuncDecl{
		Name: ast.NewIdent(escapePathSegmentFuncName),
		Recv: &ast.FieldList{
			List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(routePathsReceiverName)}, Type: ast.NewIdent(config.TemplateRoutePathsTypeName)},
			},
		},
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(valueIdent)}, Type: ast.NewIdent("string")}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("string")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.SwitchStmt{
					Tag: ast.NewIdent(valueIdent),
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							caseReturn(".", "%2E"),
							caseReturn("..", "%2E%2E"),
						},
					},
				},
				&ast.ReturnStmt{Results: []ast.Expr{
					astgen.Call(file, "url", "net/url", "PathEscape", ast.NewIdent(valueIdent)),
				}},
			},
		},
	}
}
