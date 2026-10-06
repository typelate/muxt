package cli

import (
	"github.com/typelate/muxt/internal/generate"
)

const (
	useTemplatesVariable                = "use-templates-variable"
	useReceiverType                     = "use-receiver-type"
	useReceiverTypePackage              = "use-receiver-type-package"
	outputFile                          = "output-file"
	outputReceiverInterface             = "output-receiver-interface"
	outputRoutesFunc                    = "output-routes-func"
	outputTemplateDataType              = "output-template-data-type"
	outputSSETemplateDataType           = "output-sse-template-data-type"
	outputTemplateRoutePathsType        = "output-template-route-paths-type"
	outputTemplateRouteType             = "output-template-route-type"
	outputTemplateRouteBuilderType      = "output-template-route-builder-type"
	outputRoutesFuncWithLoggerParam     = "output-routes-func-with-logger-param"
	outputRoutesFuncWithPathPrefix      = "output-routes-func-with-path-prefix-param"
	outputRoutesFuncWithMiddlewareParam = "output-routes-func-with-middleware-param"
	outputMultipleFiles                 = "output-multiple-files"
	outputHTMXHelpers                   = "output-htmx-helpers"
	outputHTMX                          = "output-htmx"
	outputDatastar                      = "output-datastar"
	outputExportedDefaultIdentifiers    = "output-exported-default-identifiers"
	outputMultipartMaxMemory            = "output-multipart-max-memory"
	outputMuxtVersion                   = "output-muxt-version"

	deprecatedPathPrefix = "path-prefix"
	deprecatedLogger     = "logger"

	deprecatedTemplatesVariable       = "templates-variable"
	deprecatedReceiverType            = "receiver-type"
	deprecatedReceiverTypePackage     = "receiver-type-package"
	deprecatedReceiverInterface       = "receiver-interface"
	deprecatedRoutesFunc              = "routes-func"
	deprecatedTemplateDataType        = "template-data-type"
	deprecatedTemplateRoutePathsType  = "template-route-paths-type"
	deprecatedFindTemplatesVariable   = "find-templates-variable"
	deprecatedFindReceiverType        = "find-receiver-type"
	deprecatedFindReceiverTypePackage = "find-receiver-type-package"

	useTemplatesVariableHelp   = `the name of the global variable with type *"html/template".Template in the working directory package.`
	useReceiverTypeHelp        = `The type name for a named type to use for looking up method signatures. If not set, all methods added to the receiver interface will have inferred signatures with argument types based on the argument identifier names. The inferred method signatures always return a single result of type any. Accepted by muxt and muxt generate; muxt check resolves receivers from the generated routes file.`
	useReceiverTypePackageHelp = `The package path to use when looking for use-receiver-type. If not set, the package in the current directory is used.`

	outputFileHelp              = `The generated file name containing the routes function and receiver interface.`
	outputReceiverInterfaceHelp = `The interface name in the generated output file listing the methods used by handler routes in the routes function.`
	outputRoutesFuncHelp        = `The function name for the package registering handler functions on an *"net/http".ServeMux.
This function also receives an argument with a type matching the name given by output-receiver-interface.`
	outputTemplateDataTypeHelp         = `The type name for the template data passed to root route templates.`
	outputSSETemplateDataTypeHelp      = `The type name for the template data passed to Server-Sent Events route templates.`
	outputTemplateRoutePathsTypeHelp   = `The type name for the type with path constructor helper methods.`
	outputTemplateRouteTypeHelp        = `The type name for the type the path constructor helper methods return.`
	outputTemplateRouteBuilderTypeHelp = `The type name for the type TemplateData.Route returns, whose methods return the route type. When not set, it is the route type name with Builder appended.`

	outputRoutesFuncWithLoggerParamHelp     = `Adds a *slog.Logger parameter to the generated routes function and uses it to log ExecuteTemplate errors and debug information in handlers.`
	outputRoutesFuncWithPathPrefixHelp      = `Adds a pathPrefix string parameter to the generated routes function and uses it in each path generator method.`
	outputRoutesFuncWithMiddlewareParamHelp = `Adds a middleware parameter with type func(next http.Handler) http.Handler to the generated routes function and wraps every registered handler with it. Passing nil registers handlers unwrapped.`
	outputMultipleFilesHelp                 = `Split generated routes into separate files per template source file. By default, all routes are written to a single file.`
	outputHTMXHelp                          = `Adds HTMX helper methods to TemplateData for setting response headers (HX-Location, HX-Redirect, etc.) and reading request headers (HX-Request, HX-Boosted, etc.).`
	outputDatastarHelp                      = `Frames Server-Sent Events with the Datastar patch protocol: SSETemplateData gains the patch option setters and WriteTo emits datastar-patch-elements events. Mutually exclusive with --output-htmx.`
	outputExportedDefaultIdentifiersHelp    = `When false, default generated identifiers (functions, types, interfaces) use lowercase/private names. Does not affect explicit --output-* flag values. Defaults to true.`
	outputMultipartMaxMemoryHelp            = `Maximum memory used by request.ParseMultipartForm in generated handlers. Accepts a human-readable byte size (e.g. 32MB, 64MiB, 1GB).`
	outputMuxtVersionHelp                   = `When false, the muxt version is left out of generated files: the "// muxt version:" header comment is omitted and no MuxtVersion method is added to TemplateData. Defaults to true.`

	errIdentSuffix = " value must be a well-formed Go identifier"
)

const (
	defaultTemplatesVariableName        = "templates"
	defaultRoutesFunctionName           = generate.DefaultRoutesFunctionName
	defaultOutputFileName               = "template_routes.go"
	defaultReceiverInterfaceName        = generate.DefaultReceiverInterfaceName
	defaultTemplateRoutePathsTypeName   = generate.DefaultTemplateRoutePathsTypeName
	defaultTemplateRouteTypeName        = generate.DefaultTemplateRouteTypeName
	defaultTemplateRouteBuilderTypeName = generate.DefaultTemplateRouteBuilderTypeName
	defaultTemplateDataTypeName         = "TemplateData"
	defaultSSETemplateDataTypeName      = "SSETemplateData"
	defaultPackageName                  = "main"
)
