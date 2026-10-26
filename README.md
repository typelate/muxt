# Muxt [![Go Reference](https://pkg.go.dev/badge/github.com/typelate/muxt.svg)](https://pkg.go.dev/github.com/typelate/muxt)

Muxt is a command-line tool that generates `net/http` handlers from `html/template` names and type-checks your templates against the Go data they render. Two commands do most of the work: `generate` and `check`.

A template name defines the contract between the hypermedia and the `http.Handler`:

```gotmpl
{{define "GET /article/{id} GetArticle(ctx, id)"}}
  <h1>{{.Result.Title}}</h1>
{{end}}
```

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) { ... }
```

`muxt generate` reads the name and writes an `http.Handler` that calls your method.

`muxt check` type-checks a template against the data it receives. It checks each `ExecuteTemplate` call on the templates variable whose template name is a literal string and whose data has a statically known type. Generated handlers meet both conditions. Rename a field and the template that still uses it fails a static `muxt check`.

Muxt is built for web apps that ship as a single Go binary serving hypermedia from the standard library, with fixi, htmx, or Datastar added when the front end needs more interaction. It supplies the statically type-checked coupling between your Go code and HTML templates that the standard library leaves out.

## Install

```bash
go install github.com/typelate/muxt@latest
```

Or as a project tool: `go get -tool github.com/typelate/muxt`.

muxt loads Go packages the way the `go` command does, so it inherits `GOWORK`, `GOFLAGS`, and `GOROOT` from its environment.

## Template Name Syntax

This is what muxt uses to map `Go` `<->` `HTML`.

Standard `http.ServeMux` pattern, optionally with a status code and method call:

```
[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]
```

Example: `"POST /user/{id} 201 CreateUser(ctx, id, form)"`

Supported parameters: `ctx`, `request`, `response`, path params, `form` (URL-encoded body, also spelled `unmarshalForm(body)`), `multipart` (file uploads, including `*multipart.FileHeader` fields), `body` (raw `io.Reader`), `unmarshalJSON(body)` (JSON request bodies; `signals` is shorthand with `--output-datastar`), `execute` (render callback), and `lastEventID`. Returns and errors flow through `TemplateData[R, T]`. Status codes can come from the template name, return values, or error types.

Wrapping the call changes the response representation: `sse(Stream(ctx, execute))` streams Server-Sent Events and `marshalJSON(GetUser(ctx))` responds `application/json`. Generate flags select a frontend library per package — `--output-htmx` adds HX* helper methods to `TemplateData`, and `--output-datastar` frames SSE events with Datastar's patch-elements protocol.

`TemplateRoutePaths` extends type safety to URLs: `{{$.Path.GetArticle 42}}` instead of hardcoded `href="/article/42"`. `{{$.Route.GetArticle 42}}` also carries the HTTP method. Change a route's wildcards and `muxt check` reports each template that still passes the old arguments; Go callers fail to compile.

## Examples

- **[Local example](./docs/examples/simple/)**: complete application with tests
- **[Sortable Example](https://github.com/typelate/sortable-example)**: HTMX-enabled table row sorting
- **[HTMX Template](https://github.com/typelate/htmx-template)**: full HTMX integration patterns

The [command tests](./cmd/muxt/testdata) double as readable examples of every feature.

## Commands

- `muxt generate`: generate `http.Handler` glue (writes `template_routes.go`)
- `muxt check`: type-check templates without generating (use in CI or editor save hooks)
- `muxt list-template-calls` / `muxt list-template-callers`: explore call sites and callers
- `muxt test-template-mutations`: vary each template action and report the ones no test catches
- `muxt explore-module` / `muxt generate-fake-server`: survey muxt packages and spin up a fake server to poke at routes
- `muxt documentation [topic]`: print these docs from the installed binary

## Documentation

- **[Quick Start](docs/tutorials/quick-start.md)**: your first Muxt server
- **[Reference](docs/reference/)**: CLI, syntax, parameters, type checking
- **[Explanation](docs/explanation/)**: design philosophy, patterns, decisions

See the [full documentation index](docs/).

### Go Standard Library

- [html/template](https://pkg.go.dev/html/template): template syntax, functions, escaping
- [net/http](https://pkg.go.dev/net/http): `ServeMux` routing patterns, `Handler` interface
- [embed](https://pkg.go.dev/embed): file embedding directives
- [Routing Enhancements for Go 1.22](https://go.dev/blog/routing-enhancements): pattern syntax Muxt extends

## License

Muxt generator: [GNU AGPLv3](LICENSE)

Generated code: not covered by AGPL. Muxt asserts no copyright over its output. Treat generated files as your own code, under whatever license your project uses.

**No warranty.** Muxt and its generated output are provided "as is", without warranty of any kind, express or implied, including but not limited to warranties of merchantability, fitness for a particular purpose, and non-infringement. You are responsible for reviewing, testing, and securing any code generated by Muxt before using it. In no event shall the authors or copyright holders be liable for any claim, damages, or other liability arising from the use of Muxt or its output.
