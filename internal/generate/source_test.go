package generate

import (
	"go/types"
	"html/template"
	"log"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/muxt"
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
// it names the receiver, as --use-receiver-type would.
func testSource(t *testing.T, goSource, receiverType, templates string) muxt.Source {
	t.Helper()
	pkg := typestest.MustCheck(t, "example.com/server", goSource)
	src := muxt.Source{
		Package: muxt.Package{Fset: typestest.FileSet, Types: pkg, Lookup: typestest.Lookup},
		Templates: []muxt.Templates{{
			Variable: "templates",
			Set:      template.Must(template.New("templates").Parse(templates)),
		}},
	}
	if receiverType != "" {
		obj := pkg.Scope().Lookup(receiverType)
		if obj == nil {
			t.Fatalf("source declares no %s", receiverType)
		}
		src.Receiver = obj.Type().(*types.Named)
	}
	return src
}

// generateOne runs generation into a single file and returns its
// content, with what generation logged.
func generateOne(t *testing.T, config RoutesFileConfiguration, src muxt.Source) (string, string) {
	t.Helper()
	var logs strings.Builder
	files, err := TemplateRoutesFiles(t.TempDir(), config, src, log.New(&logs, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("generated %d files, want 1", len(files))
	}
	return files[0].Content, logs.String()
}

// function returns the declaration of the function or method named name
// in generated source, from its signature to its closing brace.
func function(t *testing.T, source, signaturePrefix string) string {
	t.Helper()
	start := strings.Index(source, signaturePrefix)
	if start < 0 {
		t.Fatalf("generated source has no %q:\n%s", signaturePrefix, source)
	}
	end := strings.Index(source[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("declaration %q does not end", signaturePrefix)
	}
	return source[start : start+end+len("\n}")]
}
