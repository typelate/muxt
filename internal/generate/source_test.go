package generate

import (
	"go/types"
	"html/template"
	"testing"

	"github.com/typelate/muxt/internal/muxt/muxttest"
	"github.com/typelate/muxt/internal/source"
)

// This file builds what generation reads without loading a module: a
// package type checked in memory, with no standard library,
// and templates parsed from strings.

// testConfig is the configuration muxt generate runs with when no flags
// are passed.
func testConfig() RoutesFileConfiguration {
	return RoutesFileConfiguration{
		PackageName:                      "main",
		OutputFileName:                   "template_routes.go",
		RoutesFunction:                   DefaultRoutesFunctionName,
		ReceiverInterface:                DefaultReceiverInterfaceName,
		TemplateDataType:                 "TemplateData",
		SSETemplateDataType:              "SSETemplateData",
		TemplateRoutePathsTypeName:       DefaultTemplateRoutePathsTypeName,
		TemplatesVariables:               []string{"templates"},
		OutputExportedDefaultIdentifiers: true,
	}
}

// testSource type checks goSource as example.com/server and parses
// templates into the templates variable. When receiverType is not empty
// it returns that type as the receiver, as --use-receiver-type would name
// it. The source imports nothing; resolving routes over it takes a
// a muxttest checker.
func testSource(t *testing.T, goSource, receiverType, templates string) (source.Package, *types.Named) {
	t.Helper()
	pkg := muxttest.Check(t, "example.com/server", map[string]string{"server.go": goSource})
	src := source.Package{
		Fset:  muxttest.FileSet,
		Types: pkg,
		Variables: []source.Variable{{
			Name: "templates",
			Set:  template.Must(template.New("templates").Parse(templates)),
		}},
	}
	if receiverType == "" {
		return src, nil
	}
	return src, muxttest.Lookup(t, pkg, receiverType).(*types.Named)
}
