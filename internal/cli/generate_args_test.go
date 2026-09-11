package cli

import (
	"io"
	"reflect"
	"testing"

	"github.com/spf13/pflag"

	"github.com/typelate/muxt/internal/generate"
)

// The header comment muxt writes into a generated file records the flags
// that produced it, and the next run re-parses that comment to decide
// which generated files it owns and may delete. These tests hold the two
// halves of that round trip together: a flag recorded by one and not read
// by the other silently changes what a later run deletes.

// parseGeneratedHeader reads the arguments a header records, the way
// generateCommand does when it looks for files it may replace.
func parseGeneratedHeader(t *testing.T, args []string) generate.RoutesFileConfiguration {
	t.Helper()
	var (
		config     generate.RoutesFileConfiguration
		deprecated string
	)
	set := pflag.NewFlagSet("header", pflag.ContinueOnError)
	set.SetOutput(io.Discard)
	addGenerateFlags(set, &config, &deprecated)
	if err := set.Parse(args); err != nil {
		t.Fatalf("parsing the header %q: %v", args, err)
	}
	return config
}

// recorded drops what the header does not carry: where the file is being
// written, the version that wrote it, and the flags that only affect one
// run's console output.
func recorded(c generate.RoutesFileConfiguration) generate.RoutesFileConfiguration {
	c.PackageName, c.PackagePath, c.MuxtVersion = "", "", ""
	c.Verbose = false
	c.SilenceHTTPResponseWarning = false
	return c
}

// defaultsConfig is what the flags give a run that passes none of them.
func defaultsConfig() generate.RoutesFileConfiguration {
	return generate.RoutesFileConfiguration{
		TemplatesVariables:               []string{defaultTemplatesVariableName},
		OutputFileName:                   defaultOutputFileName,
		ReceiverInterface:                defaultReceiverInterfaceName,
		RoutesFunction:                   defaultRoutesFunctionName,
		TemplateDataType:                 defaultTemplateDataTypeName,
		SSETemplateDataType:              defaultSSETemplateDataTypeName,
		TemplateRoutePathsTypeName:       defaultTemplateRoutePathsTypeName,
		OutputExportedDefaultIdentifiers: true,
		OutputMuxtVersion:                true,
	}
}

// TestGeneratedHeaderRoundTrip states that what a run records is what a
// later run reads back.
func TestGeneratedHeaderRoundTrip(t *testing.T) {
	// Every flag the header records, each set to something the defaults
	// are not. A flag added to the parser but not to the header fails this
	// test as soon as it is set here, so keep this configuration complete.
	everything := generate.RoutesFileConfiguration{
		TemplatesVariables:               []string{"templates", "pages"},
		ReceiverType:                     "Server",
		ReceiverPackage:                  "example.com/server",
		OutputFileName:                   "routes_gen.go",
		ReceiverInterface:                "MyReceiver",
		RoutesFunction:                   "MyRoutes",
		TemplateDataType:                 "MyData",
		SSETemplateDataType:              "MySSEData",
		TemplateRoutePathsTypeName:       "MyPaths",
		Logger:                           true,
		PathPrefix:                       true,
		Middleware:                       true,
		OutputMultipleFiles:              true,
		OutputHTMX:                       true,
		OutputExportedDefaultIdentifiers: false,
		OutputMuxtVersion:                false,
		MultipartMaxMemory:               64 << 20,
	}

	// A configuration nobody filled in reads back as itself. Its empty
	// strings differ from the flag defaults, so they are recorded as
	// empty and come back empty; only the templates variable, which is
	// recorded as a list or not at all, falls back to its default.
	zero := generate.RoutesFileConfiguration{TemplatesVariables: []string{defaultTemplatesVariableName}}

	for _, tt := range []struct {
		name   string
		config generate.RoutesFileConfiguration
		want   generate.RoutesFileConfiguration
	}{
		{name: "every flag the header records", config: everything, want: everything},
		{name: "the defaults", config: defaultsConfig(), want: defaultsConfig()},
		{name: "a configuration nobody filled in", config: generate.RoutesFileConfiguration{}, want: zero},
		{
			name:   "datastar rather than htmx",
			config: withDatastar(defaultsConfig()),
			want:   withDatastar(defaultsConfig()),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGeneratedHeader(t, configToArgs(tt.config))
			if !reflect.DeepEqual(recorded(got), recorded(tt.want)) {
				t.Errorf("the header read back as\n%+v\nwant\n%+v", recorded(got), recorded(tt.want))
			}
		})
	}
}

func withDatastar(c generate.RoutesFileConfiguration) generate.RoutesFileConfiguration {
	c.OutputDatastar = true
	return c
}

// TestConfigToArgsRecordsWhatDiffersFromTheDefaults states that the header
// stays as short as the run was ordinary: a flag left at its default is
// not written into it, and one that was passed is.
func TestConfigToArgsRecordsWhatDiffersFromTheDefaults(t *testing.T) {
	if got := configToArgs(defaultsConfig()); len(got) != 0 {
		t.Errorf("configToArgs(defaults) = %q, want nothing recorded", got)
	}

	for _, tt := range []struct {
		name   string
		change func(*generate.RoutesFileConfiguration)
		want   []string
	}{
		{
			name:   "a routes function of its own",
			change: func(c *generate.RoutesFileConfiguration) { c.RoutesFunction = "MyRoutes" },
			want:   []string{"--output-routes-func=MyRoutes"},
		},
		{
			name:   "a receiver",
			change: func(c *generate.RoutesFileConfiguration) { c.ReceiverType = "Server" },
			want:   []string{"--use-receiver-type=Server"},
		},
		{
			name:   "a logger parameter",
			change: func(c *generate.RoutesFileConfiguration) { c.Logger = true },
			want:   []string{"--output-routes-func-with-logger-param"},
		},
		{
			name:   "the version left out",
			change: func(c *generate.RoutesFileConfiguration) { c.OutputMuxtVersion = false },
			want:   []string{"--output-muxt-version=false"},
		},
		{
			name:   "a multipart limit",
			change: func(c *generate.RoutesFileConfiguration) { c.MultipartMaxMemory = 64 << 20 },
			want:   []string{"--output-multipart-max-memory=67108864"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := defaultsConfig()
			tt.change(&config)
			if got := configToArgs(config); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("configToArgs = %q, want %q", got, tt.want)
			}
		})
	}
}
