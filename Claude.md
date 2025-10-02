# Claude.md - Muxt Development Guide

**Muxt** generates HTTP handlers from HTML template names using extended ServeMux pattern syntax: `[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]`.

Standard ServeMux: `[METHOD ][HOST]/[PATH]` → Muxt adds optional `HTTP_STATUS` and `CALL` suffix.

Example: `{{define "GET /greet/{language} 200 Greeting(ctx, language)"}}...{{end}}`

## CLI Commands

- `muxt generate` - Generate handlers from templates (flags: `--receiver-type`, `--receiver-type-package`, `--output-file`, `--routes-func`, `--templates-variable`)
- `muxt check` - Validate template/method compatibility using `typelate/check`
- `muxt version` - Print version
- `muxt documentation` - WIP docs server

## Testing

Script-based tests in `./cmd/muxt/testdata/` using `rsc.io/script/scripttest`. Run: `go test -v -cover ./cmd/muxt` or `go test --run Test/simple ./cmd/muxt`

## Code Generation

1. Scans `.gohtml` files for route patterns
2. Introspects receiver methods with `go/types`
3. Generates `http.HandlerFunc` that parses params, calls method, executes template with `TemplateData[T]`

Generates `RoutesReceiver` interface and `TemplateRoutes(mux, receiver)` registration function.

## Development Workflow

Code in `internal/source/` or `cmd/muxt/`. Test in `./cmd/muxt/testdata/`. Manual test: `./example/hypertext/`

## Planned Changes (v0.18+)

**Priority**: pflag migration (stdlib `flag` → `spf13/pflag`)

**Future**:
- File-per-template: `index.gohtml` → `index_template_routes.go` + `_test.go`
- Test generation: Given/When/Then with counterfeiter fakes
- `muxt fmt`: Add gotype comments, sort templates, detect template usage in HTMX attrs
- Input validation from HTML attributes
- Documentation rewrite (Diátaxis)

## Related

- `typelate/dom` - HTML parsing/testing
- `typelate/check` - Template type checking
