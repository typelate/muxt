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

	templateDataTypeName        = "TemplateData"
	templateDataFieldStatusCode = "statusCode"

	executeTemplateErrorMessage = "failed to render page"
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

	patterns := []string{
		wd, "encoding", "fmt", "net/http",
	}

	if config.ReceiverPackage != "" {
		patterns = append(patterns, config.ReceiverPackage)
	}

	fileSet := token.NewFileSet()
	pl, err := packages.Load(&packages.Config{
		Fset: fileSet,
		Mode: packages.NeedModule | packages.NeedName | packages.NeedFiles | packages.NeedTypes | packages.NeedSyntax | packages.NeedEmbedPatterns | packages.NeedEmbedFiles,
		Dir:  wd,
	}, patterns...)
	if err != nil {
		return "", err
	}

	file, err := source.NewFile(filepath.Join(wd, config.OutputFileName), fileSet, pl)
	if err != nil {
		return "", err
	}
	routesPkg := file.OutputPackage()

	config.PackagePath = routesPkg.PkgPath
	config.PackageName = routesPkg.Name
	var receiver *types.Named
	if config.ReceiverType != "" {
		receiverPkgPath := cmp.Or(config.ReceiverPackage, config.PackagePath)
		receiverPkg, ok := file.Package(receiverPkgPath)
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

	const (
		executeTemplateAndWriteHeadersFuncIdent = "executeTemplateAndWriteHeaders"
		executeTemplateFuncIdent                = "executeTemplate"
	)

	routesFunc := &ast.FuncDecl{
		Name: ast.NewIdent(config.RoutesFunction),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					httpServeMuxField(file),
					{
						Names: []*ast.Ident{ast.NewIdent(receiverParamName)},
						Type:  ast.NewIdent(config.ReceiverInterface),
					},
				},
			},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{}},
	}

	var (
		hasResponseWriterArg     = false
		onlyHasResponseWriterArg = true
	)
	for _, t := range templates {
		if t.hasResponseWriterArg {
			hasResponseWriterArg = true
		} else {
			onlyHasResponseWriterArg = false
		}
	}
	if !onlyHasResponseWriterArg {
		routesFunc.Body.List = append(routesFunc.Body.List, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(executeTemplateAndWriteHeadersFuncIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{executeTemplateAndWriteHeadersFunc(file)},
		})
	}
	if hasResponseWriterArg {
		routesFunc.Body.List = append(routesFunc.Body.List, &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(executeTemplateFuncIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{executeTemplateFunc(file)},
		})
	}

	const newResponseDataFuncIdent = "new" + templateDataTypeName

	for i, t := range templates {
		const dataVarIdent = "result"
		logger.Printf("routes has route for %s", t.pattern)
		if t.fun == nil {
			handlerFunc := &ast.FuncLit{
				Type: httpHandlerFuncType(file),
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.DeclStmt{
							Decl: &ast.GenDecl{
								Tok: token.VAR,
								Specs: []ast.Spec{
									&ast.ValueSpec{
										Names:  []*ast.Ident{ast.NewIdent(dataVarIdent)},
										Values: []ast.Expr{&ast.CompositeLit{Type: &ast.StructType{Fields: &ast.FieldList{}}}},
									},
								},
							},
						},
					},
				},
			}
			handlerFunc.Body.List = append(handlerFunc.Body.List, executeFuncDecl(file, t, nil, config.TemplatesVariable, executeTemplateAndWriteHeadersFuncIdent, executeTemplateFuncIdent, &ast.CallExpr{
				Fun: ast.NewIdent(newResponseDataFuncIdent),
				Args: []ast.Expr{
					ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
					ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
					ast.NewIdent(dataVarIdent),
					ast.NewIdent("true"),
					ast.NewIdent("nil"),
				},
			})...)
			routesFunc.Body.List = append(routesFunc.Body.List, t.callHandleFunc(handlerFunc))
			continue
		}

		sigs := make(map[string]*types.Signature)
		if err := ensureMethodSignature(file, sigs, &templates[i], receiver, receiverInterface, t.call, routesPkg.Types); err != nil {
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

		handlerFunc := &ast.FuncLit{
			Type: httpHandlerFuncType(file),
			Body: &ast.BlockStmt{
				List: []ast.Stmt{},
			},
		}

		if handlerFunc.Body.List, err = appendParseArgumentStatements(handlerFunc.Body.List, &templates[i], file, sigs, nil, receiver, t.call); err != nil {
			return "", err
		}

		receiverCallStatements, resultOk, resultErr, err := callReceiverMethod(dataVarIdent, sig, &ast.CallExpr{
			Fun:  callFun,
			Args: slices.Clone(t.call.Args),
		})
		if err != nil {
			return "", err
		}
		handlerFunc.Body.List = append(handlerFunc.Body.List, receiverCallStatements...)
		handlerFunc.Body.List = append(handlerFunc.Body.List, executeFuncDecl(file, t, sig.Results().At(0).Type(), config.TemplatesVariable, executeTemplateAndWriteHeadersFuncIdent, executeTemplateFuncIdent, &ast.CallExpr{
			Fun: ast.NewIdent(newResponseDataFuncIdent),
			Args: []ast.Expr{
				ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
				ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
				ast.NewIdent(dataVarIdent),
				resultOk,
				resultErr,
			},
		})...)
		routesFunc.Body.List = append(routesFunc.Body.List, t.callHandleFunc(handlerFunc))
	}

	const resultParamName = "result"

	routePathDecls, err := routePathTypeAndMethods(file, templates)
	if err != nil {
		return "", err
	}

	file.SortImports()
	is := file.ImportSpecs()
	importSpecs := make([]ast.Spec, 0, len(is))
	for _, s := range is {
		importSpecs = append(importSpecs, s)
	}
	outputFile := &ast.File{
		Name: ast.NewIdent(config.PackageName),
		Decls: append([]ast.Decl{
			// import
			&ast.GenDecl{
				Tok:   token.IMPORT,
				Specs: importSpecs,
			},

			// type
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{
					&ast.TypeSpec{Name: ast.NewIdent(config.ReceiverInterface), Type: receiverInterface},
				},
			},

			// func routes
			routesFunc,

			templateDataType(file),
			newTemplateData(file, newResponseDataFuncIdent, resultParamName),
			templateDataPathMethod(),
			templateDataResultMethod(),
			templateDataRequestMethod(file),
			templateDataStatusCodeMethod(),
			templateDataHeaderMethod(),
			templateDataOkay(),
			templateDataError(),

			// func newResultData
		}, routePathDecls...),
	}

	return source.FormatFile(filepath.Join(wd, DefaultOutputFileName), outputFile)
}

func executeFuncType(file *source.File, executeIdent string, closureType *ast.FuncType) *ast.FuncType {
	return &ast.FuncType{
		Params: &ast.FieldList{
			List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)}, Type: file.HTTPResponseWriter()},
				{Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest)}, Type: file.HTTPRequestPtr()},
				{Names: []*ast.Ident{ast.NewIdent(executeIdent)}, Type: closureType},
			},
		},
	}
}

func executeTemplateWithStatusCodeResultLiteralType(file *source.File, writerIdent string) *ast.FuncType {
	var wNames []*ast.Ident
	if writerIdent != "" {
		wNames = []*ast.Ident{ast.NewIdent(writerIdent)}
	}
	return &ast.FuncType{
		Params: &ast.FieldList{List: []*ast.Field{{
			Names: wNames,
			Type: &ast.SelectorExpr{
				X:   ast.NewIdent(file.Import("", "io")),
				Sel: ast.NewIdent("Writer"),
			}}},
		},
		Results: &ast.FieldList{
			List: []*ast.Field{
				{Type: ast.NewIdent("int")},
				{Type: ast.NewIdent("error")},
			},
		},
	}
}

func executeTemplateLiteralType(file *source.File, writerIdent string) *ast.FuncType {
	var wNames []*ast.Ident
	if writerIdent != "" {
		wNames = []*ast.Ident{ast.NewIdent(writerIdent)}
	}
	return &ast.FuncType{
		Params: &ast.FieldList{List: []*ast.Field{{
			Names: wNames,
			Type: &ast.SelectorExpr{
				X:   ast.NewIdent(file.Import("", "io")),
				Sel: ast.NewIdent("Writer"),
			}}},
		},
		Results: &ast.FieldList{
			List: []*ast.Field{
				{Type: ast.NewIdent("error")},
			},
		},
	}
}

func executeTemplateAndWriteHeadersFunc(file *source.File) *ast.FuncLit {
	const (
		bufIdent        = "buf"
		statusCodeIdent = "statusCode"
		executeIdent    = "execute"
	)
	return &ast.FuncLit{
		Type: executeFuncType(file, executeIdent, executeTemplateWithStatusCodeResultLiteralType(file, "")),
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				defineBytesBuffer(file, bufIdent),
				&ast.AssignStmt{
					Lhs: []ast.Expr{
						ast.NewIdent(statusCodeIdent),
						ast.NewIdent(errIdent),
					},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun:  ast.NewIdent(executeIdent),
						Args: []ast.Expr{ast.NewIdent(bufIdent)},
					}},
				},
				checkExecuteTemplateError(file),
				setContentTypeHeaderSetOnTemplateData(),
				&ast.ExprStmt{X: &ast.CallExpr{
					Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("Header")}, Args: []ast.Expr{}}, Sel: ast.NewIdent("Set")},
					Args: []ast.Expr{source.String("content-length"), file.StrconvItoaCall(&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Len")}, Args: []ast.Expr{}})},
				}},
				callWriteHeader(ast.NewIdent(statusCodeIdent)),
				callWriteOnResponse(bufIdent),
			},
		},
	}
}

func executeTemplateFunc(file *source.File) *ast.FuncLit {
	const (
		bufIdent     = "buf"
		executeIdent = "execute"
	)
	return &ast.FuncLit{
		Type: executeFuncType(file, executeIdent, executeTemplateLiteralType(file, "")),
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				defineBytesBuffer(file, bufIdent),
				&ast.AssignStmt{
					Lhs: []ast.Expr{
						ast.NewIdent(errIdent),
					},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{&ast.CallExpr{
						Fun:  ast.NewIdent(executeIdent),
						Args: []ast.Expr{ast.NewIdent(bufIdent)},
					}},
				},
				checkExecuteTemplateError(file),
				callWriteOnResponse(bufIdent),
			},
		},
	}
}

func callWriteHeader(statusCode ast.Expr) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("WriteHeader")},
		Args: []ast.Expr{statusCode},
	}}
}

func checkExecuteTemplateError(file *source.File) *ast.IfStmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: source.Nil()},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ExprStmt{X: executeTemplateSlogLine(file)},
				&ast.ExprStmt{X: file.HTTPErrorCall(ast.NewIdent(httpResponseField(file).Names[0].Name), source.String(executeTemplateErrorMessage), http.StatusInternalServerError)},
				&ast.ReturnStmt{},
			},
		},
	}
}

func defineBytesBuffer(file *source.File, bufIdent string) *ast.AssignStmt {
	return &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(bufIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(file.Import("", "bytes")), Sel: ast.NewIdent("NewBuffer")},
		Args: []ast.Expr{ast.NewIdent("nil")},
	}}}
}

func callWriteOnResponse(bufferIdent string) *ast.AssignStmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent("_"), ast.NewIdent("_")},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(bufferIdent),
				Sel: ast.NewIdent("WriteTo"),
			},
			Args: []ast.Expr{ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)},
		}},
	}
}

func templateDataType(file *source.File) *ast.GenDecl {
	return &ast.GenDecl{
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
							{Names: []*ast.Ident{ast.NewIdent("response")}, Type: file.HTTPResponseWriter()},
							{Names: []*ast.Ident{ast.NewIdent("request")}, Type: file.HTTPRequestPtr()},
							{Names: []*ast.Ident{ast.NewIdent("result")}, Type: ast.NewIdent("T")},
							{Names: []*ast.Ident{ast.NewIdent("statusCode")}, Type: ast.NewIdent("int")},
							{Names: []*ast.Ident{ast.NewIdent("okay")}, Type: ast.NewIdent("bool")},
							{Names: []*ast.Ident{ast.NewIdent("error")}, Type: ast.NewIdent("error")},
						},
					},
				},
			},
		},
	}
}

func newTemplateData(file *source.File, newResponseDataFuncIdent, resultParamName string) *ast.FuncDecl {
	const (
		okayIdent = "okay"
		errIdent  = "err"
	)
	return &ast.FuncDecl{
		Name: ast.NewIdent(newResponseDataFuncIdent),
		Type: &ast.FuncType{
			TypeParams: &ast.FieldList{
				List: []*ast.Field{
					{Names: []*ast.Ident{ast.NewIdent("T")}, Type: ast.NewIdent("any")},
				},
			},
			Params: &ast.FieldList{
				List: []*ast.Field{
					{Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)}, Type: file.HTTPResponseWriter()},
					{Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest)}, Type: file.HTTPRequestPtr()},
					{Names: []*ast.Ident{ast.NewIdent(resultParamName)}, Type: ast.NewIdent("T")},
					{Names: []*ast.Ident{ast.NewIdent(okayIdent)}, Type: ast.NewIdent("bool")},
					{Names: []*ast.Ident{ast.NewIdent(errIdent)}, Type: ast.NewIdent("error")},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: &ast.StarExpr{X: &ast.IndexExpr{
						X:     ast.NewIdent(templateDataTypeName),
						Index: ast.NewIdent("T"),
					}}},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{
						&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{
							Type: &ast.IndexExpr{
								X:     ast.NewIdent(templateDataTypeName),
								Index: ast.NewIdent("T"),
							},
							Elts: []ast.Expr{
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse), Value: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)},
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest), Value: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest)},
								&ast.KeyValueExpr{Key: ast.NewIdent("result"), Value: ast.NewIdent(resultParamName)},
								&ast.KeyValueExpr{Key: ast.NewIdent("okay"), Value: ast.NewIdent(okayIdent)},
								&ast.KeyValueExpr{Key: ast.NewIdent("error"), Value: ast.NewIdent(errIdent)},
							},
						}},
					},
				},
			},
		},
	}
}

const (
	templateDataReceiverName = "data"
)

func templateDataMethodReceiver() *ast.FieldList {
	return &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(templateDataReceiverName)}, Type: &ast.StarExpr{X: &ast.IndexExpr{
		X:     ast.NewIdent(templateDataTypeName),
		Index: ast.NewIdent("T"),
	}}}}}
}

func templateDataOkay() *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(),
		Name: ast.NewIdent("Ok"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("bool")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("okay")}}},
			},
		},
	}
}

func templateDataError() *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(),
		Name: ast.NewIdent("Err"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("error")}}},
			},
		},
	}
}

func templateDataPathMethod() *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(),
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
	}
}

func templateDataResultMethod() *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(),
		Name: ast.NewIdent("Result"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("T")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("result")}},
				},
			},
		},
	}
}

func templateDataRequestMethod(file *source.File) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(),
		Name: ast.NewIdent("Request"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: file.HTTPRequestPtr()}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("request")}},
				},
			},
		},
	}
}

func templateDataStatusCodeMethod() *ast.FuncDecl {
	const (
		scIdent = "statusCode"
	)
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(),
		Name: ast.NewIdent("StatusCode"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent(scIdent)},
				Type:  ast.NewIdent("int"),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: &ast.StarExpr{X: &ast.IndexExpr{
					X:     ast.NewIdent(templateDataTypeName),
					Index: ast.NewIdent("T"),
				}},
			}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent(scIdent)}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{ast.NewIdent(scIdent)},
				},
				&ast.ReturnStmt{
					Results: []ast.Expr{ast.NewIdent(templateDataReceiverName)},
				},
			},
		},
	}
}

func useTemplateDataStatusCodeField(templateDataVar, statusCodeVarIdent string) *ast.IfStmt {
	const (
		scIdent = "statusCode"
	)
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: &ast.SelectorExpr{
			X:   ast.NewIdent(templateDataVar),
			Sel: ast.NewIdent(scIdent),
		}, Op: token.NEQ, Y: source.Int(0)},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(statusCodeVarIdent)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.SelectorExpr{
					X:   ast.NewIdent(templateDataVar),
					Sel: ast.NewIdent(scIdent),
				}},
			},
		}},
	}
}

func templateDataHeaderMethod() *ast.FuncDecl {
	const (
		this       = "data"
		keyIdent   = "key"
		valueIdent = "value"
	)
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(),
		Name: ast.NewIdent("Header"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent("key"), ast.NewIdent("value")},
				Type:  ast.NewIdent("string"),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: &ast.StarExpr{X: &ast.IndexExpr{
					X:     ast.NewIdent(templateDataTypeName),
					Index: ast.NewIdent("T"),
				}},
			}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ExprStmt{X: &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X: &ast.CallExpr{
							Fun: &ast.SelectorExpr{
								X:   &ast.SelectorExpr{X: ast.NewIdent(this), Sel: ast.NewIdent("response")},
								Sel: ast.NewIdent("Header"),
							},
						},
						Sel: ast.NewIdent("Set"),
					},
					Args: []ast.Expr{ast.NewIdent(keyIdent), ast.NewIdent(valueIdent)},
				}},
				&ast.ReturnStmt{
					Results: []ast.Expr{ast.NewIdent(this)},
				},
			},
		},
	}
}

func setContentTypeHeaderSetOnTemplateData() *ast.IfStmt {
	const (
		ctIdent  = "contentType"
		ctHeader = "content-type"
	)
	return &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(ctIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X: &ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
							Sel: ast.NewIdent("Header"),
						},
					},
					Sel: ast.NewIdent("Get"),
				},
				Args: []ast.Expr{source.String(ctHeader)},
			}},
		},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(ctIdent), Op: token.EQL, Y: source.String("")},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("Header")}, Args: []ast.Expr{}}, Sel: ast.NewIdent("Set")},
				Args: []ast.Expr{source.String(ctHeader), source.String("text/html; charset=utf-8")},
			}}},
		},
	}
}

func appendParseArgumentStatements(statements []ast.Stmt, t *Template, file *source.File, sigs map[string]*types.Signature, parsed map[string]struct{}, receiver *types.Named, call *ast.CallExpr) ([]ast.Stmt, error) {
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
			parseArgStatements, err := appendParseArgumentStatements(statements, t, file, sigs, parsed, receiver, arg)
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

			callSigExpr, err := file.TypeASTExpression(callSig)
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

			callMethodStatements, err := t.callReceiverMethod(file, resultVarIdent, callSigExpr.(*ast.FuncType), arg)
			if err != nil {
				return nil, err
			}

			statements = append(parseArgStatements, callMethodStatements...)
		case *ast.Ident:
			argType, ok := defaultTemplateNameScope(file, t, arg.Name)
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
						declareFormVar, err := formVariableAssignment(file, arg, param.Type())
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
				s, err := generateParseValueFromStringStatements(file, arg.Name+"Parsed", src, param.Type(), errCheck(file), nil, singleAssignment(token.DEFINE, ast.NewIdent(arg.Name)))
				if err != nil {
					return nil, err
				}
				statements = append(statements, s...)
				t.pathValueTypes[arg.Name] = param.Type()
			case arg.Name == TemplateNameScopeIdentifierForm:
				s, err := appendParseFormToStructStatements(statements, t, file, arg, param)
				if err != nil {
					return nil, err
				}
				statements = s
			default:
				pt, _ := file.TypeASTExpression(param.Type())
				at, _ := file.TypeASTExpression(argType)
				return nil, fmt.Errorf("method expects type %s but %s is %s", source.Format(pt), arg.Name, source.Format(at))
			}
		}
	}
	return statements, nil
}

func appendParseFormToStructStatements(statements []ast.Stmt, t *Template, file *source.File, arg *ast.Ident, param types.Object) ([]ast.Stmt, error) {
	const parsedVariableName = "value"
	statements = append(statements, callParseForm())

	declareFormVar, err := formVariableDeclaration(file, arg, param.Type())
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
			X: file.HTTPErrorCall(ast.NewIdent(httpResponseField(file).Names[0].Name), source.CallError(errIdent), http.StatusBadRequest),
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
			validations, err, ok := source.GenerateValidations(file, ast.NewIdent(parsedVariableName), elemType, fmt.Sprintf("[name=%q]", inputName), inputName, httpResponseField(file).Names[0].Name, dom.NewDocumentFragment(templateNodes))
			if ok && err != nil {
				return nil, err
			}
			parseStatements, err := generateParseValueFromStringStatements(file, parsedVariableName, str, elemType, parseErrCheck, validations, parseResult)
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
			validations, err, ok := source.GenerateValidations(file, ast.NewIdent(parsedVariableName), elemType, fmt.Sprintf("[name=%q]", inputName), inputName, httpResponseField(file).Names[0].Name, dom.NewDocumentFragment(templateNodes))
			if ok && err != nil {
				return nil, err
			}
			parseStatements, err := generateParseValueFromStringStatements(file, parsedVariableName, str, elemType, parseErrCheck, validations, parseResult)
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

func formVariableDeclaration(file *source.File, arg *ast.Ident, tp types.Type) (*ast.DeclStmt, error) {
	typeExp, err := file.TypeASTExpression(tp)
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

func formVariableAssignment(file *source.File, arg *ast.Ident, tp types.Type) (*ast.DeclStmt, error) {
	typeExp, err := file.TypeASTExpression(tp)
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

func httpServeMuxField(file *source.File) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(muxParamName)},
		Type:  &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent(file.AddNetHTTP()), Sel: ast.NewIdent("ServeMux")}},
	}
}

func generateParseValueFromStringStatements(file *source.File, tmp string, str ast.Expr, valueType types.Type, errCheck func(expr ast.Expr) ast.Stmt, validations []ast.Stmt, assignment func(ast.Expr) ast.Stmt) ([]ast.Stmt, error) {
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
			return parseBlock(tmp, file.StrconvParseBoolCall(str), validations, errCheck, assignment), nil
		case "int":
			return parseBlock(tmp, file.StrconvAtoiCall(str), validations, errCheck, assignment), nil
		case "int8":
			return parseBlock(tmp, file.StrconvParseInt8Call(str), validations, errCheck, convert), nil
		case "int16":
			return parseBlock(tmp, file.StrconvParseInt16Call(str), validations, errCheck, convert), nil
		case "int32":
			return parseBlock(tmp, file.StrconvParseInt32Call(str), validations, errCheck, convert), nil
		case "int64":
			return parseBlock(tmp, file.StrconvParseInt64Call(str), validations, errCheck, assignment), nil
		case "uint":
			return parseBlock(tmp, file.StrconvParseUint0Call(str), validations, errCheck, convert), nil
		case "uint8":
			return parseBlock(tmp, file.StrconvParseUint8Call(str), validations, errCheck, convert), nil
		case "uint16":
			return parseBlock(tmp, file.StrconvParseUint16Call(str), validations, errCheck, convert), nil
		case "uint32":
			return parseBlock(tmp, file.StrconvParseUint32Call(str), validations, errCheck, convert), nil
		case "uint64":
			return parseBlock(tmp, file.StrconvParseUint64Call(str), validations, errCheck, assignment), nil
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
		if encPkg, ok := file.Types("encoding"); ok {
			if textUnmarshaler := encPkg.Scope().Lookup("TextUnmarshaler").Type().Underlying().(*types.Interface); types.Implements(types.NewPointer(tp), textUnmarshaler) {
				tp, _ := file.TypeASTExpression(valueType)
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
	tp, _ := file.TypeASTExpression(valueType)
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

func callReceiverMethod(dataVarIdent string, method *types.Signature, call *ast.CallExpr) ([]ast.Stmt, ast.Expr, ast.Expr, error) {
	const (
		okIdent = "ok"
	)
	if method.Results().Len() == 0 {
		methodIdent := call.Fun.(*ast.Ident)
		assert.NotNil(assertion, methodIdent)
		return nil, nil, nil, fmt.Errorf("method %s has no results it should have one or two", methodIdent.Name)
	} else if method.Results().Len() > 1 {
		lastResult := method.Results().At(method.Results().Len() - 1)

		errorType := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
		assert.NotNil(assertion, errorType)

		if types.Implements(lastResult.Type(), errorType) {
			statements := []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
			}
			okExpr := &ast.BinaryExpr{
				X:  ast.NewIdent(errIdent),
				Op: token.EQL,
				Y:  source.Nil(),
			}
			errExpr := ast.NewIdent(errIdent)
			return statements, okExpr, errExpr, nil
		}

		if basic, ok := lastResult.Type().(*types.Basic); ok && basic.Kind() == types.Bool {
			statements := []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent), ast.NewIdent(okIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
			}
			okExpr := ast.NewIdent(okIdent)
			errExpr := ast.NewIdent("nil")
			return statements, okExpr, errExpr, nil
		}

		return nil, nil, nil, fmt.Errorf("expected last result to be either an error or a bool")
	} else {
		okExpr := ast.NewIdent("true")
		errExpr := ast.NewIdent("nil")
		return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}}}, okExpr, errExpr, nil
	}
}

var assertion AssertionFailureReporter

type AssertionFailureReporter struct{}

func (AssertionFailureReporter) Errorf(format string, args ...interface{}) {
	log.Fatalf(format, args...)
}

func defaultTemplateNameScope(file *source.File, template *Template, argumentIdentifier string) (types.Type, bool) {
	switch argumentIdentifier {
	case TemplateNameScopeIdentifierHTTPRequest:
		pkg, ok := file.Types("net/http")
		if !ok {
			return nil, false
		}
		t := types.NewPointer(pkg.Scope().Lookup("Request").Type())
		return t, true
	case TemplateNameScopeIdentifierHTTPResponse:
		pkg, ok := file.Types("net/http")
		if !ok {
			return nil, false
		}
		t := pkg.Scope().Lookup("ResponseWriter").Type()
		return t, true
	case TemplateNameScopeIdentifierContext:
		pkg, ok := file.Types("context")
		if !ok {
			return nil, false
		}
		t := pkg.Scope().Lookup("Context").Type()
		return t, true
	case TemplateNameScopeIdentifierForm:
		pkg, ok := file.Types("net/url")
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

func ensureMethodSignature(file *source.File, signatures map[string]*types.Signature, t *Template, receiver *types.Named, receiverInterface *ast.InterfaceType, call *ast.CallExpr, templatesPackage *types.Package) error {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		isMethod := true
		mo, _, _ := types.LookupFieldOrMethod(receiver, true, receiver.Obj().Pkg(), fun.Name)
		if mo == nil {
			if m, ok := packageScopeFunc(templatesPackage, fun); ok {
				mo = m
				isMethod = false
			} else {
				ms, err := createMethodSignature(file, signatures, t, receiver, receiverInterface, call, templatesPackage)
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
					if err := ensureMethodSignature(file, signatures, t, receiver, receiverInterface, arg, templatesPackage); err != nil {
						return err
					}
				}
			}
		}
		signatures[fun.Name] = mo.Type().(*types.Signature)
		if !isMethod {
			return nil
		}
		exp, err := file.TypeASTExpression(mo.Type())
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

func createMethodSignature(file *source.File, signatures map[string]*types.Signature, t *Template, receiver *types.Named, receiverInterface *ast.InterfaceType, call *ast.CallExpr, templatesPackage *types.Package) (*types.Signature, error) {
	var params []*types.Var
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			tp, ok := defaultTemplateNameScope(file, t, arg.Name)
			if !ok {
				return nil, fmt.Errorf("could not determine a type for %s", arg.Name)
			}
			params = append(params, types.NewVar(0, receiver.Obj().Pkg(), arg.Name, tp))
		case *ast.CallExpr:
			if err := ensureMethodSignature(file, signatures, t, receiver, receiverInterface, arg, templatesPackage); err != nil {
				return nil, err
			}
		}
	}
	results := types.NewTuple(types.NewVar(0, nil, "", types.Universe.Lookup("any").Type()))
	return types.NewSignatureType(types.NewVar(0, nil, "", receiver.Obj().Type()), nil, nil, types.NewTuple(params...), results, false), nil
}

func errCheck(file *source.File) func(msg ast.Expr) ast.Stmt {
	return func(msg ast.Expr) ast.Stmt {
		return &ast.ExprStmt{
			X: file.HTTPErrorCall(ast.NewIdent(httpResponseField(file).Names[0].Name), msg, http.StatusBadRequest),
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

func httpResponseField(file *source.File) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)},
		Type:  &ast.SelectorExpr{X: ast.NewIdent(file.AddNetHTTP()), Sel: ast.NewIdent(httpResponseWriterIdent)},
	}
}

func httpRequestField(file *source.File) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest)},
		Type:  &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent(file.AddNetHTTP()), Sel: ast.NewIdent(httpRequestIdent)}},
	}
}

func httpHandlerFuncType(file *source.File) *ast.FuncType {
	return &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{httpResponseField(file), httpRequestField(file)}}}
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

func executeFuncDecl(file *source.File, t Template, resultType types.Type, templatesVariableIdent, executeTemplateAndWriteHeadersFuncIdent, executeTemplateFuncIdent string, result ast.Expr) []ast.Stmt {
	executeArgs := []ast.Expr{
		ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
		ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
		executeTemplateClosureLiteral(file, &t, resultType, templatesVariableIdent, result),
	}

	var statements []ast.Stmt

	if !t.hasResponseWriterArg {
		statements = append(statements, &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  ast.NewIdent(executeTemplateAndWriteHeadersFuncIdent),
			Args: executeArgs,
		}})
	} else {
		statements = append(statements, &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  ast.NewIdent(executeTemplateFuncIdent),
			Args: executeArgs,
		}})
	}

	return statements
}

var statusCoder = statusCoderInterface()

func executeTemplateClosureLiteral(file *source.File, t *Template, resultType types.Type, templatesVariableIdent string, result ast.Expr) *ast.FuncLit {
	const (
		writerIdent     = "w"
		resultDataIdent = "rd"
	)
	var closureType *ast.FuncType

	body := &ast.BlockStmt{
		List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(resultDataIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{result},
			},
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(errIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{
					&ast.CallExpr{
						Fun:  &ast.SelectorExpr{X: ast.NewIdent(templatesVariableIdent), Sel: ast.NewIdent("ExecuteTemplate")},
						Args: []ast.Expr{ast.NewIdent(writerIdent), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(t.name)}, ast.NewIdent(resultDataIdent)},
					}},
			},
		},
	}

	if t.hasResponseWriterArg {
		closureType = executeTemplateLiteralType(file, writerIdent)
		body.List = append(body.List, &ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(errIdent)}})
	} else {
		const statusCode = "sc"
		statusCodePriorityList := []ast.Expr{
			&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(templateDataFieldStatusCode)},
		}
		if resultType != nil {
			if types.Implements(resultType, statusCoder) {
				statusCodePriorityList = append(statusCodePriorityList, &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("result"), Sel: ast.NewIdent("StatusCode")}})
			} else if obj, _, _ := types.LookupFieldOrMethod(resultType, true, file.OutputPackage().Types, "StatusCode"); obj != nil {
				statusCodePriorityList = append(statusCodePriorityList, &ast.SelectorExpr{X: ast.NewIdent("result"), Sel: ast.NewIdent("StatusCode")})
			}
		}
		statusCodePriorityList = append(statusCodePriorityList, source.HTTPStatusCode(file, t.defaultStatusCode))
		closureType = executeTemplateWithStatusCodeResultLiteralType(file, writerIdent)
		body.List = append(body.List,
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(statusCode)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{
					file.Call("", "cmp", "Or", statusCodePriorityList),
				},
			},
			&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent(statusCode), ast.NewIdent(errIdent)}},
		)
	}
	return &ast.FuncLit{
		Type: closureType,
		Body: body,
	}
}

func executeTemplateSlogLine(file *source.File) *ast.CallExpr {
	args := []ast.Expr{
		&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Context")}},
		source.String(executeTemplateErrorMessage),

		file.SlogString("path", &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("URL")},
			Sel: ast.NewIdent("Path"),
		}),
		file.SlogString("pattern", &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Pattern")}),
		file.SlogString("error", source.CallError(errIdent)),
	}
	return file.Call("", "log/slog", "ErrorContext", args)
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
