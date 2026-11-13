package cli

import (
	"github.com/typelate/muxt/internal/generate"
)

// generateSettings declares every setting muxt generate takes, bound to the
// fields of c. List order is the order the generated file header records them
// in; --help and the configuration report sort by name.
func generateSettings(c *generate.RoutesFileConfiguration) []setting {
	return []setting{
		templatesVariableSetting(&c.TemplatesVariables),
		receiverTypeSetting(&c.ReceiverType),
		receiverPackageSetting(&c.ReceiverPackage),
		stringSetting(&c.OutputFileName, spec[string]{
			Name: outputFile, Default: defaultOutputFileName, NonEmpty: true, Example: "routes.go",
			Usage: `The generated file name containing the routes function and receiver interface. A file name in the working directory, without whitespace.`,
		}),
		stringSetting(&c.ReceiverInterface, spec[string]{
			Name: outputReceiverInterface, Default: defaultReceiverInterfaceName, NonEmpty: true,
			Deprecated: []string{deprecatedReceiverInterface}, Example: "RoutesReceiver",
			Usage: `The interface name in the generated output file listing the methods used by handler routes in the routes function.`,
		}),
		stringSetting(&c.RoutesFunction, spec[string]{
			Name: outputRoutesFunc, Default: defaultRoutesFunctionName, NonEmpty: true,
			Deprecated: []string{deprecatedRoutesFunc}, Example: "Routes",
			Usage: `The function name for the package registering handler functions on an *"net/http".ServeMux.
This function also receives an argument with a type matching the name given by output-receiver-interface.`,
		}),
		stringSetting(&c.TemplateDataType, spec[string]{
			Name: outputTemplateDataType, Default: defaultTemplateDataTypeName, NonEmpty: true,
			Deprecated: []string{deprecatedTemplateDataType}, Example: "PageData",
			Usage: `The type name for the template data passed to root route templates.`,
		}),
		stringSetting(&c.SSETemplateDataType, spec[string]{
			Name: outputSSETemplateDataType, Default: defaultSSETemplateDataTypeName, NonEmpty: true, Example: "PageEvent",
			Usage: `The type name for the template data passed to Server-Sent Events route templates.`,
		}),
		stringSetting(&c.TemplateRoutePathsTypeName, spec[string]{
			Name: outputTemplateRoutePathsType, Default: defaultTemplateRoutePathsTypeName, NonEmpty: true,
			Deprecated: []string{deprecatedTemplateRoutePathsType}, Example: "SitePaths",
			Usage: `The type name for the type with path constructor helper methods.`,
		}),
		stringSetting(&c.TemplateRouteTypeName, spec[string]{
			Name: outputTemplateRouteType, Default: defaultTemplateRouteTypeName, NonEmpty: true, Example: "SiteRoute",
			Derived: func() string {
				return defaultRouteTypeName(c.TemplateRoutePathsTypeName, c.OutputExportedDefaultIdentifiers)
			},
			Usage: `The type name for the type the route builder's methods return. When not set, it is TemplateRoute, or the route paths type name with Route appended when output-template-route-paths-type names another type.`,
		}),
		stringSetting(&c.TemplateRouteBuilderTypeName, spec[string]{
			Name: outputTemplateRouteBuilderType, Default: defaultTemplateRouteBuilderTypeName, NonEmpty: true, Example: "SiteRouteBuilder",
			Derived: func() string { return c.TemplateRouteTypeName + routeBuilderSuffix },
			Usage:   `The type name for the type TemplateData.Route returns, whose methods return the route type. When not set, it is the route type name with Builder appended.`,
		}),
		boolSetting(&c.Logger, spec[bool]{
			Name: outputRoutesFuncWithLoggerParam, Deprecated: []string{deprecatedLogger}, Example: "true",
			Usage: `Adds a *slog.Logger parameter to the generated routes function and uses it to log ExecuteTemplate errors and debug information in handlers.`,
		}),
		boolSetting(&c.PathPrefix, spec[bool]{
			Name: outputRoutesFuncWithPathPrefix, Deprecated: []string{deprecatedPathPrefix}, Example: "true",
			Usage: `Adds a pathsPrefix string parameter to the generated routes function and uses it in each path generator method.`,
		}),
		boolSetting(&c.Middleware, spec[bool]{
			Name: outputRoutesFuncWithMiddlewareParam, Example: "true",
			Usage: `Adds a middleware parameter with type func(next http.Handler) http.Handler to the generated routes function and wraps every registered handler with it. Passing nil registers handlers unwrapped.`,
		}),
		boolSetting(&c.OutputMultipleFiles, spec[bool]{
			Name: outputMultipleFiles, Example: "true",
			Usage: `Split generated routes into separate files per template source file. By default, all routes are written to a single file.`,
		}),
		boolSetting(&c.OutputHTMX, spec[bool]{
			Name: outputHTMX, Deprecated: []string{outputHTMXHelpers}, Example: "true",
			Usage: `Adds HTMX helper methods to TemplateData for setting response headers (HX-Location, HX-Redirect, etc.) and reading request headers (HX-Request, HX-Boosted, etc.).`,
		}),
		boolSetting(&c.OutputDatastar, spec[bool]{
			Name: outputDatastar, Example: "true",
			Usage: `Frames Server-Sent Events with the Datastar patch protocol: SSETemplateData gains the patch option setters and WriteTo emits datastar-patch-elements events. Mutually exclusive with --output-htmx.`,
		}),
		boolSetting(&c.OutputExportedDefaultIdentifiers, spec[bool]{
			Name: outputExportedDefaultIdentifiers, Default: true, Example: "false",
			Usage: `When false, default generated identifiers (functions, types, interfaces) use lowercase/private names. Does not affect explicit --output-* flag values. Defaults to true.`,
		}),
		boolSetting(&c.OutputMuxtVersion, spec[bool]{
			Name: outputMuxtVersion, Default: true, Example: "false",
			Usage: `When false, the muxt version is left out of generated files: the "// muxt version:" header comment is omitted and no MuxtVersion method is added to TemplateData. Defaults to true.`,
		}),
		bytesSetting(&c.MultipartMaxMemory, spec[int64]{
			Name: outputMultipartMaxMemory, Example: "64MB",
			Usage: `Maximum memory used by request.ParseMultipartForm in generated handlers. Accepts a human-readable byte size (e.g. 32MB, 64MiB, 1GB).`,
		}),
		stringSetting(&c.StaticAssetsDirectory, spec[string]{
			Name: inputStaticAssetsDirectory, Example: "static",
			Usage: `A directory inside the templates package, relative to it, whose files generate embeds and serves under --output-static-assets-path. Requires --output-static-assets-path.`,
		}),
	}
}

// templatesVariableSetting is shared by every command that reads templates.
func templatesVariableSetting(p *[]string) setting {
	return stringsSetting(p, spec[[]string]{
		Name:       useTemplatesVariable,
		Usage:      useTemplatesVariableHelp,
		Default:    []string{defaultTemplatesVariableName},
		NonEmpty:   true,
		Deprecated: []string{deprecatedFindTemplatesVariable},
		Example:    "views",
	})
}
