package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/typelate/dom/domtest"
	"golang.org/x/net/html/atom"
)

func newTestServer() (*http.ServeMux, *Server) {
	srv := new(Server)
	mux := http.NewServeMux()
	TemplateRoutes(mux, srv)
	return mux, srv
}

func TestListTodos(t *testing.T) {
	mux, _ := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	doc := domtest.ParseResponseDocument(t, res)
	require.NotNil(t, doc.QuerySelector(".todoapp"))
	require.NotNil(t, doc.QuerySelector("h1"))
	assert.Equal(t, "todos", doc.QuerySelector("h1").TextContent())
}

func TestCreateTodo(t *testing.T) {
	mux, _ := newTestServer()

	form := url.Values{"todo": []string{"Buy milk"}}
	req := httptest.NewRequest(http.MethodPost, "/todos", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	fragment := domtest.ParseResponseDocumentFragment(t, res, atom.Body)
	li := fragment.QuerySelector("li")
	require.NotNil(t, li)
	assert.Contains(t, li.QuerySelector("label").TextContent(), "Buy milk")

	footer := fragment.QuerySelector("#footer")
	require.NotNil(t, footer)
	assert.Equal(t, "true", footer.GetAttribute("hx-swap-oob"))
}

func TestToggleTodo(t *testing.T) {
	mux, srv := newTestServer()

	// Seed a todo
	srv.CreateTodo(NewTodo{Title: "Test todo"})

	req := httptest.NewRequest(http.MethodPatch, "/todos/1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	fragment := domtest.ParseResponseDocumentFragment(t, res, atom.Body)
	li := fragment.QuerySelector("li")
	require.NotNil(t, li)
	assert.Contains(t, li.GetAttribute("class"), "completed")

	footer := fragment.QuerySelector("#footer")
	require.NotNil(t, footer)
	assert.Equal(t, "true", footer.GetAttribute("hx-swap-oob"))
}

func TestDeleteTodo(t *testing.T) {
	mux, srv := newTestServer()

	// Seed a todo
	srv.CreateTodo(NewTodo{Title: "Delete me"})

	req := httptest.NewRequest(http.MethodDelete, "/todos/1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	fragment := domtest.ParseResponseDocumentFragment(t, res, atom.Body)
	footer := fragment.QuerySelector("#footer")
	require.NotNil(t, footer)
	assert.Equal(t, "true", footer.GetAttribute("hx-swap-oob"))
}

func TestToggleTodoNotFound(t *testing.T) {
	mux, _ := newTestServer()

	req := httptest.NewRequest(http.MethodPatch, "/todos/999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusNotFound, res.StatusCode)

	fragment := domtest.ParseResponseDocumentFragment(t, res, atom.Body)
	errMsg := fragment.QuerySelector(".error-banner")
	require.NotNil(t, errMsg, "error message should be rendered")
	assert.Contains(t, errMsg.TextContent(), "not found")
}

func TestDeleteTodoNotFound(t *testing.T) {
	mux, _ := newTestServer()

	req := httptest.NewRequest(http.MethodDelete, "/todos/999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
}

func TestToggleAll(t *testing.T) {
	mux, srv := newTestServer()

	// Seed mixed todos
	srv.CreateTodo(NewTodo{Title: "Active"})
	srv.CreateTodo(NewTodo{Title: "Done"})
	srv.ToggleTodo(2) // mark second as done

	req := httptest.NewRequest(http.MethodPost, "/todos/toggle-all", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	fragment := domtest.ParseResponseDocumentFragment(t, res, atom.Body)
	items := fragment.QuerySelectorAll("li[id^='todo-']")
	require.Equal(t, 2, items.Length())
	for i := range items.Length() {
		assert.Contains(t, items.Item(i).GetAttribute("class"), "completed")
	}

	footer := fragment.QuerySelector("#footer")
	require.NotNil(t, footer)
}

func TestClearCompleted(t *testing.T) {
	mux, srv := newTestServer()

	// Seed mixed todos
	srv.CreateTodo(NewTodo{Title: "Active"})
	srv.CreateTodo(NewTodo{Title: "Done"})
	srv.ToggleTodo(2) // mark second as done

	req := httptest.NewRequest(http.MethodPost, "/todos/clear-completed", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	fragment := domtest.ParseResponseDocumentFragment(t, res, atom.Body)
	items := fragment.QuerySelectorAll("li[id^='todo-']")
	assert.Equal(t, 1, items.Length())
	assert.Contains(t, items.Item(0).QuerySelector("label").TextContent(), "Active")
}

func TestFilterActive(t *testing.T) {
	mux, srv := newTestServer()

	// Seed mixed todos
	srv.CreateTodo(NewTodo{Title: "Active"})
	srv.CreateTodo(NewTodo{Title: "Done"})
	srv.ToggleTodo(2)

	req := httptest.NewRequest(http.MethodGet, "/?filter=active", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	doc := domtest.ParseResponseDocument(t, res)
	items := doc.QuerySelectorAll("#todo-list li")
	assert.Equal(t, 1, items.Length())
	assert.Contains(t, items.Item(0).QuerySelector("label").TextContent(), "Active")
}

func TestFilterCompleted(t *testing.T) {
	mux, srv := newTestServer()

	srv.CreateTodo(NewTodo{Title: "Active"})
	srv.CreateTodo(NewTodo{Title: "Done"})
	srv.ToggleTodo(2)

	req := httptest.NewRequest(http.MethodGet, "/?filter=completed", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	res := rec.Result()
	assert.Equal(t, http.StatusOK, res.StatusCode)

	doc := domtest.ParseResponseDocument(t, res)
	items := doc.QuerySelectorAll("#todo-list li")
	assert.Equal(t, 1, items.Length())
	assert.Contains(t, items.Item(0).QuerySelector("label").TextContent(), "Done")
}

func TestTodoWorkflow(t *testing.T) {
	mux, _ := newTestServer()

	// Step 1: GET / — verify page structure and hx-post on form
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	doc := domtest.ParseResponseDocument(t, rec.Result())

	form := doc.QuerySelector("form[hx-post]")
	require.NotNil(t, form, "page must have a form with hx-post")
	assert.Equal(t, "/todos", form.GetAttribute("hx-post"))

	// Step 2: POST /todos — create a todo
	formData := url.Values{"todo": []string{"Walk the dog"}}
	req = httptest.NewRequest(http.MethodPost, "/todos", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	fragment := domtest.ParseResponseDocumentFragment(t, rec.Result(), atom.Body)
	li := fragment.QuerySelector("li")
	require.NotNil(t, li)
	assert.Contains(t, li.QuerySelector("label").TextContent(), "Walk the dog")

	// Step 3: Verify hx-patch and hx-delete on todo items
	patchAttr := li.QuerySelector("[hx-patch]")
	require.NotNil(t, patchAttr, "todo item must have hx-patch")
	assert.Equal(t, "/todos/1", patchAttr.GetAttribute("hx-patch"))

	deleteAttr := li.QuerySelector("[hx-delete]")
	require.NotNil(t, deleteAttr, "todo item must have hx-delete")
	assert.Equal(t, "/todos/1", deleteAttr.GetAttribute("hx-delete"))
}

func TestPersistence(t *testing.T) {
	path := t.TempDir() + "/test-todos.json"

	// Create server, add a todo, save
	srv1 := new(Server)
	srv1.CreateTodo(NewTodo{Title: "Persist me"})
	srv1.Save(path)

	// Create new server, load, verify
	srv2 := new(Server)
	srv2.Load(path)
	page := srv2.ListTodos(TodoFilter{})
	require.Len(t, page.Todos, 1)
	assert.Equal(t, "Persist me", page.Todos[0].Title)
	assert.Equal(t, 1, page.Todos[0].ID)
}
