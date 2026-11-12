# Muxt Documentation

These pages document muxt in four kinds: tutorials, how-to guides, reference and explanation. Start with the [README](../README.md) if you have not met muxt.

## Reading order

1. [Quick Start](tutorials/quick-start.md): generate and serve a first route.
2. [HTML is the API](explanation/html-is-the-api.md): the markup a handler writes is its contract, and what to assert on.
3. [Structure a project for testing](how-to/receiver-package-and-testing.md): put the receiver where a test can reach it, and choose what to fake.
4. [Find Untested Template Behavior](tutorials/find-untested-template-behavior.md): mutation testing shows which assertions are missing (needs a checkout of this repository).

## Tutorials

- [Quick Start](tutorials/quick-start.md)
- [Find Untested Template Behavior](tutorials/find-untested-template-behavior.md): mutation testing
- [Add Logging](tutorials/add-logging.md): a `*slog.Logger` for generated handlers
- [Hot Reload with Air](tutorials/hot-reload-with-air.md): regenerate on save

## How-to guides

- [Structure a project for testing](how-to/receiver-package-and-testing.md)
- [Build a Datastar live view](how-to/datastar-live-view.md)
- [Serve full pages and fragments from one route](how-to/full-page-and-fragments.md): layout partials, `.HXRequest`, `Vary`
- [Handle expired sessions in htmx requests](how-to/expired-sessions.md): middleware, heartbeats, a lock dialog
- [Extend TemplateData](how-to/extend-template-data.md): request-aware template helpers
- [Keep permissions in one place](how-to/permissions-in-one-place.md): enforce in the method or middleware, show through a field or a TemplateData method
- [Map domain errors to status codes](how-to/domain-error-status-codes.md)
- [Serve public and admin route sets](how-to/multiple-route-sets.md)
- [Add custom template functions](how-to/template-functions.md): register `Funcs` so `muxt check` sees them
- [Keep part of a template as written](how-to/keep-template-formatting.md): `muxt:fmt` directives
- [Move generate flags into muxt.json](how-to/muxt-json.md): one file and one `go:generate` line for many packages
- [Share template fragments between packages](how-to/shared-template-fragments.md): one layout and one set of partials for several sub-packages

## Reference

- [CLI](reference/cli.md): commands and shared flags
  - [`muxt generate`](reference/commands/generate.md)
  - [`muxt check`](reference/commands/check.md)
  - [`muxt fmt`](reference/commands/fmt.md)
  - [`muxt test-template-mutations`](reference/commands/test-template-mutations.md)
  - [`muxt list-template-callers`](reference/commands/list-template-callers.md)
  - [`muxt list-template-calls`](reference/commands/list-template-calls.md)
  - [`muxt explore-module`](reference/commands/explore-module.md)
  - [`muxt generate-fake-server`](reference/commands/generate-fake-server.md)
  - [`muxt version`](reference/commands/version.md)
  - [`muxt documentation`](reference/commands/documentation.md)
- [Template Name Syntax](reference/template-names.md): method, host, path, status, call
- [Call Parameters](reference/call-parameters.md): what each argument binds and how it is parsed
- [Call Results](reference/call-results.md): what a method may return and how it renders
- [Templates Variable](reference/templates-variable.md): how muxt finds templates
- [muxt.json](reference/muxt-json.md): generate settings for several packages in one file
- [Type Checking](reference/type-checking.md): what `muxt check` verifies and what it cannot
- [Template Comments](reference/template-comments.md): which comments reach the response, and why to use `{{/* */}}`
- [Package Layout](reference/package-layout.md): a full package
- [Template Engine](reference/template-engine.md): the public `template` packages, versioned with the CLI
- [Known Issues](reference/known-issues.md): generation errors and their fixes

## Explanation

- [Manifesto](explanation/manifesto.md): the principles
- [Motivation](explanation/motivation.md): why muxt exists
- [HTML is the API](explanation/html-is-the-api.md): why tests assert on markup
- [Package Structure](explanation/package-structure.md): why the templates variable and `embed` shape the package
- [Decisions](explanation/decisions/): numbered records

## Examples

- [Simple](examples/simple): edit-in-place table with htmx
- [htmx Counter](examples/htmx-counter): `--output-htmx` header helpers
- [htmx TodoMVC](examples/htmx-todo): `execute` on a GET route, `{{.StatusCode 404}}` errors, `hx-swap-oob` footer
- [fixi + SSE Clock](examples/fixiproject-clock): Server-Sent Events
- [Datastar Counter](examples/datastar-counter): `--output-datastar` patch elements
- [Datastar Todo](examples/datastar-todo): patches, form binding, per-item actions
- [Datastar Search](examples/datastar-search): signals in, SSE patches out, plus a `marshalJSON` API
