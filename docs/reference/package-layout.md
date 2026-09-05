# Package layout

Large Muxt applications converge on this structure:

```
internal/hypertext/
├── server.go                     # Server type, interface definitions
├── template.go                   # Template config, custom functions, go:generate directive
├── functions.go                  # Template function implementations
├── template_data.go              # TemplateData method extensions
├── errors.go                     # Domain errors with StatusCode() methods
├── {domain}_*.go                 # Receiver method implementations (portfolio.go, security.go)
├── {domain}_*.gohtml             # Route templates grouped by domain
├── _*.gohtml                     # Shared partials (prefix convention)
├── template_routes.go            # Generated: main orchestration, shared types
├── *_template_routes_gen.go      # Generated with --output-multiple-files: per-source-file handlers
├── *_test.go                     # Table-driven tests using counterfeiter fakes
└── internal/fake/                # Generated: counterfeiter test doubles
    ├── database.go
    └── securities_provider.go
```

## File naming conventions

- `portfolio_list.gohtml`, `portfolio_edit.gohtml` — domain-grouped templates
- `_header.gohtml`, `_navigation.gohtml` — shared partials (underscore prefix)
- `portfolio.go` — receiver methods for portfolio routes
- `portfolio_test.go` — tests for portfolio receivers

## Separation of concerns

| File | Holds |
|------|-------|
| `{domain}.go` | Business logic; methods return domain types |
| `template_data.go` | Request-aware presentation logic ([how-to](../how-to/extend-template-data.md)) |
| `*.gohtml` | HTML structure; calls to methods and template functions |
| `functions.go` | Pure formatters and converters ([how-to](../how-to/template-functions.md)) |
| `errors.go` | Domain errors with HTTP status semantics ([how-to](../how-to/domain-error-status-codes.md)) |

## Generated code

Muxt produces `template_routes.go` (shared types, route registration); with `--output-multiple-files` it additionally splits handlers into one `*_template_routes_gen.go` per source template file. Check generated files into version control and review them in code review — they're your code.

## Testing

Unit test receiver methods with fakes of the server's service interfaces (fast, isolated); integration test the generated routes with `httptest` (routing, status codes, HTML structure). The [testing how-to](../how-to/receiver-package-and-testing.md) covers the layout and the fake-vs-mock tradeoffs.
