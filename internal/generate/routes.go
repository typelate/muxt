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
	loggerIdent         = "logger"

	errIdent                    = "err"
	templateDataFieldStatusCode = "statusCode"

	pathPrefixPathsStructFieldName = "pathsPrefix"

	bufferPoolIdent = "bytesBufferPool"

	executeTemplateErrorMessage = "failed to render page"

	DefaultRoutesFunctionName         = "TemplateRoutes"
	DefaultReceiverInterfaceName      = "RoutesReceiver"
	DefaultTemplateRoutePathsTypeName = "TemplateRoutePaths"
	DefaultTemplateRouteTypeName      = "TemplateRoute"
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
	TemplateRoutePathsTypeName,
	TemplateRouteTypeName string
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

// checkRouteBuilderTypeName reports a route builder type name, which is derived
// from the route type name and so is not a flag of its own, that another
// generated identifier already uses.
func checkRouteBuilderTypeName(config RoutesFileConfiguration) error {
	builder := routeBuilderTypeName(config)
	for _, other := range []struct{ flag, name string }{
		{"output-routes-func", config.RoutesFunction},
		{"output-receiver-interface", config.ReceiverInterface},
		{"output-template-data-type", config.TemplateDataType},
		{"output-sse-template-data-type", config.SSETemplateDataType},
		{"output-template-route-paths-type", config.TemplateRoutePathsTypeName},
	} {
		if other.name == builder {
			return fmt.Errorf("the route builder type %s, the route type name %s with Builder appended, is also the value of --%s; change one of them", builder, routeTypeName(config), other.flag)
		}
	}
	return nil
}

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

	if err := checkRouteBuilderTypeName(config); err != nil {
		return nil, err
	}

	groups, err := groupTemplates(config, defs)
	if err != nil {
		return nil, err
	}

	receiverInterface := &ast.InterfaceType{Methods: new(ast.FieldList)}
	routesFunc := routesFuncDecl(file, config, config.RoutesFunction, config.ReceiverInterface, config.PathPrefix)
	routesFunc.Type.Results = fieldList(results(ast.NewIdent(config.TemplateRoutePathsTypeName)))
	routesFunc.Body.List = routesFuncPrelude(file, config)

	var generatedFiles []GeneratedFile
	topLevelRoutes := groups.all
	if config.OutputMultipleFiles {
		templateSourceFiles := slices.Sorted(maps.Keys(groups.byFile))
		files, err := sourceFileRouteFunctionFiles(wd, config, templateSourceFiles, groups, logger, file, receiverInterface, routesFunc)
		if err != nil {
			return files, err
		}
		generatedFiles = files
		topLevelRoutes = groups.noFile
	}

	handlers, err := routeStatements(file, config, topLevelRoutes, receiverInterface, config.ReceiverInterface, logger, "")
	if err != nil {
		return nil, err
	}
	routesFunc.Body.List = append(routesFunc.Body.List, handlers...)

	routePathDecls, err := routePathTypeAndMethods(file, config, groups.all)
	if err != nil {
		return nil, err
	}
	routesFunc.Body.List = append(routesFunc.Body.List, returnRoutePaths(config))

	decls := []ast.Decl{receiverInterfaceDecl(config.ReceiverInterface, receiverInterface), routesFunc}
	decls = append(decls, templateDataDecls(file, config)...)
	// The SSETemplateData type and its methods are only needed when a route uses
	// the sse render callback, so emit them conditionally to avoid unused imports.
	if slices.ContainsFunc(groups.all, func(definition muxt.Definition) bool {
		return definition.Representation == muxt.RepresentationSSE
	}) {
		decls = append(decls, sseTemplateDataDecls(file, config)...)
	}
	decls = append(decls, routePathDecls...)

	// The import declaration is built last: building the other
	// declarations is what registers the imports they use.
	outputFile := &ast.File{
		Name:  ast.NewIdent(config.PackageName),
		Decls: append([]ast.Decl{importDecl(file)}, decls...),
	}

	filePath := filepath.Join(wd, config.OutputFileName)
	content, err := formatFile(filePath, outputFile)
	if err != nil {
		return nil, err
	}

	return append(generatedFiles, GeneratedFile{Path: filePath, Content: content, Routes: len(topLevelRoutes)}), nil
}

// routesFuncDecl declares func name(mux, receiver[, logger][, pathsPrefix][, middleware]).
func routesFuncDecl(file *File, config RoutesFileConfiguration, name, receiverInterfaceName string, prefixParam bool) *ast.FuncDecl {
	params := []*ast.Field{
		httpServeMuxField(file),
		param(ast.NewIdent(receiverInterfaceName), receiverIdent),
	}
	if config.Logger {
		params = append(params, param(astgen.SlogLoggerPtr(file), loggerIdent))
	}
	if prefixParam {
		params = append(params, param(ast.NewIdent("string"), pathPrefixPathsStructFieldName))
	}
	if config.Middleware {
		params = append(params, param(astgen.HTTPMiddlewareFuncType(file), middlewareParamName))
	}
	return &ast.FuncDecl{
		Name: ast.NewIdent(name),
		Type: &ast.FuncType{Params: &ast.FieldList{List: params}},
		Body: &ast.BlockStmt{List: []ast.Stmt{}},
	}
}

func routesFuncPrelude(file *File, config RoutesFileConfiguration) []ast.Stmt {
	stmts := []ast.Stmt{}
	if !config.PathPrefix {
		stmts = append(stmts, &ast.AssignStmt{
			Tok: token.DEFINE,
			Lhs: []ast.Expr{ast.NewIdent(pathPrefixPathsStructFieldName)},
			Rhs: []ast.Expr{astgen.String("")},
		})
	}
	if config.Middleware {
		stmts = append(stmts, middlewareNilGuard(file))
	}
	return stmts
}

// routeStatements declares the buffer pool and registers a handler for each of
// defs, adding the receiver methods they call to receiverInterface. where is
// appended to the verbose log line naming each pattern.
func routeStatements(file *File, config RoutesFileConfiguration, defs []muxt.Definition, receiverInterface *ast.InterfaceType, receiverInterfaceName string, logger *log.Logger, where string) ([]ast.Stmt, error) {
	var stmts []ast.Stmt
	if len(defs) > 0 {
		stmts = append(stmts, bytesBufferPoolDeclaration(file))
	}
	logResolutionNotes(defs, config, logger)
	if err := collectReceiverMethods(defs, file, receiverInterface); err != nil {
		return nil, err
	}
	for _, def := range defs {
		if config.Verbose {
			logger.Printf("generating handler for pattern %s%s", def.RawPattern(), where)
		}
		call, err := handleFuncStatement(file, config, def, receiverInterfaceName)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, call)
	}
	return stmts, nil
}

func handleFuncStatement(file *File, config RoutesFileConfiguration, def muxt.Definition, receiverInterfaceName string) (*ast.ExprStmt, error) {
	if def.FunctionIdentifier() == nil {
		return callHandleFunc(file, def, noReceiverMethodCall(file, def, config, receiverInterfaceName), config), nil
	}
	handlerFunc, err := callHandlerFunc(file, config, def, receiverInterfaceName)
	if err != nil {
		return nil, err
	}
	return callHandleFunc(file, def, handlerFunc, config), nil
}

func returnRoutePaths(config RoutesFileConfiguration) *ast.ReturnStmt {
	return returnExprs(pathsLiteral(config, ast.NewIdent(pathPrefixPathsStructFieldName)))
}

func receiverInterfaceDecl(name string, receiverInterface *ast.InterfaceType) *ast.GenDecl {
	return &ast.GenDecl{
		Tok:   token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{Name: ast.NewIdent(name), Type: receiverInterface}},
	}
}

func importDecl(file *File) *ast.GenDecl {
	decl := &ast.GenDecl{Tok: token.IMPORT}
	for _, spec := range file.ImportSpecs() {
		decl.Specs = append(decl.Specs, spec)
	}
	return decl
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

		// sourceFile may be an absolute path, so only its base name is used.
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
		routesFunc.Body.List = append(routesFunc.Body.List, perFileRoutesCall(config, routesFuncName))
	}
	return generatedFiles, nil
}

// perFileRoutesCall calls a per-file routes function with the arguments the
// top-level routes function received.
func perFileRoutesCall(config RoutesFileConfiguration, routesFuncName string) *ast.ExprStmt {
	args := []ast.Expr{ast.NewIdent(muxParamName), ast.NewIdent(receiverIdent)}
	if config.Logger {
		args = append(args, ast.NewIdent(loggerIdent))
	}
	args = append(args, ast.NewIdent(pathPrefixPathsStructFieldName))
	if config.Middleware {
		args = append(args, ast.NewIdent(middlewareParamName))
	}
	return &ast.ExprStmt{X: &ast.CallExpr{Fun: ast.NewIdent(routesFuncName), Args: args}}
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
	routesFunc := routesFuncDecl(file, config, funcName, receiverInterfaceName, true)
	handlers, err := routeStatements(file, config, defs, receiverInterface, receiverInterfaceName, logger, " in "+sourceFile)
	if err != nil {
		return nil, err
	}
	routesFunc.Body.List = handlers
	return routesFunc, nil
}

func generatePerFileAST(
	sourceFile string,
	defs []muxt.Definition,
	file *File,
	funcName, receiverInterfaceName string,
	logger *log.Logger,
	config RoutesFileConfiguration,
) (*ast.File, error) {
	scopedReceiverInterface := &ast.InterfaceType{Methods: new(ast.FieldList)}
	routesFunc, err := generatePerFileRouteFunction(sourceFile, defs, file, funcName, receiverInterfaceName, logger, config, scopedReceiverInterface)
	if err != nil {
		return nil, err
	}
	return &ast.File{
		Name: ast.NewIdent(config.PackageName),
		Decls: []ast.Decl{
			importDecl(file),
			receiverInterfaceDecl(receiverInterfaceName, scopedReceiverInterface),
			routesFunc,
		},
	}, nil
}

func httpServeMuxField(file *File) *ast.Field {
	return param(&ast.StarExpr{X: astgen.ExportedIdentifier(file, "", "net/http", "ServeMux")}, muxParamName)
}
