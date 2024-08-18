package muxt

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"strconv"

	"github.com/crhntr/muxt/internal/source"
)

const (
	executeIdentName = "execute"
	receiverIdent    = "receiver"

	receiverInterfaceIdent = "RoutesReceiver"

	dataVarIdent = "data"
	muxVarIdent  = "mux"

	requestPathValue         = "PathValue"
	templatesLookup          = "Lookup"
	httpRequestContextMethod = "Context"
	httpPackageIdent         = "http"
	httpResponseWriterIdent  = "ResponseWriter"
	httpServeMuxIdent        = "ServeMux"
	httpRequestIdent         = "Request"
	httpStatusCode200Ident   = "StatusOK"
	httpHandleFuncIdent      = "HandleFunc"

	contextPackageIdent     = "context"
	contextContextTypeIdent = "Context"

	stringTypeIdent = "string"

	defaultPackageName           = "main"
	DefaultTemplatesVariableName = "templates"
	DefaultRoutesFunctionName    = "Routes"
)

func Generate(patterns []Pattern, packageName, templatesVariableName, routesFunctionName, receiverTypeIdent string, fileSet *token.FileSet, receiverPackage, templatesPackage []*ast.File, log *log.Logger) (string, error) {
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
	for _, pattern := range patterns {
		var method *ast.FuncType
		if pattern.fun != nil {
			for _, funcDecl := range source.IterateFunctions(receiverPackage) {
				if !pattern.matchReceiver(funcDecl, receiverTypeIdent) {
					continue
				}
				method = funcDecl.Type
			}
			if method == nil {
				me, im := pattern.funcType()
				method = me
				imports = append(imports, im...)
			}
			receiverInterface.Methods.List = append(receiverInterface.Methods.List, &ast.Field{
				Names: []*ast.Ident{ast.NewIdent(pattern.fun.Name)},
				Type:  method,
			})
		}
		handlerFunc, handerSignatureImports, err := pattern.funcLit(templatesVariableName, method)
		if err != nil {
			return "", err
		}
		imports = source.SortImports(append(imports, handerSignatureImports...))
		routes.Body.List = append(routes.Body.List, pattern.callHandleFunc(handlerFunc))
		log.Printf("%s has route for %s", routesFunctionName, pattern.String())
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
	var (
		hasExecuteFunc = false
	)
	for _, fn := range source.IterateFunctions(templatesPackage) {
		if fn.Recv == nil && fn.Name.Name == executeIdentName {
			hasExecuteFunc = true
		}
	}
	if !hasExecuteFunc {
		file.Decls = append(file.Decls, executeFuncDecl())
		imports = append(imports, importSpec("bytes"), importSpec("html/template"))
	}
	for _, imp := range imports {
		importGen.Specs = append(importGen.Specs, imp)
	}
	return formatNode(file), nil
}

func (def Pattern) callHandleFunc(handlerFuncLit *ast.FuncLit) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(muxVarIdent),
			Sel: ast.NewIdent(httpHandleFuncIdent),
		},
		Args: []ast.Expr{
			&ast.BasicLit{
				Kind:  token.STRING,
				Value: strconv.Quote(def.Route),
			},
			handlerFuncLit,
		},
	}}
}

func (def Pattern) funcLit(templatesVariableIdent string, method *ast.FuncType) (*ast.FuncLit, []*ast.ImportSpec, error) {
	if def.Handler == "" {
		return def.httpRequestReceiverTemplateHandlerFunc(templatesVariableIdent), nil, nil
	}
	lit := &ast.FuncLit{
		Type: httpHandlerFuncType(),
		Body: &ast.BlockStmt{},
	}
	call := &ast.CallExpr{Fun: callReceiverMethod(def.fun)}
	if method != nil {
		if method.Params.NumFields() != len(def.args) {
			return nil, nil, errWrongNumberOfArguments(def, method)
		}
		for pi, pt := range fieldListTypes(method.Params) {
			if err := checkArgument(def.args[pi], pt); err != nil {
				return nil, nil, err
			}
		}
	}
	var imports []*ast.ImportSpec
	for _, arg := range def.args {
		switch arg.Name {
		case PatternScopeIdentifierHTTPRequest, PatternScopeIdentifierHTTPResponse:
			call.Args = append(call.Args, ast.NewIdent(arg.Name))
			imports = append(imports, importSpec("net/http"))
		case PatternScopeIdentifierContext:
			lit.Body.List = append(lit.Body.List, contextAssignment())
			call.Args = append(call.Args, ast.NewIdent(PatternScopeIdentifierContext))
			imports = append(imports, importSpec("context"))
		default:
			lit.Body.List = append(lit.Body.List, httpPathValueAssignment(arg))
			call.Args = append(call.Args, ast.NewIdent(arg.Name))
		}
	}

	if method != nil && len(method.Results.List) > 1 {
		dataVar := ast.NewIdent(dataVarIdent)
		errVar := ast.NewIdent("err")

		lit.Body.List = append(lit.Body.List,
			&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVar.Name), ast.NewIdent(errVar.Name)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
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
								httpStatusCode(httpStatusCode200Ident),
							}}},
						&ast.ReturnStmt{},
					},
				},
			},
		)
	}

	data := ast.NewIdent(dataVarIdent)
	lit.Body.List = append(lit.Body.List, &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(data.Name)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}})
	lit.Body.List = append(lit.Body.List, def.executeCall(ast.NewIdent(templatesVariableIdent), httpStatusCode(httpStatusCode200Ident), data))
	return lit, imports, nil
}

func (def Pattern) funcType() (*ast.FuncType, []*ast.ImportSpec) {
	method := &ast.FuncType{
		Params:  &ast.FieldList{},
		Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
	}
	var imports []*ast.ImportSpec
	for _, arg := range def.args {
		switch arg.Name {
		case PatternScopeIdentifierHTTPRequest:
			method.Params.List = append(method.Params.List, httpRequestField())
			imports = append(imports, importSpec("net/"+httpPackageIdent))
		case PatternScopeIdentifierHTTPResponse:
			method.Params.List = append(method.Params.List, httpResponseField())
			imports = append(imports, importSpec("net/"+httpPackageIdent))
		case PatternScopeIdentifierContext:
			method.Params.List = append(method.Params.List, contextContextField())
			imports = append(imports, importSpec(contextPackageIdent))
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

func errWrongNumberOfArguments(def Pattern, method *ast.FuncType) error {
	return fmt.Errorf("handler %s expects %d arguments but call %s has %d", formatNode(&ast.FuncDecl{Name: ast.NewIdent(def.fun.Name), Type: method}), method.Params.NumFields(), def.Handler, len(def.args))
}

func checkArgument(exp ast.Expr, tp ast.Expr) error {
	arg := exp.(*ast.Ident)
	switch arg.Name {
	case PatternScopeIdentifierHTTPRequest:
		if !matchSelectorIdents(tp, httpPackageIdent, httpRequestIdent, true) {
			return fmt.Errorf("method expects type %s but %s is *%s.%s", formatNode(tp), arg.Name, httpPackageIdent, httpRequestIdent)
		}
		return nil
	case PatternScopeIdentifierHTTPResponse:
		if !matchSelectorIdents(tp, httpPackageIdent, httpResponseWriterIdent, false) {
			return fmt.Errorf("method expects type %s but %s is %s.%s", formatNode(tp), arg.Name, httpPackageIdent, httpResponseWriterIdent)
		}
		return nil
	case PatternScopeIdentifierContext:
		if !matchSelectorIdents(tp, contextPackageIdent, contextContextTypeIdent, false) {
			return fmt.Errorf("method expects type %s but %s is %s.%s", formatNode(tp), arg.Name, contextPackageIdent, contextContextTypeIdent)
		}
		return nil
	default:
		ident, ok := tp.(*ast.Ident)
		if !ok || ident.Name != stringTypeIdent {
			return fmt.Errorf("method expects type %s but %s is a string", formatNode(tp), arg.Name)
		}
		return nil
	}
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
		Names: []*ast.Ident{ast.NewIdent(PatternScopeIdentifierContext)},
		Type:  contextContextType(),
	}
}

func httpResponseField() *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(PatternScopeIdentifierHTTPResponse)},
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

func httpRequestField() *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(PatternScopeIdentifierHTTPRequest)},
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

func templateTemplateField() *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent("t")},
		Type:  &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent("template"), Sel: ast.NewIdent("Template")}},
	}
}

func httpStatusCode(name string) *ast.SelectorExpr {
	return &ast.SelectorExpr{
		X:   ast.NewIdent(httpPackageIdent),
		Sel: ast.NewIdent(name),
	}
}

func contextAssignment() *ast.AssignStmt {
	return &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{ast.NewIdent(PatternScopeIdentifierContext)},
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(PatternScopeIdentifierHTTPRequest),
				Sel: ast.NewIdent(httpRequestContextMethod),
			},
		}},
	}
}

func httpPathValueAssignment(arg *ast.Ident) *ast.AssignStmt {
	return &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{ast.NewIdent(arg.Name)},
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(PatternScopeIdentifierHTTPRequest),
				Sel: ast.NewIdent(requestPathValue),
			},
			Args: []ast.Expr{
				&ast.BasicLit{
					Kind:  token.STRING,
					Value: strconv.Quote(arg.Name),
				},
			},
		}},
	}
}

func (def Pattern) executeCall(templatesVariable *ast.Ident, status, data ast.Expr) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: ast.NewIdent(executeIdentName),
		Args: []ast.Expr{
			ast.NewIdent(PatternScopeIdentifierHTTPResponse),
			ast.NewIdent(PatternScopeIdentifierHTTPRequest),
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(templatesVariable.Name),
					Sel: ast.NewIdent(templatesLookup),
				},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(def.name)}},
			},
			status,
			data,
		},
	}}
}

func (def Pattern) httpRequestReceiverTemplateHandlerFunc(templatesVariableName string) *ast.FuncLit {
	return &ast.FuncLit{
		Type: httpHandlerFuncType(),
		Body: &ast.BlockStmt{List: []ast.Stmt{def.executeCall(ast.NewIdent(templatesVariableName), httpStatusCode(httpStatusCode200Ident), ast.NewIdent(PatternScopeIdentifierHTTPRequest))}},
	}
}

func (def Pattern) matchReceiver(funcDecl *ast.FuncDecl, receiverTypeIdent string) bool {
	return funcDecl.Name.Name == def.fun.Name && funcDecl.Recv != nil && len(funcDecl.Recv.List) == 1 && funcDecl.Recv.List[0].Type.(*ast.Ident).Name == receiverTypeIdent
}

func executeFuncDecl() *ast.FuncDecl {
	return &ast.FuncDecl{
		Name: ast.NewIdent(executeIdentName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					httpResponseField(),
					httpRequestField(),
					templateTemplateField(),
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
								X:   ast.NewIdent("t"),
								Sel: ast.NewIdent("Execute"),
							},
							Args: []ast.Expr{ast.NewIdent("buf"), ast.NewIdent(dataVarIdent)},
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
									httpStatusCode(httpStatusCode200Ident),
								}}},
							&ast.ReturnStmt{},
						},
					},
				},
				&ast.ExprStmt{X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent(httpResponseField().Names[0].Name),
						Sel: ast.NewIdent("WriteHeader"),
					},
					Args: []ast.Expr{ast.NewIdent("code")}},
				},
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("_")},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("buf"),
							Sel: ast.NewIdent("WriteTo"),
						},
						Args: []ast.Expr{ast.NewIdent(httpResponseField().Names[0].Name)},
					},
					},
				},
			},
		},
	}
}
