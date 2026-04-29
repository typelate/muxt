# Muxt [![Go Reference](https://pkg.go.dev/badge/github.com/typelate/muxt.svg)](https://pkg.go.dev/github.com/typelate/muxt) [![Go](https://github.com/typelate/muxt/actions/workflows/go.yml/badge.svg)](https://github.com/typelate/muxt/actions/workflows/go.yml)

**Server-rendered HTML, type-checked at `go generate` time.**

```gotmpl
{{define "GET /article/{id} GetArticle(ctx, id)"}}
  <h1>{{.Result.Title}}</h1>
{{end}}
```

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) { ... }
```

The template name is the route, the handler call, and the parameter list. Muxt uses `go/types` to verify the whole chain — `GetArticle` exists on `Server`, `id` parses to `int`, `.Result.Title` is valid on `Article` — then writes the `http.Handler` glue. Typos and signature drift become `go generate` errors, not 5 PM pages.

## Why Muxt

- **Single source of truth.** Route, handler, and HTML live together. No separate `mux.HandleFunc` registration to drift out of sync.
- **Caught at generate time.** `go/types` flags stale field access, parameter mismatches, and missing methods before they ship.
- **No runtime reflection.** Generated code uses only `net/http` and `html/template`. Reads like hand-written Go.
- **Built for hypermedia.** HTMX, Datastar, and plain server-rendered HTML are the happy path — not an afterthought.

## Install

```bash
go install github.com/typelate/muxt@latest
```

Or as a project tool: `go get -tool github.com/typelate/muxt` (note the [license](#License)). Pre-built binaries are also attached to each [release](https://github.com/typelate/muxt/releases).

## Quick Start

1. Create a template `index.gohtml`:
   ```gotmpl
   {{define "GET / Home(ctx)"}}
   <h1>{{.Result}}</h1>
   {{end}}
   ```

2. Wire it up in `main.go`:
   ```go
   //go:embed *.gohtml
   var templateFS embed.FS

   //go:generate muxt generate --use-receiver-type=Server
   var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))

   type Server struct{}

   func (s Server) Home(ctx context.Context) string { return "Hello, Muxt!" }
   ```

3. Generate and run:
   ```bash
   go generate && go run .
   ```

The `templates` variable must be package-level — Muxt finds it via static analysis.

## Template Syntax

Standard `http.ServeMux` pattern, optionally with a status code and method call:

```
[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]
```

Example: `"POST /user/{id} 201 CreateUser(ctx, id, form)"`

Supported parameters: `ctx`, `request`, `response`, path params, `form` (URL-encoded body), `multipart` (file uploads, including `*multipart.FileHeader` fields). Returns and errors flow through `TemplateData[R, T]`. Status codes can come from the template name, return values, or error types.

`TemplateRoutePaths` extends type safety to URLs: `{{$.Path.GetArticle 42}}` instead of hardcoded `href="/article/42"`. Change the route pattern, the compiler finds every stale reference.

## Commands

- `muxt generate` — generate `http.Handler` glue (writes `template_routes.go`)
- `muxt check` — type-check templates without generating (use in CI or editor save hooks)
- `muxt list-template-calls` / `muxt list-template-callers` — explore call sites and callers

## Examples

- **[Local example](./docs/examples/simple/)** — complete application with tests ([pkg.go.dev](https://pkg.go.dev/github.com/typelate/muxt/docs/examples/simple/hypertext))
- **[Sortable Example](http://github.com/typelate/sortable-example)** — HTMX-enabled table row sorting
- **[HTMX Template](https://github.com/typelate/htmx-template)** — full HTMX integration patterns

The [command tests](./cmd/muxt/testdata) double as readable examples of every feature.

## Documentation

- **[Reference](docs/reference/)** — CLI, syntax, parameters, type checking
- **[Explanation](docs/explanation/)** — design philosophy, patterns, decisions

See the [full documentation index](docs/).

### Go Standard Library

- [html/template](https://pkg.go.dev/html/template) — template syntax, functions, escaping
- [net/http](https://pkg.go.dev/net/http) — `ServeMux` routing patterns, `Handler` interface
- [embed](https://pkg.go.dev/embed) — file embedding directives
- [Routing Enhancements for Go 1.22](https://go.dev/blog/routing-enhancements) — pattern syntax Muxt extends

## Using with AI Assistants

Claude Code skills for working with Muxt codebases:

| Skill | Use Case |
|-------|----------|
| [explore](docs/skills/muxt_explore/SKILL.md) | Trace through the template/method/route chain — pick a starting entry point (route, method, error, or fresh repo) |
| [test-driven-development](docs/skills/muxt_test-driven-development/SKILL.md) | Create new templates and receiver methods using TDD |
| [forms](docs/skills/muxt_forms/SKILL.md) | Form creation, struct binding, validation, accessible HTML |
| [debug-generation-errors](docs/skills/muxt_debug-generation-errors/SKILL.md) | Diagnose and fix `muxt generate` / `muxt check` errors |
| [refactoring](docs/skills/muxt_refactoring/SKILL.md) | Rename methods, change patterns, move templates safely |
| [htmx](docs/skills/muxt_htmx/SKILL.md) | Explore, develop, and test HTMX interactions |
| [integrate-existing-project](docs/skills/muxt_integrate-existing-project/SKILL.md) | Add Muxt to an existing Go web application |
| [sqlc](docs/skills/muxt_sqlc/SKILL.md) | Use Muxt with sqlc for type-safe SQL + HTML |
| [goland-gotype](docs/skills/muxt_goland-gotype/SKILL.md) | Add gotype comments for GoLand IDE support (GoLand-only) |
| [maintain-tools](docs/skills/muxt_maintain-tools/SKILL.md) | Install and update muxt, gofumpt, counterfeiter, and other tools |

Install as Claude Code skills:

```bash
for d in docs/skills/*/; do cp -r "$d" ~/.claude/skills/"$(basename "$d")"; done
```

## License

Muxt generator: [GNU AGPLv3](LICENSE)

Generated code: not covered by AGPL. Muxt asserts no copyright over its output — treat generated files as your own code, under whatever license your project uses.

**No warranty.** Muxt and its generated output are provided "as is", without warranty of any kind, express or implied, including but not limited to warranties of merchantability, fitness for a particular purpose, and non-infringement. You are responsible for reviewing, testing, and securing any code generated by Muxt before using it. In no event shall the authors or copyright holders be liable for any claim, damages, or other liability arising from the use of Muxt or its output.
