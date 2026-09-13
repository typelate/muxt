package generate

import (
	"go/types"
	"html/template"
	"testing"

	"github.com/typelate/muxt/internal/source"
	"github.com/typelate/muxt/internal/typestest"
)

// This file builds what generation reads without loading a module: a
// package type checked in memory against the stub standard library,
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
// it.
func testSource(t *testing.T, goSource, receiverType, templates string) (source.Package, *types.Named) {
	t.Helper()
	pkg := typestest.MustCheck(t, "example.com/server", goSource)
	src := source.Package{
		Fset:    typestest.FileSet,
		Types:   pkg,
		Imports: typestest.Packages(),
		Variables: []source.Variable{{
			Name: "templates",
			Set:  template.Must(template.New("templates").Parse(templates)),
		}},
	}
	if receiverType == "" {
		return src, nil
	}
	obj := pkg.Scope().Lookup(receiverType)
	if obj == nil {
		t.Fatalf("source declares no %s", receiverType)
	}
	return src, obj.Type().(*types.Named)
}
