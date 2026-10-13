package cli

import (
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
	"github.com/typelate/muxt/internal/mutation"
)

// configurationOf runs a command line through the flags with runners that
// record the configuration they are handed, and returns it. Nothing is
// loaded: what a command line means is decided before a runner is called.
func configurationOf(t *testing.T, commandLine string, env map[string]string) (any, error) {
	t.Helper()
	var got any
	record := func(config any) error {
		got = config
		return nil
	}
	err := commands("/work", strings.Fields(commandLine), func(key string) string { return env[key] }, func() (string, bool) { return "v1.2.3", true }, io.Discard, io.Discard, runners{
		routes:    func(_ *cobra.Command, _ string, c analysis.DefinitionsConfiguration) error { return record(c) },
		check:     func(_ *cobra.Command, _ string, c analysis.CheckConfiguration) error { return record(c) },
		generate:  func(_ *cobra.Command, _ string, c generate.RoutesFileConfiguration) error { return record(c) },
		callers:   func(_ *cobra.Command, _ string, c analysis.TemplateCallersConfiguration) error { return record(c) },
		calls:     func(_ *cobra.Command, _ string, c analysis.TemplateCallsConfiguration) error { return record(c) },
		mutations: func(_ *cobra.Command, _ string, c mutation.Configuration) error { return record(c) },
	})
	return got, err
}

// TestCommandLineConfigurations states what each command line parses into:
// the flags with their defaults applied and checked, as the configuration
// the command runs with.
//
// The implementation tests start from these literals: a test of what a
// command does with a configuration repeats one of them beside the command
// line it stands for, so searching for the literal finds both. Command
// lines that cannot work are in TestCommandLineRejections, so a
// configuration that reaches an implementation is one that can work.
func TestCommandLineConfigurations(t *testing.T) {
	for _, tt := range []struct {
		name string
		args string
		env  map[string]string
		want any
	}{
		{
			name: "the defaults",
			args: "generate",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "a receiver type",
			args: "generate --use-receiver-type=T",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverType:                     "T",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "a receiver type named Server",
			args: "generate --use-receiver-type=Server",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverType:                     "Server",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "the generated names",
			args: "generate --use-receiver-type=Server --output-file=routes.go --output-routes-func=Routes --output-receiver-interface=Handlers --output-template-data-type=Data --output-template-route-paths-type=Paths --output-template-route-type=Route",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "Routes",
				ReceiverType:                     "Server",
				ReceiverInterface:                "Handlers",
				TemplateDataType:                 "Data",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "Paths",
				TemplateRouteTypeName:            "Route",
				TemplateRouteBuilderTypeName:     "RouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "the route builder named on its own",
			args: "generate --output-template-route-type=Route --output-template-route-builder-type=Link",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "Route",
				TemplateRouteBuilderTypeName:     "Link",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			// Several route sets can share a package when each names its
			// own paths type, as they could before the route types were
			// generated: the route types are named after the paths type.
			name: "a route paths type names the route types after it",
			args: "generate --output-template-route-paths-type=P1",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "P1",
				TemplateRouteTypeName:            "P1Route",
				TemplateRouteBuilderTypeName:     "P1RouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "a deprecated route paths type flag names the route types after it",
			args: "generate --template-route-paths-type=P1",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "P1",
				TemplateRouteTypeName:            "P1Route",
				TemplateRouteBuilderTypeName:     "P1RouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "a route paths type that is the default names the default route types",
			args: "generate --output-template-route-paths-type=TemplateRoutePaths",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "a route paths type and a route builder type",
			args: "generate --output-template-route-paths-type=P1 --output-template-route-builder-type=Link",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "P1",
				TemplateRouteTypeName:            "P1Route",
				TemplateRouteBuilderTypeName:     "Link",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "unexported default identifiers name the route types after a route paths type",
			args: "generate --output-exported-default-identifiers=false --output-template-route-paths-type=p1",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                  "v1.2.3",
				PackageName:                  "main",
				RoutesFunction:               "templateRoutes",
				ReceiverInterface:            "routesReceiver",
				TemplateDataType:             "templateData",
				SSETemplateDataType:          "sseTemplateData",
				TemplateRoutePathsTypeName:   "p1",
				TemplateRouteTypeName:        "p1Route",
				TemplateRouteBuilderTypeName: "p1RouteBuilder",
				TemplatesVariables:           []string{"templates"},
				OutputFileName:               "template_routes.go",
				OutputMuxtVersion:            true,
			},
		},
		{
			name: "htmx helpers",
			args: "generate --output-htmx",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputHTMX:                       true,
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "the routes function parameters",
			args: "generate --use-receiver-type=T --output-routes-func-with-logger-param --output-routes-func-with-path-prefix-param --output-routes-func-with-middleware-param",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverType:                     "T",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				PathPrefix:                       true,
				Logger:                           true,
				Middleware:                       true,
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "a file per template file",
			args: "generate --use-receiver-type=T --output-multiple-files --output-routes-func-with-middleware-param",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverType:                     "T",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				Middleware:                       true,
				OutputMultipleFiles:              true,
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "unexported default identifiers",
			args: "generate --output-exported-default-identifiers=false",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                  "v1.2.3",
				PackageName:                  "main",
				RoutesFunction:               "templateRoutes",
				ReceiverInterface:            "routesReceiver",
				TemplateDataType:             "templateData",
				SSETemplateDataType:          "sseTemplateData",
				TemplateRoutePathsTypeName:   "templateRoutePaths",
				TemplateRouteTypeName:        "templateRoute",
				TemplateRouteBuilderTypeName: "templateRouteBuilder",
				TemplatesVariables:           []string{"templates"},
				OutputFileName:               "template_routes.go",
				OutputMuxtVersion:            true,
			},
		},
		{
			name: "unexported default identifiers keep every explicit name",
			args: "generate --output-exported-default-identifiers=false --output-receiver-interface=Handlers --output-template-data-type=Data --output-sse-template-data-type=SSEData --output-template-route-paths-type=Paths --output-template-route-type=Route",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                  "v1.2.3",
				PackageName:                  "main",
				RoutesFunction:               "templateRoutes",
				ReceiverInterface:            "Handlers",
				TemplateDataType:             "Data",
				SSETemplateDataType:          "SSEData",
				TemplateRoutePathsTypeName:   "Paths",
				TemplateRouteTypeName:        "Route",
				TemplateRouteBuilderTypeName: "RouteBuilder",
				TemplatesVariables:           []string{"templates"},
				OutputFileName:               "template_routes.go",
				OutputMuxtVersion:            true,
			},
		},
		{
			name: "unexported default identifiers with a receiver type",
			args: "generate --use-receiver-type=Server --output-exported-default-identifiers=false",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                  "v1.2.3",
				PackageName:                  "main",
				RoutesFunction:               "templateRoutes",
				ReceiverType:                 "Server",
				ReceiverInterface:            "routesReceiver",
				TemplateDataType:             "templateData",
				SSETemplateDataType:          "sseTemplateData",
				TemplateRoutePathsTypeName:   "templateRoutePaths",
				TemplateRouteTypeName:        "templateRoute",
				TemplateRouteBuilderTypeName: "templateRouteBuilder",
				TemplatesVariables:           []string{"templates"},
				OutputFileName:               "template_routes.go",
				OutputMuxtVersion:            true,
			},
		},
		{
			name: "unexported default identifiers keep an explicit name",
			args: "generate --output-exported-default-identifiers=false --output-routes-func=Routes",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                  "v1.2.3",
				PackageName:                  "main",
				RoutesFunction:               "Routes",
				ReceiverInterface:            "routesReceiver",
				TemplateDataType:             "templateData",
				SSETemplateDataType:          "sseTemplateData",
				TemplateRoutePathsTypeName:   "templateRoutePaths",
				TemplateRouteTypeName:        "templateRoute",
				TemplateRouteBuilderTypeName: "templateRouteBuilder",
				TemplatesVariables:           []string{"templates"},
				OutputFileName:               "template_routes.go",
				OutputMuxtVersion:            true,
			},
		},
		{
			name: "unexported default identifiers keep names given by deprecated flags",
			args: "generate --output-exported-default-identifiers=false --routes-func=Routes --receiver-interface=Handlers --template-data-type=Data --template-route-paths-type=Paths",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                  "v1.2.3",
				PackageName:                  "main",
				RoutesFunction:               "Routes",
				ReceiverInterface:            "Handlers",
				TemplateDataType:             "Data",
				SSETemplateDataType:          "sseTemplateData",
				TemplateRoutePathsTypeName:   "Paths",
				TemplateRouteTypeName:        "PathsRoute",
				TemplateRouteBuilderTypeName: "PathsRouteBuilder",
				TemplatesVariables:           []string{"templates"},
				OutputFileName:               "template_routes.go",
				OutputMuxtVersion:            true,
			},
		},
		{
			name: "a multipart memory limit in human units",
			args: "generate --use-receiver-type=T --output-multipart-max-memory=1MiB",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverType:                     "T",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
				MultipartMaxMemory:               1 << 20,
			},
		},
		{
			name: "datastar",
			args: "generate --use-receiver-type=T --output-datastar",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverType:                     "T",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputDatastar:                   true,
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "the version left out",
			args: "generate --output-muxt-version=false",
			want: generate.RoutesFileConfiguration{
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
			},
		},
		{
			name: "several templates variables",
			args: "generate --use-templates-variable=pages --use-templates-variable=admin",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"pages", "admin"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "a receiver in another package",
			args: "generate --use-receiver-type=Server --use-receiver-type-package=example.com/app",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverType:                     "Server",
				ReceiverPackage:                  "example.com/app",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "verbose",
			args: "generate -v",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
				Verbose:                          true,
			},
		},
		{
			name: "deprecated flags",
			args: "generate --templates-variable=pages --receiver-type=T --routes-func=Routes --logger --path-prefix --output-htmx-helpers",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "Routes",
				ReceiverType:                     "T",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"pages"},
				OutputFileName:                   "template_routes.go",
				PathPrefix:                       true,
				Logger:                           true,
				OutputHTMX:                       true,
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "an alias",
			args: "gen --use-receiver-type=T",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverType:                     "T",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "the response argument warning silenced from the environment",
			args: "generate --use-receiver-type=T",
			env:  map[string]string{envSilenceHTTPResponseWarning: "true"},
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "TemplateRoutes",
				ReceiverType:                     "T",
				ReceiverInterface:                "RoutesReceiver",
				TemplateDataType:                 "TemplateData",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "TemplateRoutePaths",
				TemplateRouteTypeName:            "TemplateRoute",
				TemplateRouteBuilderTypeName:     "TemplateRouteBuilder",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
				SilenceHTTPResponseWarning:       true,
			},
		},
		{
			name: "check",
			args: "check",
			want: analysis.CheckConfiguration{TemplatesVariables: []string{"templates"}},
		},
		{
			name: "check verbosely",
			args: "check -v",
			want: analysis.CheckConfiguration{Verbose: true, TemplatesVariables: []string{"templates"}},
		},
		{
			name: "the route listing",
			args: "--use-receiver-type=T",
			want: analysis.DefinitionsConfiguration{ReceiverType: "T", TemplatesVariables: []string{"templates"}},
		},
		{
			name: "template callers",
			args: "list-template-callers",
			want: analysis.TemplateCallersConfiguration{TemplatesVariables: []string{"templates"}},
		},
		{
			name: "template callers matching a name",
			args: "list-template-callers --match=^head",
			want: analysis.TemplateCallersConfiguration{TemplatesVariables: []string{"templates"}, FilterTemplates: []*regexp.Regexp{regexp.MustCompile("^head")}},
		},
		{
			name: "template calls",
			args: "list-template-calls",
			want: analysis.TemplateCallsConfiguration{TemplatesVariables: []string{"templates"}},
		},
		{
			name: "template calls matching a pattern",
			args: "list-template-calls --match=^Index$",
			want: analysis.TemplateCallsConfiguration{FilterTemplates: []*regexp.Regexp{regexp.MustCompile("^Index$")}, TemplatesVariables: []string{"templates"}},
		},
		{
			name: "a mutation dry run",
			args: "test-template-mutations --dry-run --seed=1 -v",
			want: mutation.Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: mutation.DefaultMaxCases, Workers: 1},
		},
		{
			name: "a mutation dry run, quietly",
			args: "test-template-mutations --dry-run --seed=1",
			want: mutation.Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Seed: 1, SeedSet: true, MaxCases: mutation.DefaultMaxCases, Workers: 1},
		},
		{
			name: "a mutation dry run of the templates a pattern matches",
			args: "test-template-mutations --dry-run --seed=1 -v --template-pattern=^footer$",
			want: mutation.Configuration{TemplatesVariables: []string{"templates"}, TemplatePattern: regexp.MustCompile("^footer$"), Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: mutation.DefaultMaxCases, Workers: 1},
		},
		{
			name: "a mutation dry run with an operand budget",
			args: "test-template-mutations --dry-run --seed=1 -v --max-cases=2",
			want: mutation.Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: 2, Workers: 1},
		},
		{
			name: "a mutation dry run of what changed since a revision",
			args: "test-template-mutations --dry-run --seed=1 -v --diff=main",
			want: mutation.Configuration{TemplatesVariables: []string{"templates"}, Packages: []string{}, DryRun: true, Verbose: true, Seed: 1, SeedSet: true, MaxCases: mutation.DefaultMaxCases, Workers: 1, Diff: "main"},
		},
		{
			name: "a mutation dry run of another templates variable",
			args: "test-template-mutations --dry-run --seed=1 --use-templates-variable=pages",
			want: mutation.Configuration{TemplatesVariables: []string{"pages"}, Packages: []string{}, DryRun: true, Seed: 1, SeedSet: true, MaxCases: mutation.DefaultMaxCases, Workers: 1},
		},
		{
			// The package directory is where the command runs; see
			// TestChangeDirectory. go test runs ./... from there.
			name: "mutations of a package directory, with patterns and go test flags",
			args: "test-template-mutations --template-pattern=^page --run=TestPage --include-test-callers --max-cases=2 --workers=4 --diff=main ./internal/preview -- -count=1",
			want: mutation.Configuration{TemplatesVariables: []string{"templates"}, TemplatePattern: regexp.MustCompile("^page"), Run: regexp.MustCompile("TestPage"), Packages: []string{}, GoTestArgs: []string{"-count=1"}, IncludeTests: true, MaxCases: 2, Workers: 4, Diff: "main"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := configurationOf(t, tt.args, tt.env)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestChangeDirectory states the working directory a command runs in: the
// one it was started in, joined with -C when -C is relative, and then with
// the package directory check and test-template-mutations take, resolved
// the same way.
func TestChangeDirectory(t *testing.T) {
	for _, tt := range []struct {
		args, want string
	}{
		{args: "generate", want: "/work"},
		{args: "-C sub generate", want: "/work/sub"},
		{args: "-C ../other check", want: "/other"},
		{args: "-C /abs check", want: "/abs"},
		{args: "check ./internal/preview", want: "/work/internal/preview"},
		{args: "check internal/preview/", want: "/work/internal/preview"},
		{args: "check /abs/preview", want: "/abs/preview"},
		{args: "-C sub check ./preview", want: "/work/sub/preview"},
		{args: "check .", want: "/work"},
		{args: "test-template-mutations ./internal/preview --dry-run", want: "/work/internal/preview"},
		{args: "test-template-mutations --dry-run ./internal/preview -- -count=1", want: "/work/internal/preview"},
		{args: "-C sub test-template-mutations ../preview", want: "/work/preview"},
		{args: "test-template-mutations -- -count=1", want: "/work"},
	} {
		t.Run(tt.args, func(t *testing.T) {
			var got string
			record := func(wd string) error {
				got = wd
				return nil
			}
			err := commands("/work", strings.Fields(tt.args), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, io.Discard, io.Discard, runners{
				check:     func(_ *cobra.Command, wd string, _ analysis.CheckConfiguration) error { return record(wd) },
				generate:  func(_ *cobra.Command, wd string, _ generate.RoutesFileConfiguration) error { return record(wd) },
				mutations: func(_ *cobra.Command, wd string, _ mutation.Configuration) error { return record(wd) },
			})
			require.NoError(t, err)
			assert.Equal(t, filepath.FromSlash(tt.want), got)
		})
	}
}

// TestCommandLineRejections states the command lines that never reach a
// command: each is refused, with the error the user sees, before anything
// is loaded.
func TestCommandLineRejections(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    string
		wantErr string
	}{
		{name: "a templates variable that is not an identifier", args: "generate --use-templates-variable=not-ok", wantErr: "variable not-ok value must be a well-formed Go identifier"},
		{name: "a routes function that is not an identifier", args: "generate --output-routes-func=1Routes", wantErr: "output-routes-func value must be a well-formed Go identifier"},
		{name: "a receiver type that is not an identifier", args: "generate --use-receiver-type=a.b", wantErr: "use-receiver-type value must be a well-formed Go identifier"},
		{name: "a receiver interface that is not an identifier", args: "generate --output-receiver-interface=a-b", wantErr: "output-receiver-interface value must be a well-formed Go identifier"},
		{name: "a template data type that is not an identifier", args: "generate --output-template-data-type=a-b", wantErr: "output-template-data-type value must be a well-formed Go identifier"},
		{name: "an sse template data type that is not an identifier", args: "generate --output-sse-template-data-type=a-b", wantErr: "output-sse-template-data-type value must be a well-formed Go identifier"},
		{name: "a route paths type that is not an identifier", args: "generate --output-template-route-paths-type=a-b", wantErr: "output-template-route-paths-type value must be a well-formed Go identifier"},
		{name: "htmx and datastar together", args: "generate --output-htmx --output-datastar", wantErr: "--output-htmx and --output-datastar are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)"},
		{name: "an output file that is not Go", args: "generate --output-file=routes.txt", wantErr: "output filename must use .go extension"},
		{name: "an output file in another directory", args: "generate --output-file=sub/routes.go", wantErr: "--output-file must be a file name in the working directory: sub/routes.go"},
		{name: "an output file with no name", args: "generate --output-file=.go", wantErr: "--output-file needs a file name before the .go extension"},
		{name: "an empty output file", args: "generate --output-file=", wantErr: "--output-file value must not be empty"},
		{name: "an empty routes function", args: "generate --output-routes-func=", wantErr: "--output-routes-func value must not be empty"},
		{name: "an empty routes function with unexported defaults", args: "generate --output-exported-default-identifiers=false --output-routes-func=", wantErr: "--output-routes-func value must not be empty"},
		{name: "an empty routes function through a deprecated flag", args: "generate --routes-func=", wantErr: "--routes-func value must not be empty"},
		{name: "an empty receiver interface", args: "generate --output-receiver-interface=", wantErr: "--output-receiver-interface value must not be empty"},
		{name: "an empty template data type", args: "generate --output-template-data-type=", wantErr: "--output-template-data-type value must not be empty"},
		{name: "an empty sse template data type", args: "generate --output-sse-template-data-type=", wantErr: "--output-sse-template-data-type value must not be empty"},
		{name: "an empty route paths type", args: "generate --output-template-route-paths-type=", wantErr: "--output-template-route-paths-type value must not be empty"},
		{name: "an empty route type", args: "generate --output-template-route-type=", wantErr: "--output-template-route-type value must not be empty"},
		{name: "an empty route builder type", args: "generate --output-template-route-builder-type=", wantErr: "--output-template-route-builder-type value must not be empty"},
		{name: "two generated types with one name", args: "generate --output-receiver-interface=Server --output-template-data-type=Server", wantErr: "--output-receiver-interface and --output-template-data-type are both Server; each generated type needs its own name"},
		{name: "a generated type named like a default", args: "generate --output-template-data-type=TemplateRoutes", wantErr: "--output-routes-func and --output-template-data-type are both TemplateRoutes; each generated type needs its own name"},
		{name: "a route type named like the paths type", args: "generate --output-template-route-type=TemplateRoutePaths", wantErr: "--output-template-route-paths-type and --output-template-route-type are both TemplateRoutePaths; each generated type needs its own name"},
		{name: "a route builder named like the route type", args: "generate --output-template-route-builder-type=TemplateRoute", wantErr: "--output-template-route-type and --output-template-route-builder-type are both TemplateRoute; each generated type needs its own name"},
		{name: "a generated type named like a derived route type", args: "generate --output-template-route-paths-type=P1 --output-template-data-type=P1Route", wantErr: "--output-template-data-type and --output-template-route-type are both P1Route; each generated type needs its own name"},
		{name: "unexported defaults and a name that collides with one", args: "generate --output-exported-default-identifiers=false --output-template-data-type=templateRoutes", wantErr: "--output-routes-func and --output-template-data-type are both templateRoutes; each generated type needs its own name"},
		{name: "a multipart limit of zero", args: "generate --output-multipart-max-memory=0", wantErr: `invalid argument "0" for "--output-multipart-max-memory" flag: multipart max memory must be positive, got "0"`},
		{name: "a repeated templates variable", args: "generate --use-templates-variable=pages --use-templates-variable=pages", wantErr: "duplicate template variable: pages"},
		{name: "the deprecated and new templates variable flags together", args: "check --templates-variable=a --use-templates-variable=b", wantErr: "deprecated flag templates-variable not permitted along with use-templates-variable"},
		{name: "a check templates variable that is not an identifier", args: "check --use-templates-variable=not-ok", wantErr: "variable not-ok value must be a well-formed Go identifier"},
		{name: "a template pattern that does not compile", args: "test-template-mutations --template-pattern=(", wantErr: "--template-pattern: error parsing regexp: missing closing ): `(`"},
		{name: "a run pattern that does not compile", args: "test-template-mutations --run=(", wantErr: "--run: error parsing regexp: missing closing ): `(`"},
		{name: "a callers match that does not compile", args: "list-template-callers --match=(", wantErr: "error parsing regexp: missing closing ): `(`"},
		{name: "check of two package directories", args: "check ./a ./b", wantErr: "check takes one package directory, got 2: ./a ./b"},
		{name: "check of a package pattern", args: "check ./...", wantErr: "check takes one package directory, not a pattern: ./..."},
		{name: "mutations of two package directories", args: "test-template-mutations ./a ./b -- -count=1", wantErr: "test-template-mutations takes one package directory, got 2: ./a ./b"},
		{name: "mutations of a package pattern", args: "test-template-mutations ./internal/... --dry-run", wantErr: "test-template-mutations takes one package directory, not a pattern: ./internal/..."},
		{name: "an unknown route listing format", args: "--format=yaml", wantErr: "unknown format: yaml"},
		{name: "an unknown callers format", args: "list-template-callers --format=yaml", wantErr: "unknown format: yaml"},
		{name: "an unknown calls format", args: "list-template-calls --format=yaml", wantErr: "unknown format: yaml"},
		{name: "an unknown mutation report format", args: "test-template-mutations --format=yaml", wantErr: "unknown format: yaml"},
		{name: "an unknown explore-module format", args: "explore-module --format=yaml", wantErr: "unknown format: yaml"},
		{name: "a go test flag muxt needs for itself", args: "test-template-mutations -- -overlay=other.json", wantErr: "go test flag -overlay cannot be passed through: muxt uses -overlay to deliver each mutant"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := configurationOf(t, tt.args, nil)
			require.EqualError(t, err, tt.wantErr)
			assert.Nil(t, got, "a rejected command line reached its command")
		})
	}
}

// TestCheckIgnoresAReceiverType states that check, which reads the
// receiver from the generated routes file, still accepts the --receiver-type
// v0.20.0 accepted, and says it is ignored rather than pointing at a flag
// check does not have.
func TestCheckIgnoresAReceiverType(t *testing.T) {
	var stderr strings.Builder
	var got any
	err := commands("/work", []string{"check", "--receiver-type=T"}, func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, io.Discard, &stderr, runners{
		check: func(_ *cobra.Command, _ string, c analysis.CheckConfiguration) error {
			got = c
			return nil
		},
	})
	require.NoError(t, err, "muxt check --receiver-type=T")
	assert.Equal(t, analysis.CheckConfiguration{TemplatesVariables: []string{"templates"}}, got, "muxt check --receiver-type=T")
	assert.Equal(t, "Flag --receiver-type has been deprecated, muxt check reads the receiver type from the generated routes file and ignores this flag\n", stderr.String(), "stderr")
}

// TestCommandLineRejectionsDoNotPrintUsage states that a command line the
// flags parse but validation rejects prints only its error: the usage text
// would bury it and says nothing about what was wrong. A flag that does not
// parse still gets the usage, as cobra prints it.
func TestCommandLineRejectionsDoNotPrintUsage(t *testing.T) {
	output := func(t *testing.T, commandLine string) string {
		t.Helper()
		var out strings.Builder
		err := commands("/work", strings.Fields(commandLine), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, &out, &out, runners{})
		require.Error(t, err, "muxt %s", commandLine)
		return out.String()
	}
	for _, commandLine := range []string{
		"generate --output-routes-func=1x",
		"check --use-templates-variable=not-ok",
		"list-template-callers --match=(",
		"list-template-calls --match=(",
		"test-template-mutations --run=(",
		"test-template-mutations --use-templates-variable=a --use-templates-variable=a",
		"list-template-callers --use-templates-variable=a --use-templates-variable=a",
		"list-template-calls --use-templates-variable=a --use-templates-variable=a",
		"--templates-variable=a --use-templates-variable=b",
	} {
		t.Run(commandLine, func(t *testing.T) {
			assert.NotContains(t, output(t, commandLine), "Usage:", "muxt %s printed the usage", commandLine)
		})
	}
	t.Run("an unknown flag", func(t *testing.T) {
		assert.Contains(t, output(t, "generate --no-such-flag"), "Usage:", "muxt generate --no-such-flag want the usage")
	})
}
