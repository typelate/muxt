package cli

import (
	"github.com/typelate/muxt/internal/generate"
)

const (
	useTemplatesVariable                = "use-templates-variable"
	useReceiverType                     = "use-receiver-type"
	useReceiverTypePackage              = "use-receiver-type-package"
	outputFile                          = "output-file"
	outputDocument                      = "output-document"
	outputFragment                      = "output-fragment"
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
	inputStaticAssetsDirectory          = "input-static-assets-directory"
	outputStaticAssetsPath              = "output-static-assets-path"

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

	identRule      = "value must be a well-formed Go identifier"
	errIdentSuffix = " " + identRule
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
