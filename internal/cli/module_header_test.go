package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/typelate/muxt/internal/analysis"
	"github.com/typelate/muxt/internal/generate"
)

func TestParseModuleHeader(t *testing.T) {
	for _, tt := range []struct {
		name   string
		args   []string
		want   analysis.PackageConfig
		wantOK bool
	}{
		{
			name: "defaults", wantOK: true,
			want: analysis.PackageConfig{
				RoutesFunction:         generate.DefaultRoutesFunctionName,
				ReceiverInterface:      generate.DefaultReceiverInterfaceName,
				TemplateRoutePathsType: generate.DefaultTemplateRoutePathsTypeName,
			},
		},
		{
			name: "flags", wantOK: true,
			args: []string{"--use-receiver-type=Server", "--output-routes-func=Routes", "--output-htmx", "--output-routes-func-with-logger-param", "--output-routes-func-with-path-prefix-param", "--output-routes-func-with-middleware-param"},
			want: analysis.PackageConfig{
				RoutesFunction:         "Routes",
				ReceiverInterface:      generate.DefaultReceiverInterfaceName,
				ReceiverType:           "Server",
				TemplateRoutePathsType: generate.DefaultTemplateRoutePathsTypeName,
				OutputHTMX:             true,
				Logger:                 true,
				PathPrefix:             true,
				Middleware:             true,
			},
		},
		{
			name: "empty value takes the default", wantOK: true,
			args: []string{"--output-routes-func="},
			want: analysis.PackageConfig{
				RoutesFunction:         generate.DefaultRoutesFunctionName,
				ReceiverInterface:      generate.DefaultReceiverInterfaceName,
				TemplateRoutePathsType: generate.DefaultTemplateRoutePathsTypeName,
			},
		},
		{name: "unknown flag", args: []string{"--no-such-flag"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseModuleHeader(tt.args)
			assert.Equal(t, tt.wantOK, ok, "parseModuleHeader(%q) ok", tt.args)
			assert.Equal(t, tt.want, got, "parseModuleHeader(%q)", tt.args)
		})
	}
}
