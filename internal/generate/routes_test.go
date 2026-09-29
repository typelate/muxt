package generate

import (
	"io"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/typelate/muxt/internal/astgen"
	"github.com/typelate/muxt/internal/fake"
	"github.com/typelate/muxt/internal/muxt"
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

	defs, err := muxt.ResolveDefinitions(pkg, receiver, fake.NewChecker().Fake())
	require.NoError(t, err)
	groups, err := groupTemplates(config, defs)
	require.NoError(t, err)
	file := newFile(pkg)
	def := groups.all[0]

	var handlers []string
	for range 2 {
		handler, err := callHandlerFunc(file, config, def, config.ReceiverInterface)
		require.NoError(t, err)
		handlers = append(handlers, astgen.Format(handler))
	}
	assert.Equal(t, handlers[0], handlers[1], "the second handler differs from the first")
	assert.Equal(t, "Article(id, Title(id))", astgen.Format(def.CallExpression()), "the route's call after generation, want it as written")
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

	defs, err := muxt.ResolveDefinitions(pkg, receiver, fake.NewChecker().Fake())
	require.NoError(t, err)
	_, err = TemplateRoutesFiles(".", config, pkg, defs, log.New(io.Discard, "", 0))
	assert.ErrorContains(t, err, "failed to determine type for fooMessage")
}
