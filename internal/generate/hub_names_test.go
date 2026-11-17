package generate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/generate"
)

func TestCheckHubTypeNames(t *testing.T) {
	admin := generate.HubPackage{ImportPath: "example.com/m/admin", Namespace: "Admin"}
	blog := generate.HubPackage{ImportPath: "example.com/m/blog", Namespace: "Blog"}
	for _, tc := range []struct {
		name     string
		names    generate.HubTypeNames
		packages []generate.HubPackage
		want     *generate.SameHubNameError // nil means no clash
	}{
		{
			name:     "paths type equals a package paths type",
			names:    generate.HubTypeNames{Paths: "AdminPaths", PackagePathsSuffix: "Paths"},
			packages: []generate.HubPackage{blog, admin},
			want: &generate.SameHubNameError{Name: "AdminPaths", Sources: [2]generate.HubNameSource{
				{Member: "output_paths_type"},
				{Member: "output_package_paths_type_suffix", ImportPath: "example.com/m/admin"},
			}},
		},
		{
			name:     "both suffixes equal",
			names:    generate.HubTypeNames{PackagePathsSuffix: "Helpers", PackageRouteBuilderSuffix: "Helpers"},
			packages: []generate.HubPackage{blog},
			want: &generate.SameHubNameError{Name: "BlogHelpers", Sources: [2]generate.HubNameSource{
				{Member: "output_package_paths_type_suffix", ImportPath: "example.com/m/blog"},
				{Member: "output_package_route_builder_type_suffix", ImportPath: "example.com/m/blog"},
			}},
		},
		{
			name:  "paths type equals the default route builder type",
			names: generate.HubTypeNames{Paths: "TemplateRouteBuilder"},
			want: &generate.SameHubNameError{Name: "TemplateRouteBuilder", Sources: [2]generate.HubNameSource{
				{Member: "output_paths_type"},
				{Member: "output_route_builder_type"},
			}},
		},
		{
			name:  "route type equals the paths constructor",
			names: generate.HubTypeNames{Paths: "Paths", Route: "NewPaths"},
			want: &generate.SameHubNameError{Name: "NewPaths", Sources: [2]generate.HubNameSource{
				{Member: "output_route_type"},
				{Member: "output_paths_type"},
			}},
		},
		{
			name:     "defaults",
			packages: []generate.HubPackage{admin, blog},
		},
		{
			name: "all set apart",
			names: generate.HubTypeNames{
				Paths: "Paths", RouteBuilder: "Routes", Route: "Route",
				PackagePathsSuffix: "Paths", PackageRouteBuilderSuffix: "Routes",
			},
			packages: []generate.HubPackage{admin, blog},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := generate.CheckHubTypeNames(tc.names, tc.packages)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			var clash *generate.SameHubNameError
			require.ErrorAs(t, err, &clash)
			assert.Equal(t, tc.want, clash)
		})
	}
}
