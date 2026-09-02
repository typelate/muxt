package main

import (
	"encoding/json"
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

func newTestServer() *http.ServeMux {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})
	StaticRoutes(mux)
	return mux
}

func get(mux *http.ServeMux, target string) *http.Response {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec.Result()
}

func postSignals(mux *http.ServeMux, target, signals string) *http.Response {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(signals))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Result()
}

// patchElements asserts res is a Datastar patch-elements stream and parses the
// event payload as the fragment the browser would morph into the page.
func patchElements(t *testing.T, res *http.Response) spec.DocumentFragment {
	t.Helper()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Contains(t, string(body), "event: datastar-patch-elements\n")
	var elements []string
	for _, line := range strings.Split(string(body), "\n") {
		if payload, ok := strings.CutPrefix(line, "data: elements "); ok {
			elements = append(elements, payload)
		}
	}
	require.NotEmpty(t, elements, "the event carries no elements payload")
	return domtest.ParseStringDocumentFragment(t, strings.Join(elements, "\n"), atom.Body)
}

// elementQueries is the query surface the helpers need; documents, fragments,
// and elements all provide it.
type elementQueries interface {
	QuerySelector(query string) spec.Element
	spec.QuerySelectorIterator
}

func requireFooter(t *testing.T, root elementQueries, want string) {
	t.Helper()
	footer := root.QuerySelector("#results-footer span")
	require.NotNil(t, footer, "every patch must refresh the footer")
	assert.Equal(t, want, strings.TrimSpace(footer.TextContent()))
}

func resultTexts(root elementQueries) []string {
	var got []string
	for li := range root.QuerySelectorSequence("#results li") {
		got = append(got, li.TextContent())
	}
	return got
}

// jsPath returns path as html/template renders it inside a data-on:*
// attribute: the attribute name puts the value in JavaScript context, where
// each / escapes to \/ — the same string once the browser evaluates it.
func jsPath(path string) string {
	return strings.ReplaceAll(path, "/", `\/`)
}

func TestSearchPage(t *testing.T) {
	t.Run("given the proverb list", func(t *testing.T) {
		mux := newTestServer()

		t.Run("when the client loads the page", func(t *testing.T) {
			doc := domtest.ParseResponseDocument(t, get(mux, "/"))
			require.NotNil(t, doc)

			t.Run("then every proverb is listed without highlights", func(t *testing.T) {
				assert.Len(t, resultTexts(doc), len(goProverbs))
				assert.Nil(t, doc.QuerySelector("#results li mark"))
				requireFooter(t, doc, "19 of 19 proverbs")
			})
			t.Run("then the input binds the query signal and posts the search action", func(t *testing.T) {
				input := doc.QuerySelector(`input[type="search"]`)
				require.NotNil(t, input)
				assert.True(t, input.HasAttribute("data-bind:query"), "data-bind:query creates the signal the handler decodes")
				assert.Equal(t, "@post('"+jsPath("/search")+"')", input.GetAttribute("data-on:input__debounce.300ms"))
			})
			t.Run("then the JSON link resolves through the mux", func(t *testing.T) {
				link := doc.QuerySelector("#results-footer a")
				require.NotNil(t, link)
				res := get(mux, link.GetAttribute("href"))
				assert.Equal(t, http.StatusOK, res.StatusCode)
				assert.Equal(t, "application/json", res.Header.Get("Content-Type"))
				require.NoError(t, res.Body.Close())
			})
		})
	})
}

func TestSearchProverbs(t *testing.T) {
	t.Run("given the client typed mutex", func(t *testing.T) {
		mux := newTestServer()

		t.Run("when datastar posts the query signal", func(t *testing.T) {
			fragment := patchElements(t, postSignals(mux, "/search", `{"query":"mutex"}`))
			require.NotNil(t, fragment)

			t.Run("then the patch lists the match with the hit highlighted", func(t *testing.T) {
				assert.Equal(t, []string{"Channels orchestrate; mutexes serialize."}, resultTexts(fragment))
				mark := fragment.QuerySelector("#results li mark")
				require.NotNil(t, mark)
				assert.Equal(t, "mutex", mark.TextContent())
				requireFooter(t, fragment, "1 of 19 proverbs")
			})
		})
	})

	t.Run("given the query differs in case", func(t *testing.T) {
		mux := newTestServer()

		t.Run("when datastar posts CLEVER", func(t *testing.T) {
			fragment := patchElements(t, postSignals(mux, "/search", `{"query":"CLEVER"}`))
			require.NotNil(t, fragment)

			t.Run("then the match keeps the proverb's own casing", func(t *testing.T) {
				mark := fragment.QuerySelector("#results li mark")
				require.NotNil(t, mark)
				assert.Equal(t, "clever", mark.TextContent())
			})
		})
	})

	t.Run("given nothing matches", func(t *testing.T) {
		mux := newTestServer()

		t.Run("when datastar posts an unmatched query", func(t *testing.T) {
			fragment := patchElements(t, postSignals(mux, "/search", `{"query":"zzzz"}`))
			require.NotNil(t, fragment)

			t.Run("then the empty state renders", func(t *testing.T) {
				empty := fragment.QuerySelector("#results li.empty")
				require.NotNil(t, empty)
				requireFooter(t, fragment, "0 of 19 proverbs")
			})
		})
	})

	t.Run("given the signals body is not JSON", func(t *testing.T) {
		mux := newTestServer()

		t.Run("when the request is malformed", func(t *testing.T) {
			res := postSignals(mux, "/search", "not-json")

			t.Run("then the handler responds 400 before streaming", func(t *testing.T) {
				assert.Equal(t, http.StatusBadRequest, res.StatusCode)
				require.NoError(t, res.Body.Close())
			})
		})
	})
}

func TestProverbsAPI(t *testing.T) {
	t.Run("given the same search as JSON", func(t *testing.T) {
		mux := newTestServer()

		t.Run("when the client requests the API with a query parameter", func(t *testing.T) {
			res := get(mux, "/api/proverbs?query=copy")
			require.Equal(t, http.StatusOK, res.StatusCode)
			require.Equal(t, "application/json", res.Header.Get("Content-Type"))

			t.Run("then the payload carries the matches and totals", func(t *testing.T) {
				var results SearchResults
				require.NoError(t, json.NewDecoder(res.Body).Decode(&results))
				require.NoError(t, res.Body.Close())
				assert.Equal(t, "copy", results.Query)
				assert.Equal(t, len(goProverbs), results.Total)
				require.Len(t, results.Matches, 1)
				assert.Equal(t, "A little copying is better than a little dependency.", results.Matches[0].Text)
			})
		})
	})
}
