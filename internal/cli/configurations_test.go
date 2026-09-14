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
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "template_routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
			},
		},
		{
			name: "the generated names",
			args: "generate --use-receiver-type=Server --output-file=routes.go --output-routes-func=Routes --output-receiver-interface=Handlers --output-template-data-type=Data --output-template-route-paths-type=Paths",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                      "v1.2.3",
				PackageName:                      "main",
				RoutesFunction:                   "Routes",
				ReceiverType:                     "Server",
				ReceiverInterface:                "Handlers",
				TemplateDataType:                 "Data",
				SSETemplateDataType:              "SSETemplateData",
				TemplateRoutePathsTypeName:       "Paths",
				TemplatesVariables:               []string{"templates"},
				OutputFileName:                   "routes.go",
				OutputExportedDefaultIdentifiers: true,
				OutputMuxtVersion:                true,
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
				MuxtVersion:                "v1.2.3",
				PackageName:                "main",
				RoutesFunction:             "templateRoutes",
				ReceiverInterface:          "routesReceiver",
				TemplateDataType:           "templateData",
				SSETemplateDataType:        "sseTemplateData",
				TemplateRoutePathsTypeName: "templateRoutePaths",
				TemplatesVariables:         []string{"templates"},
				OutputFileName:             "template_routes.go",
				OutputMuxtVersion:          true,
			},
		},
		{
			name: "unexported default identifiers keep every explicit name",
			args: "generate --output-exported-default-identifiers=false --output-receiver-interface=Handlers --output-template-data-type=Data --output-sse-template-data-type=SSEData --output-template-route-paths-type=Paths",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                "v1.2.3",
				PackageName:                "main",
				RoutesFunction:             "templateRoutes",
				ReceiverInterface:          "Handlers",
				TemplateDataType:           "Data",
				SSETemplateDataType:        "SSEData",
				TemplateRoutePathsTypeName: "Paths",
				TemplatesVariables:         []string{"templates"},
				OutputFileName:             "template_routes.go",
				OutputMuxtVersion:          true,
			},
		},
		{
			name: "unexported default identifiers keep an explicit name",
			args: "generate --output-exported-default-identifiers=false --output-routes-func=Routes",
			want: generate.RoutesFileConfiguration{
				MuxtVersion:                "v1.2.3",
				PackageName:                "main",
				RoutesFunction:             "Routes",
				ReceiverInterface:          "routesReceiver",
				TemplateDataType:           "templateData",
				SSETemplateDataType:        "sseTemplateData",
				TemplateRoutePathsTypeName: "templateRoutePaths",
				TemplatesVariables:         []string{"templates"},
				OutputFileName:             "template_routes.go",
				OutputMuxtVersion:          true,
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
			name: "mutations with packages, patterns and go test flags",
			args: "test-template-mutations --template-pattern=^page --run=TestPage --include-test-callers --max-cases=2 --workers=4 --diff=main ./... -- -count=1",
			want: mutation.Configuration{TemplatesVariables: []string{"templates"}, TemplatePattern: regexp.MustCompile("^page"), Run: regexp.MustCompile("TestPage"), Packages: []string{"./..."}, GoTestArgs: []string{"-count=1"}, IncludeTests: true, MaxCases: 2, Workers: 4, Diff: "main"},
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
// one it was started in, joined with -C when -C is relative.
func TestChangeDirectory(t *testing.T) {
	for _, tt := range []struct {
		args, want string
	}{
		{args: "generate", want: "/work"},
		{args: "-C sub generate", want: "/work/sub"},
		{args: "-C ../other check", want: "/other"},
		{args: "-C /abs check", want: "/abs"},
	} {
		t.Run(tt.args, func(t *testing.T) {
			var got string
			record := func(wd string) error {
				got = wd
				return nil
			}
			err := commands("/work", strings.Fields(tt.args), func(string) string { return "" }, func() (string, bool) { return "v1.2.3", true }, io.Discard, io.Discard, runners{
				check:    func(_ *cobra.Command, wd string, _ analysis.CheckConfiguration) error { return record(wd) },
				generate: func(_ *cobra.Command, wd string, _ generate.RoutesFileConfiguration) error { return record(wd) },
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
		{name: "a multipart limit of zero", args: "generate --output-multipart-max-memory=0", wantErr: `invalid argument "0" for "--output-multipart-max-memory" flag: multipart max memory must be positive, got "0"`},
		{name: "a repeated templates variable", args: "generate --use-templates-variable=pages --use-templates-variable=pages", wantErr: "duplicate template variable: pages"},
		{name: "the deprecated and new templates variable flags together", args: "check --templates-variable=a --use-templates-variable=b", wantErr: "deprecated flag templates-variable not permitted along with use-templates-variable"},
		{name: "a check templates variable that is not an identifier", args: "check --use-templates-variable=not-ok", wantErr: "variable not-ok value must be a well-formed Go identifier"},
		{name: "a template pattern that does not compile", args: "test-template-mutations --template-pattern=(", wantErr: "--template-pattern: error parsing regexp: missing closing ): `(`"},
		{name: "a run pattern that does not compile", args: "test-template-mutations --run=(", wantErr: "--run: error parsing regexp: missing closing ): `(`"},
		{name: "a callers match that does not compile", args: "list-template-callers --match=(", wantErr: "error parsing regexp: missing closing ): `(`"},
		{name: "a go test flag muxt needs for itself", args: "test-template-mutations -- -overlay=other.json", wantErr: "go test flag -overlay cannot be passed through: muxt uses -overlay to deliver each mutant"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := configurationOf(t, tt.args, nil)
			require.EqualError(t, err, tt.wantErr)
			assert.Nil(t, got, "a rejected command line reached its command")
		})
	}
}
