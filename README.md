# Muxt [![Go Reference](https://pkg.go.dev/badge/github.com/typelate/muxt.svg)](https://pkg.go.dev/github.com/typelate/muxt) [![Go](https://github.com/typelate/muxt/actions/workflows/go.yml/badge.svg)](https://github.com/typelate/muxt/actions/workflows/go.yml)

**Generate HTTP handlers from html/template definitions.**

Declare routes in template names using `http.ServeMux` patterns. Muxt analyzes receiver methods and generates handlers that parse parameters to match method signatures.

Muxt commands include:
- `muxt generate` to generate `http.Handler` glue (see `template_routes.go` for the output)
- `muxt` list template routes
- `muxt check` to find bugs and more safely refactor your `"text/template"` or `"html/template"` source code
- `muxt list-template-calls` list all call sites for a template (omit flag `--patterns` to list all call sites)
- `muxt list-template-callers` list all callers for a template (omit flag `--patterns` to list all callers for all templates)

## Syntax

Standard `http.ServeMux` pattern:
```
[METHOD ][HOST]/[PATH]
```

Muxt extends this with optional status codes and method calls:
```
[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]
```

For example, `"GET /article/{id} GetArticle(ctx, id)"` means: handle GET requests to `/article/{id}`, call `GetArticle` with the request context and the `id` path parameter, render the template with the result.

## How It Works

**Template names define the contract** between your HTML and your Go code. Without Muxt, this connection relies on **connascence of name** through raw strings: `templates.ExecuteTemplate(w, "user-profile", data)` uses a string that must match a template name, and `{{.Name}}` must match a field on whatever `data` happens to be. A typo in either place is a runtime error.

Muxt upgrades this to **connascence of type**. It uses `go/types` to verify at generation time that:

- The method named in the template (`GetArticle`) exists on the receiver type with the correct signature
- Path parameters (`id`) can be parsed to the method's parameter types (`string`, `int`, `bool`, custom `TextUnmarshaler`)
- The template body's field access (`.Result.Title`) is valid for the method's return type
- Form parameters (`form`) match a concrete struct type with the right fields
- Bind form data to struct fields with validation
- Inject request context, `*http.Request`, or `http.ResponseWriter` when named
- Handle errors and return values through `TemplateData[R, T]`
- Set HTTP status codes from template names, return values, or error types

If any of these are wrong, `go generate` or `muxt check` fails with a clear error pointing to the template. No runtime surprises.

`TemplateRoutePaths` extends this to URLs: instead of hardcoding `href="/article/42"`, templates use `{{$.Path.GetArticle 42}}`. If the route pattern changes, the generated method signature changes, and the compiler catches every stale reference.

**No (additional) runtime reflection.** All type checking happens at generation time. The generated code uses only `net/http` and `html/template` from the standard library.

## Installation

```bash
go install github.com/typelate/muxt@latest
```

Or add it to your project's module `go get -tool github.com/typelate/muxt` (note the project [license documentation](#License)).

## Quick Start

1. Create a template file `index.gohtml`:
```gotmpl
{{define "GET / Home(ctx)"}}
<!DOCTYPE html>
<html>
<body><h1>{{.Result}}</h1></body>
</html>
{{end}}
```

2. Add generation directives to `main.go`:
```go
//go:embed *.gohtml
var templateFS embed.FS

//go:generate muxt generate --use-receiver-type=Server
var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))

type Server struct{}

func (s Server) Home(ctx context.Context) string {
    return "Hello, Muxt!"
}
```

3. Generate handlers and run:
```bash
go generate && go run .
```

Key elements:
- `//go:embed *.gohtml` embeds template files into the binary
- `//go:generate muxt generate` tells `go generate` to run Muxt
- `--use-receiver-type=Server` tells Muxt to look up method signatures on `Server`
- The `templates` variable must be package-level (Muxt finds it via static analysis)

## Example

Define a template with a route pattern and method call:

```gotmpl
{{define "GET /{id} GetUser(ctx, id)"}}
  {{with $err := .Err}}
    <div class="error" data-type="{{printf `%T` $err}}">{{$err.Error}}</div>
  {{else}}
    <h1>{{.Result.Name}}</h1>
    <p>{{.Result.Email}}</p>
  {{end}}
{{end}}
```

Implement the receiver method:

```go
func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    return s.db.GetUser(ctx, id)  // id automatically parsed from string
}
```

Run `muxt generate --use-receiver-type=Server` to generate HTTP handlers.

## Examples

The [command tests](./cmd/muxt/testdata) were intended to be readable examples of muxt behavior.

- **[Local example](./docs/examples/simple/)** - Complete application with tests ([pkg.go.dev](https://pkg.go.dev/github.com/typelate/muxt/docs/examples/simple/hypertext))
- **[Sortable Example](http://github.com/typelate/sortable-example)** - Interactive HTMX-enabled table row sorting
- **[HTMX Template](https://github.com/typelate/htmx-template)** - Full HTMX integration patterns

## Documentation

- **[Reference](docs/reference/)** - CLI, syntax, parameters, type checking
- **[Explanation](docs/explanation/)** - Design philosophy, patterns, decisions

See the [full documentation index](docs/) for all available resources.

### Go Standard Library

- [html/template](https://pkg.go.dev/html/template) — Template syntax, functions, escaping
- [net/http](https://pkg.go.dev/net/http) — `ServeMux` routing patterns, `Handler` interface
- [embed](https://pkg.go.dev/embed) — File embedding directives
- [log/slog](https://pkg.go.dev/log/slog) — Structured logging (used by generated handlers)
- [Routing Enhancements for Go 1.22](https://go.dev/blog/routing-enhancements) — `ServeMux` pattern syntax that Muxt extends

## Using with AI Assistants

Claude Code skills for working with Muxt codebases:

| Skill | Use Case |
|-------|----------|
| [explore-from-route](docs/skills/muxt_explore-from-route/SKILL.md) | Trace from a URL path to its template and receiver method |
| [explore-from-method](docs/skills/muxt_explore-from-method/SKILL.md) | Find which routes and templates use a receiver method |
| [explore-from-error](docs/skills/muxt_explore-from-error/SKILL.md) | Trace an error message back to its handler and template |
| [explore-repo-overview](docs/skills/muxt_explore-repo-overview/SKILL.md) | Map all routes, templates, and the receiver type |
| [template-driven-development](docs/skills/muxt_test-driven-development/SKILL.md) | Create new templates and receiver methods using TDD |
| [forms](docs/skills/muxt_forms/SKILL.md) | Form creation, struct binding, validation, and accessible form HTML |
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

Generated code: [MIT License](https://choosealicense.com/licenses/mit/) - The Go code generated by Muxt is not covered by AGPL. It is provided as-is without warranty. Use it freely in your projects.

## Star History

[![Star History Chart](https://api.star-history.com/image?repos=typelate/muxt&type=date&legend=top-left)](https://www.star-history.com/?repos=typelate%2Fmuxt&type=date&legend=top-left)
