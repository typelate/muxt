package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/typelate/dom/domtest"
	"github.com/typelate/dom/spec"
	"golang.org/x/net/html/atom"
)

func newTestServer(todos ...Todo) (*http.ServeMux, *Server) {
	srv := new(Server)
	for _, todo := range todos {
		srv.items = append(srv.items, todo)
		srv.nextID = max(srv.nextID, todo.ID)
	}
	mux := http.NewServeMux()
	TemplateRoutes(mux, srv)
	StaticRoutes(mux)
	return mux, srv
}

func postForm(mux *http.ServeMux, target string, form url.Values) *http.Response {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Result()
}

func do(mux *http.ServeMux, method, target string) *http.Response {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
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

func requireTitles(t *testing.T, root elementQueries, want ...string) {
	t.Helper()
	var got []string
	for label := range root.QuerySelectorSequence("#todo-list li label") {
		got = append(got, label.TextContent())
	}
	assert.Equal(t, want, got)
}

func requireFooter(t *testing.T, root elementQueries, want string) {
	t.Helper()
	footer := root.QuerySelector("#todo-footer")
	require.NotNil(t, footer, "every patch must refresh the footer")
	assert.Equal(t, want, strings.TrimSpace(footer.TextContent()))
}

// jsPath returns path as html/template renders it inside a data-on:*
// attribute: the attribute name puts the value in JavaScript context, where
// each / escapes to \/ — the same string once the browser evaluates it.
func jsPath(path string) string {
	return strings.ReplaceAll(path, "/", `\/`)
}

func TestTodoPage(t *testing.T) {
	t.Run("given there are no todos", func(t *testing.T) {
		mux, _ := newTestServer()

		t.Run("when the client loads the page", func(t *testing.T) {
			doc := domtest.ParseResponseDocument(t, do(mux, http.MethodGet, "/"))
			require.NotNil(t, doc)

			t.Run("then the list is empty and the footer says so", func(t *testing.T) {
				require.NotNil(t, doc.QuerySelector("ul#todo-list"))
				requireTitles(t, doc)
				requireFooter(t, doc, "0 of 0 remaining")
			})
			t.Run("then the stylesheet it links is served", func(t *testing.T) {
				link := doc.QuerySelector(`link[rel="stylesheet"]`)
				require.NotNil(t, link)
				res := do(mux, http.MethodGet, link.GetAttribute("href"))
				assert.Equal(t, http.StatusOK, res.StatusCode)
				assert.Contains(t, res.Header.Get("Content-Type"), "text/css")
				require.NoError(t, res.Body.Close())
			})
			t.Run("then the form posts a datastar action with the field the handler reads", func(t *testing.T) {
				form := doc.QuerySelector("form")
				require.NotNil(t, form)
				assert.Contains(t, form.GetAttribute("data-on:submit"), "@post('"+jsPath("/todos")+"'")
				require.NotNil(t, form.QuerySelector(`input[name="title"]`))
			})
		})
	})
}

func TestCreateTodo(t *testing.T) {
	t.Run("given there are no todos", func(t *testing.T) {
		mux, _ := newTestServer()

		t.Run("when the client submits a new todo", func(t *testing.T) {
			fragment := patchElements(t, postForm(mux, "/todos", url.Values{"title": {"walk the dog"}}))
			require.NotNil(t, fragment)

			t.Run("then the patch renders the todo and its actions", func(t *testing.T) {
				requireTitles(t, fragment, "walk the dog")
				requireFooter(t, fragment, "1 of 1 remaining")

				li := fragment.QuerySelector("#todo-list li")
				require.NotNil(t, li)
				assert.Equal(t, "@post('"+jsPath("/todos/1/toggle")+"')", li.QuerySelector(`input[type="checkbox"]`).GetAttribute("data-on:change"))
				assert.Equal(t, "@delete('"+jsPath("/todos/1")+"')", li.QuerySelector("button").GetAttribute("data-on:click"))
			})
		})
	})

	t.Run("given a todo title contains markup", func(t *testing.T) {
		mux, _ := newTestServer()

		t.Run("when the client submits it", func(t *testing.T) {
			fragment := patchElements(t, postForm(mux, "/todos", url.Values{"title": {"<b>bold</b> move"}}))
			require.NotNil(t, fragment)

			t.Run("then the title renders as text, not markup", func(t *testing.T) {
				requireTitles(t, fragment, "<b>bold</b> move")
				assert.Nil(t, fragment.QuerySelector("#todo-list li label b"))
			})
		})
	})

	t.Run("given the client submits a blank title", func(t *testing.T) {
		mux, srv := newTestServer()

		t.Run("when the form posts whitespace", func(t *testing.T) {
			fragment := patchElements(t, postForm(mux, "/todos", url.Values{"title": {"   "}}))
			require.NotNil(t, fragment)

			t.Run("then nothing is added", func(t *testing.T) {
				requireTitles(t, fragment)
				requireFooter(t, fragment, "0 of 0 remaining")
				assert.Empty(t, srv.items)
			})
		})
	})
}

func TestToggleTodo(t *testing.T) {
	t.Run("given one open todo", func(t *testing.T) {
		mux, _ := newTestServer(Todo{ID: 7, Title: "walk the dog"})

		t.Run("when the client toggles it", func(t *testing.T) {
			fragment := patchElements(t, do(mux, http.MethodPost, "/todos/7/toggle"))
			require.NotNil(t, fragment)

			t.Run("then the patch marks it done and updates the footer", func(t *testing.T) {
				li := fragment.QuerySelector("#todo-list li")
				require.NotNil(t, li)
				assert.Equal(t, "done", li.GetAttribute("class"))
				require.NotNil(t, li.QuerySelector(`input[checked]`))
				requireFooter(t, fragment, "0 of 1 remaining")
			})
		})
	})

	t.Run("given a todo id that does not exist", func(t *testing.T) {
		mux, _ := newTestServer(Todo{ID: 1, Title: "walk the dog"})

		t.Run("when the client toggles it", func(t *testing.T) {
			fragment := patchElements(t, do(mux, http.MethodPost, "/todos/99/toggle"))
			require.NotNil(t, fragment)

			t.Run("then the patch leaves the list unchanged", func(t *testing.T) {
				requireTitles(t, fragment, "walk the dog")
				requireFooter(t, fragment, "1 of 1 remaining")
			})
		})
	})
}

func TestDeleteTodo(t *testing.T) {
	t.Run("given two todos", func(t *testing.T) {
		mux, _ := newTestServer(
			Todo{ID: 1, Title: "walk the dog"},
			Todo{ID: 2, Title: "water the plants", Done: true},
		)

		t.Run("when the client deletes the done one", func(t *testing.T) {
			fragment := patchElements(t, do(mux, http.MethodDelete, "/todos/2"))
			require.NotNil(t, fragment)

			t.Run("then the patch renders the remaining todo", func(t *testing.T) {
				requireTitles(t, fragment, "walk the dog")
				requireFooter(t, fragment, "1 of 1 remaining")
			})
		})
	})
}
