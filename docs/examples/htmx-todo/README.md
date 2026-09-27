# htmx TodoMVC

[TodoMVC](https://todomvc.com/) with htmx and a JSON file for persistence.

## Run

```bash
go generate ./...
go run .
```

Open http://localhost:8000.

| Flag | Default | Meaning |
|---|---|---|
| `-port` | `PORT` or `8000` | port to listen on |
| `-data` | `todos.json` | file the list is loaded from at start and saved to on interrupt |

## Read in this order

1. [template.gohtml](template.gohtml): route templates plus the `todo-item`, `footer`, and `error` fragments.
2. [main.go](main.go): `Server` and its `Load` and `Save` methods.
3. [template_routes_test.go](template_routes_test.go): tests through the generated handlers.
4. [template_test.go](template_test.go): `muxt check` run as a test.

## Routes

| Template name | Method |
|---|---|
| `GET /{$} ListTodos(form, execute)` | `ListTodos(TodoFilter, func(TodoPage) error) error` |
| `POST /todos CreateTodo(form)` | `CreateTodo(NewTodo) TodoChange` |
| `PATCH /todos/{id} ToggleTodo(id)` | `ToggleTodo(int) (TodoChange, error)` |
| `DELETE /todos/{id} DeleteTodo(id)` | `DeleteTodo(int) TodoChange` |
| `POST /todos/toggle-all ToggleAll()` | `ToggleAll() TodoListChange` |
| `POST /todos/clear-completed ClearCompleted()` | `ClearCompleted() TodoListChange` |

`ListTodos` renders through `execute` while holding the mutex, so the page sees one consistent snapshot ([the `execute` callback](../../reference/call-results.md#the-execute-callback)).

Every successful mutation response ends with the `footer` fragment; its `hx-swap-oob="true"` updates the items-left count and filter links with the list. `ToggleTodo` on an unknown id sets [`{{.StatusCode 404}}`](../../reference/call-results.md#status-code-control) and renders the `error` fragment; the checkbox and delete button carry `hx-target-4*="#error"` from the `response-targets` extension, since stock htmx does not swap 4xx bodies. The banner's `data-remove-me="5s"` from the `remove-me` extension clears it.
