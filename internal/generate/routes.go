package generate

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

	"github.com/stretchr/testify/assert"
	"github.com/typelate/dom"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/tools/go/packages"

	"github.com/typelate/muxt/internal/asteval"
	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

const (
	receiverIdent = "receiver"

	muxVarIdent = "mux"

	requestPathValue         = "PathValue"
	httpRequestContextMethod = "Context"
	httpHandleFuncIdent      = "HandleFunc"

	InputAttributeNameStructTag     = "name"
	InputAttributeTemplateStructTag = "template"

	muxParamName = "mux"

	errIdent                    = "err"
	templateDataFieldStatusCode = "statusCode"

	pathPrefixPathsStructFieldName = "pathsPrefix"

	executeTemplateErrorMessage = "failed to render page"
)

type GeneratedFile struct {
	Path    string
	Content string
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
	PathPrefix     bool
	Logger         bool
	Verbose        bool
}

// groupTemplatesBySourceFile groups templates by their sourceFile field.
// Returns a map where keys are source filenames and values are template slices.
// Templates with empty sourceFile (Parse-based) are grouped under "".
func groupTemplatesBySourceFile(defs []muxt.Definition) map[string][]muxt.Definition {
	groups := make(map[string][]muxt.Definition)
	for _, d := range defs {
		key := d.SourceFile()
		groups[key] = append(groups[key], d)
	}
	return groups
}

func TemplateRoutesFile(wd string, logger *log.Logger, config RoutesFileConfiguration) ([]GeneratedFile, error) {
	if !token.IsIdentifier(config.PackageName) {
		return nil, fmt.Errorf("package name %q is not an identifier", config.PackageName)
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
		return nil, err
	}

	file, err := newFile(filepath.Join(wd, config.OutputFileName), fileSet, pl)
	if err != nil {
		return nil, err
	}
	routesPkg := file.OutputPackage()

	config.PackagePath = routesPkg.PkgPath
	config.PackageName = routesPkg.Name
	receiver, err := resolveReceiver(config, file, routesPkg)
	if err != nil {
		return nil, err
	}

	ts, _, err := asteval.Templates(wd, config.TemplatesVariable, routesPkg)
	if err != nil {
		return nil, err
	}
	templates, err := muxt.Definitions(ts)
	if err != nil {
		return nil, err
	}

	// Group templates by source file
	definitionGroups := groupTemplatesBySourceFile(templates)
	parseBasedDefinitions := definitionGroups[""]
	delete(definitionGroups, "") // Remove parse-based templates from groups

	// Separate valid file paths from non-file-path source names
	// Non-file-path sources (containing spaces, slashes, etc.) should be treated like Parse-based templates
	var sourceFiles []string
	for sourceFile := range definitionGroups {
		// Check if sourceFile is a valid file path (no spaces, path separators in basename)
		baseName := filepath.Base(sourceFile)
		if strings.ContainsAny(baseName, " /\\()") {
			// Not a valid file path - move definitions to parseBasedmuxt.Definitions
			parseBasedDefinitions = append(parseBasedDefinitions, definitionGroups[sourceFile]...)
			delete(definitionGroups, sourceFile)
		} else {
			sourceFiles = append(sourceFiles, sourceFile)
		}
	}
	slices.Sort(sourceFiles)

	// Generate per-file route files
	var generatedFiles []GeneratedFile
	for _, sourceFile := range sourceFiles {
		definitions := definitionGroups[sourceFile]
		if config.Verbose {
			logger.Printf("generating routes for %s (%d templates)", sourceFile, len(definitions))
		}

		perFileAST, err := generatePerFileAST(sourceFile, definitions, file, logger, config, receiver, routesPkg)
		if err != nil {
			return nil, fmt.Errorf("failed to generate routes for %s: %w", sourceFile, err)
		}

		// Generate filename: strip .gohtml extension, add _template_routes_gen.go
		baseFileName := strings.TrimSuffix(sourceFile, filepath.Ext(sourceFile))
		outputFileName := baseFileName + "_template_routes_gen.go"
		outputFilePath := filepath.Join(wd, outputFileName)

		content, err := astgen.FormatFile(outputFilePath, perFileAST)
		if err != nil {
			return nil, fmt.Errorf("failed to format %s: %w", outputFileName, err)
		}

		generatedFiles = append(generatedFiles, GeneratedFile{
			Path:    outputFilePath,
			Content: content,
		})
	}

	// Build main receiver interface that combines all file-based interfaces
	receiverInterface := &ast.InterfaceType{
		Methods: new(ast.FieldList),
	}

	// Embed all file-scoped receiver interfaces
	for _, sourceFile := range sourceFiles {
		fileIdentifier := muxt.FileNameToPrivateIdentifier(sourceFile)
		receiverInterfaceName := fileIdentifier + "RoutesReceiver"
		receiverInterface.Methods.List = append(receiverInterface.Methods.List, &ast.Field{
			Type: ast.NewIdent(receiverInterfaceName),
		})
	}

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

	// Call per-file route functions
	for _, sourceFile := range sourceFiles {
		fileIdentifier := muxt.FileNameToPrivateIdentifier(sourceFile)
		funcName := fileIdentifier + config.RoutesFunction

		callArgs := []ast.Expr{ast.NewIdent(muxParamName), ast.NewIdent(receiverIdent)}
		if config.Logger {
			callArgs = append(callArgs, ast.NewIdent("logger"))
		}
		// Always pass pathsPrefix to per-file functions
		callArgs = append(callArgs, ast.NewIdent(pathPrefixPathsStructFieldName))

		routesFunc.Body.List = append(routesFunc.Body.List, &ast.ExprStmt{
			X: &ast.CallExpr{
				Fun:  ast.NewIdent(funcName),
				Args: callArgs,
			},
		})
	}

	// Generate handlers for parse-based templates (empty sourceFile)
	sigs := make(map[string]*types.Signature)
	for i := range parseBasedDefinitions {
		def := parseBasedDefinitions[i]
		const dataVarIdent = "result"
		if config.Verbose {
			logger.Printf("generating handler for pattern %s", def.Pattern())
		}
		if def.FunctionIdentifier() == nil {
			handlerFunc := noReceiverMethodCall(file, def, config, config.ReceiverInterface)
			call := callHandleFunc(file, def, handlerFunc, config)
			routesFunc.Body.List = append(routesFunc.Body.List, call)
			continue
		}
		handlerFunc, err := methodHandlerFunc(file, config, def, sigs, receiver, receiverInterface, routesPkg.Types, dataVarIdent, config.ReceiverInterface)
		if err != nil {
			return nil, err
		}
		call := callHandleFunc(file, def, handlerFunc, config)
		routesFunc.Body.List = append(routesFunc.Body.List, call)
	}

	routePathDecls, err := routePathTypeAndMethods(file, config, templates)
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
			templateDataMuxtVersionMethod(config),
			templateDataPathMethod(config),
			templateDataResultMethod(config.TemplateDataType),
			templateDataRequestMethod(file, config.TemplateDataType),
			templateDataStatusCodeMethod(config.TemplateDataType),
			templateDataHeaderMethod(config.TemplateDataType),
			templateDataOkay(config.TemplateDataType),
			templateDataError(file, config.TemplateDataType),
			templateDataReceiver(ast.NewIdent(config.ReceiverInterface), config.TemplateDataType),
			templateRedirect(file, config),

			// func newResultData
		}, routePathDecls...),
	}

	filePath := filepath.Join(wd, config.OutputFileName)
	content, err := astgen.FormatFile(filePath, outputFile)
	if err != nil {
		return nil, err
	}

	// Append main file to generated files
	generatedFiles = append(generatedFiles, GeneratedFile{Path: filePath, Content: content})

	return generatedFiles, nil
}

func callHandleFunc(file *File, def muxt.Definition, handlerFuncLit *ast.FuncLit, config RoutesFileConfiguration) *ast.ExprStmt {
	pattern := ast.Expr(astgen.String(def.Pattern()))
	if config.PathPrefix {
		i := strings.Index(def.Pattern(), "/")
		pattern = &ast.BinaryExpr{
			X:  astgen.String(def.Pattern()[:i]),
			Op: token.ADD,
			Y:  astgen.Call(file, "path", "path", "Join", ast.NewIdent(pathPrefixPathsStructFieldName), astgen.String(def.Pattern()[i:])),
		}
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent(muxVarIdent),
			Sel: ast.NewIdent(httpHandleFuncIdent),
		},
		Args: []ast.Expr{pattern, handlerFuncLit},
	}}
}

func resolveReceiver(config RoutesFileConfiguration, file *File, routesPkg *packages.Package) (*types.Named, error) {
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

// generatePerFileRouteFunction creates a route registration function for templates from a specific source file.
// For example, for "index.gohtml", it generates IndexTemplateRoutes(mux, receiver, ...).
func generatePerFileRouteFunction(
	sourceFile string,
	defs []muxt.Definition,
	file *File,
	logger *log.Logger,
	config RoutesFileConfiguration,
	receiver *types.Named,
	receiverInterface *ast.InterfaceType,
	routesPkg *packages.Package,
) (*ast.FuncDecl, error) {
	if sourceFile == "" {
		return nil, fmt.Errorf("sourceFile cannot be empty")
	}

	fileIdentifier := muxt.FileNameToPrivateIdentifier(sourceFile)
	if fileIdentifier == "" {
		return nil, fmt.Errorf("could not generate identifier from filename: %s", sourceFile)
	}

	funcName := fileIdentifier + config.RoutesFunction
	receiverInterfaceName := fileIdentifier + "RoutesReceiver"

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

	// Generate handlers for each template
	sigs := make(map[string]*types.Signature)
	for i := range defs {
		t := defs[i]
		const dataVarIdent = "result"
		if config.Verbose {
			logger.Printf("generating handler for pattern %s in %s", t.Pattern(), sourceFile)
		}
		if t.FunctionIdentifier() == nil {
			handlerFunc := noReceiverMethodCall(file, t, config, receiverInterfaceName)
			call := callHandleFunc(file, t, handlerFunc, config)
			routesFunc.Body.List = append(routesFunc.Body.List, call)
			continue
		}
		handlerFunc, err := methodHandlerFunc(file, config, t, sigs, receiver, receiverInterface, routesPkg.Types, dataVarIdent, receiverInterfaceName)
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
	logger *log.Logger,
	config RoutesFileConfiguration,
	receiver *types.Named,
	routesPkg *packages.Package,
) (*ast.File, error) {
	if sourceFile == "" {
		return nil, fmt.Errorf("sourceFile cannot be empty")
	}

	fileIdentifier := muxt.FileNameToPrivateIdentifier(sourceFile)
	if fileIdentifier == "" {
		return nil, fmt.Errorf("could not generate identifier from filename: %s", sourceFile)
	}

	receiverInterfaceName := fileIdentifier + "RoutesReceiver"

	// Create a scoped receiver interface for this file's templates
	scopedReceiverInterface := &ast.InterfaceType{
		Methods: new(ast.FieldList),
	}

	// Generate the route function
	routesFunc, err := generatePerFileRouteFunction(
		sourceFile,
		defs,
		file,
		logger,
		config,
		receiver,
		scopedReceiverInterface,
		routesPkg,
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
				&ast.AssignStmt{
					Lhs: []ast.Expr{ast.NewIdent(bufIdent)},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{astgen.BytesNewBuffer(file, astgen.Nil())},
				},
			},
		},
	}

	if config.Logger {
		handlerFunc.Body.List = append(handlerFunc.Body.List, logDebugStatement(file, "handling request", def.Pattern()))
	}

	execTemplates := checkExecuteTemplateError(file, config.Logger, def.Pattern())
	execTemplates.Init = &ast.AssignStmt{
		Lhs: []ast.Expr{
			ast.NewIdent(errIdent),
		},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(config.TemplatesVariable), Sel: ast.NewIdent("ExecuteTemplate")},
			Args: []ast.Expr{ast.NewIdent(bufIdent), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(def.Name())}, &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(templateDataVarIdent)}},
		}},
	}

	handlerFunc.Body.List = append(handlerFunc.Body.List, execTemplates)

	handlerFunc.Body.List = append(handlerFunc.Body.List, writeStatusAndHeaders(file, def, types.NewStruct(nil, nil), def.DefaultStatusCode(), statusCodeIdent, bufIdent, templateDataVarIdent, func() ast.Expr {
		panic("when no receiver method is called, then the result variable should not be needed")
	})...)
	return handlerFunc
}

func methodHandlerFunc(file *File, config RoutesFileConfiguration, def muxt.Definition, sigs map[string]*types.Signature, receiver *types.Named, receiverInterface *ast.InterfaceType, outputPkg *types.Package, dataVarIdent string, receiverInterfaceName string) (*ast.FuncLit, error) {
	const (
		bufIdent        = "buf"
		statusCodeIdent = "statusCode"
		resultDataIdent = "td"
	)
	if err := ensureMethodSignature(file, sigs, def, receiver, receiverInterface, def.CallExpression(), outputPkg); err != nil {
		return nil, err
	}
	sig, ok := sigs[def.FunctionIdentifier().Name]
	if !ok {
		return nil, fmt.Errorf("failed to determine call signature %s", def.FunctionIdentifier().Name)
	}
	if sig.Results().Len() == 0 {
		return nil, fmt.Errorf("method for pattern %q has no results it should have one or two", def.Name())
	}
	var callFun ast.Expr
	obj, _, _ := types.LookupFieldOrMethod(receiver, true, receiver.Obj().Pkg(), def.FunctionIdentifier().Name)
	isMethodCall := obj != nil
	if isMethodCall {
		callFun = &ast.SelectorExpr{
			X:   ast.NewIdent(receiverIdent),
			Sel: ast.NewIdent(def.FunctionIdentifier().Name),
		}
	} else {
		callFun = ast.NewIdent(def.FunctionIdentifier().Name)
	}

	resultType := sig.Results().At(0).Type()
	typeExpr, err := file.TypeASTExpression(resultType)
	if err != nil {
		return nil, err
	}

	handlerFunc := &ast.FuncLit{
		Type: astgen.HTTPHandlerFuncType(file, muxt.TemplateNameScopeIdentifierHTTPResponse, muxt.TemplateNameScopeIdentifierHTTPRequest),
		Body: &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.DeclStmt{
					Decl: &ast.GenDecl{
						Tok: token.VAR,
						Specs: []ast.Spec{&ast.ValueSpec{
							Names: []*ast.Ident{ast.NewIdent(resultDataIdent)},
							Values: []ast.Expr{&ast.CompositeLit{Type: &ast.IndexListExpr{
								X:       ast.NewIdent(config.TemplateDataType),
								Indices: []ast.Expr{ast.NewIdent(receiverInterfaceName), typeExpr},
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

	if handlerFunc.Body.List, err = appendParseArgumentStatements(handlerFunc.Body.List, def, file, resultType, sigs, nil, receiver, resultDataIdent, config, def.CallExpression(), func(s string) *ast.BlockStmt {
		errBlock := appendTemplateDataError(file, resultDataIdent, astgen.ErrorsNew(file, astgen.String(s)))
		errBlock.List = append(errBlock.List, assignTemplateDataErrStatusCode(file, resultDataIdent, http.StatusBadRequest))
		return errBlock
	}); err != nil {
		return nil, err
	}

	receiverCallStatements, err := callReceiverMethod(file, resultDataIdent, &ast.SelectorExpr{
		X:   ast.NewIdent(resultDataIdent),
		Sel: ast.NewIdent(TemplateDataFieldIdentifierResult),
	}, sig, &ast.CallExpr{
		Fun:  callFun,
		Args: slices.Clone(def.CallExpression().Args),
	})
	if err != nil {
		return nil, err
	}
	handlerFunc.Body.List = append(handlerFunc.Body.List, &ast.IfStmt{
		Cond: &ast.BinaryExpr{
			X: astgen.CallBuiltinLen(&ast.SelectorExpr{
				X:   ast.NewIdent(resultDataIdent),
				Sel: ast.NewIdent(TemplateDataFieldIdentifierError),
			}),
			Op: token.EQL,
			Y:  astgen.Int(0),
		},
		Body: &ast.BlockStmt{
			List: receiverCallStatements,
		},
	})

	handlerFunc.Body.List = append(handlerFunc.Body.List, &ast.AssignStmt{
		Lhs: []ast.Expr{ast.NewIdent(bufIdent)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{astgen.BytesNewBuffer(file, astgen.Nil())},
	})

	if config.Logger {
		handlerFunc.Body.List = append(handlerFunc.Body.List, logDebugStatement(file, "handling request", def.Pattern()))
	}

	execTemplates := checkExecuteTemplateError(file, config.Logger, def.Pattern())
	execTemplates.Init = &ast.AssignStmt{
		Lhs: []ast.Expr{
			ast.NewIdent(errIdent),
		},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ast.NewIdent(config.TemplatesVariable), Sel: ast.NewIdent("ExecuteTemplate")},
			Args: []ast.Expr{ast.NewIdent(bufIdent), &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(def.Name())}, &ast.UnaryExpr{Op: token.AND, X: ast.NewIdent(resultDataIdent)}},
		}},
	}

	handlerFunc.Body.List = append(handlerFunc.Body.List, execTemplates)

	if !def.HasResponseWriterArg() {
		handlerFunc.Body.List = append(handlerFunc.Body.List, writeStatusAndHeaders(file, def, resultType, def.DefaultStatusCode(), statusCodeIdent, bufIdent, resultDataIdent, func() ast.Expr {
			return &ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierResult)}
		})...)
	} else {
		handlerFunc.Body.List = append(handlerFunc.Body.List, callWriteOnResponse(bufIdent))
	}

	return handlerFunc, nil
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

func appendParseArgumentStatements(statements []ast.Stmt, def muxt.Definition, file *File, resultType types.Type, sigs map[string]*types.Signature, parsed map[string]struct{}, receiver *types.Named, rdIdent string, config RoutesFileConfiguration, call *ast.CallExpr, validationFailureBlock ValidationErrorBlock) ([]ast.Stmt, error) {
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
		return nil, fmt.Errorf("handler func %s expects %d arguments but call %s has %d", sigStr, signature.Params().Len(), astgen.Format(call), len(call.Args))
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
			parseArgStatements, err := appendParseArgumentStatements(statements, def, file, resultType, sigs, parsed, receiver, rdIdent, config, arg, validationFailureBlock)
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

			callMethodStatements, err := callReceiverMethod(file, rdIdent, ast.NewIdent(resultVarIdent), callSig, arg)
			if err != nil {
				return nil, err
			}
			if len(callMethodStatements) > 2 {
				callMethodStatements = callMethodStatements[1 : len(callMethodStatements)-1]
			}
			if assign, ok := callMethodStatements[0].(*ast.AssignStmt); ok {
				assign.Tok = token.DEFINE
				callMethodStatements[0] = assign
			}

			statements = append(parseArgStatements, callMethodStatements...)
		case *ast.Ident:
			argType, ok := defaultTemplateNameScope(file, def, arg.Name)
			if !ok {
				return nil, fmt.Errorf("failed to determine type for %s", arg.Name)
			}
			src := &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
					Sel: ast.NewIdent(requestPathValue),
				},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(arg.Name)}},
			}
			if types.AssignableTo(argType, param.Type()) {
				if _, ok := parsed[arg.Name]; !ok {
					parsed[arg.Name] = struct{}{}
					switch arg.Name {
					case muxt.TemplateNameScopeIdentifierForm:
						declareFormVar, err := formVariableAssignment(file, arg, param.Type())
						if err != nil {
							return nil, err
						}
						statements = append(statements, callParseForm(), declareFormVar)
					case muxt.TemplateNameScopeIdentifierContext:
						statements = append(statements, contextAssignment(muxt.TemplateNameScopeIdentifierContext))
					default:
						if slices.Contains(def.PathValueIdentifiers(), arg.Name) {
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
			case slices.Contains(def.PathValueIdentifiers(), arg.Name):
				parsed[arg.Name] = struct{}{}
				s, err := generateParseValueFromStringStatements(file, def, arg.Name+"Parsed", resultType, src, param.Type(), nil, singleAssignment(token.DEFINE, ast.NewIdent(arg.Name)), rdIdent)
				if err != nil {
					return nil, err
				}
				statements = append(statements, s...)
				def.SetArgumentType(arg.Name, param.Type())
			case arg.Name == muxt.TemplateNameScopeIdentifierForm:
				s, err := appendParseFormToStructStatements(statements, def, file, resultType, arg, param, validationFailureBlock, rdIdent)
				if err != nil {
					return nil, err
				}
				statements = s
			default:
				pt, _ := file.TypeASTExpression(param.Type())
				at, _ := file.TypeASTExpression(argType)
				return nil, fmt.Errorf("method expects type %s but %s is %s", astgen.Format(pt), arg.Name, astgen.Format(at))
			}
		}
	}
	return statements, nil
}

func appendParseFormToStructStatements(statements []ast.Stmt, def muxt.Definition, file *File, resultType types.Type, arg *ast.Ident, param types.Object, validationBlock ValidationErrorBlock, rdIdent string) ([]ast.Stmt, error) {
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
			fieldTemplate = def.Template().Lookup(name)
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
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierForm), Sel: ast.NewIdent(field.Name())}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{astgen.CallBuiltinAppend(&ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierForm), Sel: ast.NewIdent(field.Name())}, expr)},
				}
			}
			str = ast.NewIdent("val")
			elemType = ft.Elem()
			validations, err, ok := GenerateValidations(file, ast.NewIdent(parsedVariableName), elemType, fmt.Sprintf("[name=%q]", inputName), inputName, muxt.TemplateNameScopeIdentifierHTTPResponse, dom.NewDocumentFragment(templateNodes), validationBlock)
			if ok && err != nil {
				return nil, err
			}
			parseStatements, err := generateParseValueFromStringStatements(file, def, parsedVariableName, resultType, str, elemType, validations, parseResult, rdIdent)
			if err != nil {
				return nil, fmt.Errorf("failed to generate parse statements for form field %s: %w", field.Name(), err)
			}
			statements = append(statements, &ast.RangeStmt{
				Key:   ast.NewIdent("_"),
				Value: ast.NewIdent("val"),
				Tok:   token.DEFINE,
				X:     &ast.IndexExpr{X: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("Form")}, Index: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(inputName)}},
				Body:  &ast.BlockStmt{List: parseStatements},
			})
		default:
			parseResult = func(expr ast.Expr) ast.Stmt {
				return &ast.AssignStmt{
					Lhs: []ast.Expr{&ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierForm), Sel: ast.NewIdent(field.Name())}},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{expr},
				}
			}
			str = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest), Sel: ast.NewIdent("FormValue")}, Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(inputName)}}}
			elemType = field.Type()
			validations, err, ok := GenerateValidations(file, ast.NewIdent(parsedVariableName), elemType, fmt.Sprintf("[name=%q]", inputName), inputName, muxt.TemplateNameScopeIdentifierHTTPResponse, dom.NewDocumentFragment(templateNodes), validationBlock)
			if ok && err != nil {
				return nil, err
			}
			parseStatements, err := generateParseValueFromStringStatements(file, def, parsedVariableName, resultType, str, elemType, validations, parseResult, rdIdent)
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

func formVariableDeclaration(file *File, arg *ast.Ident, tp types.Type) (*ast.DeclStmt, error) {
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

func formVariableAssignment(file *File, arg *ast.Ident, tp types.Type) (*ast.DeclStmt, error) {
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

func generateParseValueFromStringStatements(file *File, _ muxt.Definition, tmp string, resultType types.Type, str ast.Expr, valueType types.Type, validations []ast.Stmt, assignment func(ast.Expr) ast.Stmt, rdIdent string) ([]ast.Stmt, error) {
	errBlock := appendTemplateDataError(file, rdIdent, ast.NewIdent(errIdent))
	errBlock.List = append(errBlock.List, assignTemplateDataErrStatusCode(file, rdIdent, http.StatusBadRequest))
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
			return parseBlock(tmp, astgen.StrconvParseBoolCall(file, str), validations, errBlock, assignment), nil
		case "int":
			return parseBlock(tmp, astgen.StrconvAtoiCall(file, str), validations, errBlock, assignment), nil
		case "int8":
			return parseBlock(tmp, astgen.StrconvParseInt8Call(file, str), validations, errBlock, convert), nil
		case "int16":
			return parseBlock(tmp, astgen.StrconvParseInt16Call(file, str), validations, errBlock, convert), nil
		case "int32":
			return parseBlock(tmp, astgen.StrconvParseInt32Call(file, str), validations, errBlock, convert), nil
		case "int64":
			return parseBlock(tmp, astgen.StrconvParseInt64Call(file, str), validations, errBlock, assignment), nil
		case "uint":
			return parseBlock(tmp, astgen.StrconvParseUint0Call(file, str), validations, errBlock, convert), nil
		case "uint8":
			return parseBlock(tmp, astgen.StrconvParseUint8Call(file, str), validations, errBlock, convert), nil
		case "uint16":
			return parseBlock(tmp, astgen.StrconvParseUint16Call(file, str), validations, errBlock, convert), nil
		case "uint32":
			return parseBlock(tmp, astgen.StrconvParseUint32Call(file, str), validations, errBlock, convert), nil
		case "uint64":
			return parseBlock(tmp, astgen.StrconvParseUint64Call(file, str), validations, errBlock, assignment), nil
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
	return nil, fmt.Errorf("unsupported type: %s", astgen.Format(tp))
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

func callReceiverMethod(file *File, rdIdent string, dataVar ast.Expr, method *types.Signature, call *ast.CallExpr) ([]ast.Stmt, error) {
	const (
		okIdent = "ok"
	)
	switch method.Results().Len() {
	default:
		methodIdent := call.Fun.(*ast.Ident)
		assert.NotNil(assertion, methodIdent)
		return nil, fmt.Errorf("method %s has no results it should have one or two", methodIdent.Name)
	case 1:
		return []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{dataVar}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
			&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{
				X:   ast.NewIdent(rdIdent),
				Sel: ast.NewIdent(TemplateDataFieldIdentifierOkay),
			}}, Tok: token.ASSIGN, Rhs: []ast.Expr{astgen.Bool(true)}},
		}, nil
	case 2:
		lastResult := method.Results().At(method.Results().Len() - 1).Type()

		errorType := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
		assert.NotNil(assertion, errorType)

		errBlock := appendTemplateDataError(file, rdIdent, ast.NewIdent(errIdent))
		errBlock.List = append(errBlock.List, assignTemplateDataErrStatusCode(file, rdIdent, http.StatusInternalServerError))
		if types.Implements(lastResult, errorType) {
			return []ast.Stmt{
				&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent(errIdent)}, Type: ast.NewIdent("error")}}}},
				&ast.AssignStmt{Lhs: []ast.Expr{dataVar, ast.NewIdent(errIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{X: ast.NewIdent(errIdent), Op: token.NEQ, Y: astgen.Nil()},
					Body: errBlock,
				},
				&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{
					X:   ast.NewIdent(rdIdent),
					Sel: ast.NewIdent(TemplateDataFieldIdentifierResult),
				}}, Tok: token.ASSIGN, Rhs: []ast.Expr{dataVar}},
			}, nil
		}

		if basic, ok := lastResult.(*types.Basic); ok && basic.Kind() == types.Bool {
			return []ast.Stmt{
				&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ast.NewIdent("ok")}, Type: ast.NewIdent("bool")}}}},
				&ast.AssignStmt{Lhs: []ast.Expr{dataVar, ast.NewIdent(okIdent)}, Tok: token.ASSIGN, Rhs: []ast.Expr{call}},
				&ast.IfStmt{
					Cond: &ast.UnaryExpr{Op: token.NOT, X: ast.NewIdent(okIdent)},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.ReturnStmt{},
						},
					},
				},
				&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{
					X:   ast.NewIdent(rdIdent),
					Sel: ast.NewIdent(TemplateDataFieldIdentifierResult),
				}}, Tok: token.ASSIGN, Rhs: []ast.Expr{dataVar}},
				&ast.AssignStmt{Lhs: []ast.Expr{&ast.SelectorExpr{
					X:   ast.NewIdent(rdIdent),
					Sel: ast.NewIdent(TemplateDataFieldIdentifierOkay),
				}}, Tok: token.ASSIGN, Rhs: []ast.Expr{astgen.Bool(true)}},
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

func defaultTemplateNameScope(file *File, def muxt.Definition, argumentIdentifier string) (types.Type, bool) {
	switch argumentIdentifier {
	case muxt.TemplateNameScopeIdentifierHTTPRequest:
		pkg, ok := file.Types("net/http")
		if !ok {
			return nil, false
		}
		t := types.NewPointer(pkg.Scope().Lookup("Request").Type())
		return t, true
	case muxt.TemplateNameScopeIdentifierHTTPResponse:
		pkg, ok := file.Types("net/http")
		if !ok {
			return nil, false
		}
		t := pkg.Scope().Lookup("ResponseWriter").Type()
		return t, true
	case muxt.TemplateNameScopeIdentifierContext:
		pkg, ok := file.Types("context")
		if !ok {
			return nil, false
		}
		t := pkg.Scope().Lookup("Context").Type()
		return t, true
	case muxt.TemplateNameScopeIdentifierForm:
		pkg, ok := file.Types("net/url")
		if !ok {
			return nil, false
		}
		t := pkg.Scope().Lookup("Values").Type()
		return t, true
	default:
		if slices.Contains(def.PathValueIdentifiers(), argumentIdentifier) {
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

func ensureMethodSignature(file *File, signatures map[string]*types.Signature, def muxt.Definition, receiver *types.Named, receiverInterface *ast.InterfaceType, call *ast.CallExpr, templatesPackage *types.Package) error {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		isMethod := true
		mo, _, _ := types.LookupFieldOrMethod(receiver, true, receiver.Obj().Pkg(), fun.Name)
		if mo == nil {
			if m, ok := packageScopeFunc(templatesPackage, fun); ok {
				mo = m
				isMethod = false
			} else {
				ms, err := createMethodSignature(file, signatures, def, receiver, receiverInterface, call, templatesPackage)
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
					if err := ensureMethodSignature(file, signatures, def, receiver, receiverInterface, arg, templatesPackage); err != nil {
						return err
					}
				}
			}
		}
		if _, ok := signatures[fun.Name]; ok {
			return nil
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

func createMethodSignature(file *File, signatures map[string]*types.Signature, def muxt.Definition, receiver *types.Named, receiverInterface *ast.InterfaceType, call *ast.CallExpr, templatesPackage *types.Package) (*types.Signature, error) {
	var params []*types.Var
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.Ident:
			tp, ok := defaultTemplateNameScope(file, def, arg.Name)
			if !ok {
				return nil, fmt.Errorf("could not determine a type for %s", arg.Name)
			}
			params = append(params, types.NewVar(0, receiver.Obj().Pkg(), arg.Name, tp))
		case *ast.CallExpr:
			if err := ensureMethodSignature(file, signatures, def, receiver, receiverInterface, arg, templatesPackage); err != nil {
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
			X:   ast.NewIdent(muxt.TemplateNameScopeIdentifierHTTPRequest),
			Sel: ast.NewIdent("ParseForm"),
		},
		Args: []ast.Expr{},
	}}
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

var statusCoder = statusCoderInterface()

func writeStatusAndHeaders(file *File, def muxt.Definition, resultType types.Type, fallbackStatusCode int, statusCode, bufIdent, resultDataIdent string, resultVar func() ast.Expr) []ast.Stmt {
	statusCodePriorityList := []ast.Expr{
		&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(templateDataFieldStatusCode)},
		&ast.SelectorExpr{X: ast.NewIdent(resultDataIdent), Sel: ast.NewIdent(TemplateDataFieldIdentifierErrStatusCode)},
	}
	if types.Implements(resultType, statusCoder) {
		statusCodePriorityList = append(statusCodePriorityList, &ast.CallExpr{Fun: &ast.SelectorExpr{X: resultVar(), Sel: ast.NewIdent("StatusCode")}})
	} else if obj, _, _ := types.LookupFieldOrMethod(resultType, true, file.OutputPackage().Types, "StatusCode"); obj != nil {
		statusCodePriorityList = append(statusCodePriorityList, &ast.SelectorExpr{X: resultVar(), Sel: ast.NewIdent("StatusCode")})
	}
	statusCodePriorityList = append(statusCodePriorityList, astgen.HTTPStatusCode(file, fallbackStatusCode))
	list := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ast.NewIdent(statusCode)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{astgen.CmpOr(file, statusCodePriorityList...)},
		},
	}

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

func statusCoderInterface() *types.Interface {
	sig := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(),
		types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.Int])),
		false)

	method := types.NewFunc(token.NoPos, nil, "StatusCode", sig)
	return types.NewInterfaceType([]*types.Func{method}, nil).Complete()
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
