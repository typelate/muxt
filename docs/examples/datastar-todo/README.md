# Datastar Todo

A todo list served over [Datastar](https://data-star.dev) patches. It demonstrates the `--output-datastar` flag with form binding: every mutation streams one patch-elements event that refreshes the list and the footer together.

## How it works

The page and every mutation render the same `sseTodos` fragment — two elements, morphed by id:

```gotmpl
{{define "sseTodos"}}<ul id="todo-list">…</ul>
<footer id="todo-footer">{{.Result.Remaining}} of {{.Result.Total}} remaining</footer>{{end}}
```

The form posts itself as a Datastar action, the checkbox and delete button carry per-todo actions:

```gotmpl
<form data-on:submit="evt.preventDefault(); @post('/todos', {contentType: 'form'}); el.reset()">
<input type="checkbox" data-on:change="@post('/todos/{{.ID}}/toggle')">
<button data-on:click="@delete('/todos/{{.ID}}')">✕</button>
```

Each route binds its inputs and streams the snapshot through the `sseTodos` callback:

```gotmpl
{{define "POST /todos sse(CreateTodo(ctx, form, sseTodos))"}}{{end}}
{{define "POST /todos/{id}/toggle sse(ToggleTodo(ctx, id, sseTodos))"}}{{end}}
{{define "DELETE /todos/{id} sse(DeleteTodo(ctx, id, sseTodos))"}}{{end}}
```

```go
func (s *Server) CreateTodo(_ context.Context, form TodoForm, sseTodos func(Todos) error)
```

## Run it

```bash
go generate ./...
go run .
# open http://localhost:8002
```

The tests are Given/When/Then subtests over [domtest](https://pkg.go.dev/github.com/typelate/dom/domtest): `patchElements` in [template_test.go](template_test.go) asserts the wire contract and hands back a queryable fragment, so the same selectors cover the full page and every patch.
