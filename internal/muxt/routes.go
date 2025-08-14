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

	defaultPackageName                = "main"
	DefaultTemplatesVariableName      = "templates"
	DefaultRoutesFunctionName         = "TemplateRoutes"
	DefaultOutputFileName             = "template_routes.go"
	DefaultReceiverInterfaceName      = "RoutesReceiver"
	DefaultTemplateRoutePathsTypeName = "TemplateRoutePaths"

	InputAttributeNameStructTag     = "name"
	InputAttributeTemplateStructTag = "template"

	muxParamName      = "mux"
	receiverParamName = "receiver"

	errIdent = "err"

	DefaultTemplateDataTypeName = "TemplateData"
	templateDataFieldStatusCode = "statusCode"

	executeTemplateErrorMessage = "failed to render page"
)

func newResponseDataFuncIdent(templateDataTypeName string) string {
	return "new" + templateDataTypeName
}

type RoutesFileConfiguration struct {
	MuxtVersion,
	PackageName,
	PackagePath,
	TemplatesVariable,
	RoutesFunction,
	ReceiverType,
	ReceiverPackage,
	ReceiverInterface,
	TemplateDataType,
	TemplateRoutePathsTypeName string
	OutputFileName string
}

func (config RoutesFileConfiguration) applyDefaults() RoutesFileConfiguration {
	config.PackageName = cmp.Or(config.PackageName, defaultPackageName)
	config.TemplatesVariable = cmp.Or(config.TemplatesVariable, DefaultTemplatesVariableName)
	config.RoutesFunction = cmp.Or(config.RoutesFunction, DefaultRoutesFunctionName)
	config.ReceiverInterface = cmp.Or(config.ReceiverInterface, DefaultReceiverInterfaceName)
	config.TemplateDataType = cmp.Or(config.TemplateDataType, DefaultTemplateDataTypeName)
	config.TemplateRoutePathsTypeName = cmp.Or(config.TemplateRoutePathsTypeName, DefaultTemplateRoutePathsTypeName)
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
	receiver, err := resolveReceiver(config, file, routesPkg)
	if err != nil {
		return "", err
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

	for i := range templates {
		t := &templates[i]
		const dataVarIdent = "result"
		logger.Printf("generating handler for pattern %s", t.pattern)
		if t.fun == nil {
			handlerFunc := noReceiverMethodCall(file, t, config.TemplateDataType, config.TemplatesVariable, dataVarIdent)
			call := t.callHandleFunc(handlerFunc)
			routesFunc.Body.List = append(routesFunc.Body.List, call)
			continue
		}
		handlerFunc, err := methodHandlerFunc(file, t, receiver, receiverInterface, routesPkg.Types, config.TemplateDataType, config.TemplatesVariable, dataVarIdent)
		if err != nil {
			return "", err
		}
		call := t.callHandleFunc(handlerFunc)
		routesFunc.Body.List = append(routesFunc.Body.List, call)
	}

	routePathDecls, err := routePathTypeAndMethods(file, templates, config.TemplateRoutePathsTypeName)
	if err != nil {
		return "", err
	}

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

			templateDataType(file, config.TemplateDataType, ast.NewIdent(config.ReceiverInterface)),
			newTemplateData(file, ast.NewIdent(config.ReceiverInterface), config.TemplateDataType),
			templateDataMuxtVersionMethod(config),
			templateDataPathMethod(config.TemplateDataType, config.TemplateRoutePathsTypeName),
			templateDataResultMethod(config.TemplateDataType),
			templateDataRequestMethod(file, config.TemplateDataType),
			templateDataStatusCodeMethod(config.TemplateDataType),
			templateDataHeaderMethod(config.TemplateDataType),
			templateDataOkay(config.TemplateDataType),
			templateDataError(config.TemplateDataType),
			templateDataReceiver(ast.NewIdent(config.ReceiverInterface), config.TemplateDataType),
			templateRedirect(file, config.TemplateDataType),

			// func newResultData
		}, routePathDecls...),
	}

	return source.FormatFile(filepath.Join(wd, config.OutputFileName), outputFile)
}

func resolveReceiver(config RoutesFileConfiguration, file *source.File, routesPkg *packages.Package) (*types.Named, error) {
	if config.ReceiverType == "" {
		receiver := types.NewNamed(types.NewTypeName(0, routesPkg.Types, "Receiver", nil), types.NewStruct(nil, nil), nil)
		return receiver, nil
	}

	receiverPkgPath := cmp.Or(config.ReceiverPackage, config.PackagePath)
	receiverPkg, ok := file.Package(receiverPkgPath)
	if !ok {
		return nil, fmt.Errorf("could not determine receiver package %s", receiverPkgPath)
	}
	obj := receiverPkg.Types.Scope().Lookup(config.ReceiverType)
	if config.ReceiverType != "" && obj == nil {
		return nil, fmt.Errorf("could not find receiver type %s in %s", config.ReceiverType, receiverPkg.PkgPath)
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return nil, fmt.Errorf("expected receiver %s to be a named type", config.ReceiverType)
	}

	return named, nil
}

func noReceiverMethodCall(file *source.File, t *Template, templateDataTypeIdent, templatesVariableIdent, dataVarIdent string) *ast.FuncLit {
	const (
		bufIdent             = "buf"
		statusCodeIdent      = "statusCode"
		templateDataVarIdent = "td"
	)
	handlerFunc := &ast.FuncLit{
		Type: httpHandlerFuncType(file),
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{&ast.CompositeLit{Type: source.EmptyStructType()}},
				},
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(bufIdent)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{file.BytesNewBuffer(source.Nil())},
				},
			},
		},
	}

	callNewTemplateData := &ast.CallExpr{
		Fun: ast.NewIdent(newResponseDataFuncIdent(templateDataTypeIdent)),
		Args: []ast.Expr{
			ast.NewIdent(receiverIdent),
			ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
			ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
			ast.NewIdent(dataVarIdent),
			ast.NewIdent("true"),
			ast.NewIdent("nil"),
		},
	}
	handlerFunc.Body.List = append(handlerFunc.Body.List, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(templateDataVarIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{callNewTemplateData},
	})

	execTemplates := checkExecuteTemplateError(file)
	execTemplates.Init = &ast.AssignStmt{
		Lhs: []ast.Expr{
			ast.NewIdent(errIdent),
		},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(templatesVariableIdent), Sel: ast.NewIdent("ExecuteTemplate")},
			Args: []ast.Expr{ast.NewIdent(bufIdent), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(t.name)}, ast.NewIdent(templateDataVarIdent)},
		}},
	}

	handlerFunc.Body.List = append(handlerFunc.Body.List, execTemplates)

	handlerFunc.Body.List = append(handlerFunc.Body.List, writeStatusAndHeaders(file, t, types.NewStruct(nil, nil), t.defaultStatusCode, statusCodeIdent, bufIdent, templateDataVarIdent)...)
	return handlerFunc
}

func methodHandlerFunc(file *source.File, t *Template, receiver *types.Named, receiverInterface *ast.InterfaceType, outputPkg *types.Package, templateDataTypeIdent, templatesVariableIdent, dataVarIdent string) (*ast.FuncLit, error) {
	const (
		bufIdent        = "buf"
		statusCodeIdent = "statusCode"
		resultDataIdent = "td"
	)
	sigs := make(map[string]*types.Signature)
	if err := ensureMethodSignature(file, sigs, t, receiver, receiverInterface, t.call, outputPkg); err != nil {
		return nil, err
	}
	sig, ok := sigs[t.fun.Name]
	if !ok {
		return nil, fmt.Errorf("failed to determine call signature %s", t.fun.Name)
	}
	if sig.Results().Len() == 0 {
		return nil, fmt.Errorf("method for pattern %q has no results it should have one or two", t.name)
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

	resultType := sig.Results().At(0).Type()

	var err error
	if handlerFunc.Body.List, err = appendParseArgumentStatements(handlerFunc.Body.List, t, file, resultType, sigs, nil, receiver, templateDataTypeIdent, templatesVariableIdent, t.call, func(s string) *ast.BlockStmt {
		errBlock, err := errorResultBlock(file, t, resultType, http.StatusBadRequest, templateDataTypeIdent, templatesVariableIdent, &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(file.Import("", "errors")), Sel: ast.NewIdent("New")},
			Args: []ast.Expr{source.String(s)},
		})
		if err != nil {
			panic(err)
		}
		return errBlock
	}); err != nil {
		return nil, err
	}

	receiverCallStatements, err := callReceiverMethod(file, t, templateDataTypeIdent, templatesVariableIdent, dataVarIdent, sig, &ast.CallExpr{
		Fun:  callFun,
		Args: slices.Clone(t.call.Args),
	})
	if err != nil {
		return nil, err
	}
	handlerFunc.Body.List = append(handlerFunc.Body.List, receiverCallStatements...)

	handlerFunc.Body.List = append(handlerFunc.Body.List, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(resultDataIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: ast.NewIdent(newResponseDataFuncIdent(templateDataTypeIdent)),
			Args: []ast.Expr{
				ast.NewIdent(receiverIdent),
				ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
				ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
				ast.NewIdent(dataVarIdent),
				ast.NewIdent("true"),
				ast.NewIdent("nil"),
			},
		}},
	})

	handlerFunc.Body.List = append(handlerFunc.Body.List, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(bufIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{file.BytesNewBuffer(source.Nil())},
	})

	execTemplates := checkExecuteTemplateError(file)
	execTemplates.Init = &ast.AssignStmt{
		Lhs: []ast.Expr{
			ast.NewIdent(errIdent),
		},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(templatesVariableIdent), Sel: ast.NewIdent("ExecuteTemplate")},
			Args: []ast.Expr{ast.NewIdent(bufIdent), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(t.name)}, ast.NewIdent(resultDataIdent)},
		}},
	}

	handlerFunc.Body.List = append(handlerFunc.Body.List, execTemplates)

	if !t.hasResponseWriterArg {
		handlerFunc.Body.List = append(handlerFunc.Body.List, writeStatusAndHeaders(file, t, resultType, t.defaultStatusCode, statusCodeIdent, bufIdent, resultDataIdent)...)
	} else {
		handlerFunc.Body.List = append(handlerFunc.Body.List, callWriteOnResponse(bufIdent))
	}

	return handlerFunc, nil
}

func writeBodyAndWriteHeadersFunc(file *source.File, bufIdent, statusCodeIdent string) []ast.Stmt {
	return []ast.Stmt{
		setContentTypeHeaderSetOnTemplateData(),
		&ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("Header")}, Args: []ast.Expr{}}, Sel: ast.NewIdent("Set")},
			Args: []ast.Expr{source.String("content-length"), file.StrconvItoaCall(&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Len")}, Args: []ast.Expr{}})},
		}},
		callWriteHeader(ast.NewIdent(statusCodeIdent)),
		callWriteOnResponse(bufIdent),
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
				&ast.ExprStmt{X: executeTemplateFailedLogLine(file, executeTemplateErrorMessage, errIdent)},
				&ast.ExprStmt{X: file.HTTPErrorCall(ast.NewIdent(httpResponseField(file).Names[0].Name), source.String(executeTemplateErrorMessage), http.StatusInternalServerError)},
				&ast.ReturnStmt{},
			},
		},
	}
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

func templateDataType(file *source.File, templateTypeIdent string, receiverType ast.Expr) *ast.GenDecl {
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name: ast.NewIdent(templateTypeIdent),
				TypeParams: &ast.FieldList{
					List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("T")}, Type: ast.NewIdent("any")}},
				},
				Type: &ast.StructType{
					Fields: &ast.FieldList{
						List: []*ast.Field{
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierReceiver)}, Type: receiverType},
							{Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)}, Type: file.HTTPResponseWriter()},
							{Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest)}, Type: file.HTTPRequestPtr()},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierResult)}, Type: ast.NewIdent("T")},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierStatusCode)}, Type: ast.NewIdent("int")},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierOkay)}, Type: ast.NewIdent("bool")},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierError)}, Type: ast.NewIdent("error")},
							{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierRedirectURL)}, Type: ast.NewIdent("string")},
						},
					},
				},
			},
		},
	}
}

func newTemplateData(file *source.File, receiverType ast.Expr, templateDataTypeIdent string) *ast.FuncDecl {
	const (
		okayIdent       = "okay"
		resultParamName = "result"
	)
	return &ast.FuncDecl{
		Name: ast.NewIdent(newResponseDataFuncIdent(templateDataTypeIdent)),
		Type: &ast.FuncType{
			TypeParams: &ast.FieldList{
				List: []*ast.Field{
					{Names: []*ast.Ident{ast.NewIdent("T")}, Type: ast.NewIdent("any")},
				},
			},
			Params: &ast.FieldList{
				List: []*ast.Field{
					{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierReceiver)}, Type: receiverType},
					{Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)}, Type: file.HTTPResponseWriter()},
					{Names: []*ast.Ident{ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest)}, Type: file.HTTPRequestPtr()},
					{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierResult)}, Type: ast.NewIdent("T")},
					{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierOkay)}, Type: ast.NewIdent("bool")},
					{Names: []*ast.Ident{ast.NewIdent(TemplateDataFieldIdentifierError)}, Type: ast.NewIdent("error")},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: &ast.StarExpr{X: &ast.IndexExpr{
						X:     ast.NewIdent(templateDataTypeIdent),
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
								X:     ast.NewIdent(templateDataTypeIdent),
								Index: ast.NewIdent("T"),
							},
							Elts: []ast.Expr{
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierReceiver), Value: ast.NewIdent(TemplateDataFieldIdentifierReceiver)},
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse), Value: ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse)},
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest), Value: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest)},
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierResult), Value: ast.NewIdent(resultParamName)},
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierOkay), Value: ast.NewIdent(okayIdent)},
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierError), Value: ast.NewIdent(TemplateDataFieldIdentifierError)},
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierRedirectURL), Value: source.String("")},
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

func templateDataMethodReceiver(templateDataTypeIdent string) *ast.FieldList {
	return &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent(templateDataReceiverName)}, Type: &ast.StarExpr{X: &ast.IndexExpr{
		X:     ast.NewIdent(templateDataTypeIdent),
		Index: ast.NewIdent("T"),
	}}}}}
}

func templateDataOkay(templateDataTypeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Ok"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("bool")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("okay")}},
				},
			},
		},
	}
}

func templateDataError(templateDataTypeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Err"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("error")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent(TemplateDataFieldIdentifierError)}},
				},
			},
		},
	}
}

func templateDataReceiver(receiverType ast.Expr, templateDataTypeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Receiver"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Type: receiverType}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.ReturnStmt{
					Results: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent("receiver")}},
				},
			},
		},
	}
}

func templateRedirect(file *source.File, templateDataTypeIdent string) *ast.FuncDecl {
	const (
		codeParamIdent = "code"
		urlParamIdent  = "url"
	)
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Redirect"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ast.NewIdent(urlParamIdent)}, Type: ast.NewIdent("string")},
				{Names: []*ast.Ident{ast.NewIdent(codeParamIdent)}, Type: ast.NewIdent("int")},
			}},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: &ast.StarExpr{X: &ast.IndexExpr{X: ast.NewIdent(templateDataTypeIdent), Index: ast.NewIdent("T")}}},
					{Type: ast.NewIdent("error")},
				},
			},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{
						X: &ast.BinaryExpr{
							X:  ast.NewIdent(codeParamIdent),
							Op: token.LSS,
							Y:  source.Int(300),
						},
						Op: token.LOR,
						Y: &ast.BinaryExpr{
							X:  ast.NewIdent(codeParamIdent),
							Op: token.GEQ,
							Y:  source.Int(400),
						},
					},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.ReturnStmt{Results: []ast.Expr{
								ast.NewIdent(templateDataReceiverName),
								file.Call("", "fmt", "Errorf", []ast.Expr{source.String("invalid status code %d for redirect"), ast.NewIdent("code")}),
							}},
						},
					},
				},
				&ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(templateDataReceiverName), Sel: ast.NewIdent(TemplateDataFieldIdentifierRedirectURL)}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{ast.NewIdent("url")},
				},
				&ast.ReturnStmt{Results: []ast.Expr{
					&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent(templateDataReceiverName),
							Sel: ast.NewIdent("StatusCode"),
						},
						Args: []ast.Expr{ast.NewIdent(codeParamIdent)},
					},
					source.Nil(),
				}},
			},
		},
	}
}

func templateDataMuxtVersionMethod(config RoutesFileConfiguration) *ast.FuncDecl {
	const versionIdent = "muxtVersion"
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(config.TemplateDataType),
		Name: ast.NewIdent("MuxtVersion"),
		Type: &ast.FuncType{
			Results: &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ast.NewIdent("")}, Type: ast.NewIdent("string")}}},
		},
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.DeclStmt{
					Decl: &ast.GenDecl{
						Tok: token.CONST,
						Specs: []ast.Spec{
							&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(versionIdent)}, Values: []ast.Expr{source.String(config.MuxtVersion)}},
						},
					},
				},
				&ast.ReturnStmt{
					Results: []ast.Expr{ast.NewIdent(versionIdent)},
				},
			},
		},
	}
}

func templateDataPathMethod(templateDataTypeIdent, urlHelperTypeName string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
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

func templateDataResultMethod(templateDataTypeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
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

func templateDataRequestMethod(file *source.File, templateDataTypeIdent string) *ast.FuncDecl {
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
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

func templateDataStatusCodeMethod(templateDataTypeIdent string) *ast.FuncDecl {
	const (
		scIdent = "statusCode"
	)
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("StatusCode"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent(scIdent)},
				Type:  ast.NewIdent("int"),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: &ast.StarExpr{X: &ast.IndexExpr{
					X:     ast.NewIdent(templateDataTypeIdent),
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

func templateDataHeaderMethod(templateDataTypeIdent string) *ast.FuncDecl {
	const (
		this       = "data"
		keyIdent   = "key"
		valueIdent = "value"
	)
	return &ast.FuncDecl{
		Recv: templateDataMethodReceiver(templateDataTypeIdent),
		Name: ast.NewIdent("Header"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent("key"), ast.NewIdent("value")},
				Type:  ast.NewIdent("string"),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{
				Type: &ast.StarExpr{X: &ast.IndexExpr{
					X:     ast.NewIdent(templateDataTypeIdent),
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

func appendParseArgumentStatements(statements []ast.Stmt, t *Template, file *source.File, resultType types.Type, sigs map[string]*types.Signature, parsed map[string]struct{}, receiver *types.Named, templateDataTypeIdent, templatesVariableIdent string, call *ast.CallExpr, validationFailureBlock source.ValidationErrorBlock) ([]ast.Stmt, error) {
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
			parseArgStatements, err := appendParseArgumentStatements(statements, t, file, resultType, sigs, parsed, receiver, templateDataTypeIdent, templatesVariableIdent, arg, validationFailureBlock)
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

			if isMethodCall {
				arg.Fun = &ast.SelectorExpr{
					X:   ast.NewIdent(receiverIdent),
					Sel: ast.NewIdent(arg.Fun.(*ast.Ident).Name),
				}
			} else {
				arg.Fun = ast.NewIdent(arg.Fun.(*ast.Ident).Name)
			}

			callMethodStatements, err := callReceiverMethod(file, t, templateDataTypeIdent, templatesVariableIdent, resultVarIdent, callSig, arg)
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
				s, err := generateParseValueFromStringStatements(file, t, arg.Name+"Parsed", resultType, src, param.Type(), nil, singleAssignment(token.DEFINE, ast.NewIdent(arg.Name)), templateDataTypeIdent, templatesVariableIdent)
				if err != nil {
					return nil, err
				}
				statements = append(statements, s...)
				t.pathValueTypes[arg.Name] = param.Type()
			case arg.Name == TemplateNameScopeIdentifierForm:
				s, err := appendParseFormToStructStatements(statements, t, file, resultType, arg, param, validationFailureBlock, templateDataTypeIdent, templatesVariableIdent)
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

func appendParseFormToStructStatements(statements []ast.Stmt, t *Template, file *source.File, resultType types.Type, arg *ast.Ident, param types.Object, validationBlock source.ValidationErrorBlock, templateDataTypeIdent, templatesVariableIdent string) ([]ast.Stmt, error) {
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
			validations, err, ok := source.GenerateValidations(file, ast.NewIdent(parsedVariableName), elemType, fmt.Sprintf("[name=%q]", inputName), inputName, httpResponseField(file).Names[0].Name, dom.NewDocumentFragment(templateNodes), validationBlock)
			if ok && err != nil {
				return nil, err
			}
			parseStatements, err := generateParseValueFromStringStatements(file, t, parsedVariableName, resultType, str, elemType, validations, parseResult, templateDataTypeIdent, templatesVariableIdent)
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
			validations, err, ok := source.GenerateValidations(file, ast.NewIdent(parsedVariableName), elemType, fmt.Sprintf("[name=%q]", inputName), inputName, httpResponseField(file).Names[0].Name, dom.NewDocumentFragment(templateNodes), validationBlock)
			if ok && err != nil {
				return nil, err
			}
			parseStatements, err := generateParseValueFromStringStatements(file, t, parsedVariableName, resultType, str, elemType, validations, parseResult, templateDataTypeIdent, templatesVariableIdent)
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

func errorResultBlock(file *source.File, t *Template, resultType types.Type, fallbackStatusCode int, templateDataTypeIdent, templatesVariableIdent string, errExp ast.Expr) (*ast.BlockStmt, error) {
	const (
		statusCodeIdent = "sc"
		errIdent        = "err"
		resultDataIdent = "rd"
		zeroValueIdent  = "zv"
		bufIdent        = "buf"
	)

	typeExpr, err := file.TypeASTExpression(resultType)
	if err != nil {
		return nil, err
	}

	execTemplate := checkExecuteTemplateError(file)
	execTemplate.Init = &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(errIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(templatesVariableIdent), Sel: ast.NewIdent("ExecuteTemplate")},
			Args: []ast.Expr{ast.NewIdent(bufIdent), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(t.name)}, ast.NewIdent(resultDataIdent)},
		}},
	}

	list := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{
				&ast.ValueSpec{
					Names: []*ast.Ident{ast.NewIdent(zeroValueIdent)},
					Type:  typeExpr,
				},
			},
		}},
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(resultDataIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: ast.NewIdent(newResponseDataFuncIdent(templateDataTypeIdent)),
				Args: []ast.Expr{
					ast.NewIdent(receiverIdent),
					ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
					ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
					ast.NewIdent(zeroValueIdent),
					source.Bool(false),
					errExp,
				},
			}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(bufIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{file.BytesNewBuffer(source.Nil())},
		},
		execTemplate,
	}

	list = append(list, writeStatusAndHeaders(file, t, resultType, fallbackStatusCode, statusCodeIdent, bufIdent, resultDataIdent)...)

	block := &ast.BlockStmt{List: append(list, &ast.ReturnStmt{})}
	return block, nil
}

func generateParseValueFromStringStatements(file *source.File, t *Template, tmp string, resultType types.Type, str ast.Expr, valueType types.Type, validations []ast.Stmt, assignment func(ast.Expr) ast.Stmt, templateDataTypeIdent, templatesVariableIdent string) ([]ast.Stmt, error) {
	errBlock, err := errorResultBlock(file, t, resultType, http.StatusBadRequest, templateDataTypeIdent, templatesVariableIdent, ast.NewIdent(errIdent))
	if err != nil {
		return nil, err
	}
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
			return parseBlock(tmp, file.StrconvParseBoolCall(str), validations, errBlock, assignment), nil
		case "int":
			return parseBlock(tmp, file.StrconvAtoiCall(str), validations, errBlock, assignment), nil
		case "int8":
			return parseBlock(tmp, file.StrconvParseInt8Call(str), validations, errBlock, convert), nil
		case "int16":
			return parseBlock(tmp, file.StrconvParseInt16Call(str), validations, errBlock, convert), nil
		case "int32":
			return parseBlock(tmp, file.StrconvParseInt32Call(str), validations, errBlock, convert), nil
		case "int64":
			return parseBlock(tmp, file.StrconvParseInt64Call(str), validations, errBlock, assignment), nil
		case "uint":
			return parseBlock(tmp, file.StrconvParseUint0Call(str), validations, errBlock, convert), nil
		case "uint8":
			return parseBlock(tmp, file.StrconvParseUint8Call(str), validations, errBlock, convert), nil
		case "uint16":
			return parseBlock(tmp, file.StrconvParseUint16Call(str), validations, errBlock, convert), nil
		case "uint32":
			return parseBlock(tmp, file.StrconvParseUint32Call(str), validations, errBlock, convert), nil
		case "uint64":
			return parseBlock(tmp, file.StrconvParseUint64Call(str), validations, errBlock, assignment), nil
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
						Body: errBlock,
					},
					assignment(ast.NewIdent(tmp)),
				}, nil
			}
		}
	}
	tp, _ := file.TypeASTExpression(valueType)
	return nil, fmt.Errorf("unsupported type: %s", source.Format(tp))
}

func parseBlock(tmpIdent string, parseCall ast.Expr, validations []ast.Stmt, errBlock *ast.BlockStmt, handleResult func(out ast.Expr) ast.Stmt) []ast.Stmt {
	const errIdent = "err"
	callParse := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(tmpIdent), ast.NewIdent(errIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{parseCall},
	}
	errCheckStmt := &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: source.Nil()},
		Body: errBlock,
	}
	block := &ast.BlockStmt{List: []ast.Stmt{callParse, errCheckStmt}}
	block.List = append(block.List, validations...)
	block.List = append(block.List, handleResult(ast.NewIdent(tmpIdent)))
	return block.List
}

func callReceiverMethod(file *source.File, t *Template, templateDataTypeIdent, templatesVariableIdent, dataVarIdent string, method *types.Signature, call *ast.CallExpr) ([]ast.Stmt, error) {
	const (
		okIdent = "ok"
	)
	switch method.Results().Len() {
	default:
		methodIdent := call.Fun.(*ast.Ident)
		assert.NotNil(assertion, methodIdent)
		return nil, fmt.Errorf("method %s has no results it should have one or two", methodIdent.Name)
	case 1:
		return []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}}}, nil
	case 2:
		firstResult := method.Results().At(0).Type()
		lastResult := method.Results().At(method.Results().Len() - 1).Type()

		errorType := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
		assert.NotNil(assertion, errorType)

		if types.Implements(lastResult, errorType) {
			errBlock, err := errorResultBlock(file, t, firstResult, http.StatusInternalServerError, templateDataTypeIdent, templatesVariableIdent, ast.NewIdent(errIdent))
			if err != nil {
				return nil, err
			}

			return []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(dataVarIdent), ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{call}},
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: source.Nil()},
					Body: errBlock,
				},
			}, nil
		}

		if basic, ok := lastResult.(*types.Basic); ok && basic.Kind() == types.Bool {
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

var statusCoder = statusCoderInterface()

func writeStatusAndHeaders(file *source.File, _ *Template, resultType types.Type, fallbackStatusCode int, statusCode, bufIdent, resultDataIdent string) []ast.Stmt {
	statusCodePriorityList := []ast.Expr{
		&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(templateDataFieldStatusCode)},
	}
	if types.Implements(resultType, statusCoder) {
		statusCodePriorityList = append(statusCodePriorityList, &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent("result"), Sel: ast.NewIdent("StatusCode")}})
	} else if obj, _, _ := types.LookupFieldOrMethod(resultType, true, file.OutputPackage().Types, "StatusCode"); obj != nil {
		statusCodePriorityList = append(statusCodePriorityList, &ast.SelectorExpr{X: ast.NewIdent("result"), Sel: ast.NewIdent("StatusCode")})
	}
	statusCodePriorityList = append(statusCodePriorityList, source.HTTPStatusCode(file, fallbackStatusCode))
	list := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(statusCode)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{
				file.Call("", "cmp", "Or", statusCodePriorityList),
			},
		},
		&ast.IfStmt{ // TODO: make this conditional on a redirect call in the template actions
			Cond: &ast.BinaryExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent(resultDataIdent),
					Sel: ast.NewIdent(TemplateDataFieldIdentifierRedirectURL),
				},
				Op: token.NEQ,
				Y:  source.String(""),
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{
						X: file.Call("", "net/http", "Redirect", []ast.Expr{
							ast.NewIdent(TemplateNameScopeIdentifierHTTPResponse),
							ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest),
							&ast.SelectorExpr{
								X:   ast.NewIdent(resultDataIdent),
								Sel: ast.NewIdent(TemplateDataFieldIdentifierRedirectURL),
							},
							ast.NewIdent(statusCode),
						}),
					},
					&ast.ReturnStmt{},
				},
			},
		},
	}
	return append(list, writeBodyAndWriteHeadersFunc(file, bufIdent, statusCode)...)
}

func executeTemplateFailedLogLine(file *source.File, message, errIdent string) *ast.CallExpr {
	args := []ast.Expr{
		&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Context")}},
		source.String(message),

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
