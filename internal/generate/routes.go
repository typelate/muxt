package generate

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/token"
	"log"
	"maps"
	"path/filepath"
	"slices"
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

// OutputDirectory is the directory of the output file, written relative to
// wd. The routes file belongs to the package in that directory.
func (c RoutesFileConfiguration) OutputDirectory(wd string) string {
	return filepath.Dir(filepath.Join(wd, c.OutputFileName))
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
		&ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{
				&ast.TypeSpec{Name: ast.NewIdent(config.ReceiverInterface), Type: receiverInterface},
			},
		},
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

	generatedFiles = append(generatedFiles, GeneratedFile{Path: filePath, Content: content, Routes: len(topLevelTemplateRoutes)})

	return generatedFiles, nil
}

// logResolutionNotes reports what resolution found for defs. With a
// --use-receiver-type run, methods the named receiver does not define are
// announced with their inferred signatures, one line per method and the
// explanation once after the list; the default mode synthesizes every method
// by design, so it stays quiet.
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

	routesFunc.Type.Params.List = append(routesFunc.Type.Params.List, &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(pathPrefixPathsStructFieldName)}, Type: ast.NewIdent("string"),
	})

	if config.Middleware {
		routesFunc.Type.Params.List = append(routesFunc.Type.Params.List, &ast.Field{
			Names: []*ast.Ident{ast.NewIdent(middlewareParamName)},
			Type:  astgen.HTTPMiddlewareFuncType(file),
		})
	}

	if len(defs) > 0 {
		routesFunc.Body.List = append(routesFunc.Body.List, bytesBufferPoolDeclaration(file))
	}

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
	scopedReceiverInterface := &ast.InterfaceType{
		Methods: new(ast.FieldList),
	}

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

	is := file.ImportSpecs()
	importSpecs := make([]ast.Spec, 0, len(is))
	for _, s := range is {
		importSpecs = append(importSpecs, s)
	}

	outputFile := &ast.File{
		Name: ast.NewIdent(config.PackageName),
		Decls: []ast.Decl{
			&ast.GenDecl{
				Tok:   token.IMPORT,
				Specs: importSpecs,
			},
			&ast.GenDecl{
				Tok: token.TYPE,
				Specs: []ast.Spec{
					&ast.TypeSpec{
						Name: ast.NewIdent(receiverInterfaceName),
						Type: scopedReceiverInterface,
					},
				},
			},
			routesFunc,
		},
	}

	return outputFile, nil
}

func httpServeMuxField(file *File) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ast.NewIdent(muxParamName)},
		Type:  &ast.StarExpr{X: &ast.SelectorExpr{X: ast.NewIdent(astgen.AddNetHTTP(file)), Sel: ast.NewIdent("ServeMux")}},
	}
}
