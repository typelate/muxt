package generate

import (
	"testing"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
)

// TestHandlerGenerationLeavesTheRouteAsResolved generates one handler
// twice. Rendering argument parsing rewrites the call to the locals it
// declares; were that done to the route itself, the second handler would
// be generated from the first one's rewrites.
func TestHandlerGenerationLeavesTheRouteAsResolved(t *testing.T) {
	config := testConfig()
	config.ReceiverType = "T"
	src := testSource(t, `package server

type T struct{}

func (T) Article(id int, title string) (string, error) { return "", nil }

func (T) Title(id int) string { return "" }
`, "T", `{{define "GET /article/{id} Article(id, Title(id))"}}{{end}}`)

	groups, err := groupTemplates(config, src.Templates)
	if err != nil {
		t.Fatal(err)
	}
	file := newFile(src.Package)
	def := groups.all[0]
	if err := muxt.ResolveCall(&def, src.Package, src.Receiver); err != nil {
		t.Fatal(err)
	}

	var handlers []string
	for range 2 {
		handler, err := callHandlerFunc(file, config, def, config.ReceiverInterface)
		if err != nil {
			t.Fatal(err)
		}
		handlers = append(handlers, astgen.Format(handler))
	}
	if handlers[0] != handlers[1] {
		t.Errorf("the second handler differs from the first:\n%s\nsecond:\n%s", handlers[0], handlers[1])
	}
	if got := astgen.Format(def.CallExpression()); got != "Article(id, Title(id))" {
		t.Errorf("the route's call is %s after generation, want it as written", got)
	}
}
