package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/generate"
)

func TestValidateGenerateConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name    string
		config  generate.RoutesFileConfiguration
		wantErr string
	}{
		{name: "empty", config: generate.RoutesFileConfiguration{}},
		{name: "valid identifiers", config: generate.RoutesFileConfiguration{
			TemplatesVariables: []string{"templates", "pages"}, RoutesFunction: "Routes", ReceiverType: "T",
			ReceiverInterface: "I", TemplateDataType: "D", SSETemplateDataType: "S", TemplateRoutePathsTypeName: "P",
			TemplateRouteTypeName: "Q", OutputFileName: "x.go",
		}},
		{name: "empty templates variable is left to fixTemplateVariables", config: generate.RoutesFileConfiguration{TemplatesVariables: []string{""}}},
		{name: "templates variable", config: generate.RoutesFileConfiguration{TemplatesVariables: []string{"ok", "not-ok"}}, wantErr: "variable not-ok value must be a well-formed Go identifier"},
		{name: "routes function", config: generate.RoutesFileConfiguration{RoutesFunction: "1x"}, wantErr: "output-routes-func value must be a well-formed Go identifier"},
		{name: "receiver type", config: generate.RoutesFileConfiguration{ReceiverType: "a.b"}, wantErr: "use-receiver-type value must be a well-formed Go identifier"},
		{name: "receiver interface", config: generate.RoutesFileConfiguration{ReceiverInterface: "a b"}, wantErr: "output-receiver-interface value must be a well-formed Go identifier"},
		{name: "template data type", config: generate.RoutesFileConfiguration{TemplateDataType: "a-b"}, wantErr: "output-template-data-type value must be a well-formed Go identifier"},
		{name: "sse template data type", config: generate.RoutesFileConfiguration{SSETemplateDataType: "a-b"}, wantErr: "output-sse-template-data-type value must be a well-formed Go identifier"},
		{name: "route paths type", config: generate.RoutesFileConfiguration{TemplateRoutePathsTypeName: "a-b"}, wantErr: "output-template-route-paths-type value must be a well-formed Go identifier"},
		{name: "route type", config: generate.RoutesFileConfiguration{TemplateRouteTypeName: "a-b"}, wantErr: "output-template-route-type value must be a well-formed Go identifier"},
		{name: "keyword is not an identifier", config: generate.RoutesFileConfiguration{ReceiverType: "func"}, wantErr: "use-receiver-type value must be a well-formed Go identifier"},
		{name: "first failure wins", config: generate.RoutesFileConfiguration{RoutesFunction: "1x", TemplateDataType: "2y"}, wantErr: "output-routes-func value must be a well-formed Go identifier"},
		{name: "both frontends", config: generate.RoutesFileConfiguration{OutputHTMX: true, OutputDatastar: true}, wantErr: "--output-htmx and --output-datastar are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)"},
		{name: "output file extension", config: generate.RoutesFileConfiguration{OutputFileName: "x.txt"}, wantErr: "output filename must use .go extension"},
		{name: "output file in the working directory", config: generate.RoutesFileConfiguration{OutputFileName: "./x.go"}},
		{name: "output file in a subdirectory", config: generate.RoutesFileConfiguration{OutputFileName: "sub/x.go"}, wantErr: "--output-file must be a file name in the working directory: sub/x.go"},
		{name: "output file in a parent directory", config: generate.RoutesFileConfiguration{OutputFileName: "../x.go"}, wantErr: "--output-file must be a file name in the working directory: ../x.go"},
		{name: "output file with no stem", config: generate.RoutesFileConfiguration{OutputFileName: ".go"}, wantErr: "--output-file needs a file name before the .go extension"},
		// The generated header records the command line as words joined by
		// spaces, so a value holding whitespace would read back as two.
		{name: "output file with a space", config: generate.RoutesFileConfiguration{OutputFileName: "my routes.go"}, wantErr: `--output-file must not contain whitespace: "my routes.go"`},
		{name: "receiver package with a space", config: generate.RoutesFileConfiguration{ReceiverPackage: "example.com/a b"}, wantErr: `--use-receiver-type-package must not contain whitespace: "example.com/a b"`},
		{name: "a static assets directory with a space", config: generate.RoutesFileConfiguration{StaticAssetsDirectory: "a b", StaticAssetsPath: "/static"}, wantErr: `--input-static-assets-directory must not contain whitespace: "a b"`},
		{name: "a static assets path with a space", config: generate.RoutesFileConfiguration{StaticAssetsDirectory: "assets", StaticAssetsPath: "/a b"}, wantErr: `--output-static-assets-path must not contain whitespace: "/a b"`},
		{name: "a static assets directory with a trailing slash", config: generate.RoutesFileConfiguration{StaticAssetsDirectory: "assets/", StaticAssetsPath: "/static/"}},
		{name: "a static assets directory with dots inside an element", config: generate.RoutesFileConfiguration{StaticAssetsDirectory: "assets..v2", StaticAssetsPath: "/static"}},
		{name: "a static assets directory with a parent element", config: generate.RoutesFileConfiguration{StaticAssetsDirectory: "a/../b", StaticAssetsPath: "/static"}, wantErr: `--input-static-assets-directory must be a slash-separated path inside the package directory: "a/../b"`},
		{name: "a static assets directory naming the package", config: generate.RoutesFileConfiguration{StaticAssetsDirectory: ".", StaticAssetsPath: "/static"}, wantErr: `--input-static-assets-directory must be a slash-separated path inside the package directory: "."`},
		{name: "two generated names alike", config: generate.RoutesFileConfiguration{RoutesFunction: "X", TemplateRouteBuilderTypeName: "X"}, wantErr: "--output-routes-func and --output-template-route-builder-type are both X; each generated type needs its own name"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGenerateConfiguration(tt.config, fromCommandLine)
			if tt.wantErr == "" {
				require.NoError(t, err, "validateGenerateConfiguration()")
				return
			}
			require.EqualError(t, err, tt.wantErr, "validateGenerateConfiguration()")
		})
	}
}

// TestSettingSourceNamesSettings states that a rule error names a setting the
// way its source spells it, and that RuleError joins the settings it is about.
func TestSettingSourceNamesSettings(t *testing.T) {
	for _, tt := range []struct {
		from           settingSource
		wantFlag, bare string
	}{
		{fromCommandLine, "--output-file", "output-file"},
		{fromMuxtJSON, "output_file", "output_file"},
	} {
		assert.Equal(t, tt.wantFlag, tt.from.flag(outputFile), "flag(%q)", outputFile)
		assert.Equal(t, tt.bare, tt.from.bare(outputFile), "bare(%q)", outputFile)
	}
	assert.EqualError(t, &RuleError{Rule: "output filename must use .go extension"}, "output filename must use .go extension")
	assert.EqualError(t, &RuleError{Settings: []string{"a", "b"}, Rule: "are mutually exclusive"}, "a and b are mutually exclusive")
}

// TestCheckNonEmptyReportsCurrentNamesFirst states that a line giving an empty
// deprecated spelling and an empty current name reports the current name,
// whatever the list order.
func TestCheckNonEmptyReportsCurrentNamesFirst(t *testing.T) {
	var config generate.RoutesFileConfiguration
	settings := generateSettings(&config)
	got := givenSettings{
		outputRoutesFunc:       {deprecatedRoutesFunc},
		outputTemplateDataType: {outputTemplateDataType},
	}
	err := checkNonEmpty(settings, got, fromCommandLine)
	assert.EqualError(t, err, "--output-template-data-type value must not be empty")
}
