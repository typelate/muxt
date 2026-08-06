package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/typelate/dom/domtest"
)

func TestTemplates(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "go", "run", "github.com/typelate/muxt", "check", "--receiver-type=Server")
	var buf bytes.Buffer
	cmd.Stderr = &buf
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		t.Log(buf.String())
		t.Fatal(err)
	}
}

func newTestServer() (*http.ServeMux, *Server) {
	srv := new(Server)
	mux := http.NewServeMux()
	TemplateRoutes(mux, srv)
	return mux, srv
}

func TestHome(t *testing.T) {
	mux, _ := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	doc := domtest.ParseResponseDocument(t, res)
	count := doc.QuerySelector("#count")
	require.NotNil(t, count)
	assert.Equal(t, "0", count.TextContent())

	// The generated action must be an executable Datastar expression, not a
	// quoted string literal (the attribute value is entity-decoded here, as a
	// browser would before Datastar reads it).
	increment := doc.QuerySelector("#increment")
	require.NotNil(t, increment)
	assert.Equal(t, "@post('/increment')", increment.GetAttribute("data-on:click"))
}

func TestIncrement(t *testing.T) {
	mux, _ := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "/increment", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
	assert.Equal(t, "no-store", res.Header.Get("Cache-Control"))

	// One datastar-patch-elements frame; Datastar swaps it in by element id.
	assert.Equal(t, "event: datastar-patch-elements\ndata: elements <output id=\"count\">1</output>\n\n\n", rec.Body.String())
}

func TestGreet(t *testing.T) {
	mux, _ := newTestServer()

	t.Run("signals decode into the parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/greet", strings.NewReader(`{"name":"Ada"}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, "event: datastar-patch-elements\ndata: elements <p id=\"greeting\">Hello, Ada!</p>\n\n\n", rec.Body.String())
	})

	t.Run("absent signals leave the parameter zero-valued", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/greet", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Contains(t, rec.Body.String(), "Hello, stranger!")
	})
}
