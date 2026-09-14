package generate

import (
	"io"
	"log"
	"strings"
	"testing"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/muxt"
	"github.com/typelate/muxt/internal/muxt/muxttest"
)

// TestHandlerGenerationLeavesTheRouteAsResolved generates one handler
// twice. Rendering argument parsing rewrites the call to the locals it
// declares; were that done to the route itself, the second handler would
// be generated from the first one's rewrites.
func TestHandlerGenerationLeavesTheRouteAsResolved(t *testing.T) {
	config := testConfig()
	config.ReceiverType = "T"
	pkg, receiver := testSource(t, `package server

type T struct{}

func (T) Article(id int, title string) (string, error) { return "", nil }

func (T) Title(id int) string { return "" }
`, "T", `{{define "GET /article/{id} Article(id, Title(id))"}}{{end}}`)

	groups, err := groupTemplates(config, pkg.Variables)
	if err != nil {
		t.Fatal(err)
	}
	file := newFile(pkg)
	def := groups.all[0]
	if err := muxt.ResolveCall(&def, pkg, receiver, muxttest.NewChecker().Fake()); err != nil {
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

// TestHandlerGenerationRejectsAnArgumentWithNoRequestValue generates an sse
// handler whose call passes a message, which names no request value to
// parse.
func TestHandlerGenerationRejectsAnArgumentWithNoRequestValue(t *testing.T) {
	config := testConfig()
	config.ReceiverType = "T"
	pkg, receiver := testSource(t, `package server

type T struct{}

func (T) Stream(string) {}
`, "T", `{{define "GET /x sse(Stream(fooMessage))"}}{{end}}{{define "fooMessage"}}{{end}}`)

	_, err := TemplateRoutesFiles(".", config, pkg, receiver, muxttest.NewChecker().Fake(), log.New(io.Discard, "", 0))
	if err == nil || !strings.Contains(err.Error(), "failed to determine type for fooMessage") {
		t.Errorf("got error %v, want it to say it failed to determine type for fooMessage", err)
	}
}
