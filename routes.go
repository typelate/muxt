package muxt

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/crhntr/dom"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/tools/go/packages"

	"github.com/crhntr/muxt/internal/source"
)

const (
	executeIdentName = "execute"
	receiverIdent    = "receiver"

	dataVarIdent = "data"
	muxVarIdent  = "mux"

	requestPathValue         = "PathValue"
	httpRequestContextMethod = "Context"
	httpResponseWriterIdent  = "ResponseWriter"
	httpServeMuxIdent        = "ServeMux"
	httpRequestIdent         = "Request"
	httpHandleFuncIdent      = "HandleFunc"

	contextContextTypeIdent = "Context"

	defaultPackageName           = "main"
	DefaultTemplatesVariableName = "templates"
	DefaultRoutesFunctionName    = "routes"
	DefaultOutputFileName        = "template_routes.go"
	DefaultReceiverInterfaceName = "RoutesReceiver"

	InputAttributeNameStructTag     = "name"
	InputAttributeTemplateStructTag = "template"

	errIdent = "err"
)

func Routes(templates []Template, packageName, templatesVariableName, routesFunctionName, receiverTypeIdent, receiverInterfaceIdent, output string, packageList []*packages.Package, log *log.Logger) (string, error) {
	packageName = cmp.Or(packageName, defaultPackageName)
	templatesVariableName = cmp.Or(templatesVariableName, DefaultTemplatesVariableName)
	routesFunctionName = cmp.Or(routesFunctionName, DefaultRoutesFunctionName)
	receiverInterfaceIdent = cmp.Or(receiverInterfaceIdent, DefaultReceiverInterfaceName)

	imports := source.NewImports(&ast.GenDecl{Tok: token.IMPORT})
	var (
		receiverType    *types.Named
		pkg             *packages.Package
		receiverPackage []*ast.File
	)
	for _, p := range packageList {
		if p.Types.Name() == packageName {
			pkg = p
			receiverPackage = pkg.Syntax
			break
		}
	}
	if pkg != nil {
		receiverTypeObj := pkg.Types.Scope().Lookup(receiverTypeIdent)
		if receiverTypeObj != nil {
			named, ok := receiverTypeObj.Type().(*types.Named)
			if ok {
				receiverType = named
			}
		}
	}
	receiverInterface := receiverInterfaceType(imports, packageList, receiverType, templates)
	routesFunc, err := routesFuncDeclaration(imports, routesFunctionName, receiverInterfaceIdent, receiverInterface, receiverPackage, templates, log)
	if err != nil {
		return "", err
	}

	typesDecl := &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{Name: ast.NewIdent(receiverInterfaceIdent), Type: receiverInterface},
		},
	}

	imports.SortImports()
	file := &ast.File{
		Name: ast.NewIdent(packageName),
		Decls: []ast.Decl{
			imports.GenDecl,
			typesDecl,
			routesFunc,
		},
	}
	if pkg != nil {
		if obj := pkg.Types.Scope().Lookup(executeIdentName); obj == nil {
			file.Decls = append(file.Decls, executeFuncDecl(imports, templatesVariableName))
		} else {
			pos := pkg.Fset.Position(obj.Pos())
			if filepath.Base(pos.Filename) == output {
				file.Decls = append(file.Decls, executeFuncDecl(imports, templatesVariableName))
			}
		}
	} else {
		file.Decls = append(file.Decls, executeFuncDecl(imports, templatesVariableName))
	}

	return source.Format(file), nil
}

func routesFuncDeclaration(imports *source.Imports, routesFunctionName, receiverInterfaceIdent string, receiverInterfaceType *ast.InterfaceType, receiverPackage []*ast.File, templateNames []Template, log *log.Logger) (*ast.FuncDecl, error) {
	routes := &ast.FuncDecl{
		Name: ast.NewIdent(routesFunctionName),
		Type: routesFuncType(imports, ast.NewIdent(receiverInterfaceIdent)),
		Body: &ast.BlockStmt{},
	}

	for _, tn := range templateNames {
		log.Printf("%s has route for %s", routesFunctionName, tn.endpoint)
		if tn.fun == nil {
			hf := tn.httpRequestReceiverTemplateHandlerFunc(imports, tn.statusCode)
			routes.Body.List = append(routes.Body.List, tn.callHandleFunc(hf))
			continue
		}

		hf, err := tn.funcLit(imports, receiverInterfaceType, receiverPackage)
		if err != nil {
			return nil, err
		}
		routes.Body.List = append(routes.Body.List, tn.callHandleFunc(hf))
	}

	return routes, nil
}

func receiverInterfaceType(imports *source.Imports, packageList []*packages.Package, receiverType *types.Named, templateNames []Template) *ast.InterfaceType {
	interfaceMethods := new(ast.FieldList)
	for _, tn := range templateNames {
		if tn.fun == nil {
			continue
		}
		for _, arg := range tn.call.Args {
			switch exp := arg.(type) {
			case *ast.CallExpr:
				callIdent, ok := exp.Fun.(*ast.Ident)
				if !ok {
					continue
				}
				method, ok := interfaceMethodForCall(imports, interfaceMethods, callIdent, exp, receiverType, packageList)
				if !ok {
					continue
				}
				interfaceMethods.List = append(interfaceMethods.List, method)
			}
		}
		if source.HasFieldWithName(interfaceMethods, tn.fun.Name) {
			continue
		}
		method, ok := interfaceMethodForCall(imports, interfaceMethods, tn.fun, tn.call, receiverType, packageList)
		if !ok {
			continue
		}
		interfaceMethods.List = append(interfaceMethods.List, method)
	}
	return &ast.InterfaceType{Methods: interfaceMethods}
}

func interfaceMethodForCall(imports *source.Imports, interfaceMethods *ast.FieldList, methodName *ast.Ident, call *ast.CallExpr, receiverType *types.Named, packageList []*packages.Package) (*ast.Field, bool) {
	if source.HasFieldWithName(interfaceMethods, methodName.Name) {
		return nil, false
	}
	if receiverType != nil {
		obj, _, _ := types.LookupFieldOrMethod(receiverType, true, receiverType.Obj().Pkg(), methodName.Name)
		if obj != nil {
			fn, ok := obj.(*types.Func)
			if !ok {
				return nil, false
			}
			for _, pkg := range packageList {
				if pkg.PkgPath != obj.Pkg().Path() {
					continue
				}
				for _, file := range pkg.Syntax {
					for _, decl := range file.Decls {
						fd, ok := decl.(*ast.FuncDecl)
						if !ok || fd.Name.Pos() != fn.Pos() {
							continue
						}
						return &ast.Field{
							Names: []*ast.Ident{ast.NewIdent(fd.Name.Name)},
							Type:  fd.Type,
						}, true
					}
				}
			}
		}
	}
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(methodName.Name)},
		Type:  generateFuncTypeFromArguments(imports, call),
	}, true
}

func (t Template) callHandleFunc(handlerFuncLit *ast.FuncLit) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(muxVarIdent),
			Sel: ast.NewIdent(httpHandleFuncIdent),
		},
		Args: []ast.Expr{source.String(t.endpoint), handlerFuncLit},
	}}
}

func (t Template) funcLit(imports *source.Imports, receiverInterfaceType *ast.InterfaceType, files []*ast.File) (*ast.FuncLit, error) {
	methodField, ok := source.FindFieldWithName(receiverInterfaceType.Methods, t.fun.Name)
	if !ok {
		log.Fatalf("receiver does not have a method declaration for %s", t.fun.Name)
	}
	method := methodField.Type.(*ast.FuncType)
	lit := &ast.FuncLit{
		Type: httpHandlerFuncType(imports),
		Body: &ast.BlockStmt{},
	}
	call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(receiverIdent), Sel: ast.NewIdent(t.fun.Name)}}
	if method.Params.NumFields() != len(t.call.Args) {
		return nil, errWrongNumberOfArguments(t, method)
	}
	argTypes := slices.Collect(fieldListTypes(method.Params))
	resultArgCount := 0
	var handledIdents []string
	for i, arg := range t.call.Args {
		argType := argTypes[i]
		switch exp := arg.(type) {
		case *ast.Ident:
			call.Args = append(call.Args, ast.NewIdent(exp.Name))
			if slices.Contains(handledIdents, exp.Name) {
				continue
			}
			statements, err := t.identifierArgument(imports, i, exp, argType, method, files)
			if err != nil {
				return nil, err
			}
			lit.Body.List = append(lit.Body.List, statements...)
			handledIdents = append(handledIdents, exp.Name)
		case *ast.CallExpr:
			callMethodIdent, ok := exp.Fun.(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("argument call expression function must be an identifier: got %s", source.Format(exp.Fun))
			}
			callMethodField, ok := source.FindFieldWithName(receiverInterfaceType.Methods, callMethodIdent.Name)
			if !ok {
				log.Fatalf("receiver does not have a method declaration for %s", callMethodIdent.Name)
			}
			callMethod := callMethodField.Type.(*ast.FuncType)
			callArgTypes := slices.Collect(fieldListTypes(callMethod.Params))
			callCall := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(receiverIdent), Sel: ast.NewIdent(callMethodIdent.Name)}}

			if aCount, pCount := len(exp.Args), callMethod.Params.NumFields(); aCount != pCount {
				return nil, fmt.Errorf("expected %d arguments for method %s with %d parameters", aCount, callMethodIdent.Name, pCount)
			}

			for j, callArg := range exp.Args {
				callArgType := callArgTypes[j]
				switch callArgExp := callArg.(type) {
				case *ast.Ident:
					callCall.Args = append(callCall.Args, ast.NewIdent(callArgExp.Name))
					if slices.Contains(handledIdents, callArgExp.Name) {
						continue
					}
					parseStatements, err := t.identifierArgument(imports, j, callArgExp, callArgType, callMethod, files)
					if err != nil {
						return nil, err
					}
					lit.Body.List = append(lit.Body.List, parseStatements...)
					handledIdents = append(handledIdents, callArgExp.Name)
				}
			}
			resultVar := "result" + strconv.Itoa(resultArgCount)

			receiverCallStatements, err := t.callReceiverMethod(imports, resultVar, callMethod, callCall)
			if err != nil {
				return nil, err
			}
			lit.Body.List = append(lit.Body.List, receiverCallStatements...)
			call.Args = append(call.Args, ast.NewIdent(resultVar))
			resultArgCount++
		}
	}
	const dataVarIdent = "data"
	receiverCallStatements, err := t.callReceiverMethod(imports, dataVarIdent, method, call)
	if err != nil {
		return nil, err
	}
	lit.Body.List = append(lit.Body.List, receiverCallStatements...)

	lit.Body.List = append(lit.Body.List, t.executeCall(source.HTTPStatusCode(imports, t.statusCode), ast.NewIdent(dataVarIdent), t.callWriteHeader(receiverInterfaceType)))
	return lit, nil
}

func (t Template) callReceiverMethod(imports *source.Imports, dataVarIdent string, method *ast.FuncType, call *ast.CallExpr) ([]ast.Stmt, error) {
	const (
		okIdent = "ok"
	)
	if method.Results == nil || len(method.Results.List) == 0 {
		return nil, fmt.Errorf("method for endpoint %q has no results it should have one or two", t)
	} else if len(method.Results.List) > 1 {
		_, lastResultType, ok := source.FieldIndex(method.Results.List, method.Results.NumFields()-1)
		if !ok {
			return nil, fmt.Errorf("failed to get the last method result")
		}
		switch rt := lastResultType.(type) {
		case *ast.Ident:
			switch rt.Name {
			case "error":
				return []ast.Stmt{
					&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
					&ast.IfStmt{
						Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: source.Nil()},
						Body: &ast.BlockStmt{
							List: []ast.Stmt{
								&ast.ExprStmt{X: imports.HTTPErrorCall(ast.NewIdent(httpResponseField(imports).Names[0].Name), source.CallError(errIdent), http.StatusInternalServerError)},
								&ast.ReturnStmt{},
							},
						},
					},
				}, nil
			case "bool":
				return []ast.Stmt{
					&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent), ast.NewIdent(okIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
					&ast.IfStmt{
						Cond: &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent(okIdent)},
						Body: &ast.BlockStmt{
							List: []ast.Stmt{
								&ast.ReturnStmt{},
							},
						},
					},
				}, nil
			default:
				return nil, fmt.Errorf("expected last result to be either an error or a bool")
			}
		default:
			return nil, fmt.Errorf("expected last result to be either an error or a bool")
		}
	} else {
		return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}}}, nil
	}
}

func (t Template) identifierArgument(imports *source.Imports, i int, arg *ast.Ident, argType ast.Expr, method *ast.FuncType, files []*ast.File) ([]ast.Stmt, error) {
	switch arg.Name {
	case TemplateNameScopeIdentifierHTTPResponse:
		if !matchSelectorIdents(argType, imports.AddNetHTTP(), httpResponseWriterIdent, false) {
			return nil, fmt.Errorf("method expects type %s but %s is %s.%s", source.Format(argType), arg.Name, imports.AddNetHTTP(), httpResponseWriterIdent)
		}
		imports.AddNetHTTP()
		return nil, nil
	case TemplateNameScopeIdentifierHTTPRequest:
		if !matchSelectorIdents(argType, imports.AddNetHTTP(), httpRequestIdent, true) {
			return nil, fmt.Errorf("method expects type %s but %s is *%s.%s", source.Format(argType), arg.Name, imports.AddNetHTTP(), httpRequestIdent)
		}
		imports.AddNetHTTP()
		return nil, nil
	case TemplateNameScopeIdentifierContext:
		if !matchSelectorIdents(argType, imports.AddContext(), contextContextTypeIdent, false) {
			return nil, fmt.Errorf("method expects type %s but %s is %s.%s", source.Format(argType), arg.Name, imports.AddContext(), contextContextTypeIdent)
		}
		imports.AddContext()
		return []ast.Stmt{contextAssignment(TemplateNameScopeIdentifierContext)}, nil
	case TemplateNameScopeIdentifierForm:
		_, tp, _ := source.FieldIndex(method.Params.List, i)
		statements := []ast.Stmt{
			&ast.ExprStmt{X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
					Sel: ast.NewIdent("ParseForm"),
				},
				Args: []ast.Expr{},
			}},
			formDeclaration(imports, arg.Name, tp),
		}
		var formStruct *ast.StructType
		if s, ok := findFormStruct(argType, files); ok {
			formStruct = s
		}
		if formStruct == nil && !matchSelectorIdents(argType, "url", "Values", false) {
			return nil, fmt.Errorf("method expects form to have type url.Values or T (where T is some struct type)")
		}
		if formStruct != nil {
			for _, field := range formStruct.Fields.List {
				for _, name := range field.Names {
					fieldExpr := &ast.SelectorExpr{
						X:   ast.NewIdent(arg.Name),
						Sel: ast.NewIdent(name.Name),
					}

					fieldTemplate := formInputTemplate(field, t.template)

					errCheck := func(exp ast.Expr) ast.Stmt {
						return &ast.ExprStmt{
							X: imports.HTTPErrorCall(ast.NewIdent(httpResponseField(imports).Names[0].Name), source.CallError(errIdent), http.StatusBadRequest),
						}
					}

					const parsedVariableName = "value"
					if fieldType, ok := field.Type.(*ast.ArrayType); ok {
						inputName := formInputName(field, name)
						const valVar = "val"
						assignment := appendAssignment(token.ASSIGN, &ast.SelectorExpr{
							X:   ast.NewIdent(arg.Name),
							Sel: ast.NewIdent(name.Name),
						})
						var templateNodes []*html.Node
						if fieldTemplate != nil {
							templateNodes, _ = html.ParseFragment(strings.NewReader(fieldTemplate.Tree.Root.String()), &html.Node{
								Type:     html.ElementNode,
								DataAtom: atom.Body,
								Data:     atom.Body.String(),
							})
						}
						validations, err, ok := source.GenerateValidations(imports, ast.NewIdent(parsedVariableName), fieldType.Elt, fmt.Sprintf("[name=%q]", inputName), inputName, httpResponseField(imports).Names[0].Name, dom.NewDocumentFragment(templateNodes))
						if ok && err != nil {
							return nil, err
						}
						loopBlockStatements, err := source.GenerateParseValueFromStringStatements(imports, parsedVariableName, ast.NewIdent(valVar), fieldType.Elt, errCheck, validations, assignment)
						if err != nil {
							return nil, fmt.Errorf("failed to generate parse statements for form field %s: %w", name.Name, err)
						}

						forLoop := &ast.RangeStmt{
							Key:   ast.NewIdent("_"),
							Value: ast.NewIdent(valVar),
							Tok:   token.DEFINE,
							X: &ast.IndexExpr{
								X: &ast.SelectorExpr{
									X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
									Sel: ast.NewIdent("Form"),
								},
								Index: source.String(inputName),
							},
							Body: &ast.BlockStmt{
								List: loopBlockStatements,
							},
						}

						statements = append(statements, forLoop)
					} else {
						assignment := singleAssignment(token.ASSIGN, fieldExpr)
						inputName := formInputName(field, name)
						str := &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
								Sel: ast.NewIdent("FormValue"),
							},
							Args: []ast.Expr{
								&ast.BasicLit{
									Kind:  token.STRING,
									Value: strconv.Quote(inputName),
								},
							},
						}
						var templateNodes []*html.Node
						if fieldTemplate != nil {
							templateNodes, _ = html.ParseFragment(strings.NewReader(fieldTemplate.Tree.Root.String()), &html.Node{
								Type:     html.ElementNode,
								DataAtom: atom.Body,
								Data:     atom.Body.String(),
							})
						}
						validations, err, ok := source.GenerateValidations(imports, ast.NewIdent(parsedVariableName), field.Type, fmt.Sprintf("[name=%q]", inputName), inputName, httpResponseField(imports).Names[0].Name, dom.NewDocumentFragment(templateNodes))
						if ok && err != nil {
							return nil, err
						}
						parseStatements, err := source.GenerateParseValueFromStringStatements(imports, parsedVariableName, str, field.Type, errCheck, validations, assignment)
						if err != nil {
							return nil, fmt.Errorf("failed to generate parse statements for form field %s: %w", name.Name, err)
						}

						if len(parseStatements) > 1 {
							parseStatements = []ast.Stmt{&ast.BlockStmt{
								List: parseStatements,
							}}
						}

						statements = append(statements, parseStatements...)
					}
				}
			}
		} else {
			imports.Add("", "net/url")
		}
		return statements, nil
	default:
		for paramIndex, paramType := range source.IterateFieldTypes(method.Params.List) {
			if i != paramIndex {
				continue
			}
			if err := compareTypes(paramType, argType); err != nil {
				return nil, fmt.Errorf("method argument and param mismatch: %w", err)
			}
			break
		}
		errCheck := func(msg ast.Expr) ast.Stmt {
			return &ast.ExprStmt{
				X: imports.HTTPErrorCall(ast.NewIdent(httpResponseField(imports).Names[0].Name), msg, http.StatusBadRequest),
			}
		}
		src := &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
				Sel: ast.NewIdent(requestPathValue),
			},
			Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(arg.Name)}},
		}
		statements, err := httpPathValueAssignment(imports, method, i, arg, src, token.DEFINE, errCheck)
		if err != nil {
			return nil, err
		}
		return statements, nil
	}
}

func formInputName(field *ast.Field, name *ast.Ident) string {
	if field.Tag != nil {
		v, _ := strconv.Unquote(field.Tag.Value)
		tags := reflect.StructTag(v)
		n, hasInputTag := tags.Lookup(InputAttributeNameStructTag)
		if hasInputTag {
			return n
		}
	}
	return name.Name
}

func formInputTemplate(field *ast.Field, t *template.Template) *template.Template {
	if field.Tag != nil {
		v, _ := strconv.Unquote(field.Tag.Value)
		tags := reflect.StructTag(v)
		n, hasInputTag := tags.Lookup(InputAttributeTemplateStructTag)
		if hasInputTag {
			return t.Lookup(n)
		}
	}
	return t
}

func generateFuncTypeFromArguments(imports *source.Imports, call *ast.CallExpr) *ast.FuncType {
	method := &ast.FuncType{
		Params:  &ast.FieldList{},
		Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
	}
	for _, a := range call.Args {
		arg, ok := a.(*ast.Ident)
		if !ok {
			continue
		}
		switch arg.Name {
		case TemplateNameScopeIdentifierHTTPRequest:
			method.Params.List = append(method.Params.List, httpRequestField(imports))
		case TemplateNameScopeIdentifierHTTPResponse:
			method.Params.List = append(method.Params.List, httpResponseField(imports))
		case TemplateNameScopeIdentifierContext:
			method.Params.List = append(method.Params.List, contextContextField(imports))
		case TemplateNameScopeIdentifierForm:
			method.Params.List = append(method.Params.List, urlValuesField(imports, arg.Name))
		default:
			method.Params.List = append(method.Params.List, pathValueField(arg.Name))
		}
	}
	return method
}

func fieldListTypes(fieldList *ast.FieldList) func(func(ast.Expr) bool) {
	return func(yield func(ast.Expr) bool) {
		for _, param := range fieldList.List {
			if len(param.Names) == 0 {
				if !yield(param.Type) {
					return
				}
				continue
			}
			for range param.Names {
				if !yield(param.Type) {
					return
				}
			}
		}
	}
}

func errWrongNumberOfArguments(def Template, method *ast.FuncType) error {
	return fmt.Errorf("handler %s expects %d arguments but call %s has %d", source.Format(&ast.FuncDecl{Name: ast.NewIdent(def.fun.Name), Type: method}), method.Params.NumFields(), def.handler, len(def.call.Args))
}

func compareTypes(expA, expB ast.Expr) error {
	if a, b, ok := matchExpressionType[*ast.Ident](expA, expB); ok && a.Name == b.Name {
		return nil
	}
	if a, b, ok := matchExpressionType[*ast.SelectorExpr](expA, expB); ok && a.Sel == b.Sel {
		if _, _, ok = matchExpressionType[*ast.Ident](a.X, b.X); ok {
			return nil
		}
	}
	return fmt.Errorf("type %s is not assignable to %s", source.Format(expA), source.Format(expB))
}

func matchExpressionType[T ast.Expr](a, b ast.Expr) (T, T, bool) {
	ax, aOk := a.(T)
	bx, bOk := b.(T)
	return ax, bx, aOk && bOk
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

func contextContextField(imports *source.Imports) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierContext)},
		Type:  contextContextType(imports.AddContext()),
	}
}

func httpResponseField(imports *source.Imports) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)},
		Type:  &ast.SelectorExpr{X: ast.NewIdent(imports.AddNetHTTP()), Sel: ast.NewIdent(httpResponseWriterIdent)},
	}
}

func routesFuncType(imports *source.Imports, receiverType ast.Expr) *ast.FuncType {
	return &ast.FuncType{Params: &ast.FieldList{
		List: []*ast.Field{
			{Names: []*ast.Ident{ast.NewIdent(muxVarIdent)}, Type: &ast.StarExpr{
				X: &ast.SelectorExpr{X: ast.NewIdent(imports.AddNetHTTP()), Sel: ast.NewIdent(httpServeMuxIdent)},
			}},
			{Names: []*ast.Ident{ast.NewIdent(receiverIdent)}, Type: receiverType},
		},
	}}
}

func urlValuesField(imports *source.Imports, ident string) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(ident)},
		Type:  &ast.SelectorExpr{X: ast.NewIdent(imports.Add("", "net/url")), Sel: ast.NewIdent("Values")},
	}
}

func httpRequestField(imports *source.Imports) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest)},
		Type:  &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent(imports.AddNetHTTP()), Sel: ast.NewIdent(httpRequestIdent)}},
	}
}

func httpHandlerFuncType(imports *source.Imports) *ast.FuncType {
	return &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{httpResponseField(imports), httpRequestField(imports)}}}
}

func contextContextType(contextPackageIdent string) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: ast.NewIdent(contextPackageIdent), Sel: ast.NewIdent(contextContextTypeIdent)}
}

func contextAssignment(ident string) *ast.AssignStmt {
	return &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{ast.NewIdent(ident)},
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
				Sel: ast.NewIdent(httpRequestContextMethod),
			},
		}},
	}
}

func formDeclaration(imports *source.Imports, ident string, typeExp ast.Expr) *ast.DeclStmt {
	if matchSelectorIdents(typeExp, imports.Ident("net/url"), "Values", false) {
		imports.Add("", "net/url")
		return &ast.DeclStmt{
			Decl: &ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{
					&ast.ValueSpec{
						Names: []*ast.Ident{ast.NewIdent(ident)},
						Type:  typeExp,
						Values: []ast.Expr{
							&ast.SelectorExpr{X: ast.NewIdent(httpRequestField(imports).Names[0].Name), Sel: ast.NewIdent("Form")},
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

func httpPathValueAssignment(imports *source.Imports, method *ast.FuncType, i int, arg *ast.Ident, str ast.Expr, assignTok token.Token, errCheck func(stmt ast.Expr) ast.Stmt) ([]ast.Stmt, error) {
	for typeIndex, typeExp := range source.IterateFieldTypes(method.Params.List) {
		if typeIndex != i {
			continue
		}
		assignment := singleAssignment(assignTok, ast.NewIdent(arg.Name))
		return source.GenerateParseValueFromStringStatements(imports, arg.Name+"Parsed", str, typeExp, errCheck, nil, assignment)
	}
	return nil, fmt.Errorf("type for argumement %d not found", i)
}

func singleAssignment(assignTok token.Token, result ast.Expr) func(exp ast.Expr) ast.Stmt {
	return func(exp ast.Expr) ast.Stmt {
		return &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{exp},
		}
	}
}

func appendAssignment(assignTok token.Token, result ast.Expr) func(exp ast.Expr) ast.Stmt {
	return func(exp ast.Expr) ast.Stmt {
		return &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  ast.NewIdent("append"),
				Args: []ast.Expr{result, exp},
			}},
		}
	}
}

func (t Template) executeCall(status, data ast.Expr, writeHeader bool) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: ast.NewIdent(executeIdentName),
		Args: []ast.Expr{
			ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
			ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
			ast.NewIdent(strconv.FormatBool(writeHeader)),
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(t.name)},
			status,
			data,
		},
	}}
}

func (t Template) httpRequestReceiverTemplateHandlerFunc(imports *source.Imports, statusCode int) *ast.FuncLit {
	return &ast.FuncLit{
		Type: httpHandlerFuncType(imports),
		Body: &ast.BlockStmt{List: []ast.Stmt{t.executeCall(source.HTTPStatusCode(imports, statusCode), ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest), true)}},
	}
}

func (t Template) matchReceiver(funcDecl *ast.FuncDecl, receiverTypeIdent string) bool {
	if funcDecl == nil || funcDecl.Name == nil || funcDecl.Name.Name != t.fun.Name ||
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

func executeFuncDecl(imports *source.Imports, templatesVariableIdent string) *ast.FuncDecl {
	const writeHeaderIdent = "writeHeader"
	return &ast.FuncDecl{
		Name: ast.NewIdent(executeIdentName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					httpResponseField(imports),
					httpRequestField(imports),
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
							X:   ast.NewIdent(imports.Add("", "bytes")),
							Sel: ast.NewIdent("NewBuffer"),
						},
						Args: []ast.Expr{source.Nil()},
					}},
				},
				&ast.IfStmt{
					Init: &ast.AssignStmt{
						Lhs: []ast.Expr{ast.NewIdent(errIdent)},
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
						X:  ast.NewIdent(errIdent),
						Op: token.NEQ,
						Y:  source.Nil(),
					},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.ExprStmt{X: imports.HTTPErrorCall(ast.NewIdent(httpResponseField(imports).Names[0].Name), source.CallError(errIdent), http.StatusInternalServerError)},
							&ast.ReturnStmt{},
						},
					},
				},
				&ast.IfStmt{
					Cond: ast.NewIdent(writeHeaderIdent),
					Body: &ast.BlockStmt{List: []ast.Stmt{
						&ast.ExprStmt{X: &ast.CallExpr{
							Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("Header")}, Args: []ast.Expr{}}, Sel: ast.NewIdent("Set")},
							Args: []ast.Expr{source.String("content-type"), source.String("text/html; charset=utf-8")},
						}},
						&ast.ExprStmt{X: &ast.CallExpr{
							Fun:  &ast.SelectorExpr{X: ast.NewIdent(httpResponseField(imports).Names[0].Name), Sel: ast.NewIdent("WriteHeader")},
							Args: []ast.Expr{ast.NewIdent("code")},
						}},
					}},
				},
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("_")},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("buf"),
							Sel: ast.NewIdent("WriteTo"),
						},
						Args: []ast.Expr{ast.NewIdent(httpResponseField(imports).Names[0].Name)},
					}},
				},
			},
		},
	}
}
