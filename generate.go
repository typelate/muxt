package muxt

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"html/template"
	"log"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/crhntr/muxt/internal/source"
)

const (
	executeIdentName = "execute"
	receiverIdent    = "receiver"

	dataVarIdent = "data"
	muxVarIdent  = "mux"

	requestPathValue         = "PathValue"
	httpRequestContextMethod = "Context"
	httpPackageIdent         = "http"
	httpResponseWriterIdent  = "ResponseWriter"
	httpServeMuxIdent        = "ServeMux"
	httpRequestIdent         = "Request"
	httpHandleFuncIdent      = "HandleFunc"

	contextPackageIdent     = "context"
	contextContextTypeIdent = "Context"

	defaultPackageName           = "main"
	DefaultTemplatesVariableName = "templates"
	DefaultRoutesFunctionName    = "routes"
	DefaultOutputFileName        = "template_routes.go"
	receiverInterfaceIdent       = "RoutesReceiver"
)

func Generate(templateNames []TemplateName, _ *template.Template, packageName, templatesVariableName, routesFunctionName, receiverTypeIdent string, _ *token.FileSet, receiverPackage, templatesPackage []*ast.File, log *log.Logger) (string, error) {
	packageName = cmp.Or(packageName, defaultPackageName)
	templatesVariableName = cmp.Or(templatesVariableName, DefaultTemplatesVariableName)
	routesFunctionName = cmp.Or(routesFunctionName, DefaultRoutesFunctionName)
	file := &ast.File{
		Name: ast.NewIdent(packageName),
	}
	routes := &ast.FuncDecl{
		Name: ast.NewIdent(routesFunctionName),
		Type: routesFuncType(ast.NewIdent(receiverInterfaceIdent)),
		Body: &ast.BlockStmt{},
	}
	receiverInterface := &ast.InterfaceType{
		Methods: &ast.FieldList{},
	}
	imports := []*ast.ImportSpec{
		importSpec("net/" + httpPackageIdent),
	}
	for _, name := range templateNames {
		var method *ast.FuncType
		if name.fun != nil {
			for _, funcDecl := range source.IterateFunctions(receiverPackage) {
				if !name.matchReceiver(funcDecl, receiverTypeIdent) {
					continue
				}
				method = funcDecl.Type
				break
			}
			if method == nil {
				me, im := name.funcType()
				method = me
				imports = append(imports, im...)
			}
			receiverInterface.Methods.List = append(receiverInterface.Methods.List, &ast.Field{
				Names: []*ast.Ident{ast.NewIdent(name.fun.Name)},
				Type:  method,
			})
		}
		handlerFunc, methodImports, err := name.funcLit(method, receiverPackage)
		if err != nil {
			return "", err
		}
		imports = sortImports(append(imports, methodImports...))
		routes.Body.List = append(routes.Body.List, name.callHandleFunc(handlerFunc))
		log.Printf("%s has route for %s", routesFunctionName, name.String())
	}
	importGen := &ast.GenDecl{
		Tok: token.IMPORT,
	}
	file.Decls = append(file.Decls, importGen)
	file.Decls = append(file.Decls, &ast.GenDecl{
		Tok:   token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(receiverInterfaceIdent), Type: receiverInterface}},
	})
	file.Decls = append(file.Decls, routes)
	hasExecuteFunc := false
	for _, fn := range source.IterateFunctions(templatesPackage) {
		if fn.Recv == nil && fn.Name.Name == executeIdentName {
			hasExecuteFunc = true
		}
	}
	if !hasExecuteFunc {
		file.Decls = append(file.Decls, executeFuncDecl(templatesVariableName))
		imports = append(imports, importSpec("bytes"))
	}
	for _, imp := range imports {
		importGen.Specs = append(importGen.Specs, imp)
	}
	return source.Format(file), nil
}

func (def TemplateName) callHandleFunc(handlerFuncLit *ast.FuncLit) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(muxVarIdent),
			Sel: ast.NewIdent(httpHandleFuncIdent),
		},
		Args: []ast.Expr{
			&ast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote(def.endpoint),
			},
			handlerFuncLit,
		},
	}}
}

func (def TemplateName) funcLit(method *ast.FuncType, files []*ast.File) (*ast.FuncLit, []*ast.ImportSpec, error) {
	if method == nil {
		return def.httpRequestReceiverTemplateHandlerFunc(), nil, nil
	}
	lit := &ast.FuncLit{
		Type: httpHandlerFuncType(),
		Body: &ast.BlockStmt{},
	}
	call := &ast.CallExpr{Fun: callReceiverMethod(def.fun)}
	if method.Params.NumFields() != len(def.call.Args) {
		return nil, nil, errWrongNumberOfArguments(def, method)
	}
	var formStruct *ast.StructType
	for pi, pt := range fieldListTypes(method.Params) {
		if err := checkArgument(method, pi, def.call.Args[pi], pt, files); err != nil {
			return nil, nil, err
		}
		if s, ok := findFormStruct(pt, files); ok {
			formStruct = s
		}
	}
	const errVarIdent = "err"
	var (
		imports     []*ast.ImportSpec
		writeHeader = true
	)
	for i, a := range def.call.Args {
		arg := a.(*ast.Ident)
		switch arg.Name {
		case TemplateNameScopeIdentifierHTTPResponse:
			writeHeader = false
			fallthrough
		case TemplateNameScopeIdentifierHTTPRequest:
			call.Args = append(call.Args, ast.NewIdent(arg.Name))
			imports = append(imports, importSpec("net/http"))
		case TemplateNameScopeIdentifierContext:
			lit.Body.List = append(lit.Body.List, contextAssignment())
			call.Args = append(call.Args, ast.NewIdent(TemplateNameScopeIdentifierContext))
			imports = append(imports, importSpec("context"))
		case TemplateNameScopeIdentifierForm:
			_, tp, _ := source.FieldIndex(method.Params.List, i)
			lit.Body.List = append(lit.Body.List,
				&ast.ExprStmt{X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
						Sel: ast.NewIdent("ParseForm"),
					},
					Args: []ast.Expr{},
				}},
				formDeclaration(arg.Name, tp))
			if formStruct != nil {
				for _, field := range formStruct.Fields.List {
					for _, name := range field.Names {
						fieldExpr := &ast.SelectorExpr{
							X:   ast.NewIdent(arg.Name),
							Sel: ast.NewIdent(name.Name),
						}
						errCheck := source.ErrorCheckReturn(errVarIdent, &ast.ExprStmt{X: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   ast.NewIdent(httpPackageIdent),
								Sel: ast.NewIdent("Error"),
							},
							Args: []ast.Expr{
								ast.NewIdent(httpResponseField().Names[0].Name),
								&ast.CallExpr{
									Fun:  &ast.SelectorExpr{X: ast.NewIdent("err"), Sel: ast.NewIdent("Error")},
									Args: []ast.Expr{},
								},
								source.HTTPStatusCode(httpPackageIdent, http.StatusBadRequest),
							},
						}}, &ast.ReturnStmt{})

						statements, parseImports, err := parseStringStatements(name.Name, fieldExpr, errVarIdent, &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
								Sel: ast.NewIdent("FormValue"),
							},
							Args: []ast.Expr{
								&ast.BasicLit{
									Kind:  token.STRING,
									Value: strconv.Quote(formInputName(field, name)),
								},
							},
						}, field.Type, token.ASSIGN, errCheck)
						if err != nil {
							return nil, nil, fmt.Errorf("failed to generate parse statements for form field %s: %w", name.Name, err)
						}
						lit.Body.List = append(lit.Body.List, statements...)
						imports = append(imports, parseImports...)
					}
				}
			} else {
				imports = append(imports, importSpec("net/url"))
			}
			call.Args = append(call.Args, ast.NewIdent(arg.Name))
		default:
			errCheck := source.ErrorCheckReturn(errVarIdent, &ast.ExprStmt{X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(httpPackageIdent),
					Sel: ast.NewIdent("Error"),
				},
				Args: []ast.Expr{
					ast.NewIdent(httpResponseField().Names[0].Name),
					&ast.CallExpr{
						Fun:  &ast.SelectorExpr{X: ast.NewIdent("err"), Sel: ast.NewIdent("Error")},
						Args: []ast.Expr{},
					},
					source.HTTPStatusCode(httpPackageIdent, http.StatusBadRequest),
				},
			}}, &ast.ReturnStmt{})
			src := &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
					Sel: ast.NewIdent(requestPathValue),
				},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(arg.Name)}},
			}
			statements, parseImports, err := httpPathValueAssignment(method, i, arg, errVarIdent, src, token.DEFINE, errCheck)
			if err != nil {
				return nil, nil, err
			}
			lit.Body.List = append(lit.Body.List, statements...)
			call.Args = append(call.Args, ast.NewIdent(arg.Name))
			imports = append(imports, parseImports...)
		}
	}

	const dataVarIdent = "data"
	if len(method.Results.List) > 1 {
		errVar := ast.NewIdent("err")

		lit.Body.List = append(lit.Body.List,
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent), ast.NewIdent(errVar.Name)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errVar.Name), Op: token.NEQ, Y: ast.NewIdent("nil")},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ExprStmt{X: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   ast.NewIdent(httpPackageIdent),
								Sel: ast.NewIdent("Error"),
							},
							Args: []ast.Expr{
								ast.NewIdent(httpResponseField().Names[0].Name),
								&ast.CallExpr{
									Fun: &ast.SelectorExpr{
										X:   ast.NewIdent("err"),
										Sel: ast.NewIdent("Error"),
									},
									Args: []ast.Expr{},
								},
								source.HTTPStatusCode(httpPackageIdent, http.StatusInternalServerError),
							},
						}},
						&ast.ReturnStmt{},
					},
				},
			},
		)
	} else {
		lit.Body.List = append(lit.Body.List, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}})
	}
	lit.Body.List = append(lit.Body.List, def.executeCall(source.HTTPStatusCode(httpPackageIdent, http.StatusOK), ast.NewIdent(dataVarIdent), writeHeader))
	return lit, imports, nil
}

func formInputName(field *ast.Field, name *ast.Ident) string {
	if field.Tag != nil {
		v, _ := strconv.Unquote(field.Tag.Value)
		tags := reflect.StructTag(v)
		n, hasInputTag := tags.Lookup("input")
		if hasInputTag {
			return n
		}
	}
	return name.Name
}

func (def TemplateName) funcType() (*ast.FuncType, []*ast.ImportSpec) {
	method := &ast.FuncType{
		Params:  &ast.FieldList{},
		Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
	}
	var imports []*ast.ImportSpec
	for _, a := range def.call.Args {
		arg := a.(*ast.Ident)
		switch arg.Name {
		case TemplateNameScopeIdentifierHTTPRequest:
			method.Params.List = append(method.Params.List, httpRequestField())
			imports = append(imports, importSpec("net/"+httpPackageIdent))
		case TemplateNameScopeIdentifierHTTPResponse:
			method.Params.List = append(method.Params.List, httpResponseField())
			imports = append(imports, importSpec("net/"+httpPackageIdent))
		case TemplateNameScopeIdentifierContext:
			method.Params.List = append(method.Params.List, contextContextField())
			imports = append(imports, importSpec(contextPackageIdent))
		case TemplateNameScopeIdentifierForm:
			method.Params.List = append(method.Params.List, urlValuesField(arg.Name))
		default:
			method.Params.List = append(method.Params.List, pathValueField(arg.Name))
		}
	}
	return method, imports
}

func importSpec(path string) *ast.ImportSpec {
	return &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(path)}}
}

func fieldListTypes(fieldList *ast.FieldList) func(func(int, ast.Expr) bool) {
	return func(yield func(int, ast.Expr) bool) {
		paramIndex := 0
		for _, param := range fieldList.List {
			if len(param.Names) == 0 {
				if !yield(paramIndex, param.Type) {
					return
				}
				paramIndex++
				continue
			}
			for range param.Names {
				if !yield(paramIndex, param.Type) {
					return
				}
				paramIndex++
			}
		}
	}
}

func errWrongNumberOfArguments(def TemplateName, method *ast.FuncType) error {
	return fmt.Errorf("handler %s expects %d arguments but call %s has %d", source.Format(&ast.FuncDecl{Name: ast.NewIdent(def.fun.Name), Type: method}), method.Params.NumFields(), def.handler, len(def.call.Args))
}

func checkArgument(method *ast.FuncType, argIndex int, exp ast.Expr, argType ast.Expr, files []*ast.File) error {
	// TODO: rewrite to "cannot use 32 (untyped int constant) as string value in argument to strings.ToUpper"
	arg := exp.(*ast.Ident)
	switch arg.Name {
	case TemplateNameScopeIdentifierHTTPRequest:
		if !matchSelectorIdents(argType, httpPackageIdent, httpRequestIdent, true) {
			return fmt.Errorf("method expects type %s but %s is *%s.%s", source.Format(argType), arg.Name, httpPackageIdent, httpRequestIdent)
		}
		return nil
	case TemplateNameScopeIdentifierHTTPResponse:
		if !matchSelectorIdents(argType, httpPackageIdent, httpResponseWriterIdent, false) {
			return fmt.Errorf("method expects type %s but %s is %s.%s", source.Format(argType), arg.Name, httpPackageIdent, httpResponseWriterIdent)
		}
		return nil
	case TemplateNameScopeIdentifierContext:
		if !matchSelectorIdents(argType, contextPackageIdent, contextContextTypeIdent, false) {
			return fmt.Errorf("method expects type %s but %s is %s.%s", source.Format(argType), arg.Name, contextPackageIdent, contextContextTypeIdent)
		}
		return nil
	case TemplateNameScopeIdentifierForm:
		if matchSelectorIdents(argType, "url", "Values", false) {
			return nil
		}
		_, ok := findFormStruct(argType, files)
		if !ok {
			return fmt.Errorf("method expects form to have type url.Values or T (where T is some struct type)")
		}
		return nil
	default:
		for paramIndex, paramType := range source.IterateFieldTypes(method.Params.List) {
			if argIndex != paramIndex {
				continue
			}
			paramTypeIdent, paramOk := paramType.(*ast.Ident)
			argTypeIdent, argOk := argType.(*ast.Ident)
			if !argOk || !paramOk || argTypeIdent.Name != paramTypeIdent.Name {
				return fmt.Errorf("method expects type %s but %s is a %s", source.Format(argType), arg.Name, paramTypeIdent.Name)
			}
			break
		}
		return nil
	}
}

func findFormStruct(argType ast.Expr, files []*ast.File) (*ast.StructType, bool) {
	if argTypeIdent, ok := argType.(*ast.Ident); ok {
		for _, file := range files {
			for _, d := range file.Decls {
				decl, ok := d.(*ast.GenDecl)
				if !ok || decl.Tok != token.TYPE {
					continue
				}
				for _, s := range decl.Specs {
					spec := s.(*ast.TypeSpec)
					structType, isStruct := spec.Type.(*ast.StructType)
					if isStruct && spec.Name.Name == argTypeIdent.Name {
						return structType, true
					}
				}
			}
		}
	}
	return nil, false
}

func matchSelectorIdents(expr ast.Expr, pkg, name string, star bool) bool {
	if star {
		st, ok := expr.(*ast.StarExpr)
		if !ok {
			return false
		}
		expr = st.X
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && sel.Sel.Name == name && id.Name == pkg
}

func pathValueField(name string) *ast.Field {
	return &ast.Field{
		Type:  ast.NewIdent("string"),
		Names: []*ast.Ident{ast.NewIdent(name)},
	}
}

func contextContextField() *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierContext)},
		Type:  contextContextType(),
	}
}

func httpResponseField() *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)},
		Type:  &ast.SelectorExpr{X: ast.NewIdent(httpPackageIdent), Sel: ast.NewIdent(httpResponseWriterIdent)},
	}
}

func routesFuncType(receiverType ast.Expr) *ast.FuncType {
	return &ast.FuncType{Params: &ast.FieldList{
		List: []*ast.Field{
			{Names: []*ast.Ident{ast.NewIdent(muxVarIdent)}, Type: &ast.StarExpr{
				X: &ast.SelectorExpr{X: ast.NewIdent(httpPackageIdent), Sel: ast.NewIdent(httpServeMuxIdent)},
			}},
			{Names: []*ast.Ident{ast.NewIdent(receiverIdent)}, Type: receiverType},
		},
	}}
}

func urlValuesField(ident string) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(ident)},
		Type:  &ast.SelectorExpr{X: ast.NewIdent("url"), Sel: ast.NewIdent("Values")},
	}
}

func httpRequestField() *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest)},
		Type:  &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent(httpPackageIdent), Sel: ast.NewIdent(httpRequestIdent)}},
	}
}

func httpHandlerFuncType() *ast.FuncType {
	return &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{httpResponseField(), httpRequestField()}}}
}

func callReceiverMethod(fun *ast.Ident) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: ast.NewIdent(receiverIdent), Sel: ast.NewIdent(fun.Name)}
}

func contextContextType() *ast.SelectorExpr {
	return &ast.SelectorExpr{X: ast.NewIdent(contextPackageIdent), Sel: ast.NewIdent(contextContextTypeIdent)}
}

func contextAssignment() *ast.AssignStmt {
	return &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{ast.NewIdent(TemplateNameScopeIdentifierContext)},
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
				Sel: ast.NewIdent(httpRequestContextMethod),
			},
		}},
	}
}

func formDeclaration(ident string, typeExp ast.Expr) *ast.DeclStmt {
	if matchSelectorIdents(typeExp, "url", "Values", false) {
		return &ast.DeclStmt{
			Decl: &ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names: []*ast.Ident{ast.NewIdent(ident)},
						Type:  typeExp,
						Values: []ast.Expr{
							&ast.SelectorExpr{X: ast.NewIdent(httpResponseField().Names[0].Name), Sel: ast.NewIdent("Form")},
						},
					},
				},
			},
		}
	}

	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(ident)},
					Type:  typeExp,
				},
			},
		},
	}
}

func httpPathValueAssignment(method *ast.FuncType, i int, arg *ast.Ident, errVarIdent string, str ast.Expr, assignTok token.Token, errCheck *ast.IfStmt) ([]ast.Stmt, []*ast.ImportSpec, error) {
	for typeIndex, typeExp := range source.IterateFieldTypes(method.Params.List) {
		if typeIndex != i {
			continue
		}
		return parseStringStatements(arg.Name, ast.NewIdent(arg.Name), errVarIdent, str, typeExp, assignTok, errCheck)
	}
	return nil, nil, fmt.Errorf("type for argumement %d not found", i)
}

func parseStringStatements(name string, result ast.Expr, errVarIdent string, str, typeExp ast.Expr, assignTok token.Token, errCheck *ast.IfStmt) ([]ast.Stmt, []*ast.ImportSpec, error) {
	const parsedVarSuffix = "Parsed"
	paramTypeIdent, ok := typeExp.(*ast.Ident)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported type: %s", source.Format(typeExp))
	}
	base10 := source.Int(10)
	switch paramTypeIdent.Name {
	default:
		return nil, nil, fmt.Errorf("method param type %s not supported", source.Format(typeExp))
	case "bool":
		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result, ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("ParseBool"),
				},
				Args: []ast.Expr{str},
			}},
		}

		return []ast.Stmt{assign, errCheck}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "int":
		tmp := name + parsedVarSuffix

		parse := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp), ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("Atoi"),
				},
				Args: []ast.Expr{str},
			}},
		}

		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  ast.NewIdent(paramTypeIdent.Name),
				Args: []ast.Expr{ast.NewIdent(tmp)},
			}},
		}

		return []ast.Stmt{parse, errCheck, assign}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "int16":
		tmp := name + parsedVarSuffix

		parse := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp), ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("ParseInt"),
				},
				Args: []ast.Expr{str, base10, source.Int(16)},
			}},
		}

		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  ast.NewIdent(paramTypeIdent.Name),
				Args: []ast.Expr{ast.NewIdent(tmp)},
			}},
		}

		return []ast.Stmt{parse, errCheck, assign}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "int32":
		tmp := name + parsedVarSuffix

		parse := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp), ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("ParseInt"),
				},
				Args: []ast.Expr{str, base10, source.Int(32)},
			}},
		}

		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  ast.NewIdent(paramTypeIdent.Name),
				Args: []ast.Expr{ast.NewIdent(tmp)},
			}},
		}

		return []ast.Stmt{parse, errCheck, assign}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "int8":
		tmp := name + parsedVarSuffix

		parse := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp), ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("ParseInt"),
				},
				Args: []ast.Expr{str, base10, source.Int(8)},
			}},
		}

		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  ast.NewIdent(paramTypeIdent.Name),
				Args: []ast.Expr{ast.NewIdent(tmp)},
			}},
		}

		return []ast.Stmt{parse, errCheck, assign}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "int64":
		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result, ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("ParseInt"),
				},
				Args: []ast.Expr{str, base10, source.Int(64)},
			}},
		}

		return []ast.Stmt{assign, errCheck}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "uint":
		tmp := name + parsedVarSuffix

		parse := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp), ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("ParseUint"),
				},
				Args: []ast.Expr{str, base10, source.Int(64)},
			}},
		}

		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  ast.NewIdent(paramTypeIdent.Name),
				Args: []ast.Expr{ast.NewIdent(tmp)},
			}},
		}

		return []ast.Stmt{parse, errCheck, assign}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "uint16":
		tmp := name + parsedVarSuffix

		parse := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp), ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("ParseUint"),
				},
				Args: []ast.Expr{str, base10, source.Int(16)},
			}},
		}

		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  ast.NewIdent(paramTypeIdent.Name),
				Args: []ast.Expr{ast.NewIdent(tmp)},
			}},
		}

		return []ast.Stmt{parse, errCheck, assign}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "uint32":
		tmp := name + parsedVarSuffix

		parse := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp), ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("ParseUint"),
				},
				Args: []ast.Expr{str, base10, source.Int(32)},
			}},
		}

		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  ast.NewIdent(paramTypeIdent.Name),
				Args: []ast.Expr{ast.NewIdent(tmp)},
			}},
		}

		return []ast.Stmt{parse, errCheck, assign}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "uint64":

		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result, ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("ParseUint"),
				},
				Args: []ast.Expr{str, base10, source.Int(64)},
			}},
		}

		return []ast.Stmt{assign, errCheck}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "uint8":
		tmp := name + parsedVarSuffix

		parse := &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp), ast.NewIdent(errVarIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent("strconv"),
					Sel: ast.NewIdent("ParseUint"),
				},
				Args: []ast.Expr{str, base10, source.Int(8)},
			}},
		}

		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  ast.NewIdent(paramTypeIdent.Name),
				Args: []ast.Expr{ast.NewIdent(tmp)},
			}},
		}

		return []ast.Stmt{parse, errCheck, assign}, []*ast.ImportSpec{importSpec("strconv")}, nil
	case "string":
		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{str},
		}
		return []ast.Stmt{assign}, nil, nil
	}
}

func (def TemplateName) executeCall(status, data ast.Expr, writeHeader bool) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: ast.NewIdent(executeIdentName),
		Args: []ast.Expr{
			ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
			ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
			ast.NewIdent(strconv.FormatBool(writeHeader)),
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(def.name)},
			status,
			data,
		},
	}}
}

func (def TemplateName) httpRequestReceiverTemplateHandlerFunc() *ast.FuncLit {
	return &ast.FuncLit{
		Type: httpHandlerFuncType(),
		Body: &ast.BlockStmt{List: []ast.Stmt{def.executeCall(source.HTTPStatusCode(httpPackageIdent, http.StatusOK), ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest), true)}},
	}
}

func (def TemplateName) matchReceiver(funcDecl *ast.FuncDecl, receiverTypeIdent string) bool {
	if funcDecl == nil || funcDecl.Name == nil || funcDecl.Name.Name != def.fun.Name ||
		funcDecl.Recv == nil || len(funcDecl.Recv.List) < 1 {
		return false
	}
	exp := funcDecl.Recv.List[0].Type
	if star, ok := exp.(*ast.StarExpr); ok {
		exp = star.X
	}
	ident, ok := exp.(*ast.Ident)
	return ok && ident.Name == receiverTypeIdent
}

func executeFuncDecl(templatesVariableIdent string) *ast.FuncDecl {
	const writeHeaderIdent = "writeHeader"
	return &ast.FuncDecl{
		Name: ast.NewIdent(executeIdentName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					httpResponseField(),
					httpRequestField(),
					{Names: []*ast.Ident{ast.NewIdent(writeHeaderIdent)}, Type: ast.NewIdent("bool")},
					{Names: []*ast.Ident{ast.NewIdent("name")}, Type: ast.NewIdent("string")},
					{Names: []*ast.Ident{ast.NewIdent("code")}, Type: ast.NewIdent("int")},
					{Names: []*ast.Ident{ast.NewIdent(dataVarIdent)}, Type: ast.NewIdent("any")},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent("buf")},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("bytes"),
							Sel: ast.NewIdent("NewBuffer"),
						},
						Args: []ast.Expr{ast.NewIdent("nil")},
					}},
				},
				&ast.IfStmt{
					Init: &ast.AssignStmt{
						Lhs: []ast.Expr{ast.NewIdent("err")},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{&ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   ast.NewIdent(templatesVariableIdent),
								Sel: ast.NewIdent("ExecuteTemplate"),
							},
							Args: []ast.Expr{ast.NewIdent("buf"), ast.NewIdent("name"), ast.NewIdent(dataVarIdent)},
						}},
					},
					Cond: &ast.BinaryExpr{
						X:  ast.NewIdent("err"),
						Op: token.NEQ,
						Y:  ast.NewIdent("nil"),
					},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.ExprStmt{X: &ast.CallExpr{
								Fun: &ast.SelectorExpr{
									X:   ast.NewIdent(httpPackageIdent),
									Sel: ast.NewIdent("Error"),
								},
								Args: []ast.Expr{
									ast.NewIdent(httpResponseField().Names[0].Name),
									&ast.CallExpr{
										Fun: &ast.SelectorExpr{
											X:   ast.NewIdent("err"),
											Sel: ast.NewIdent("Error"),
										},
										Args: []ast.Expr{},
									},
									source.HTTPStatusCode(httpPackageIdent, http.StatusInternalServerError),
								},
							}},
							&ast.ReturnStmt{},
						},
					},
				},
				&ast.IfStmt{
					Cond: ast.NewIdent(writeHeaderIdent),
					Body: &ast.BlockStmt{List: []ast.Stmt{
						&ast.ExprStmt{
							X: &ast.CallExpr{
								Fun: &ast.SelectorExpr{
									X:   ast.NewIdent(httpResponseField().Names[0].Name),
									Sel: ast.NewIdent("WriteHeader"),
								},
								Args: []ast.Expr{ast.NewIdent("code")},
							},
						},
					},
					}},
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("_")},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("buf"),
							Sel: ast.NewIdent("WriteTo"),
						},
						Args: []ast.Expr{ast.NewIdent(httpResponseField().Names[0].Name)},
					}},
				},
			},
		},
	}
}

func sortImports(input []*ast.ImportSpec) []*ast.ImportSpec {
	slices.SortFunc(input, func(a, b *ast.ImportSpec) int { return strings.Compare(a.Path.Value, b.Path.Value) })
	return slices.CompactFunc(input, func(a, b *ast.ImportSpec) bool { return a.Path.Value == b.Path.Value })
}
