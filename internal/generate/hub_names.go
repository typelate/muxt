package generate

import (
	"fmt"
	"slices"
	"strings"
)

// The muxt.json route_paths members that name the declarations of the hub file.
const (
	memberOutputPathsType                     = "output_paths_type"
	memberOutputRouteBuilderType              = "output_route_builder_type"
	memberOutputRouteType                     = "output_route_type"
	memberOutputPackagePathsTypeSuffix        = "output_package_paths_type_suffix"
	memberOutputPackageRouteBuilderTypeSuffix = "output_package_route_builder_type_suffix"
)

// HubPackage is a package whose types the hub file declares.
type HubPackage struct {
	ImportPath string
	Namespace  string
}

// HubNameSource is what makes a declaration of the hub file get its name.
type HubNameSource struct {
	Member     string // the route_paths member in muxt.json
	ImportPath string // of the package, for a type that each package has
}

func (s HubNameSource) String() string {
	if s.ImportPath == "" {
		return s.Member
	}
	return fmt.Sprintf("%s for package %q", s.Member, s.ImportPath)
}

// SameHubNameError is two declarations of the hub file that would get one name.
type SameHubNameError struct {
	Name    string
	Sources [2]HubNameSource
}

func (e *SameHubNameError) Error() string {
	return fmt.Sprintf("route paths name %q is made by both %s and %s; change one of them",
		e.Name, e.Sources[0], e.Sources[1])
}

// CheckHubTypeNames reports the first two package-level declarations of the
// hub file that would get one name. The hub's own names come first, then the
// packages by import path.
func CheckHubTypeNames(names HubTypeNames, packages []HubPackage) error {
	type declaration struct {
		name   string
		source HubNameSource
	}
	declarations := []declaration{
		{names.PathsType(), HubNameSource{Member: memberOutputPathsType}},
		{names.RouteBuilderType(), HubNameSource{Member: memberOutputRouteBuilderType}},
		{names.RouteType(), HubNameSource{Member: memberOutputRouteType}},
		{hubConstructorName(names.PathsType()), HubNameSource{Member: memberOutputPathsType}},
		{hubConstructorName(names.RouteBuilderType()), HubNameSource{Member: memberOutputRouteBuilderType}},
	}
	for _, p := range slices.SortedFunc(slices.Values(packages), func(a, b HubPackage) int {
		return strings.Compare(a.ImportPath, b.ImportPath)
	}) {
		declarations = append(declarations,
			declaration{names.PackagePathsType(p.Namespace), HubNameSource{Member: memberOutputPackagePathsTypeSuffix, ImportPath: p.ImportPath}},
			declaration{names.PackageRouteBuilderType(p.Namespace), HubNameSource{Member: memberOutputPackageRouteBuilderTypeSuffix, ImportPath: p.ImportPath}},
		)
	}
	seen := make(map[string]HubNameSource, len(declarations))
	for _, d := range declarations {
		if first, ok := seen[d.name]; ok {
			return &SameHubNameError{Name: d.name, Sources: [2]HubNameSource{first, d.source}}
		}
		seen[d.name] = d.source
	}
	return nil
}
