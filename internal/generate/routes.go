package generate

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ettle/strcase"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/source"
)

const (
	receiverIdent = "receiver"

	muxVarIdent = "mux"

	requestPathValue         = "PathValue"
	httpRequestContextMethod = "Context"
	httpHandleFuncIdent      = "HandleFunc"
	httpHandleIdent          = "Handle"

	muxParamName        = "mux"
	middlewareParamName = "middleware"

	errIdent                    = "err"
	templateDataFieldStatusCode = "statusCode"

	pathPrefixPathsStructFieldName = "pathsPrefix"

	bufferPoolIdent = "bytesBufferPool"

	executeTemplateErrorMessage = "failed to render page"

	DefaultRoutesFunctionName         = "TemplateRoutes"
	DefaultReceiverInterfaceName      = "RoutesReceiver"
	DefaultTemplateRoutePathsTypeName = "TemplateRoutePaths"
)

type GeneratedFile struct {
	Path    string
	Content string

	// Routes counts the route handlers registered in this file, for the
	// one-line success report after the file is written.
	Routes int
}

type RoutesFileConfiguration struct {
	MuxtVersion,
	PackageName,
	PackagePath,
	RoutesFunction,
	ReceiverType,
	ReceiverPackage,
	ReceiverInterface,
	TemplateDataType,
	SSETemplateDataType,
	TemplateRoutePathsTypeName string
	TemplatesVariables               []string
	OutputFileName                   string
	PathPrefix                       bool
	Logger                           bool
	Middleware                       bool
	Verbose                          bool
	OutputMultipleFiles              bool
	OutputHTMX                       bool
	OutputDatastar                   bool
	OutputExportedDefaultIdentifiers bool
	// OutputMuxtVersion controls whether the muxt version is recorded in
	// generated files (the "// muxt version:" comment and the TemplateData
	// MuxtVersion method). Defaults to true.
	OutputMuxtVersion bool
	// MultipartMaxMemory is the maxMemory value passed to request.ParseMultipartForm.
	// Defaults to 32 MiB when zero.
	MultipartMaxMemory int64

	// SilenceHTTPResponseWarning suppresses the warning printed for
	// routes that take the response argument. Set from the
	// MUXT_SILENCE_WARNING_HTTP_RESPONSE_ARGUMENT environment variable.
	SilenceHTTPResponseWarning bool
}

// DefaultMultipartMaxMemory is the default maxMemory value passed to
// request.ParseMultipartForm when no override is set.
const DefaultMultipartMaxMemory int64 = 32 << 20

// TemplateRoutesFiles generates the routes files for pkg, written into wd:
// the package the files belong to, which is the one in the output file's
// directory. defs are pkg's route definitions, resolved by muxt.ResolveDefinitions.
func TemplateRoutesFiles(wd string, config RoutesFileConfiguration, pkg source.Package, defs []muxt.Definition, logger *log.Logger) ([]GeneratedFile, error) {
	if !token.IsIdentifier(config.PackageName) {
		return nil, fmt.Errorf("package name %q is not an identifier", config.PackageName)
	}

	file := newFile(pkg)

	config.PackagePath = pkg.Types.Path()
	config.PackageName = pkg.Types.Name()
	config.SSETemplateDataType = cmp.Or(config.SSETemplateDataType, "SSETemplateData")

	groups, err := groupTemplates(config, defs)
	if err != nil {
		return nil, err
	}

	var (
		receiverInterface   = &ast.InterfaceType{Methods: new(ast.FieldList)}
		templateSourceFiles = slices.Sorted(maps.Keys(groups.byFile))
	)

	// Build main routes function
	routesFunc := &ast.FuncDecl{
		Name: ast.NewIdent(config.RoutesFunction),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					httpServeMuxField(file),
					{
						Names: []*ast.Ident{ast.NewIdent(receiverIdent)},
						Type:  ast.NewIdent(config.ReceiverInterface),
					},
				},
			},
			Results: &ast.FieldList{
				List: []*ast.Field{
					{Type: ast.NewIdent(config.TemplateRoutePathsTypeName)},
				},
			},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{}},
	}
	if config.Logger {
		routesFunc.Type.Params.List = append(routesFunc.Type.Params.List, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent("logger")},
			Type:  astgen.SlogLoggerPtr(file),
		})
	}
	if config.PathPrefix {
		routesFunc.Type.Params.List = append(routesFunc.Type.Params.List, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string"),
		})
	} else {
		routesFunc.Body.List = append(routesFunc.Body.List, &ast.AssignStmt{
			Tok: token.DEFINE,
			Lhs: []ast.Expr{ast.NewIdent(pathPrefixPathsStructFieldName)},
			Rhs: []ast.Expr{astgen.String("")},
		})
	}
	if config.Middleware {
		routesFunc.Type.Params.List = append(routesFunc.Type.Params.List, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(middlewareParamName)},
			Type:  astgen.HTTPMiddlewareFuncType(file),
		})
		routesFunc.Body.List = append(routesFunc.Body.List, middlewareNilGuard(file))
	}

	var (
		topLevelTemplateRoutes []muxt.Definition
		generatedFiles         []GeneratedFile
	)
	if config.OutputMultipleFiles {
		files, err := sourceFileRouteFunctionFiles(wd, config, templateSourceFiles, groups, logger, file, receiverInterface, routesFunc)
		if err != nil {
			return files, err
		}
		generatedFiles = append(generatedFiles, files...)
		topLevelTemplateRoutes = groups.noFile
	} else {
		topLevelTemplateRoutes = groups.all
	}

	if len(topLevelTemplateRoutes) > 0 {
		routesFunc.Body.List = append(routesFunc.Body.List, bytesBufferPoolDeclaration(file))
	}

	logResolutionNotes(topLevelTemplateRoutes, config, logger)
	if err := collectReceiverMethods(topLevelTemplateRoutes, file, receiverInterface); err != nil {
		return nil, err
	}
	for _, def := range topLevelTemplateRoutes {
		if config.Verbose {
			logger.Printf("generating handler for pattern %s", def.RawPattern())
		}
		if def.FunctionIdentifier() == nil {
			handlerFunc := noReceiverMethodCall(file, def, config, config.ReceiverInterface)
			call := callHandleFunc(file, def, handlerFunc, config)
			routesFunc.Body.List = append(routesFunc.Body.List, call)
			continue
		}
		handlerFunc, err := callHandlerFunc(file, config, def, config.ReceiverInterface)
		if err != nil {
			return nil, err
		}
		call := callHandleFunc(file, def, handlerFunc, config)
		routesFunc.Body.List = append(routesFunc.Body.List, call)
	}

	routePathDecls, err := routePathTypeAndMethods(file, config, groups.all)
	if err != nil {
		return nil, err
	}
	routesFunc.Body.List = append(routesFunc.Body.List, &ast.ReturnStmt{
		Results: []ast.Expr{
			&ast.CompositeLit{
				Type: ast.NewIdent(config.TemplateRoutePathsTypeName),
				Elts: []ast.Expr{
					&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: ast.NewIdent(pathPrefixPathsStructFieldName)},
				},
			},
		},
	})

	// The import declaration is filled in last: building the other
	// declarations is what registers the imports they use.
	importDecl := &ast.GenDecl{Tok: token.IMPORT}
	decls := []ast.Decl{
		importDecl,

		// type
		&ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{
				&ast.TypeSpec{Name: ast.NewIdent(config.ReceiverInterface), Type: receiverInterface},
			},
		},

		// func routes
		routesFunc,
	}
	decls = append(decls, templateDataDecls(file, config, ast.NewIdent(config.ReceiverInterface))...)
	// The SSETemplateData type and its methods are only needed when a route uses
	// the sse render callback, so emit them conditionally to avoid unused imports.
	if slices.ContainsFunc(groups.all, func(definition muxt.Definition) bool {
		return definition.Representation == muxt.RepresentationSSE
	}) {
		decls = append(decls, sseTemplateDataDecls(file, config)...)
	}
	decls = append(decls, routePathDecls...)
	for _, spec := range file.ImportSpecs() {
		importDecl.Specs = append(importDecl.Specs, spec)
	}
	outputFile := &ast.File{
		Name:  ast.NewIdent(config.PackageName),
		Decls: decls,
	}

	filePath := filepath.Join(wd, config.OutputFileName)
	content, err := formatFile(filePath, outputFile)
	if err != nil {
		return nil, err
	}

	// Append main file to generated files
	generatedFiles = append(generatedFiles, GeneratedFile{Path: filePath, Content: content, Routes: len(topLevelTemplateRoutes)})

	return generatedFiles, nil
}

// hydrateGroup resolves each definition's call, collecting every
// resolution error so one run reports them all. When noteSynthesized is
// set (a --use-receiver-type run with a non-nil logger), methods the
// named receiver does not define are announced with their inferred
// signatures — one line per method and the explanation once after the
// list; the default mode synthesizes every method by design, so it
// stays quiet.
// logResolutionNotes reports what resolution found for defs.
func logResolutionNotes(defs []muxt.Definition, config RoutesFileConfiguration, logger *log.Logger) {
	if logger == nil {
		return
	}
	synthesized := 0
	for _, def := range defs {
		if !config.SilenceHTTPResponseWarning && def.HasResponseWriterArg() {
			// Taking over the http.ResponseWriter is an escape hatch:
			// muxt then leaves the response entirely to the method.
			logger.Printf("warning: %s uses the response argument, so muxt does not manage this route's status codes, headers, or rendering; silence with MUXT_SILENCE_WARNING_HTTP_RESPONSE_ARGUMENT=true", def.Pattern())
		}
		if config.ReceiverType == "" {
			continue
		}
		for _, sig := range def.SynthesizedMethods() {
			logger.Printf("note: %s does not define %s", config.ReceiverType, sig)
			synthesized++
		}
	}
	if synthesized > 0 {
		// The results are any until the methods exist, so field checks
		// on .Result are deferred; say so once.
		logger.Printf("note: the inferred signatures return any — implement the methods to type-check the templates against real types")
	}
}

// collectReceiverMethods adds the receiver methods the calls in defs need to
// receiverInterface.
func collectReceiverMethods(defs []muxt.Definition, file *File, receiverInterface *ast.InterfaceType) error {
	for _, def := range defs {
		if def.FunctionIdentifier() == nil {
			continue
		}
		if err := accumulateReceiverMethods(def.FunctionIdentifier().Name, def.Signature(), def.IsMethod(), def.Arguments, file, receiverInterface); err != nil {
			return err
		}
	}
	return nil
}

func accumulateReceiverMethods(name string, sig source.Type, isMethod bool, args []muxt.Argument, file *File, receiverInterface *ast.InterfaceType) error {
	// Recurse into nested call arguments regardless of whether this call is a
	// receiver method: a package-scope function may receive nested receiver
	// method calls that must appear in the interface.
	for _, a := range args {
		if a.Type != muxt.ArgumentTypeCall {
			continue
		}
		if err := accumulateReceiverMethods(a.Identifier, a.Signature(), a.IsMethod(), a.Arguments(), file, receiverInterface); err != nil {
			return err
		}
	}
	if !isMethod {
		return nil
	}
	if slices.ContainsFunc(receiverInterface.Methods.List, func(field *ast.Field) bool {
		return field.Names[0].Name == name
	}) {
		return nil
	}
	exp, err := file.TypeExpr(sig)
	if err != nil {
		return err
	}
	receiverInterface.Methods.List = append(receiverInterface.Methods.List, &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(name)},
		Type:  exp,
	})
	return nil
}

func sourceFileRouteFunctionFiles(wd string, config RoutesFileConfiguration, templateSourceFiles []string, groups templateGroups, logger *log.Logger, file *File, receiverInterface *ast.InterfaceType, routesFunc *ast.FuncDecl) ([]GeneratedFile, error) {
	var generatedFiles []GeneratedFile
	for _, sourceFile := range templateSourceFiles {
		definitions := groups.byFile[sourceFile]
		if config.Verbose {
			logger.Printf("generating routes for %s (%d templates)", sourceFile, len(definitions))
		}

		fileIdentifier := muxt.FileNameToPrivateIdentifier(filepath.Base(sourceFile))
		receiverInterfaceName := strcase.ToGoCamel(fileIdentifier + " " + config.ReceiverInterface)
		routesFuncName := strcase.ToGoCamel(fileIdentifier + " " + config.RoutesFunction)

		perFileAST, err := generatePerFileAST(sourceFile, definitions, newFile(file.OutputPackage()), routesFuncName, receiverInterfaceName, logger, config)
		if err != nil {
			return nil, fmt.Errorf("failed to generate routes for %s: %w", sourceFile, err)
		}

		// Generate filename: strip .gohtml extension, add _template_routes_gen.go
		// sourceFile may be an absolute path, so extract just the base filename
		baseFileName := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
		outputFileName := baseFileName + "_template_routes_gen.go"
		outputFilePath := filepath.Join(wd, outputFileName)

		content, err := formatFile(outputFilePath, perFileAST)
		if err != nil {
			return nil, fmt.Errorf("failed to format %s: %w", outputFileName, err)
		}

		generatedFiles = append(generatedFiles, GeneratedFile{
			Path:    outputFilePath,
			Content: content,
			Routes:  len(definitions),
		})

		receiverInterface.Methods.List = append(receiverInterface.Methods.List, &ast.Field{
			Type: ast.NewIdent(receiverInterfaceName),
		})

		callArgs := []ast.Expr{ast.NewIdent(muxParamName), ast.NewIdent(receiverIdent)}
		if config.Logger {
			callArgs = append(callArgs, ast.NewIdent("logger"))
		}
		// Always pass pathsPrefix to per-file functions
		callArgs = append(callArgs, ast.NewIdent(pathPrefixPathsStructFieldName))
		if config.Middleware {
			callArgs = append(callArgs, ast.NewIdent(middlewareParamName))
		}

		routesFunc.Body.List = append(routesFunc.Body.List, &ast.ExprStmt{
			X: &ast.CallExpr{
				Fun:  ast.NewIdent(routesFuncName),
				Args: callArgs,
			},
		})
	}
	return generatedFiles, nil
}

// bytesBufferPoolDeclaration returns the statement declaring the per-function
// sync.Pool of *bytes.Buffer that handler closures draw their render buffers
// from: bytesBufferPool := sync.Pool{New: func() any { return bytes.NewBuffer(nil) }}.
func bytesBufferPoolDeclaration(file *File) ast.Stmt {
	return &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{ast.NewIdent(bufferPoolIdent)},
		Rhs: []ast.Expr{astgen.SyncPoolBytesBuffer(file)},
	}
}

// middlewareNilGuard emits the statement that makes a nil middleware argument
// a no-op:
//
//	if middleware == nil {
//		middleware = func(next http.Handler) http.Handler { return next }
//	}
func middlewareNilGuard(file *File) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(middlewareParamName), Op: token.EQL, Y: astgen.Nil()},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(middlewareParamName)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.FuncLit{
					Type: astgen.HTTPMiddlewareFuncType(file),
					Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("next")}}}},
				}},
			},
		}},
	}
}

func callHandleFunc(file *File, def muxt.Definition, handlerFuncLit *ast.FuncLit, config RoutesFileConfiguration) *ast.ExprStmt {
	normalized := def.Pattern()
	pattern := ast.Expr(astgen.String(normalized))
	if config.PathPrefix {
		i := strings.Index(normalized, "/")
		pattern = &ast.BinaryExpr{
			X:  astgen.String(normalized[:i]),
			Op: token.ADD,
			Y:  astgen.Call(file, "path", "path", "Join", ast.NewIdent(pathPrefixPathsStructFieldName), astgen.String(normalized[i:])),
		}
	}
	method, handler := httpHandleFuncIdent, ast.Expr(handlerFuncLit)
	if config.Middleware {
		method = httpHandleIdent
		handler = &ast.CallExpr{
			Fun: ast.NewIdent(middlewareParamName),
			Args: []ast.Expr{&ast.CallExpr{
				Fun:  astgen.ExportedIdentifier(file, "http", "net/http", "HandlerFunc"),
				Args: []ast.Expr{handlerFuncLit},
			}},
		}
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(muxVarIdent),
			Sel: ast.NewIdent(method),
		},
		Args: []ast.Expr{pattern, handler},
	}}
}

// generatePerFileRouteFunction creates a route registration function for templates from a specific source file.
// For example, for "index.gohtml", it generates IndexTemplateRoutes(mux, receiver, ...).
func generatePerFileRouteFunction(
	sourceFile string,
	defs []muxt.Definition,
	file *File,
	funcName,
	receiverInterfaceName string,
	logger *log.Logger,
	config RoutesFileConfiguration,
	receiverInterface *ast.InterfaceType,
) (*ast.FuncDecl, error) {
	if sourceFile == "" {
		return nil, fmt.Errorf("sourceFile cannot be empty")
	}

	// Create the function declaration
	routesFunc := &ast.FuncDecl{
		Name: ast.NewIdent(funcName),
		Type: &ast.FuncType{
			Params: &ast.FieldList{
				List: []*ast.Field{
					httpServeMuxField(file),
					{Names: []*ast.Ident{ast.NewIdent(receiverIdent)}, Type: ast.NewIdent(receiverInterfaceName)},
				},
			},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{}},
	}

	if config.Logger {
		routesFunc.Type.Params.List = append(routesFunc.Type.Params.List, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent("logger")},
			Type:  astgen.SlogLoggerPtr(file),
		})
	}

	// Per-file functions always accept pathsPrefix parameter
	routesFunc.Type.Params.List = append(routesFunc.Type.Params.List, &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string"),
	})

	if config.Middleware {
		routesFunc.Type.Params.List = append(routesFunc.Type.Params.List, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(middlewareParamName)},
			Type:  astgen.HTTPMiddlewareFuncType(file),
		})
	}

	// Declare the buffer pool shared by this file's handlers.
	if len(defs) > 0 {
		routesFunc.Body.List = append(routesFunc.Body.List, bytesBufferPoolDeclaration(file))
	}

	// Generate handlers for each template
	logResolutionNotes(defs, config, logger)
	if err := collectReceiverMethods(defs, file, receiverInterface); err != nil {
		return nil, err
	}
	for i := range defs {
		t := defs[i]
		if config.Verbose {
			logger.Printf("generating handler for pattern %s in %s", t.RawPattern(), sourceFile)
		}
		if t.FunctionIdentifier() == nil {
			handlerFunc := noReceiverMethodCall(file, t, config, receiverInterfaceName)
			call := callHandleFunc(file, t, handlerFunc, config)
			routesFunc.Body.List = append(routesFunc.Body.List, call)
			continue
		}
		handlerFunc, err := callHandlerFunc(file, config, t, receiverInterfaceName)
		if err != nil {
			return nil, err
		}
		call := callHandleFunc(file, t, handlerFunc, config)
		routesFunc.Body.List = append(routesFunc.Body.List, call)
	}

	return routesFunc, nil
}

// generatePerFileAST creates a complete AST file for templates from a specific source file.
// Returns an *ast.File ready to be formatted and written.
func generatePerFileAST(
	sourceFile string,
	defs []muxt.Definition,
	file *File,
	funcName, receiverInterfaceName string,
	logger *log.Logger,
	config RoutesFileConfiguration,
) (*ast.File, error) {
	if sourceFile == "" {
		return nil, fmt.Errorf("sourceFile cannot be empty")
	}
	// Create a scoped receiver interface for this file's templates
	scopedReceiverInterface := &ast.InterfaceType{
		Methods: new(ast.FieldList),
	}

	// Generate the route function
	routesFunc, err := generatePerFileRouteFunction(
		sourceFile,
		defs,
		file,
		funcName,
		receiverInterfaceName,
		logger,
		config,
		scopedReceiverInterface,
	)
	if err != nil {
		return nil, err
	}

	// Get import specs
	is := file.ImportSpecs()
	importSpecs := make([]ast.Spec, 0, len(is))
	for _, s := range is {
		importSpecs = append(importSpecs, s)
	}

	// Build the output file
	outputFile := &ast.File{
		Name: ast.NewIdent(config.PackageName),
		Decls: []ast.Decl{
			// imports
			&ast.GenDecl{
				Tok:   token.IMPORT,
				Specs: importSpecs,
			},
			// receiver interface for this file
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{
					&ast.TypeSpec{
						Name: ast.NewIdent(receiverInterfaceName),
						Type: scopedReceiverInterface,
					},
				},
			},
			// routes function
			routesFunc,
		},
	}

	return outputFile, nil
}

func noReceiverMethodCall(file *File, def muxt.Definition, config RoutesFileConfiguration, receiverInterfaceName string) *ast.FuncLit {
	const (
		bufIdent             = "buf"
		statusCodeIdent      = "statusCode"
		templateDataVarIdent = "td"
	)
	handlerFunc := &ast.FuncLit{
		Type: astgen.HTTPHandlerFuncType(file, muxt.TemplateNameScopeIdentifierHTTPResponse, muxt.TemplateNameScopeIdentifierHTTPRequest),
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.DeclStmt{
					Decl: &ast.GenDecl{
						Tok: token.VAR,
						Specs: []ast.Spec{&ast.ValueSpec{
							Names: []*ast.Ident{ast.NewIdent(templateDataVarIdent)},
							Values: []ast.Expr{&ast.CompositeLit{Type: &ast.IndexListExpr{
								X:       ast.NewIdent(config.TemplateDataType),
								Indices: []ast.Expr{ast.NewIdent(receiverInterfaceName), astgen.EmptyStructType()},
							}, Elts: []ast.Expr{
								&ast.KeyValueExpr{Key: ast.NewIdent(TemplateDataFieldIdentifierReceiver), Value: ast.NewIdent(TemplateDataFieldIdentifierReceiver)},
								&ast.KeyValueExpr{Key: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), Value: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse)},
								&ast.KeyValueExpr{Key: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Value: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest)},
								&ast.KeyValueExpr{Key: ast.NewIdent(pathPrefixPathsStructFieldName), Value: ast.NewIdent(pathPrefixPathsStructFieldName)},
							}}},
						}},
					},
				},
			},
		},
	}

	handlerFunc.Body.List = append(handlerFunc.Body.List, astgen.GetBufferFromPool(file, bufferPoolIdent, bufIdent)...)

	callExecuteTemplate(file, config, def, handlerFunc, bufIdent, templateDataVarIdent)

	handlerFunc.Body.List = append(handlerFunc.Body.List, writeStatusAndHeaders(file, def, def.DefaultStatusCode(), statusCodeIdent, bufIdent, templateDataVarIdent, func() ast.Expr {
		panic("when no receiver method is called, then the result variable should not be needed")
	})...)
	return handlerFunc
}

func callHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, receiverInterfaceName string) (*ast.FuncLit, error) {
	const (
		bufIdent        = "buf"
		statusCodeIdent = "statusCode"
		resultDataIdent = "td"
	)

	if def.Signature().IsZero() {
		return nil, fmt.Errorf("call for pattern %s was not resolved", def.Pattern())
	}
	switch def.Representation {
	case muxt.RepresentationSSE:
		return sseMethodHandlerFunc(file, config, def, receiverInterfaceName)
	case muxt.RepresentationMarshalJSON:
		return marshalJSONHandlerFunc(file, config, def, resultDataIdent, receiverInterfaceName, bufIdent, statusCodeIdent)
	default:
		return newHTMLTemplateHandler(file, config, def, resultDataIdent, receiverInterfaceName, bufIdent, statusCodeIdent)
	}
}

func appendTemplateDataError(_ *File, tdIdent string, err ast.Expr) *ast.BlockStmt {
	return &ast.BlockStmt{
		List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(tdIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierError)}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{astgen.CallBuiltinAppend(&ast.SelectorExpr{X: ast.NewIdent(tdIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierError)}, err)},
			},
		},
	}
}

func writeBodyAndWriteHeadersFunc(file *File, bufIdent, statusCodeIdent string) []ast.Stmt {
	return []ast.Stmt{
		setContentTypeHeaderSetOnTemplateData(),
		&ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("Header")}, Args: []ast.Expr{}}, Sel: ast.NewIdent("Set")},
			Args: []ast.Expr{astgen.String("content-length"), astgen.StrconvItoaCall(file, &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Len")}, Args: []ast.Expr{}})},
		}},
		callWriteHeader(ast.NewIdent(statusCodeIdent)),
		callWriteOnResponse(bufIdent),
	}
}

func callWriteHeader(statusCode ast.Expr) *ast.ExprStmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), Sel: ast.NewIdent("WriteHeader")},
		Args: []ast.Expr{statusCode},
	}}
}

func checkExecuteTemplateError(file *File, withLogger bool, pattern string) *ast.IfStmt {
	var logStmts []ast.Stmt
	if withLogger {
		logStmts = []ast.Stmt{
			&ast.ExprStmt{X: loggerErrorCall(file, executeTemplateErrorMessage, pattern, errIdent)},
		}
	} else {
		logStmts = []ast.Stmt{
			&ast.ExprStmt{X: executeTemplateFailedLogLine(file, executeTemplateErrorMessage, errIdent)},
		}
	}
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: &ast.BlockStmt{
			List: append(logStmts,
				&ast.ExprStmt{X: astgen.HTTPErrorCall(file, ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), astgen.String(executeTemplateErrorMessage), http.StatusInternalServerError)},
				&ast.ReturnStmt{},
			),
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
			Args: []ast.Expr{ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse)},
		}},
	}
}

func appendParseArgumentStatements(statements []ast.Stmt, file *File, args []muxt.Argument, rdIdent string, config RoutesFileConfiguration, call *ast.CallExpr, validationFailureBlock ValidationErrorBlock, parseErrBlock func() *ast.BlockStmt) ([]ast.Stmt, error) {
	if parseErrBlock == nil {
		// Normal handlers accumulate scalar-parse failures into the template
		// data (and respond with the recorded error status). SSE handlers pass
		// their own factory to respond 400 before opening the event stream.
		parseErrBlock = func() *ast.BlockStmt { return templateDataParseErrBlock(file, rdIdent) }
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("expected function to be identifier")
	}
	if len(args) != len(call.Args) {
		return nil, fmt.Errorf("call %s was not resolved", fun.Name)
	}
	resultCount := 0
	for i, a := range call.Args {
		switch arg := a.(type) {
		default:
			// TODO: add error case
		case *ast.CallExpr:
			nestedArg := args[i]
			if nestedArg.Type == muxt.ArgumentTypeRequestBodyJSON {
				const bodyValueIdent = "bodyValue"
				decodeStatements, err := decodeJSONBodyStatements(file, bodyValueIdent, nestedArg.ParamType(), parseErrBlock)
				if err != nil {
					return nil, err
				}
				statements = append(statements, decodeStatements...)
				call.Args[i] = ast.NewIdent(bodyValueIdent)
				continue
			}
			parseArgStatements, err := appendParseArgumentStatements(statements, file, nestedArg.Arguments(), rdIdent, config, arg, validationFailureBlock, parseErrBlock)
			if err != nil {
				return nil, err
			}
			resultVarIdent := "result" + strconv.Itoa(resultCount)
			call.Args[i] = ast.NewIdent(resultVarIdent)
			resultCount++

			funcIdent := arg.Fun.(*ast.Ident).Name

			if nestedArg.IsMethod() {
				arg.Fun = &ast.SelectorExpr{
					X:   ast.NewIdent(receiverIdent),
					Sel: ast.NewIdent(funcIdent),
				}
			} else {
				arg.Fun = ast.NewIdent(funcIdent)
			}

			errBody := appendTemplateDataError(file, rdIdent, ast.NewIdent(errIdent))
			errBody.List = append(errBody.List, assignTemplateDataErrStatusCode(file, rdIdent, http.StatusInternalServerError))
			nestedCall, err := callReceiverMethod(rdIdent, ast.NewIdent(resultVarIdent), nestedArg.ResultShape(), funcIdent, arg, errBody)
			if err != nil {
				return nil, err
			}

			statements = append(parseArgStatements, nestedCall.DefineStmts()...)
		case *ast.Ident:
			argument := args[i]
			if argument.Type == muxt.ArgumentTypeExecute || argument.Type == muxt.ArgumentTypeSignalsCallback {
				// Render and signals callbacks are validated and wired into the
				// call in the sse handler assembly. They are not parsed from
				// the request.
				continue
			}
			name := arg.Name
			ident := name
			if argument.Type == muxt.ArgumentTypeRequestPathValue {
				ident = pathParamIdent(name)
				call.Args[i] = ast.NewIdent(ident)
			}
			if argument.Direct() {
				if argument.Declares() {
					switch argument.Type {
					case muxt.ArgumentTypeRequestForm:
						declareFormVar, err := formVariableAssignment(file, arg, argument.ParamType())
						if err != nil {
							return nil, err
						}
						statements = append(statements, callParseForm(file), declareFormVar)
					case muxt.ArgumentTypeRequestMultipartForm:
						declareMultipartVar, err := multipartVariableAssignment(file, arg, argument.ParamType())
						if err != nil {
							return nil, err
						}
						statements = append(statements, callParseMultipartForm(file, config, parseErrBlock()), declareMultipartVar)
					case muxt.ArgumentTypeRequestContext:
						statements = append(statements, contextAssignment(muxt.TemplateNameScopeIdentifierContext))
					case muxt.ArgumentTypeRequestBody, muxt.ArgumentTypeRequestPathValue, muxt.ArgumentTypeLastEventID:
						src, err := requestArgumentSource(argument)
						if err != nil {
							return nil, err
						}
						statements = append(statements, singleAssignment(token.DEFINE, ast.NewIdent(ident))(src))
					}
				}
				continue
			}
			switch argument.Type {
			case muxt.ArgumentTypeRequestPathValue, muxt.ArgumentTypeLastEventID:
				if !argument.Declares() {
					continue
				}
				src, err := requestArgumentSource(argument)
				if err != nil {
					return nil, err
				}
				s, err := generateParseValueFromStringStatements(file, name+"Parsed", src, argument.ParamType(), argument.UnmarshalMethod(), nil, singleAssignment(token.DEFINE, ast.NewIdent(ident)), parseErrBlock())
				if err != nil {
					return nil, err
				}
				statements = append(statements, s...)
			case muxt.ArgumentTypeRequestForm:
				if !argument.Declares() {
					continue
				}
				s, err := appendParseFormToStructStatements(statements, file, arg, argument, validationFailureBlock, parseErrBlock)
				if err != nil {
					return nil, err
				}
				statements = s
			case muxt.ArgumentTypeRequestMultipartForm:
				if !argument.Declares() {
					continue
				}
				s, err := appendParseMultipartFormToStructStatements(statements, file, arg, argument, validationFailureBlock, parseErrBlock, config)
				if err != nil {
					return nil, err
				}
				statements = s
			default:
				if argument.ScopeType().IsZero() {
					return nil, fmt.Errorf("failed to determine type for %s", name)
				}
				pt, _ := file.TypeExpr(argument.ParamType())
				at, _ := file.TypeExpr(argument.ScopeType())
				return nil, fmt.Errorf("method expects type %s but %s is %s", astgen.Format(pt), arg.Name, astgen.Format(at))
			}
		}
	}
	return statements, nil
}

func appendParseFormToStructStatements(statements []ast.Stmt, file *File, arg *ast.Ident, argument muxt.Argument, validationBlock ValidationErrorBlock, parseErrBlock func() *ast.BlockStmt) ([]ast.Stmt, error) {
	return appendStructFieldParseStatements(statements, file, arg, argument, validationBlock, parseErrBlock, callParseForm(file))
}

// appendStructFieldParseStatements renders the per-field parse statements for
// a form or multipart struct parameter from the field bindings resolved by
// muxt.ResolveCall. Used by both `form` (parseCall = callParseForm(file)) and
// `multipart` (parseCall = callParseMultipartForm(...)).
func appendStructFieldParseStatements(statements []ast.Stmt, file *File, arg *ast.Ident, argument muxt.Argument, validationBlock ValidationErrorBlock, parseErrBlock func() *ast.BlockStmt, parseCall ast.Stmt) ([]ast.Stmt, error) {
	const parsedVariableName = "value"
	statements = append(statements, parseCall)

	declareVar, err := formVariableDeclaration(file, arg, argument.ParamType())
	if err != nil {
		return nil, err
	}
	statements = append(statements, declareVar)

	for _, fb := range argument.FormFields() {
		if fb.FileHeader {
			if fb.Slice {
				statements = append(statements, fileHeaderSliceAssignment(arg, fb.Name, fb.InputName))
			} else {
				statements = append(statements, fileHeaderSingleAssignment(arg, fb.Name, fb.InputName))
			}
			continue
		}

		validations := renderValidations(file, ast.NewIdent(parsedVariableName), fb.Validations, validationBlock)
		if fb.Slice {
			parseResult := func(expr ast.Expr) ast.Stmt {
				return &ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(arg.Name), Sel: ast.NewIdent(fb.Name)}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{astgen.CallBuiltinAppend(&ast.SelectorExpr{X: ast.NewIdent(arg.Name), Sel: ast.NewIdent(fb.Name)}, expr)},
				}
			}
			parseStatements, err := generateParseValueFromStringStatements(file, parsedVariableName, ast.NewIdent("val"), fb.Elem(), fb.Method, validations, parseResult, parseErrBlock())
			if err != nil {
				return nil, fmt.Errorf("failed to generate parse statements for %s field %s: %w", arg.Name, fb.Name, err)
			}
			statements = append(statements, &ast.RangeStmt{
				Key:   ast.NewIdent("_"),
				Value: ast.NewIdent("val"),
				Tok:   token.DEFINE,
				X:     &ast.IndexExpr{X: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Form")}, Index: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(fb.InputName)}},
				Body:  &ast.BlockStmt{List: parseStatements},
			})
		} else {
			parseResult := func(expr ast.Expr) ast.Stmt {
				return &ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(arg.Name), Sel: ast.NewIdent(fb.Name)}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{expr},
				}
			}
			str := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("FormValue")}, Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(fb.InputName)}}}
			parseStatements, err := generateParseValueFromStringStatements(file, parsedVariableName, str, fb.Elem(), fb.Method, validations, parseResult, parseErrBlock())
			if err != nil {
				return nil, fmt.Errorf("failed to generate parse statements for %s field %s: %w", arg.Name, fb.Name, err)
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

// appendParseMultipartFormToStructStatements is a thin wrapper over
// appendStructFieldParseStatements that emits a ParseMultipartForm call.
// FileHeader field bindings (from request.MultipartForm.File) are resolved by
// muxt.ResolveCall; all other field-binding behavior is shared with the form
// codepath.
func appendParseMultipartFormToStructStatements(statements []ast.Stmt, file *File, arg *ast.Ident, argument muxt.Argument, validationBlock ValidationErrorBlock, parseErrBlock func() *ast.BlockStmt, config RoutesFileConfiguration) ([]ast.Stmt, error) {
	return appendStructFieldParseStatements(statements, file, arg, argument, validationBlock, parseErrBlock, callParseMultipartForm(file, config, parseErrBlock()))
}

// fileHeaderSingleAssignment emits:
//
//	if request.MultipartForm != nil {
//	    if fhs := request.MultipartForm.File["<inputName>"]; len(fhs) > 0 {
//	        <arg>.Field = fhs[0]
//	    }
//	}
func fileHeaderSingleAssignment(arg *ast.Ident, fieldName, inputName string) ast.Stmt {
	const tmp = "fhs"
	inner := &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(tmp)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.IndexExpr{
				X: &ast.SelectorExpr{
					X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("MultipartForm")},
					Sel: ast.NewIdent("File"),
				},
				Index: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(inputName)},
			}},
		},
		Cond: &ast.BinaryExpr{
			X:  astgen.CallBuiltinLen(ast.NewIdent(tmp)),
			Op: token.GTR,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.AssignStmt{
				Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(arg.Name), Sel: ast.NewIdent(fieldName)}},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.IndexExpr{X: ast.NewIdent(tmp), Index: &ast.BasicLit{Kind: token.INT, Value: "0"}}},
			},
		}},
	}
	return wrapInMultipartFormNotNil(inner)
}

// fileHeaderSliceAssignment emits:
//
//	if request.MultipartForm != nil {
//	    <arg>.Field = request.MultipartForm.File["<inputName>"]
//	}
func fileHeaderSliceAssignment(arg *ast.Ident, fieldName, inputName string) ast.Stmt {
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(arg.Name), Sel: ast.NewIdent(fieldName)}},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.IndexExpr{
			X: &ast.SelectorExpr{
				X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("MultipartForm")},
				Sel: ast.NewIdent("File"),
			},
			Index: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(inputName)},
		}},
	}
	return wrapInMultipartFormNotNil(assign)
}

// wrapInMultipartFormNotNil wraps stmt in `if request.MultipartForm != nil { stmt }`.
// ParseMultipartForm leaves request.MultipartForm nil if it returns an error,
// so we guard accesses to avoid nil-pointer panics on malformed bodies.
func wrapInMultipartFormNotNil(stmt ast.Stmt) ast.Stmt {
	return &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X:  &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("MultipartForm")},
			Op: token.NEQ,
			Y:  ast.NewIdent("nil"),
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{stmt}},
	}
}

func formVariableDeclaration(file *File, arg *ast.Ident, tp source.Type) (*ast.DeclStmt, error) {
	typeExp, err := file.TypeExpr(tp)
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

func formVariableAssignment(file *File, arg *ast.Ident, tp source.Type) (*ast.DeclStmt, error) {
	typeExp, err := file.TypeExpr(tp)
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
							X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
							Sel: ast.NewIdent("Form"),
						},
					},
				},
			},
		},
	}, nil
}

func httpServeMuxField(file *File) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(muxParamName)},
		Type:  &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent(astgen.AddNetHTTP(file)), Sel: ast.NewIdent("ServeMux")}},
	}
}

// templateDataParseErrBlock builds the standard scalar-parse failure block used
// by normal handlers: it appends the error to the template data and sets the
// error status code to 400.
func templateDataParseErrBlock(file *File, rdIdent string) *ast.BlockStmt {
	b := appendTemplateDataError(file, rdIdent, ast.NewIdent(errIdent))
	b.List = append(b.List, assignTemplateDataErrStatusCode(file, rdIdent, http.StatusBadRequest))
	return b
}

// generateParseValueFromStringStatements emits the statements that parse str
// into valueType and pass the result to assignment. On a parse failure it runs
// errBlock, which callers supply so the failure can be handled differently per
// context (normal handlers accumulate into the template data; SSE handlers
// respond 400 before establishing the stream).
func generateParseValueFromStringStatements(file *File, tmp string, str ast.Expr, valueType source.Type, method muxt.UnmarshalMethod, validations []ast.Stmt, assignment func(ast.Expr) ast.Stmt, errBlock *ast.BlockStmt) ([]ast.Stmt, error) {
	typeExpr, err := file.TypeExpr(valueType)
	if err != nil {
		return nil, err
	}
	// convert wraps the parsed value in a conversion to the target basic type
	// for the strconv functions that return a wider type (ParseInt/ParseUint).
	convert := func(exp ast.Expr) ast.Stmt {
		return assignment(&ast.CallExpr{
			Fun:  typeExpr,
			Args: []ast.Expr{exp},
		})
	}
	switch method {
	case muxt.UnmarshalBool:
		return parseBlock(tmp, astgen.StrconvParseBoolCall(file, str), validations, errBlock, assignment), nil
	case muxt.UnmarshalInt:
		return parseBlock(tmp, astgen.StrconvAtoiCall(file, str), validations, errBlock, assignment), nil
	case muxt.UnmarshalInt8:
		return parseBlock(tmp, astgen.StrconvParseInt8Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalInt16:
		return parseBlock(tmp, astgen.StrconvParseInt16Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalInt32:
		return parseBlock(tmp, astgen.StrconvParseInt32Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalInt64:
		return parseBlock(tmp, astgen.StrconvParseInt64Call(file, str), validations, errBlock, assignment), nil
	case muxt.UnmarshalUint:
		return parseBlock(tmp, astgen.StrconvParseUint0Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalUint8:
		return parseBlock(tmp, astgen.StrconvParseUint8Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalUint16:
		return parseBlock(tmp, astgen.StrconvParseUint16Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalUint32:
		return parseBlock(tmp, astgen.StrconvParseUint32Call(file, str), validations, errBlock, convert), nil
	case muxt.UnmarshalUint64:
		return parseBlock(tmp, astgen.StrconvParseUint64Call(file, str), validations, errBlock, assignment), nil
	case muxt.UnmarshalFloat32:
		return parseBlock(tmp, astgen.StrconvParseFloatCall(file, str, 32), validations, errBlock, convert), nil
	case muxt.UnmarshalFloat64:
		return parseBlock(tmp, astgen.StrconvParseFloatCall(file, str, 64), validations, errBlock, assignment), nil
	case muxt.UnmarshalString:
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
	case muxt.UnmarshalTextUnmarshaler:
		return []ast.Stmt{
			&ast.DeclStmt{
				Decl: &ast.GenDecl{
					Tok: token.VAR,
					Specs: []ast.Spec{
						&ast.ValueSpec{
							Names: []*ast.Ident{ast.NewIdent(tmp)},
							Type:  typeExpr,
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
	default:
		return nil, fmt.Errorf("unsupported type: %s", astgen.Format(typeExpr))
	}
}

func parseBlock(tmpIdent string, parseCall ast.Expr, validations []ast.Stmt, errBlock *ast.BlockStmt, handleResult func(out ast.Expr) ast.Stmt) []ast.Stmt {
	const errIdent = "err"
	callParse := &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(tmpIdent), ast.NewIdent(errIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{parseCall},
	}
	errCheckStmt := &ast.IfStmt{
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: errBlock,
	}
	if len(validations) > 0 {
		errCheckStmt.Else = &ast.BlockStmt{List: validations}
	}
	block := &ast.BlockStmt{List: []ast.Stmt{callParse, errCheckStmt}}
	block.List = append(block.List, handleResult(ast.NewIdent(tmpIdent)))
	return block.List
}

type receiverMethodCall struct {
	VarDecl ast.Stmt        // var err/ok declaration; nil for 1-result
	Assign  *ast.AssignStmt // dataVar[, err/ok] = call
	Check   ast.Stmt        // if err != nil / if !ok; nil for 1-result
	SetOkay *ast.AssignStmt // td.okay = true; nil for error-returning methods
}

// DefineStmts returns statements for use as an inline call where the result
// variable is being defined. It sets Assign to use := and includes the Check
// if present.
func (r *receiverMethodCall) DefineStmts() []ast.Stmt {
	r.Assign.Tok = token.DEFINE
	stmts := []ast.Stmt{r.Assign}
	if r.Check != nil {
		stmts = append(stmts, r.Check)
	}
	return stmts
}

func (r *receiverMethodCall) Stmts() []ast.Stmt {
	var stmts []ast.Stmt
	if r.VarDecl != nil {
		stmts = append(stmts, r.VarDecl)
	}
	stmts = append(stmts, r.Assign)
	if r.Check != nil {
		stmts = append(stmts, r.Check)
	}
	if r.SetOkay != nil {
		stmts = append(stmts, r.SetOkay)
	}
	return stmts
}

func callReceiverMethod(rdIdent string, dataVar ast.Expr, shape muxt.ResultShape, callIdent string, call *ast.CallExpr, errBody *ast.BlockStmt) (*receiverMethodCall, error) {
	const okIdent = "ok"
	switch shape {
	default:
		return nil, fmt.Errorf("method %s has no results it should have one or two", callIdent)
	case muxt.ResultShapeData:
		return &receiverMethodCall{
			Assign: &ast.AssignStmt{Lhs: []ast.Expr{dataVar}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
			SetOkay: &ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{
				X:   ast.NewIdent(rdIdent),
				Sel: ast.NewIdent(TemplateDataFieldIdentifierOkay),
			}}, Tok: token.ASSIGN, Rhs: []ast.Expr{astgen.Bool(true)}},
		}, nil
	case muxt.ResultShapeDataError:
		return &receiverMethodCall{
			VarDecl: &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errIdent)}, Type: ast.NewIdent("error")}}}},
			Assign:  &ast.AssignStmt{Lhs: []ast.Expr{dataVar, ast.NewIdent(errIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
			Check: &ast.IfStmt{
				Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
				Body: errBody,
			},
		}, nil
	case muxt.ResultShapeDataOK:
		return &receiverMethodCall{
			VarDecl: &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(okIdent)}, Type: ast.NewIdent("bool")}}}},
			Assign:  &ast.AssignStmt{Lhs: []ast.Expr{dataVar, ast.NewIdent(okIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
			Check: &ast.IfStmt{
				Cond: &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent(okIdent)},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{}}},
			},
			SetOkay: &ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{
				X:   ast.NewIdent(rdIdent),
				Sel: ast.NewIdent(TemplateDataFieldIdentifierOkay),
			}}, Tok: token.ASSIGN, Rhs: []ast.Expr{astgen.Bool(true)}},
		}, nil
	}
}

// decodeJSONBodyStatements declares valueIdent with the parameter type and
// decodes the JSON request body into it:
//
//	var bodyValue T
//	if err := json.NewDecoder(request.Body).Decode(&bodyValue); err != nil { <parseErrBlock> }
func decodeJSONBodyStatements(file *File, valueIdent string, paramType source.Type, parseErrBlock func() *ast.BlockStmt) ([]ast.Stmt, error) {
	typeExpr, err := file.TypeExpr(paramType)
	if err != nil {
		return nil, err
	}
	declare := &ast.DeclStmt{Decl: &ast.GenDecl{
		Tok:   token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(valueIdent)}, Type: typeExpr}},
	}}
	decode := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: astgen.Call(file, "json", "encoding/json", "NewDecoder",
				&ast.SelectorExpr{
					X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
					Sel: ast.NewIdent("Body"),
				},
			),
			Sel: ast.NewIdent("Decode"),
		},
		Args: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(valueIdent)}},
	}
	checkErr := &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ast.NewIdent(errIdent)}, Tok: token.DEFINE, Rhs: []ast.Expr{decode}},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: parseErrBlock(),
	}
	return []ast.Stmt{declare, checkErr}, nil
}

// lastEventIDHeader is the canonical request header the lastEventID argument is
// sourced from. http.Header.Get canonicalizes lookups, so this matches a
// client's "Last-Event-ID" as well.
const lastEventIDHeader = "Last-Event-Id"

// requestArgumentSource returns the expression a scalar argument is parsed
// from: request.Body for body, request.Header.Get("Last-Event-Id") for
// lastEventID, request.PathValue(argument.Identifier) for a path value, and
// an error for any other argument type.
func requestArgumentSource(argument muxt.Argument) (ast.Expr, error) {
	switch argument.Type {
	case muxt.ArgumentTypeRequestBody:
		return &ast.SelectorExpr{
			X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
			Sel: ast.NewIdent("Body"),
		}, nil
	case muxt.ArgumentTypeLastEventID:
		return &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Header")},
				Sel: ast.NewIdent("Get"),
			},
			Args: []ast.Expr{astgen.String(lastEventIDHeader)},
		}, nil
	case muxt.ArgumentTypeRequestPathValue:
		return &ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
				Sel: ast.NewIdent(requestPathValue),
			},
			Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(argument.Identifier)}},
		}, nil
	default:
		return nil, fmt.Errorf("no request source for argument %s", argument.Identifier)
	}
}

// callParseForm emits:
//
//	if err := request.ParseForm(); err != nil {
//	    http.Error(response, err.Error(), http.StatusBadRequest)
//	    return
//	}
func callParseForm(file *File) *ast.IfStmt {
	errBlock := &ast.BlockStmt{List: []ast.Stmt{
		&ast.ExprStmt{X: astgen.HTTPErrorCall(file, ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse), astgen.CallError(errIdent), http.StatusBadRequest)},
		&ast.ReturnStmt{},
	}}
	return &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(errIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
					Sel: ast.NewIdent("ParseForm"),
				},
			}},
		},
		Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
		Body: errBlock,
	}
}

// callParseMultipartForm emits:
//
//	if err := request.ParseMultipartForm(<maxMemory>); err != nil && !errors.Is(err, http.ErrNotMultipart) {
//	    <errBlock>
//	}
//
// The caller supplies errBlock so the failure is handled per handler
// kind: normal handlers accumulate the error into the template data
// with a 400 status; SSE handlers respond 400 and return before the
// event stream is established.
//
// http.ErrNotMultipart is exempted because ParseMultipartForm calls ParseForm
// internally, so url-encoded POSTs to a multipart route still populate
// request.PostForm — the receiver method should run with text fields bound
// (file fields stay nil). Other errors (truncated body, bad boundary,
// underlying ParseForm failures) are real.
func callParseMultipartForm(file *File, config RoutesFileConfiguration, errBlock *ast.BlockStmt) *ast.IfStmt {
	maxMemory := config.MultipartMaxMemory
	if maxMemory <= 0 {
		maxMemory = DefaultMultipartMaxMemory
	}
	httpPkg := astgen.AddNetHTTP(file)
	notNil := &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: ast.NewIdent("nil")}
	notErrNotMultipart := &ast.UnaryExpr{
		Op: token.NOT,
		X: astgen.Call(file, "", "errors", "Is",
			ast.NewIdent(errIdent),
			&ast.SelectorExpr{X: ast.NewIdent(httpPkg), Sel: ast.NewIdent("ErrNotMultipart")},
		),
	}
	return &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(errIdent)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
					Sel: ast.NewIdent("ParseMultipartForm"),
				},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(maxMemory, 10)}},
			}},
		},
		Cond: &ast.BinaryExpr{X: notNil, Op: token.LAND, Y: notErrNotMultipart},
		Body: errBlock,
	}
}

// multipartVariableAssignment emits `var <arg> <Type> = request.MultipartForm`
// for raw-mode multipart binding.
func multipartVariableAssignment(file *File, arg *ast.Ident, tp source.Type) (*ast.DeclStmt, error) {
	typeExp, err := file.TypeExpr(tp)
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
							X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
							Sel: ast.NewIdent("MultipartForm"),
						},
					},
				},
			},
		},
	}, nil
}

func contextAssignment(ident string) *ast.AssignStmt {
	return &ast.AssignStmt{
		Tok: token.DEFINE,
		Lhs: []ast.Expr{ast.NewIdent(ident)},
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: &ast.SelectorExpr{
				X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
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

func writeStatusAndHeaders(file *File, def muxt.Definition, fallbackStatusCode int, statusCode, bufIdent, resultDataIdent string, resultVar func() ast.Expr) []ast.Stmt {
	statusCodePriorityList := []ast.Expr{
		&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(templateDataFieldStatusCode)},
		&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierErrStatusCode)},
	}
	switch def.ResultStatusCode() {
	case muxt.ResultStatusCodeMethod:
		statusCodePriorityList = append(statusCodePriorityList, &ast.CallExpr{Fun: &ast.SelectorExpr{X: resultVar(), Sel: ast.NewIdent("StatusCode")}})
	case muxt.ResultStatusCodeField:
		statusCodePriorityList = append(statusCodePriorityList, &ast.SelectorExpr{X: resultVar(), Sel: ast.NewIdent("StatusCode")})
	}
	var list []ast.Stmt
	if fallbackStatusCode == http.StatusOK {
		const defaultStatusIdent = "defaultStatusCode"
		list = append(list,
			&ast.AssignStmt{
				Lhs: []ast.Expr{ast.NewIdent(defaultStatusIdent)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{astgen.HTTPStatusCode(file, http.StatusOK)},
			},
			&ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X:  &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(bufIdent), Sel: ast.NewIdent("Len")}},
					Op: token.EQL,
					Y:  astgen.Int(0),
				},
				Body: &ast.BlockStmt{List: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{ast.NewIdent(defaultStatusIdent)},
						Tok: token.ASSIGN,
						Rhs: []ast.Expr{astgen.HTTPStatusCode(file, http.StatusNoContent)},
					},
				}},
			},
		)
		statusCodePriorityList = append(statusCodePriorityList, ast.NewIdent(defaultStatusIdent))
	} else {
		statusCodePriorityList = append(statusCodePriorityList, astgen.HTTPStatusCode(file, fallbackStatusCode))
	}
	list = append(list, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(statusCode)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{astgen.CmpOr(file, statusCodePriorityList...)},
	})

	// Only add redirect block if the template can call Redirect
	if def.MayRedirect() {
		list = append(list, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X: &ast.SelectorExpr{
					X:   ast.NewIdent(resultDataIdent),
					Sel: ast.NewIdent(TemplateDataFieldIdentifierRedirectURL),
				},
				Op: token.NEQ,
				Y:  astgen.String(""),
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{
						X: astgen.Call(file, "", "net/http", "Redirect",
							ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPResponse),
							ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
							&ast.SelectorExpr{
								X:   ast.NewIdent(resultDataIdent),
								Sel: ast.NewIdent(TemplateDataFieldIdentifierRedirectURL),
							},
							ast.NewIdent(statusCode),
						),
					},
					&ast.ReturnStmt{},
				},
			},
		})
	}

	return append(list, writeBodyAndWriteHeadersFunc(file, bufIdent, statusCode)...)
}

func executeTemplateFailedLogLine(file *File, message, errIdent string) *ast.CallExpr {
	args := []ast.Expr{
		&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Context")}},
		astgen.String(message),

		astgen.SlogString(file, "path", &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("URL")},
			Sel: ast.NewIdent("Path"),
		}),
		astgen.SlogString(file, "pattern", &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Pattern")}),
		astgen.SlogString(file, "error", astgen.CallError(errIdent)),
	}
	return astgen.Call(file, "", "log/slog", "ErrorContext", args...)
}

func loggerErrorCall(file *File, message, pattern, errIdent string) *ast.CallExpr {
	args := []ast.Expr{
		&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Context")}},
		astgen.String(message),
		astgen.SlogString(file, "pattern", astgen.String(pattern)),
		astgen.SlogString(file, "path", &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("URL")},
			Sel: ast.NewIdent("Path"),
		}),
		astgen.SlogString(file, "error", astgen.CallError(errIdent)),
	}
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ast.NewIdent("logger"), Sel: ast.NewIdent("ErrorContext")},
		Args: args,
	}
}

func logDebugStatement(file *File, message, pattern string) *ast.ExprStmt {
	args := []ast.Expr{
		&ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Context")}},
		astgen.String(message),
		astgen.SlogString(file, "pattern", astgen.String(pattern)),
		astgen.SlogString(file, "path", &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("URL")},
			Sel: ast.NewIdent("Path"),
		}),
		astgen.SlogString(file, "method", &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Method")}),
	}
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent("logger"), Sel: ast.NewIdent("DebugContext")},
			Args: args,
		},
	}
}

func assignTemplateDataErrStatusCode(file *File, rdIdent string, code int) *ast.AssignStmt {
	return &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.SelectorExpr{
			X:   ast.NewIdent(rdIdent),
			Sel: ast.NewIdent(TemplateDataFieldIdentifierErrStatusCode),
		}},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{
			astgen.HTTPStatusCode(file, code),
		},
	}
}
