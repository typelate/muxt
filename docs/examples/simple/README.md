# Simple: Edit-in-Place Fruit Table

htmx swaps each row of a fruit table between its view and edit fragments.

## Run

```bash
go generate ./...
go run .
```

Open http://localhost:8080. `PORT` overrides the port.

## Read in this order

1. [index.gohtml](index.gohtml): the route templates and the `view-row` and `edit-row` fragments they swap between.
2. [main.go](main.go): `Backend`, the receiver, and `main` calling `TemplateRoutes(mux, backend)`.
3. [templates.go](templates.go): the `templates` variable, the `//go:generate` directive, and the `Row` and `EditRow` types.
4. [template_routes_test.go](template_routes_test.go): tests through the generated handlers.

## Routes

| Template name | Method |
|---|---|
| `GET /{$} List(ctx)` | `List(context.Context) []Row` |
| `GET /fruits/{id}/edit GetFormEditRow(id)` | `GetFormEditRow(int) (Row, error)` |
| `PATCH /fruits/{id} SubmitFormEditRow(id, form)` | `SubmitFormEditRow(int, EditRow) (Row, error)` |
| `GET /help` | none |

The directive passes `--use-receiver-type Backend`, so arguments parse to the method's types ([type resolution](../../reference/call-parameters.md#type-resolution)).
