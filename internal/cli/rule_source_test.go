package cli

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/composite"
)

// TestEntryRuleErrorsNameTheKey states that an entry from muxt.json that breaks
// a rule gets an error naming the key as the file spells it, where a command
// line names the flag. The texts that name neither stay as they are.
func TestEntryRuleErrorsNameTheKey(t *testing.T) {
	const mutuallyExclusive = "are mutually exclusive; a package targets one frontend library (to mix frontends, generate separate packages that share a mux)"
	for _, tt := range []struct {
		name, entry, want string
	}{
		{name: "templates variable empty", entry: `{"use_templates_variable": [""]}`, want: "use_templates_variable value must not be empty"},
		{name: "output file empty", entry: `{"output_file": ""}`, want: "output_file value must not be empty"},
		{name: "receiver interface empty", entry: `{"output_receiver_interface": ""}`, want: "output_receiver_interface value must not be empty"},
		{name: "routes function empty", entry: `{"output_routes_func": ""}`, want: "output_routes_func value must not be empty"},
		{name: "template data type empty", entry: `{"output_template_data_type": ""}`, want: "output_template_data_type value must not be empty"},
		{name: "sse template data type empty", entry: `{"output_sse_template_data_type": ""}`, want: "output_sse_template_data_type value must not be empty"},
		{name: "route paths type empty", entry: `{"output_template_route_paths_type": ""}`, want: "output_template_route_paths_type value must not be empty"},
		{name: "route type empty", entry: `{"output_template_route_type": ""}`, want: "output_template_route_type value must not be empty"},
		{name: "route builder type empty", entry: `{"output_template_route_builder_type": ""}`, want: "output_template_route_builder_type value must not be empty"},

		{name: "routes function identifier", entry: `{"output_routes_func": "1x"}`, want: "output_routes_func value must be a well-formed Go identifier"},
		{name: "receiver type identifier", entry: `{"use_receiver_type": "a.b"}`, want: "use_receiver_type value must be a well-formed Go identifier"},
		{name: "receiver interface identifier", entry: `{"output_receiver_interface": "a-b"}`, want: "output_receiver_interface value must be a well-formed Go identifier"},
		{name: "template data type identifier", entry: `{"output_template_data_type": "a-b"}`, want: "output_template_data_type value must be a well-formed Go identifier"},
		{name: "sse template data type identifier", entry: `{"output_sse_template_data_type": "a-b"}`, want: "output_sse_template_data_type value must be a well-formed Go identifier"},
		{name: "route paths type identifier", entry: `{"output_template_route_paths_type": "a-b"}`, want: "output_template_route_paths_type value must be a well-formed Go identifier"},
		{name: "route type identifier", entry: `{"output_template_route_type": "a-b"}`, want: "output_template_route_type value must be a well-formed Go identifier"},
		{name: "route builder type identifier", entry: `{"output_template_route_builder_type": "a-b"}`, want: "output_template_route_builder_type value must be a well-formed Go identifier"},

		{name: "both frontends", entry: `{"output_htmx": true, "output_datastar": true}`, want: "output_htmx and output_datastar " + mutuallyExclusive},

		{name: "static assets given apart", entry: `{"input_static_assets_directory": "assets"}`, want: "input_static_assets_directory and output_static_assets_path must be given together"},
		{name: "static assets path without a slash", entry: `{"input_static_assets_directory": "assets", "output_static_assets_path": "static"}`, want: `output_static_assets_path must start with "/": "static"`},
		{name: "static assets path at the root", entry: `{"input_static_assets_directory": "assets", "output_static_assets_path": "/"}`, want: `output_static_assets_path must name a path below "/": "/"`},
		{name: "static assets path with a wildcard", entry: `{"input_static_assets_directory": "assets", "output_static_assets_path": "/{x}"}`, want: `output_static_assets_path must be a literal path without wildcards or empty segments: "/{x}"`},
		{name: "static assets directory outside the package", entry: `{"input_static_assets_directory": "../a", "output_static_assets_path": "/static"}`, want: `input_static_assets_directory must be a slash-separated path inside the package directory: "../a"`},
		{name: "static assets directory with whitespace", entry: `{"input_static_assets_directory": "a b", "output_static_assets_path": "/static"}`, want: `input_static_assets_directory must not contain whitespace: "a b"`},
		{name: "static assets path with whitespace", entry: `{"input_static_assets_directory": "assets", "output_static_assets_path": "/a b"}`, want: `output_static_assets_path must not contain whitespace: "/a b"`},

		{name: "two generated names alike", entry: `{"output_routes_func": "X", "output_template_route_builder_type": "X"}`, want: "output_routes_func and output_template_route_builder_type are both X; each generated type needs its own name"},

		{name: "output file in a subdirectory", entry: `{"output_file": "sub/x.go"}`, want: "output_file must be a file name in the working directory: sub/x.go"},
		{name: "output file with no stem", entry: `{"output_file": ".go"}`, want: "output_file needs a file name before the .go extension"},
		{name: "output file with whitespace", entry: `{"output_file": "my routes.go"}`, want: `output_file must not contain whitespace: "my routes.go"`},
		{name: "receiver package with whitespace", entry: `{"use_receiver_type_package": "example.com/a b"}`, want: `use_receiver_type_package must not contain whitespace: "example.com/a b"`},

		{name: "output file extension names no key", entry: `{"output_file": "x.txt"}`, want: "output filename must use .go extension"},
		{name: "duplicate variable names no key", entry: `{"use_templates_variable": ["a", "a"]}`, want: "duplicate template variable: a"},
		{name: "variable identifier names no key", entry: `{"use_templates_variable": ["a-b"]}`, want: "variable a-b value must be a well-formed Go identifier"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var entry composite.Entry
			require.NoError(t, json.Unmarshal([]byte(tt.entry), &entry), "entry")

			_, err := entryConfiguration(entry, fromMuxtJSON, generateEnv{Dir: t.TempDir(), GetEnv: func(string) string { return "" }, Version: noVersion})

			require.EqualError(t, err, tt.want, "entryConfiguration() error")
		})
	}
}
