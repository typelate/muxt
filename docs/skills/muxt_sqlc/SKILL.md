---
name: muxt-sqlc
description: "Muxt: Use when building a Muxt application backed by a SQL database using sqlc for type-safe query generation. Covers project layout, form-to-query alignment, transaction handling, error wrapping, and returning rows directly from handlers."
---

# Muxt with sqlc

Muxt and [sqlc](https://docs.sqlc.dev) share an ethos: write in a declarative language (HTML templates, SQL queries) and get type-safe Go code generated. Together your HTML, SQL, and Go are all statically checked end to end.

## When to use this skill

- Building a Muxt app whose data layer uses sqlc-generated queries.
- Wiring form fields to sqlc params structs.
- Choosing transaction ownership (handler vs service vs caller).
- Wrapping DB errors so users see safe text, not internal details.
- Setting up real-database tests (Postgres or SQLite).

## Project layout

- **Small apps:** collocated templates and queries in one directory.
- **Recommended:** separate `internal/database/` (sqlc) and `internal/hypertext/` (muxt) packages.

Full directory layouts: `references/examples.md`.

## sqlc configuration

Set `query_parameter_limit` to control when sqlc generates a params struct. Default `1` → multi-parameter queries get a struct automatically. Set `0` to always generate one.

See [sqlc configuration reference](https://docs.sqlc.dev/en/latest/reference/config.html). Engine-specific `sqlc.yaml` examples in `references/examples.md`.

## Embedding queries on the receiver

Embed `*database.Queries` as a private `db` field. Keep `*sql.DB` for transactions. Simple reads call `s.db.GetArticle(ctx, id)` directly. Writes use `WithTx`. Full pattern with `BeginTx`/`Commit`/`Rollback` in `references/examples.md`.

See [sqlc transactions](https://docs.sqlc.dev/en/latest/howto/transactions.html).

## Aligning form fields with sqlc params

For a multi-parameter `INSERT` query, sqlc generates a params struct (`CreateArticleParams`). Name HTML form `name="..."` attributes to match those struct fields, then use the params struct directly as the `form` parameter type:

```go
func (s *Server) CreateArticle(ctx context.Context, form database.CreateArticleParams) (database.Article, error) {
    article, err := s.db.CreateArticle(ctx, form)
    return article, NewPublishableError(err)
}
```

Connascence-of-name flows from SQL columns through generated struct fields to HTML form names. Renames propagate via `go generate`; the compiler catches mismatches. Full template + form HTML in `references/examples.md`.

## Returning rows directly

For read-only handlers, return the sqlc-generated row type directly (`database.Article`, `[]database.Article`). Templates access fields with `.Result.Title`, `.Result.CreatedAt`. Renaming a column → `sqlc generate` updates the struct → `muxt check` / `go generate` catches template mismatches.

## Wrapping database errors

**Never return database errors directly to the user.** They leak table names, column names, query structure, connection details — an information disclosure vulnerability. See [OWASP Error Handling Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html).

Create a domain error type with a `StatusCode()` method so muxt sets the HTTP status automatically. The `Error()` method should return safe text from `http.StatusText` (e.g. "Not Found"); the original error is preserved via `Unwrap()` for `errors.Is`/`errors.As` and logged with context server-side. Full `PublishableError` implementation in `references/examples.md`.

For more patterns (validation errors, authorization, multiple states): [Domain Errors with HTTP Semantics](../../explanation/advanced-patterns.md).

## Testing with a real database

Test against a real database. Exercises your SQL queries, sqlc-generated code, receiver methods, generated handlers, and template rendering end to end. Fakes are for outside services, not your own database.

- **Postgres** — [pgtestdb](https://github.com/peterldowns/pgtestdb) for isolated per-test DBs, or docker-compose for a consistent local + CI service. `docker-compose.yml` and `sqlc.yaml` snippets in `references/examples.md`.
- **SQLite** — [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go, no CGo) with `sql.Open("sqlite", ":memory:")`. Each test gets a fresh in-memory DB. Walkthrough: [sqlc SQLite tutorial](https://docs.sqlc.dev/en/stable/tutorials/getting-started-sqlite.html).

### End-to-end with domtest

Build up a small domain language of test helpers (`testApp`, `postForm`, `getPage`) so test bodies read like user behavior. Full helper definitions and example tests (workflow + 404 case) in `references/examples.md`.

### chromedp for HTMX islands

Where JS matters (HTMX swaps after initial render integrating with echarts/sortable/etc.), `domtest` can't verify. Use chromedp behind `//go:build chromedp` and `testing.Short()`. Navigate to the parent page (loads htmx.js), not the fragment URL. Full pattern in `references/examples.md`.

## Reference files

- `references/examples.md` — directory layouts (collocated and recommended), full embedding + transactions code, aligned form/template/method, `PublishableError` type, Postgres/SQLite setup snippets, end-to-end test helpers, chromedp island test.

## External reference

- [sqlc Documentation](https://docs.sqlc.dev), [Configuration](https://docs.sqlc.dev/en/latest/reference/config.html), [Transactions](https://docs.sqlc.dev/en/latest/howto/transactions.html), [SQLite tutorial](https://docs.sqlc.dev/en/stable/tutorials/getting-started-sqlite.html)
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite), [pgtestdb](https://github.com/peterldowns/pgtestdb)
- [OWASP Error Handling Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Error_Handling_Cheat_Sheet.html)
- [chromedp](https://github.com/chromedp/chromedp), [domtest](https://github.com/typelate/dom)
- [Template-Driven Development](../muxt_test-driven-development/SKILL.md), [Call Parameters](../../reference/call-parameters.md)
- [Domain Errors with HTTP Semantics](../../explanation/advanced-patterns.md)

### Test cases (`cmd/muxt/testdata/`)

- `howto_form_with_struct.txt`, `howto_form_with_field_tag.txt`, `reference_form_field_types.txt` — form binding patterns used for sqlc params.
- `reference_status_codes.txt` — status codes in template names (`201` for POST create).
- `reference_call_with_error_return.txt` — error return handling used by all sqlc handlers.
- `tutorial_blog_example.txt` — complete blog example with domtest.
