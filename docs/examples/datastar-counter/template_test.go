package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/typelate/dom/domtest"
	"github.com/typelate/dom/spec"
	"golang.org/x/net/html/atom"
)

func newTestServer(count int64) *http.ServeMux {
	srv := new(Server)
	srv.count.Store(count)
	mux := http.NewServeMux()
	TemplateRoutes(mux, srv)
	StaticRoutes(mux)
	return mux
}

// readEventStream asserts res is an SSE stream and returns its body.
func readEventStream(t *testing.T, res *http.Response) string {
	t.Helper()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
	require.Equal(t, "no-store", res.Header.Get("Cache-Control"))
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	return string(body)
}

// elementsFragment parses the patch-elements payload as the fragment the
// browser would morph into the page.
func elementsFragment(t *testing.T, stream string) spec.DocumentFragment {
	t.Helper()
	require.Contains(t, stream, "event: datastar-patch-elements\n")
	var elements []string
	for _, line := range strings.Split(stream, "\n") {
		if payload, ok := strings.CutPrefix(line, "data: elements "); ok {
			elements = append(elements, payload)
		}
	}
	require.NotEmpty(t, elements, "the event carries no elements payload")
	return domtest.ParseStringDocumentFragment(t, strings.Join(elements, "\n"), atom.Body)
}

// jsPath returns path as html/template renders it inside a data-on:*
// attribute: the attribute name puts the value in JavaScript context, where
// each / escapes to \/ — the same string once the browser evaluates it.
func jsPath(path string) string {
	return strings.ReplaceAll(path, "/", `\/`)
}

func TestCounterPage(t *testing.T) {
	t.Run("given the counter is at zero", func(t *testing.T) {
		mux := newTestServer(0)

		t.Run("when the client loads the page", func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			doc := domtest.ParseResponseDocument(t, rec.Result())
			require.NotNil(t, doc)

			t.Run("then the count reads zero", func(t *testing.T) {
				count := doc.QuerySelector("output#count")
				require.NotNil(t, count)
				assert.Equal(t, "0", count.TextContent())
			})
			t.Run("then the stylesheet it links is served", func(t *testing.T) {
				link := doc.QuerySelector(`link[rel="stylesheet"]`)
				require.NotNil(t, link)
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, link.GetAttribute("href"), nil))
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
				assert.Contains(t, res.Header.Get("Content-Type"), "text/css")
				require.NoError(t, res.Body.Close())
			})
			t.Run("then the delta signal is declared and displayed", func(t *testing.T) {
				main := doc.QuerySelector("main")
				require.NotNil(t, main)
				assert.True(t, main.HasAttribute("data-signals-delta"))
				delta := doc.QuerySelector("output#delta")
				require.NotNil(t, delta)
				assert.Equal(t, "$delta", delta.GetAttribute("data-text"))
			})
			t.Run("then each button posts a datastar action", func(t *testing.T) {
				increment := doc.QuerySelector("#increment")
				require.NotNil(t, increment)
				assert.Equal(t, "@post('"+jsPath("/increment")+"')", increment.GetAttribute("data-on:click"))

				decrement := doc.QuerySelector("#decrement")
				require.NotNil(t, decrement)
				assert.Equal(t, "@post('"+jsPath("/decrement")+"')", decrement.GetAttribute("data-on:click"))
			})
		})
	})
}

func TestIncrement(t *testing.T) {
	t.Run("given the counter is at 41", func(t *testing.T) {
		mux := newTestServer(41)

		t.Run("when the client increments", func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/increment", nil))

			stream := readEventStream(t, rec.Result())

			t.Run("then one patch morphs the count to 42", func(t *testing.T) {
				fragment := elementsFragment(t, stream)
				require.NotNil(t, fragment)
				count := fragment.QuerySelector("output#count")
				require.NotNil(t, count, "the patch must carry the element the page displays")
				assert.Equal(t, "42", count.TextContent())
			})
			t.Run("then a second event patches the delta signal", func(t *testing.T) {
				assert.Contains(t, stream, "event: datastar-patch-signals\n")
				assert.Contains(t, stream, `data: signals {"delta":"+1"}`+"\n")
				assert.Less(t, strings.Index(stream, "data: elements"), strings.Index(stream, "data: signals"),
					"the count patch precedes the signal patch")
			})
		})
	})
}

func TestDecrement(t *testing.T) {
	t.Run("given the counter is at 1", func(t *testing.T) {
		mux := newTestServer(1)

		t.Run("when the client decrements twice", func(t *testing.T) {
			for range 2 {
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/decrement", nil))
				stream := readEventStream(t, rec.Result())
				require.NotNil(t, elementsFragment(t, stream))
				assert.Contains(t, stream, `data: signals {"delta":"-1"}`+"\n")
			}

			t.Run("then the page shows the count below zero", func(t *testing.T) {
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
				doc := domtest.ParseResponseDocument(t, rec.Result())
				require.NotNil(t, doc)
				count := doc.QuerySelector("output#count")
				require.NotNil(t, count)
				assert.Equal(t, "-1", count.TextContent())
			})
		})
	})
}
