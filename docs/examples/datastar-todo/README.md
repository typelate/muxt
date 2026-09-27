# Datastar Todo

A todo list over [Datastar](https://data-star.dev) patches, generated with [`--output-datastar`](../../reference/cli.md#generate-output-flags). Every mutation streams one patch-elements event that carries the list and the footer together.

## Run

```bash
go generate ./...
go run .
```

Open http://localhost:8002. `PORT` overrides the port.

## Read in this order

1. [template.gohtml](template.gohtml): the `sseTodos` fragment rendered by the page and every patch, and the form, checkbox and delete button sending Datastar actions through `.Path` helpers.
2. [main.go](main.go): each mutation taking a snapshot under the mutex and handing it to `sseTodos`.
3. [template_test.go](template_test.go): `patchElements`, which asserts the wire contract and returns a fragment the same selectors can query.

## Routes

| Template name | Method |
|---|---|
| `GET / List(ctx)` | `List(context.Context) (Todos, error)` |
| `POST /todos sse(CreateTodo(ctx, form, sseTodos))` | `CreateTodo(context.Context, TodoForm, func(Todos) error)` |
| `POST /todos/{id}/toggle sse(ToggleTodo(ctx, id, sseTodos))` | `ToggleTodo(context.Context, int, func(Todos) error)` |
| `DELETE /todos/{id} sse(DeleteTodo(ctx, id, sseTodos))` | `DeleteTodo(context.Context, int, func(Todos) error)` |

The form posts with `{contentType: 'form'}` so [`form`](../../reference/call-parameters.md#form-structs) binds as for a plain submit.
