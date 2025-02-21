package muxt

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/ettle/strcase"

	"github.com/crhntr/muxt/internal/source"
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

type BasicPathSegment string

func (segment BasicPathSegment) Expr() ast.Expr {
	return &ast.BasicLit{
		Kind:  token.STRING,
		Value: strconv.Quote(string(segment)),
	}
}

type HostSlashPathSegment string

func (segment HostSlashPathSegment) Expr() ast.Expr {
	return &ast.BasicLit{
		Kind:  token.STRING,
		Value: strconv.Quote(string(segment)),
	}
}

type StringPathValuePathSegment string

func (segment StringPathValuePathSegment) Expr() ast.Expr {
	return ast.NewIdent(string(segment))
}

func encodeVariable(imports *source.Imports, name string, syntax ast.Expr, tp types.Type) (ast.Expr, error) {
	basicType, ok := tp.Underlying().(*types.Basic)
	if !ok {
		return nil, fmt.Errorf("unsupported type %s for path parameters: %s", source.Format(syntax), name)
	}
	switch basicType.Kind() {
	case types.Bool, types.UntypedBool:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatBool")},
			Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("bool"), Args: []ast.Expr{ast.NewIdent(name)}}},
		}, nil
	case types.Int, types.UntypedInt:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("Itoa")},
			Args: []ast.Expr{ast.NewIdent(name)},
		}, nil
	case types.Int8:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatInt")},
			Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{ast.NewIdent(name)}}, source.Int(8)},
		}, nil
	case types.Int16:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatInt")},
			Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{ast.NewIdent(name)}}, source.Int(16)},
		}, nil
	case types.Int32:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatInt")},
			Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{ast.NewIdent(name)}}, source.Int(32)},
		}, nil
	case types.Int64:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatInt")},
			Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("int64"), Args: []ast.Expr{ast.NewIdent(name)}}, source.Int(64)},
		}, nil
	case types.Uint:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatUint")},
			Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{ast.NewIdent(name)}}, source.Int(64)},
		}, nil
	case types.Uint8:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatUint")},
			Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{ast.NewIdent(name)}}, source.Int(8)},
		}, nil
	case types.Uint16:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatUint")},
			Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{ast.NewIdent(name)}}, source.Int(16)},
		}, nil
	case types.Uint32:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatUint")},
			Args: []ast.Expr{&ast.CallExpr{Fun: ast.NewIdent("uint64"), Args: []ast.Expr{ast.NewIdent(name)}}, source.Int(32)},
		}, nil
	case types.Uint64:
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "strconv")), Sel: ast.NewIdent("FormatUint")},
			Args: []ast.Expr{ast.NewIdent(name), source.Int(64)},
		}, nil
	case types.String:
		return ast.NewIdent(name), nil
	default:
		return nil, fmt.Errorf("unsupported basic type for path parameters: %s", name)
	}
}

func routePathFunc(imports *source.Imports, t *Template) (*ast.FuncDecl, error) {
	encodingPkg, ok := imports.Types("encoding")
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
				{Type: ast.NewIdent(urlHelperTypeName)},
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
		method.Body.List = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"/"`}}}}
		return method, nil
	}

	templatePath, hasDollarSuffix := strings.CutSuffix(t.path, "{$}")
	segmentStrings := strings.Split(templatePath, "/")
	var (
		fields []*ast.Field
		last   types.Type

		segmentExpressions []ast.Expr
		identIndex         = 0

		segmentIdentifiers = t.parsePathValueNames()
	)
	if len(segmentIdentifiers) == 0 {
		method.Body.List = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(templatePath)}}}}
		return method, nil
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
					prevBasic.Value = strconv.Quote(prevVal + segment)
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
		tpNode, err := astTypeExpression(imports, pathValueType)
		if err != nil {
			return nil, err
		}
		if last != nil && len(fields) > 0 && types.Identical(last, pathValueType) {
			fields[len(fields)-1].Names = append(fields[len(fields)-1].Names, ast.NewIdent(ident))
			continue
		}
		fields = append(fields, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(ident)},
			Type:  tpNode,
		})
		last = pathValueType

		if types.Implements(pathValueType, textMarshalerInterface) {
			if len(method.Type.Results.List) == 1 {
				method.Type.Results.List = append(method.Type.Results.List, &ast.Field{
					Type: ast.NewIdent("error"),
				})
			}
			segmentIdent := fmt.Sprintf("segment%d", si)
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
								ast.NewIdent("err"),
							},
						},
					},
				},
			})
			segmentExpressions = append(segmentExpressions, ast.NewIdent(segmentIdent))
			continue
		}

		exp, err := encodeVariable(imports, ident, tpNode, pathValueType)
		if err != nil {
			return nil, err
		}
		segmentExpressions = append(segmentExpressions, exp)
	}

	returnStmt := &ast.BinaryExpr{
		X: &ast.BasicLit{
			Kind:  token.STRING,
			Value: strconv.Quote("/"),
		},
		Op: token.ADD,
		Y: &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(imports.Add("", "path")),
				Sel: ast.NewIdent("Join"),
			},
			Args: segmentExpressions,
		},
	}
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

	method.Body.List = append(method.Body.List, &ast.ReturnStmt{Results: []ast.Expr{returnStmt}})
	method.Type.Params.List = fields

	return method, nil
}

func routePathTypeAndMethods(imports *source.Imports, templates []Template, receiver *types.Named) ([]ast.Decl, error) {
	decls := []ast.Decl{
		&ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{
				&ast.TypeSpec{Name: ast.NewIdent(urlHelperTypeName), Type: &ast.StructType{Fields: &ast.FieldList{}}},
			},
		},
		&ast.FuncDecl{
			Name: ast.NewIdent("TemplateRoutePath"),
			Type: &ast.FuncType{Results: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("")}, Type: ast.NewIdent(urlHelperTypeName)}}}},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ReturnStmt{
						Results: []ast.Expr{&ast.CompositeLit{Type: ast.NewIdent(urlHelperTypeName)}},
					},
				},
			},
		},
	}
	for _, t := range templates {
		decl, err := routePathFunc(imports, &t)
		if err != nil {
			return nil, err
		}
		decls = append(decls, decl)
	}
	return decls, nil
}
