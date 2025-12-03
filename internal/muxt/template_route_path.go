package muxt

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
)

func routePathTypeAndMethods(imports *File, config RoutesFileConfiguration, defs []Definition) ([]ast.Decl, error) {
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
	for _, t := range defs {
		decl, err := routePathFunc(imports, config, &t)
		if err != nil {
			return nil, err
		}
		decls = append(decls, decl)
	}
	return decls, nil
}

func routePathFunc(file *File, config RoutesFileConfiguration, def *Definition) (*ast.FuncDecl, error) {
	const methodReceiverName = "routePaths"
	encodingPkg, ok := file.Types("encoding")
	if !ok {
		return nil, fmt.Errorf(`the "encoding" package must be loaded`)
	}
	scope := encodingPkg.Scope()
	textMarshalerObject := scope.Lookup("TextMarshaler")
	textMarshalerType := textMarshalerObject.Type()
	textMarshalerUnderlying := textMarshalerType.Underlying()
	textMarshalerInterface := textMarshalerUnderlying.(*types.Interface)

	method := &ast.FuncDecl{
		Name: ast.NewIdent(def.identifier),
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

	if def.path == "/" || def.path == "/{$}" {
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
		return method, nil
	}

	templatePath, hasDollarSuffix := strings.CutSuffix(def.path, "{$}")
	segmentStrings := strings.Split(templatePath, "/")
	var (
		fields []*ast.Field
		last   types.Type

		identIndex = 0

		segmentIdentifiers = def.parsePathValueNames()
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

		ident := segmentIdentifiers[identIndex]
		pathValueType, ok := def.pathValueTypes[ident]
		identIndex++
		if !ok {
			pathValueType = types.Universe.Lookup("string").Type()
		}
		tpNode, err := file.TypeASTExpression(pathValueType)
		if err != nil {
			return nil, err
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
		summer.Write([]byte(def.name))
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
									astgen.String(fmt.Sprintf("failed to marshal path value {%s} (segment %d) in %s: %%w", ident, si, def.path)),
									ast.NewIdent("err"),
								),
							},
						},
					},
				},
			})
			segmentExpressions = append(segmentExpressions, &ast.CallExpr{
				Fun:  ast.NewIdent("string"),
				Args: []ast.Expr{ast.NewIdent(segmentIdent)},
			})
			continue
		}

		basicType, ok := pathValueType.Underlying().(*types.Basic)
		if !ok {
			return nil, fmt.Errorf("unsupported type %s for path parameters: %s", astgen.Format(tpNode), ident)
		}
		exp, err := astgen.ConvertToString(file, ast.NewIdent(ident), basicType.Kind())
		if err != nil {
			return nil, fmt.Errorf("failed to encode variable %s: %v", ident, err)
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

	return method, nil
}
