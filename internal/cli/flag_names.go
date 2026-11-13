package cli

import (
	"strings"

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

	outputDatastarHelp                   = `Frames Server-Sent Events with the Datastar patch protocol: SSETemplateData gains the patch option setters and WriteTo emits datastar-patch-elements events. Mutually exclusive with --output-htmx.`
	outputExportedDefaultIdentifiersHelp = `When false, default generated identifiers (functions, types, interfaces) use lowercase/private names. Does not affect explicit --output-* flag values. Defaults to true.`
	outputMultipartMaxMemoryHelp         = `Maximum memory used by request.ParseMultipartForm in generated handlers. Accepts a human-readable byte size (e.g. 32MB, 64MiB, 1GB).`
	outputMuxtVersionHelp                = `When false, the muxt version is left out of generated files: the "// muxt version:" header comment is omitted and no MuxtVersion method is added to TemplateData. Defaults to true.`

	inputStaticAssetsDirectoryHelp = `A directory inside the templates package, relative to it, whose files generate embeds and serves under --output-static-assets-path. Requires --output-static-assets-path.`
	outputStaticAssetsPathHelp     = `The URL path the files of --input-static-assets-directory are served under with GET, such as /static (a trailing / is added). Adds a TemplateRoutePaths method returning it. Requires --input-static-assets-directory.`

	errIdentSuffix = " value must be a well-formed Go identifier"
)

// muxtJSONKey returns the muxt.json key for a generate flag name: the name
// with each "-" written as "_". Keys use underscores so the file's spelling
// matches the identifiers a Go developer already types, and the flag names stay
// as the command line spells them.
func muxtJSONKey(flag string) string {
	return strings.ReplaceAll(flag, "-", "_")
}

// deprecatedMuxtJSONKeys maps each deprecated generate flag name, as a muxt.json
// key, to the key that replaces it, the keys a muxt.json entry must not use. It
// is built from the same names the flags are registered with, so a flag marked
// deprecated there has its row here.
func deprecatedMuxtJSONKeys() map[string]string {
	return map[string]string{
		muxtJSONKey(deprecatedTemplatesVariable):       muxtJSONKey(useTemplatesVariable),
		muxtJSONKey(deprecatedReceiverType):            muxtJSONKey(useReceiverType),
		muxtJSONKey(deprecatedReceiverTypePackage):     muxtJSONKey(useReceiverTypePackage),
		muxtJSONKey(deprecatedReceiverInterface):       muxtJSONKey(outputReceiverInterface),
		muxtJSONKey(deprecatedRoutesFunc):              muxtJSONKey(outputRoutesFunc),
		muxtJSONKey(deprecatedTemplateDataType):        muxtJSONKey(outputTemplateDataType),
		muxtJSONKey(deprecatedTemplateRoutePathsType):  muxtJSONKey(outputTemplateRoutePathsType),
		muxtJSONKey(deprecatedLogger):                  muxtJSONKey(outputRoutesFuncWithLoggerParam),
		muxtJSONKey(deprecatedPathPrefix):              muxtJSONKey(outputRoutesFuncWithPathPrefix),
		muxtJSONKey(deprecatedFindTemplatesVariable):   muxtJSONKey(useTemplatesVariable),
		muxtJSONKey(deprecatedFindReceiverType):        muxtJSONKey(useReceiverType),
		muxtJSONKey(deprecatedFindReceiverTypePackage): muxtJSONKey(useReceiverTypePackage),
	}
}

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
