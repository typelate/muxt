package cli

import (
	"testing"

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
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGenerateConfiguration(tt.config)
			if tt.wantErr == "" {
				require.NoError(t, err, "validateGenerateConfiguration()")
				return
			}
			require.EqualError(t, err, tt.wantErr, "validateGenerateConfiguration()")
		})
	}
}
