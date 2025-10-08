package muxt

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ettle/strcase"

	"github.com/typelate/muxt/internal/source"
)

func (t Template) generateEndpointPatternIdentifier(sb *strings.Builder) string {
	if sb == nil {
		sb = new(strings.Builder)
	}
	sb.Reset()
	switch t.method {
	case http.MethodPost:
		sb.WriteString("Create")
	case http.MethodGet:
		sb.WriteString("Read")
	case http.MethodPut:
		sb.WriteString("Replace")
	case http.MethodPatch:
		sb.WriteString("Update")
	case http.MethodDelete:
		sb.WriteString("Delete")
	default:
		sb.WriteString(strcase.ToGoPascal(t.method))
	}
	var pathParams []string
	if t.path == "/" {
		if t.host != "" {
			sb.WriteString(strcase.ToGoPascal(t.host))
		}
		sb.WriteString("Index")
	} else {
		pathSegments := []string{t.host}
		pathSegments = append(pathSegments, strings.Split(t.path, "/")...)
		for _, pathSegment := range pathSegments {
			isPathParam := false
			if len(pathSegment) > 2 && pathSegment[0] == '{' && pathSegment[len(pathSegment)-1] == '}' {
				pathSegment = pathSegment[1 : len(pathSegment)-1]
				isPathParam = true
			}
			if len(pathSegment) == 0 {
				continue
			}
			if pathSegment == "$" {
				sb.WriteString("Index")
				continue
			}
			pathSegment = strings.TrimRight(pathSegment, ".")
			pathSegment = strcase.ToGoPascal(pathSegment)
			if isPathParam {
				pathParams = append(pathParams, pathSegment)
				continue
			}
			sb.WriteString(pathSegment)
		}
	}
	if len(pathParams) > 0 {
		sb.WriteString("By")
	}
	for i, pathParam := range pathParams {
		if len(pathParams) > 1 && i == len(pathParams)-1 {
			sb.WriteString("And")
		}
		sb.WriteString(pathParam)
	}
	return sb.String()
}

func calculateIdentifiers(in []Template) {
	var (
		sb     strings.Builder
		idents = make([]string, 0, len(in))
		dupes  []string
	)
	for i, t := range in {
		if t.fun != nil && t.fun.Name != "" {
			ident := t.fun.Name
			if j := slices.Index(idents, ident); j > 0 {
				routePrev := in[j].generateEndpointPatternIdentifier(&sb)
				idents[i] = routePrev + "Calling" + ident
				route := t.generateEndpointPatternIdentifier(&sb)
				idents = append(idents, route+"Calling"+t.fun.Name)
				dupes = append(dupes, idents[j])
				in[i].identifier = ident
				continue
			}
			if slices.Contains(dupes, ident) {
				route := t.generateEndpointPatternIdentifier(&sb)
				idents = append(idents, route+"Calling"+t.fun.Name)
				in[i].identifier = ident
				continue
			}
			idents = append(idents, t.fun.Name)
			in[i].identifier = ident
			continue
		}
		ident := t.generateEndpointPatternIdentifier(&sb)
		in[i].identifier = ident
	}
}

func routePathFunc(file *source.File, config RoutesFileConfiguration, t *Template) (*ast.FuncDecl, error) {
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
		Name: ast.NewIdent(t.identifier),
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

	if t.path == "/" || t.path == "/{$}" {
		if config.PathPrefix {
			method.Body.List = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
				file.Call("path", "path", "Join", []ast.Expr{
					file.Call("cmp", "cmp", "Or", []ast.Expr{
						&ast.SelectorExpr{
							X:   ast.NewIdent(methodReceiverName),
							Sel: ast.NewIdent(pathPrefixPathsStructFieldName),
						},
						source.String("/"),
					}),
				}),
			}}}
		} else {
			method.Body.List = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{source.String("/")}}}
		}
		return method, nil
	}

	templatePath, hasDollarSuffix := strings.CutSuffix(t.path, "{$}")
	segmentStrings := strings.Split(templatePath, "/")
	var (
		fields []*ast.Field
		last   types.Type

		identIndex = 0

		segmentIdentifiers = t.parsePathValueNames()
	)

	hasErrorResult := false
	segmentExpressions := []ast.Expr{
		file.Call("cmp", "cmp", "Or", []ast.Expr{
			&ast.SelectorExpr{
				X:   ast.NewIdent(methodReceiverName),
				Sel: ast.NewIdent(pathPrefixPathsStructFieldName),
			},
			source.String("/"),
		}),
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
		pathValueType, ok := t.pathValueTypes[ident]
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
		summer.Write([]byte(t.name))
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
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: source.Nil()},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ReturnStmt{
							Results: []ast.Expr{
								&ast.BasicLit{Kind: token.STRING, Value: `""`},
								file.Call("fmt", "fmt", "Errorf", []ast.Expr{
									source.String(fmt.Sprintf("failed to marshal path value {%s} (segment %d) in %s: %%w", ident, si, t.path)),
									ast.NewIdent("err"),
								}),
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
			return nil, fmt.Errorf("unsupported type %s for path parameters: %s", source.Format(tpNode), ident)
		}
		exp, err := file.Format(ast.NewIdent(ident), basicType.Kind())
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
		method.Body.List = append(method.Body.List, &ast.ReturnStmt{Results: []ast.Expr{returnStmt, source.Nil()}})
	} else {
		method.Body.List = append(method.Body.List, &ast.ReturnStmt{Results: []ast.Expr{returnStmt}})
	}

	method.Type.Params.List = fields

	return method, nil
}

func routePathTypeAndMethods(imports *source.File, config RoutesFileConfiguration, templates []Template) ([]ast.Decl, error) {
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
	for _, t := range templates {
		decl, err := routePathFunc(imports, config, &t)
		if err != nil {
			return nil, err
		}
		decls = append(decls, decl)
	}
	return decls, nil
}

// fileNameToIdentifier converts a template source filename to a Go identifier prefix.
// For example: "index.gohtml" -> "Index", "user-profile.gohtml" -> "UserProfile"
// Returns empty string for empty filenames.
func fileNameToIdentifier(filename string) string {
	if filename == "" {
		return ""
	}
	// Strip the extension
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if base == "" {
		return ""
	}
	// Convert to PascalCase using strcase
	return strcase.ToGoPascal(base)
}
