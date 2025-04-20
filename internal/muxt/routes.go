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
	"text/template/parse"

	"github.com/crhntr/dom"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/tools/go/packages"

	"github.com/crhntr/muxt/internal/source"
)

const (
	receiverIdent = "receiver"

	muxVarIdent = "mux"

	requestPathValue         = "PathValue"
	httpRequestContextMethod = "Context"
	httpResponseWriterIdent  = "ResponseWriter"
	httpRequestIdent         = "Request"
	httpHandleFuncIdent      = "HandleFunc"

	defaultPackageName           = "main"
	DefaultTemplatesVariableName = "templates"
	DefaultRoutesFunctionName    = "TemplateRoutes"
	DefaultOutputFileName        = "template_routes.go"
	DefaultReceiverInterfaceName = "RoutesReceiver"
	urlHelperTypeName            = "TemplateRoutePaths"

	InputAttributeNameStructTag     = "name"
	InputAttributeTemplateStructTag = "template"

	muxParamName      = "mux"
	receiverParamName = "receiver"

	errIdent = "err"

	templateDataTypeName = "TemplateData"
)

type RoutesFileConfiguration struct {
	PackageName,
	PackagePath,
	TemplatesVariable,
	RoutesFunction,
	ReceiverType,
	ReceiverPackage,
	ReceiverInterface,
	OutputFileName string
}

func (config RoutesFileConfiguration) applyDefaults() RoutesFileConfiguration {
	config.PackageName = cmp.Or(config.PackageName, defaultPackageName)
	config.TemplatesVariable = cmp.Or(config.TemplatesVariable, DefaultTemplatesVariableName)
	config.RoutesFunction = cmp.Or(config.RoutesFunction, DefaultRoutesFunctionName)
	config.ReceiverInterface = cmp.Or(config.ReceiverInterface, DefaultReceiverInterfaceName)
	return config
}

func TemplateRoutesFile(wd string, logger *log.Logger, config RoutesFileConfiguration) (string, error) {
	config = config.applyDefaults()
	if !token.IsIdentifier(config.PackageName) {
		return "", fmt.Errorf("package name %q is not an identifier", config.PackageName)
	}
	imports := source.NewImports(&ast.GenDecl{Tok: token.IMPORT})

	patterns := []string{
		wd, "encoding", "fmt", "net/http",
	}

	if config.ReceiverPackage != "" {
		patterns = append(patterns, config.ReceiverPackage)
	}

	pl, err := packages.Load(&packages.Config{
		Fset: imports.FileSet(),
		Mode: packages.NeedModule | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles,
		Dir:  wd,
	}, patterns...)
	if err != nil {
		return "", err
	}
	imports.AddPackages(pl...)
	routesPkg, ok := imports.PackageAtFilepath(wd)
	if !ok {
		return "", fmt.Errorf("could not find package in working directory %q", wd)
	}
	imports.SetOutputPackage(routesPkg.Types)
	config.PackagePath = routesPkg.PkgPath
	config.PackageName = routesPkg.Name
	var receiver *types.Named
	if config.ReceiverType != "" {
		receiverPkgPath := cmp.Or(config.ReceiverPackage, config.PackagePath)
		receiverPkg, ok := imports.Package(receiverPkgPath)
		if !ok {
			return "", fmt.Errorf("could not determine receiver package %s", receiverPkgPath)
		}
		obj := receiverPkg.Types.Scope().Lookup(config.ReceiverType)
		if config.ReceiverType != "" && obj == nil {
			return "", fmt.Errorf("could not find receiver type %s in %s", config.ReceiverType, receiverPkg.PkgPath)
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			return "", fmt.Errorf("expected receiver %s to be a named type", config.ReceiverType)
		}
		receiver = named
	} else {
		receiver = types.NewNamed(types.NewTypeName(0, routesPkg.Types, "Receiver", nil), types.NewStruct(nil, nil), nil)
	}

	ts, _, err := source.Templates(wd, config.TemplatesVariable, routesPkg)
	if err != nil {
		return "", err
	}
	templates, err := Templates(ts)
	if err != nil {
		return "", err
	}

	receiverInterface := &ast.InterfaceType{
		Methods: new(ast.FieldList),
	}

	routesFunc := &ast.FuncDecl{
		Name: ast.NewIdent(config.RoutesFunction),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					httpServeMuxField(imports),
					{
						Names: []*ast.Ident{ast.NewIdent(receiverParamName)},
						Type:  ast.NewIdent(config.ReceiverInterface),
					},
				},
			},
		},
		Body: new(ast.BlockStmt),
	}

	const newResponseDataFuncIdent = "new" + templateDataTypeName

	for i, t := range templates {
		const dataVarIdent = "result"
		logger.Printf("routes has route for %s", t.pattern)
		if t.fun == nil {
			handlerFunc := &ast.FuncLit{
				Type: httpHandlerFuncType(imports),
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.AssignStmt{
							Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{&ast.CompositeLit{Type: &ast.StructType{Fields: &ast.FieldList{}}}},
						},
					},
				},
			}
			handlerFunc.Body.List = append(handlerFunc.Body.List, executeFuncDecl(imports, t, nil, config.TemplatesVariable, true, &ast.CallExpr{
				Fun: ast.NewIdent(newResponseDataFuncIdent),
				Args: []ast.Expr{
					ast.NewIdent(dataVarIdent),
					ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
				},
			})...)
			routesFunc.Body.List = append(routesFunc.Body.List, t.callHandleFunc(handlerFunc))
			continue
		}

		sigs := make(map[string]*types.Signature)
		if err := ensureMethodSignature(imports, sigs, &templates[i], receiver, receiverInterface, t.call, routesPkg.Types); err != nil {
			return "", err
		}
		sig, ok := sigs[t.fun.Name]
		if !ok {
			return "", fmt.Errorf("failed to determine call signature %s", t.fun.Name)
		}
		if sig.Results().Len() == 0 {
			return "", fmt.Errorf("method for pattern %q has no results it should have one or two", t.name)
		}
		var callFun ast.Expr
		obj, _, _ := types.LookupFieldOrMethod(receiver, true, receiver.Obj().Pkg(), t.fun.Name)
		isMethodCall := obj != nil
		if isMethodCall {
			callFun = &ast.SelectorExpr{
				X:   ast.NewIdent(receiverIdent),
				Sel: t.fun,
			}
		} else {
			callFun = ast.NewIdent(t.fun.Name)
		}

		writeHeader := !hasHTTPResponseWriterArgument(t.call)

		handlerFunc := &ast.FuncLit{
			Type: httpHandlerFuncType(imports),
			Body: &ast.BlockStmt{},
		}

		if handlerFunc.Body.List, err = appendParseArgumentStatements(handlerFunc.Body.List, &templates[i], imports, sigs, nil, receiver, t.call); err != nil {
			return "", err
		}

		receiverCallStatements, err := callReceiverMethod(imports, dataVarIdent, sig, &ast.CallExpr{
			Fun:  callFun,
			Args: slices.Clone(t.call.Args),
		})
		if err != nil {
			return "", err
		}
		handlerFunc.Body.List = append(handlerFunc.Body.List, receiverCallStatements...)
		handlerFunc.Body.List = append(handlerFunc.Body.List, executeFuncDecl(imports, t, sig.Results().At(0).Type(), config.TemplatesVariable, writeHeader, &ast.CallExpr{
			Fun: ast.NewIdent(newResponseDataFuncIdent),
			Args: []ast.Expr{
				ast.NewIdent(dataVarIdent),
				ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
			},
		})...)
		routesFunc.Body.List = append(routesFunc.Body.List, t.callHandleFunc(handlerFunc))
	}

	const resultParamName = "result"
	newResponseDataFunc := &ast.FuncDecl{
		Name: ast.NewIdent(newResponseDataFuncIdent),
		Type: &ast.FuncType{
			TypeParams: &ast.FieldList{
				List: []*ast.Field{
					{Names: []*ast.Ident{ast.NewIdent("T")}, Type: ast.NewIdent("any")},
				},
			},
			Params: &ast.FieldList{
				List: []*ast.Field{
					{Names: []*ast.Ident{ast.NewIdent(resultParamName)}, Type: ast.NewIdent("T")},
					{Names: []*ast.Ident{ast.NewIdent("request")}, Type: imports.HTTPRequestPtr()},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: &ast.IndexExpr{
						X:     ast.NewIdent(templateDataTypeName),
						Index: ast.NewIdent("T"),
					}},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{
						&ast.CompositeLit{
							Type: &ast.IndexExpr{
								X:     ast.NewIdent(templateDataTypeName),
								Index: ast.NewIdent("T"),
							},
							Elts: []ast.Expr{
								&ast.KeyValueExpr{
									Key:   ast.NewIdent("result"),
									Value: ast.NewIdent(resultParamName),
								},
								&ast.KeyValueExpr{
									Key:   ast.NewIdent("request"),
									Value: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
								},
							},
						},
					},
				},
			},
		},
	}

	routePathDecls, err := routePathTypeAndMethods(imports, templates)
	if err != nil {
		return "", err
	}

	imports.SortImports()
	file := &ast.File{
		Name: ast.NewIdent(config.PackageName),
		Decls: append([]ast.Decl{
			// import
			imports.GenDecl,

			// type
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{
					&ast.TypeSpec{Name: ast.NewIdent(config.ReceiverInterface), Type: receiverInterface},
				},
			},

			// func routes
			routesFunc,

			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{
					&ast.TypeSpec{
						Name: ast.NewIdent(templateDataTypeName),
						TypeParams: &ast.FieldList{
							List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("T")}, Type: ast.NewIdent("any")}},
						},
						Type: &ast.StructType{
							Fields: &ast.FieldList{
								List: []*ast.Field{
									{Names: []*ast.Ident{ast.NewIdent("request")}, Type: imports.HTTPRequestPtr()},
									{Names: []*ast.Ident{ast.NewIdent("result")}, Type: ast.NewIdent("T")},
								},
							},
						},
					},
				},
			},

			&ast.FuncDecl{
				Recv: &ast.FieldList{List: []*ast.Field{{Type: &ast.IndexExpr{
					X:     ast.NewIdent(templateDataTypeName),
					Index: ast.NewIdent("T"),
				}}}},
				Name: ast.NewIdent("Path"),
				Type: &ast.FuncType{
					Results: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("")}, Type: ast.NewIdent(urlHelperTypeName)}}},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ReturnStmt{
							Results: []ast.Expr{&ast.CompositeLit{Type: ast.NewIdent(urlHelperTypeName)}},
						},
					},
				},
			},

			&ast.FuncDecl{
				Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("data")},
					Type: &ast.IndexExpr{
						X:     ast.NewIdent(templateDataTypeName),
						Index: ast.NewIdent("T"),
					}}}},
				Name: ast.NewIdent("Result"),
				Type: &ast.FuncType{
					Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("T")}}},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ReturnStmt{
							Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent("data"), Sel: ast.NewIdent("result")}},
						},
					},
				},
			},

			&ast.FuncDecl{
				Recv: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("data")},
					Type: &ast.IndexExpr{
						X:     ast.NewIdent(templateDataTypeName),
						Index: ast.NewIdent("T"),
					}}}},
				Name: ast.NewIdent("Request"),
				Type: &ast.FuncType{
					Results: &ast.FieldList{List: []*ast.Field{{Type: imports.HTTPRequestPtr()}}},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ReturnStmt{
							Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent("data"), Sel: ast.NewIdent("request")}},
						},
					},
				},
			},

			// func newResultData
			newResponseDataFunc,
		}, routePathDecls...),
	}

	return source.FormatFile(filepath.Join(wd, DefaultOutputFileName), file)
}

func appendParseArgumentStatements(statements []ast.Stmt, t *Template, imports *source.Imports, sigs map[string]*types.Signature, parsed map[string]struct{}, receiver *types.Named, call *ast.CallExpr) ([]ast.Stmt, error) {
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("expected function to be identifier")
	}
	signature, ok := sigs[fun.Name]
	if !ok {
		return nil, fmt.Errorf("failed to get signature for %s", fun.Name)
	}
	// const parsedVariableName = "parsed"
	if exp := signature.Params().Len(); exp != len(call.Args) { // TODO: (signature.Variadic() && exp > len(call.Args))
		sigStr := fun.Name + strings.TrimPrefix(signature.String(), "func")
		return nil, fmt.Errorf("handler func %s expects %d arguments but call %s has %d", sigStr, signature.Params().Len(), source.Format(call), len(call.Args))
	}
	if parsed == nil {
		parsed = make(map[string]struct{})
	}
	resultCount := 0
	for i, a := range call.Args {
		param := signature.Params().At(i)

		switch arg := a.(type) {
		default:
			// TODO: add error case
		case *ast.CallExpr:
			parseArgStatements, err := appendParseArgumentStatements(statements, t, imports, sigs, parsed, receiver, arg)
			if err != nil {
				return nil, err
			}
			resultVarIdent := "result" + strconv.Itoa(resultCount)
			call.Args[i] = ast.NewIdent(resultVarIdent)
			resultCount++

			callSig, ok := sigs[arg.Fun.(*ast.Ident).Name]
			if !ok {
				return nil, fmt.Errorf("failed to get signature for %s", fun.Name)
			}
			obj, _, _ := types.LookupFieldOrMethod(receiver.Obj().Type(), true, receiver.Obj().Pkg(), arg.Fun.(*ast.Ident).Name)
			isMethodCall := obj != nil

			if isMethodCall && !types.Identical(callSig, obj.Type()) {
				log.Panicf("unexpected signature mismatch %s != %s", callSig, obj.Type())
			}

			callSigExpr, err := imports.TypeASTExpression(callSig)
			if err != nil {
				return nil, err
			}

			if isMethodCall {
				arg.Fun = &ast.SelectorExpr{
					X:   ast.NewIdent(receiverIdent),
					Sel: ast.NewIdent(arg.Fun.(*ast.Ident).Name),
				}
			} else {
				arg.Fun = ast.NewIdent(arg.Fun.(*ast.Ident).Name)
			}

			callMethodStatements, err := t.callReceiverMethod(imports, resultVarIdent, callSigExpr.(*ast.FuncType), arg)
			if err != nil {
				return nil, err
			}

			statements = append(parseArgStatements, callMethodStatements...)
		case *ast.Ident:
			argType, ok := defaultTemplateNameScope(imports, t, arg.Name)
			if !ok {
				return nil, fmt.Errorf("failed to determine type for %s", arg.Name)
			}
			src := &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
					Sel: ast.NewIdent(requestPathValue),
				},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(arg.Name)}},
			}
			if types.AssignableTo(argType, param.Type()) {
				if _, ok := parsed[arg.Name]; !ok {
					parsed[arg.Name] = struct{}{}
					switch arg.Name {
					case TemplateNameScopeIdentifierForm:
						declareFormVar, err := formVariableAssignment(imports, arg, param.Type())
						if err != nil {
							return nil, err
						}
						statements = append(statements, callParseForm(), declareFormVar)
					case TemplateNameScopeIdentifierContext:
						statements = append(statements, contextAssignment(TemplateNameScopeIdentifierContext))
					default:
						if slices.Contains(t.parsePathValueNames(), arg.Name) {
							statements = append(statements, singleAssignment(token.DEFINE, ast.NewIdent(arg.Name))(src))
						}
					}
				}
				continue
			}
			if _, ok := parsed[arg.Name]; ok {
				continue
			}
			switch {
			case slices.Contains(t.parsePathValueNames(), arg.Name):
				parsed[arg.Name] = struct{}{}
				s, err := generateParseValueFromStringStatements(imports, arg.Name+"Parsed", src, param.Type(), errCheck(imports), nil, singleAssignment(token.DEFINE, ast.NewIdent(arg.Name)))
				if err != nil {
					return nil, err
				}
				statements = append(statements, s...)
				t.pathValueTypes[arg.Name] = param.Type()
			case arg.Name == TemplateNameScopeIdentifierForm:
				s, err := appendParseFormToStructStatements(statements, t, imports, arg, param)
				if err != nil {
					return nil, err
				}
				statements = s
			default:
				pt, _ := imports.TypeASTExpression(param.Type())
				at, _ := imports.TypeASTExpression(argType)
				return nil, fmt.Errorf("method expects type %s but %s is %s", source.Format(pt), arg.Name, source.Format(at))
			}
		}
	}
	return statements, nil
}

func appendParseFormToStructStatements(statements []ast.Stmt, t *Template, imports *source.Imports, arg *ast.Ident, param types.Object) ([]ast.Stmt, error) {
	const parsedVariableName = "value"
	statements = append(statements, callParseForm())

	declareFormVar, err := formVariableDeclaration(imports, arg, param.Type())
	if err != nil {
		return nil, err
	}
	statements = append(statements, declareFormVar)

	form, ok := param.Type().Underlying().(*types.Struct)
	if !ok {
		return nil, fmt.Errorf("expected form parameter type to be a struct")
	}

	parseErrCheck := func(exp ast.Expr) ast.Stmt {
		return &ast.ExprStmt{
			X: imports.HTTPErrorCall(ast.NewIdent(httpResponseField(imports).Names[0].Name), source.CallError(errIdent), http.StatusBadRequest),
		}
	}

	for i := 0; i < form.NumFields(); i++ {
		field, tags := form.Field(i), reflect.StructTag(form.Tag(i))
		inputName := field.Name()
		if name, found := tags.Lookup(InputAttributeNameStructTag); found {
			inputName = name
		}
		var fieldTemplate *template.Template
		if name, found := tags.Lookup(InputAttributeTemplateStructTag); found {
			fieldTemplate = t.template.Lookup(name)
		}
		var templateNodes []*html.Node
		if fieldTemplate != nil {
			templateNodes, _ = html.ParseFragment(strings.NewReader(fieldTemplate.Tree.Root.String()), &html.Node{
				Type:     html.ElementNode,
				DataAtom: atom.Body,
				Data:     atom.Body.String(),
			})
		}
		var (
			parseResult func(expr ast.Expr) ast.Stmt
			str         ast.Expr
			elemType    types.Type
		)
		switch ft := field.Type().(type) {
		case *types.Slice:
			parseResult = func(expr ast.Expr) ast.Stmt {
				return &ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierForm), Sel: ast.NewIdent(field.Name())}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun:  ast.NewIdent("append"),
						Args: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierForm), Sel: ast.NewIdent(field.Name())}, expr},
					}},
				}
			}
			str = ast.NewIdent("val")
			elemType = ft.Elem()
			validations, err, ok := source.GenerateValidations(imports, ast.NewIdent(parsedVariableName), elemType, fmt.Sprintf("[name=%q]", inputName), inputName, httpResponseField(imports).Names[0].Name, dom.NewDocumentFragment(templateNodes))
			if ok && err != nil {
				return nil, err
			}
			parseStatements, err := generateParseValueFromStringStatements(imports, parsedVariableName, str, elemType, parseErrCheck, validations, parseResult)
			if err != nil {
				return nil, fmt.Errorf("failed to generate parse statements for form field %s: %w", field.Name(), err)
			}
			statements = append(statements, &ast.RangeStmt{
				Key:   ast.NewIdent("_"),
				Value: ast.NewIdent("val"),
				Tok:   token.DEFINE,
				X:     &ast.IndexExpr{X: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Form")}, Index: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(inputName)}},
				Body:  &ast.BlockStmt{List: parseStatements},
			})
		default:
			parseResult = func(expr ast.Expr) ast.Stmt {
				return &ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierForm), Sel: ast.NewIdent(field.Name())}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{expr},
				}
			}
			str = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("FormValue")}, Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(inputName)}}}
			elemType = field.Type()
			validations, err, ok := source.GenerateValidations(imports, ast.NewIdent(parsedVariableName), elemType, fmt.Sprintf("[name=%q]", inputName), inputName, httpResponseField(imports).Names[0].Name, dom.NewDocumentFragment(templateNodes))
			if ok && err != nil {
				return nil, err
			}
			parseStatements, err := generateParseValueFromStringStatements(imports, parsedVariableName, str, elemType, parseErrCheck, validations, parseResult)
			if err != nil {
				return nil, fmt.Errorf("failed to generate parse statements for form field %s: %w", field.Name(), err)
			}
			if len(parseStatements) > 1 {
				statements = append(statements, &ast.BlockStmt{
					List: parseStatements,
				})
			} else {
				statements = append(statements, parseStatements...)
			}
		}
	}

	return statements, nil
}

func formVariableDeclaration(imports *source.Imports, arg *ast.Ident, tp types.Type) (*ast.DeclStmt, error) {
	typeExp, err := imports.TypeASTExpression(tp)
	if err != nil {
		return nil, err
	}
	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(arg.Name)},
					Type:  typeExp,
				},
			},
		},
	}, nil
}

func formVariableAssignment(imports *source.Imports, arg *ast.Ident, tp types.Type) (*ast.DeclStmt, error) {
	typeExp, err := imports.TypeASTExpression(tp)
	if err != nil {
		return nil, err
	}
	return &ast.DeclStmt{
		Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(arg.Name)},
					Type:  typeExp,
					Values: []ast.Expr{
						&ast.SelectorExpr{
							X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
							Sel: ast.NewIdent("Form"),
						},
					},
				},
			},
		},
	}, nil
}

func httpServeMuxField(imports *source.Imports) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(muxParamName)},
		Type:  &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent(imports.AddNetHTTP()), Sel: ast.NewIdent("ServeMux")}},
	}
}

func generateParseValueFromStringStatements(imports *source.Imports, tmp string, str ast.Expr, valueType types.Type, errCheck func(expr ast.Expr) ast.Stmt, validations []ast.Stmt, assignment func(ast.Expr) ast.Stmt) ([]ast.Stmt, error) {
	switch tp := valueType.(type) {
	case *types.Basic:
		convert := func(exp ast.Expr) ast.Stmt {
			return assignment(&ast.CallExpr{
				Fun:  ast.NewIdent(tp.Name()),
				Args: []ast.Expr{exp},
			})
		}
		switch tp.Name() {
		default:
			return nil, fmt.Errorf("method param type %s not supported", valueType.String())
		case "bool":
			return parseBlock(tmp, imports.StrconvParseBoolCall(str), validations, errCheck, assignment), nil
		case "int":
			return parseBlock(tmp, imports.StrconvAtoiCall(str), validations, errCheck, assignment), nil
		case "int8":
			return parseBlock(tmp, imports.StrconvParseInt8Call(str), validations, errCheck, convert), nil
		case "int16":
			return parseBlock(tmp, imports.StrconvParseInt16Call(str), validations, errCheck, convert), nil
		case "int32":
			return parseBlock(tmp, imports.StrconvParseInt32Call(str), validations, errCheck, convert), nil
		case "int64":
			return parseBlock(tmp, imports.StrconvParseInt64Call(str), validations, errCheck, assignment), nil
		case "uint":
			return parseBlock(tmp, imports.StrconvParseUint0Call(str), validations, errCheck, convert), nil
		case "uint8":
			return parseBlock(tmp, imports.StrconvParseUint8Call(str), validations, errCheck, convert), nil
		case "uint16":
			return parseBlock(tmp, imports.StrconvParseUint16Call(str), validations, errCheck, convert), nil
		case "uint32":
			return parseBlock(tmp, imports.StrconvParseUint32Call(str), validations, errCheck, convert), nil
		case "uint64":
			return parseBlock(tmp, imports.StrconvParseUint64Call(str), validations, errCheck, assignment), nil
		case "string":
			if len(validations) == 0 {
				assign := assignment(str)
				statements := slices.Concat(validations, []ast.Stmt{assign})
				return statements, nil
			}
			statements := slices.Concat([]ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(tmp)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{str},
			}}, validations, []ast.Stmt{assignment(ast.NewIdent(tmp))})
			return statements, nil
		}
	case *types.Named:
		if encPkg, ok := imports.Types("encoding"); ok {
			if textUnmarshaler := encPkg.Scope().Lookup("TextUnmarshaler").Type().Underlying().(*types.Interface); types.Implements(types.NewPointer(tp), textUnmarshaler) {
				tp, _ := imports.TypeASTExpression(valueType)
				return []ast.Stmt{
					&ast.DeclStmt{
						Decl: &ast.GenDecl{
							Tok: token.VAR,
							Specs: []ast.Spec{
								&ast.ValueSpec{
									Names: []*ast.Ident{ast.NewIdent(tmp)},
									Type:  tp,
								},
							},
						},
					},
					&ast.IfStmt{
						Init: &ast.AssignStmt{
							Lhs: []ast.Expr{ast.NewIdent(errIdent)},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{&ast.CallExpr{
								Fun: &ast.SelectorExpr{
									X:   ast.NewIdent(tmp),
									Sel: ast.NewIdent("UnmarshalText"),
								},
								Args: []ast.Expr{&ast.CallExpr{
									Fun: &ast.ArrayType{
										Elt: ast.NewIdent("byte"),
									},
									Args: []ast.Expr{str},
								}},
							}},
						},
						Cond: &ast.BinaryExpr{
							X:  ast.NewIdent(errIdent),
							Op: token.NEQ,
							Y:  ast.NewIdent("nil"),
						},
						Body: &ast.BlockStmt{
							List: []ast.Stmt{
								errCheck(&ast.CallExpr{
									Fun: &ast.SelectorExpr{
										X:   ast.NewIdent(errIdent),
										Sel: ast.NewIdent("Error"),
									},
									Args: []ast.Expr{},
								}),
								new(ast.ReturnStmt),
							},
						},
					},
					assignment(ast.NewIdent(tmp)),
				}, nil
			}
		}
	}
	tp, _ := imports.TypeASTExpression(valueType)
	return nil, fmt.Errorf("unsupported type: %s", source.Format(tp))
}

func parseBlock(tmpIdent string, parseCall ast.Expr, validations []ast.Stmt, handleErr, handleResult func(out ast.Expr) ast.Stmt) []ast.Stmt {
	const errIdent = "err"
	callParse := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(tmpIdent), ast.NewIdent(errIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{parseCall},
	}
	errCheck := source.ErrorCheckReturn(errIdent, handleErr(&ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(errIdent),
			Sel: ast.NewIdent("Error"),
		},
		Args: []ast.Expr{},
	}))
	block := &ast.BlockStmt{List: []ast.Stmt{callParse, errCheck}}
	block.List = append(block.List, validations...)
	block.List = append(block.List, handleResult(ast.NewIdent(tmpIdent)))
	return block.List
}

func callReceiverMethod(imports *source.Imports, dataVarIdent string, method *types.Signature, call *ast.CallExpr) ([]ast.Stmt, error) {
	const (
		okIdent = "ok"
	)
	if method.Results().Len() == 0 {
		mathodIdent := call.Fun.(*ast.Ident)
		assert.NotNil(assertion, mathodIdent)
		return nil, fmt.Errorf("method %s has no results it should have one or two", mathodIdent.Name)
	} else if method.Results().Len() > 1 {
		lastResult := method.Results().At(method.Results().Len() - 1)

		errorType := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
		assert.NotNil(assertion, errorType)

		if types.Implements(lastResult.Type(), errorType) {
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
		}

		if basic, ok := lastResult.Type().(*types.Basic); ok && basic.Kind() == types.Bool {
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
		}

		return nil, fmt.Errorf("expected last result to be either an error or a bool")
	} else {
		return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}}}, nil
	}
}

var assertion AssertionFailureReporter

type AssertionFailureReporter struct{}

func (AssertionFailureReporter) Errorf(format string, args ...interface{}) {
	log.Fatalf(format, args...)
}

func defaultTemplateNameScope(imports *source.Imports, template *Template, argumentIdentifier string) (types.Type, bool) {
	switch argumentIdentifier {
	case TemplateNameScopeIdentifierHTTPRequest:
		pkg, ok := imports.Types("net/http")
		if !ok {
			return nil, false
		}
		t := types.NewPointer(pkg.Scope().Lookup("Request").Type())
		return t, true
	case TemplateNameScopeIdentifierHTTPResponse:
		pkg, ok := imports.Types("net/http")
		if !ok {
			return nil, false
		}
		t := pkg.Scope().Lookup("ResponseWriter").Type()
		return t, true
	case TemplateNameScopeIdentifierContext:
		pkg, ok := imports.Types("context")
		if !ok {
			return nil, false
		}
		t := pkg.Scope().Lookup("Context").Type()
		return t, true
	case TemplateNameScopeIdentifierForm:
		pkg, ok := imports.Types("net/url")
		if !ok {
			return nil, false
		}
		t := pkg.Scope().Lookup("Values").Type()
		return t, true
	default:
		if slices.Contains(template.parsePathValueNames(), argumentIdentifier) {
			return types.Universe.Lookup("string").Type(), true
		}
		return nil, false
	}
}

func packageScopeFunc(pkg *types.Package, fun *ast.Ident) (types.Object, bool) {
	obj := pkg.Scope().Lookup(fun.Name)
	if obj == nil {
		return nil, false
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return nil, false
	}
	if sig.Recv() != nil {
		return nil, false
	}
	return obj, true
}

func ensureMethodSignature(imports *source.Imports, signatures map[string]*types.Signature, t *Template, receiver *types.Named, receiverInterface *ast.InterfaceType, call *ast.CallExpr, templatesPackage *types.Package) error {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		isMethod := true
		mo, _, _ := types.LookupFieldOrMethod(receiver, true, receiver.Obj().Pkg(), fun.Name)
		if mo == nil {
			if m, ok := packageScopeFunc(templatesPackage, fun); ok {
				mo = m
				isMethod = false
			} else {
				ms, err := createMethodSignature(imports, signatures, t, receiver, receiverInterface, call, templatesPackage)
				if err != nil {
					return err
				}
				fn := types.NewFunc(0, receiver.Obj().Pkg(), fun.Name, ms)
				receiver.AddMethod(fn)
				mo = fn
			}
		} else {
			for _, a := range call.Args {
				switch arg := a.(type) {
				case *ast.CallExpr:
					if err := ensureMethodSignature(imports, signatures, t, receiver, receiverInterface, arg, templatesPackage); err != nil {
						return err
					}
				}
			}
		}
		signatures[fun.Name] = mo.Type().(*types.Signature)
		if !isMethod {
			return nil
		}
		exp, err := imports.TypeASTExpression(mo.Type())
		if err != nil {
			return err
		}
		receiverInterface.Methods.List = append(receiverInterface.Methods.List, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(fun.Name)},
			Type:  exp,
		})
		return nil
	default:
		return fmt.Errorf("expected a method identifier")
	}
}

func createMethodSignature(imports *source.Imports, signatures map[string]*types.Signature, t *Template, receiver *types.Named, receiverInterface *ast.InterfaceType, call *ast.CallExpr, templatesPackage *types.Package) (*types.Signature, error) {
	var params []*types.Var
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			tp, ok := defaultTemplateNameScope(imports, t, arg.Name)
			if !ok {
				return nil, fmt.Errorf("could not determine a type for %s", arg.Name)
			}
			params = append(params, types.NewVar(0, receiver.Obj().Pkg(), arg.Name, tp))
		case *ast.CallExpr:
			if err := ensureMethodSignature(imports, signatures, t, receiver, receiverInterface, arg, templatesPackage); err != nil {
				return nil, err
			}
		}
	}
	results := types.NewTuple(types.NewVar(0, nil, "", types.Universe.Lookup("any").Type()))
	return types.NewSignatureType(types.NewVar(0, nil, "", receiver.Obj().Type()), nil, nil, types.NewTuple(params...), results, false), nil
}

func hasHTTPResponseWriterArgument(call *ast.CallExpr) bool {
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			if arg.Name == TemplateNameScopeIdentifierHTTPResponse {
				return true
			}
		case *ast.CallExpr:
			if hasHTTPResponseWriterArgument(arg) {
				return true
			}
		}
	}
	return false
}

func errCheck(imports *source.Imports) func(msg ast.Expr) ast.Stmt {
	return func(msg ast.Expr) ast.Stmt {
		return &ast.ExprStmt{
			X: imports.HTTPErrorCall(ast.NewIdent(httpResponseField(imports).Names[0].Name), msg, http.StatusBadRequest),
		}
	}
}

func callParseForm() *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
			Sel: ast.NewIdent("ParseForm"),
		},
		Args: []ast.Expr{},
	}}
}

func httpResponseField(imports *source.Imports) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)},
		Type:  &ast.SelectorExpr{X: ast.NewIdent(imports.AddNetHTTP()), Sel: ast.NewIdent(httpResponseWriterIdent)},
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

func singleAssignment(assignTok token.Token, result ast.Expr) func(exp ast.Expr) ast.Stmt {
	return func(exp ast.Expr) ast.Stmt {
		return &ast.AssignStmt{
			Lhs: []ast.Expr{result},
			Tok: assignTok,
			Rhs: []ast.Expr{exp},
		}
	}
}

func executeFuncDecl(imports *source.Imports, t Template, resultType types.Type, templatesVariableIdent string, writeHeader bool, result ast.Expr) []ast.Stmt {
	statements := make([]ast.Stmt, 0, 5)
	statements = append(statements,
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent("buf")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{imports.BytesNewBuffer(source.Nil())},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent("rd")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{result},
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
					Args: []ast.Expr{ast.NewIdent("buf"), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(t.name)}, ast.NewIdent("rd")},
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
	)
	if writeHeader {
		statements = append(statements, &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("Header")}, Args: []ast.Expr{}}, Sel: ast.NewIdent("Set")},
			Args: []ast.Expr{source.String("content-type"), source.String("text/html; charset=utf-8")},
		}}, &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("Header")}, Args: []ast.Expr{}}, Sel: ast.NewIdent("Set")},
			Args: []ast.Expr{source.String("content-length"), imports.StrconvItoaCall(&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("buf"), Sel: ast.NewIdent("Len")}, Args: []ast.Expr{}})},
		}})

		code := source.HTTPStatusCode(imports, t.defaultStatusCode)

		if resultType != nil {
			statements = append(statements, &ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent("statusCode")},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{source.HTTPStatusCode(imports, t.defaultStatusCode)},
			})
			statusCoder := statusCoderInterface()
			if types.Implements(resultType, statusCoder) {
				statements = append(statements, &ast.IfStmt{
					Cond: &ast.BinaryExpr{X: ast.NewIdent("sc"), Op: token.NEQ, Y: source.Int(0)},
					Init: &ast.AssignStmt{
						Lhs: []ast.Expr{ast.NewIdent("sc")},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("result"), Sel: ast.NewIdent("StatusCode")}}},
					},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.AssignStmt{
								Lhs: []ast.Expr{ast.NewIdent("statusCode")},
								Tok: token.ASSIGN,
								Rhs: []ast.Expr{ast.NewIdent("sc")},
							},
						},
					},
				})
			} else if obj, _, _ := types.LookupFieldOrMethod(resultType, true, imports.OutputPackageType(), "StatusCode"); obj != nil {
				statements = append(statements, &ast.IfStmt{
					Cond: &ast.BinaryExpr{X: ast.NewIdent("sc"), Op: token.NEQ, Y: source.Int(0)},
					Init: &ast.AssignStmt{
						Lhs: []ast.Expr{ast.NewIdent("sc")},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent("result"), Sel: ast.NewIdent("StatusCode")}},
					},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.AssignStmt{
								Lhs: []ast.Expr{ast.NewIdent("statusCode")},
								Tok: token.ASSIGN,
								Rhs: []ast.Expr{ast.NewIdent("sc")},
							},
						},
					},
				})
			}
			code = ast.NewIdent("statusCode")
		}

		statements = append(statements, &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(httpResponseField(imports).Names[0].Name), Sel: ast.NewIdent("WriteHeader")},
			Args: []ast.Expr{code},
		}})
	}
	statements = append(statements, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("_")},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent("buf"),
				Sel: ast.NewIdent("WriteTo"),
			},
			Args: []ast.Expr{ast.NewIdent(httpResponseField(imports).Names[0].Name)},
		}},
	})
	return statements
}

type forest template.Template

func newForrest(templates *template.Template) *forest {
	return (*forest)(templates)
}

func (f *forest) FindTree(name string) (*parse.Tree, bool) {
	ts := (*template.Template)(f).Lookup(name)
	if ts == nil {
		return nil, false
	}
	return ts.Tree, true
}

func statusCoderInterface() *types.Interface {
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(),
		types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.Int])),
		false)

	method := types.NewFunc(token.NoPos, nil, "StatusCode", sig)
	return types.NewInterfaceType([]*types.Func{method}, nil).Complete()
}
