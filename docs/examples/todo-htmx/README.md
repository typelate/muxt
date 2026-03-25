# TodoMVC with HTMX

A [TodoMVC](https://todomvc.com/) implementation using Muxt and HTMX.

## Features

- Add, toggle, delete todos
- Toggle all / clear completed
- Filter by all, active, completed
- State persisted to `todos.json`

## Usage

```bash
go generate ./...
go run .
```

Open `http://localhost:8000`.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `PORT` env or `8000` | Port to listen on |
| `-data` | `todos.json` | Path to the JSON file for persisting todos |

```bash
go run . -port 3000 -data ~/my-todos.json
```

## How it works

Template names define routes and method calls:

| Template Name | Method | Purpose |
|---|---|---|
| `GET /{$} ListTodos(form)` | `ListTodos(TodoFilter) TodoPage` | Full page with filter |
| `POST /todos CreateTodo(form)` | `CreateTodo(NewTodo) TodoChange` | Add todo |
| `PATCH /todos/{id} ToggleTodo(id)` | `ToggleTodo(int) TodoChange` | Toggle todo |
| `DELETE /todos/{id} DeleteTodo(id)` | `DeleteTodo(int) TodoChange` | Delete todo |
| `POST /todos/toggle-all ToggleAll()` | `ToggleAll() TodoListChange` | Toggle all |
| `POST /todos/clear-completed ClearCompleted()` | `ClearCompleted() TodoListChange` | Clear completed |

HTMX attributes on form/button elements trigger requests. Mutation responses include an out-of-band footer (`hx-swap-oob="true"`) to update the items-left count and filter links.
