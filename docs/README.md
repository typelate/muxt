# Muxt Documentation

The [README](../README.md) introduces muxt.

## Reading order

1. [Quick Start](tutorials/quick-start.md): generate and serve a first route.
2. [HTML is the API](explanation/html-is-the-api.md): the markup a handler writes is its contract, and what to assert on.
3. [Structure a project for testing](how-to/receiver-package-and-testing.md): put the receiver where a test can reach it, and choose what to fake.
4. [Find Untested Template Behavior](tutorials/find-untested-template-behavior.md): mutation testing shows which assertions are missing (needs a checkout of this repository).

## Tutorials

- [Quick Start](tutorials/quick-start.md)
- [Find Untested Template Behavior](tutorials/find-untested-template-behavior.md)
- [Add Logging](tutorials/add-logging.md): a `*slog.Logger` for generated handlers
- [Hot Reload with Air](tutorials/hot-reload-with-air.md)

## How-to guides

- [Structure a project for testing](how-to/receiver-package-and-testing.md)
- [Build a Datastar live view](how-to/datastar-live-view.md)
- [Extend TemplateData](how-to/extend-template-data.md): request-aware template helpers
- [Map domain errors to status codes](how-to/domain-error-status-codes.md)
- [Serve public and admin route sets](how-to/multiple-route-sets.md)
- [Add custom template functions](how-to/template-functions.md)

## Reference

- [CLI](reference/cli.md): commands and shared flags
  - [`muxt generate`](reference/commands/generate.md)
  - [`muxt check`](reference/commands/check.md)
  - [`muxt test-template-mutations`](reference/commands/test-template-mutations.md)
  - [`muxt list-template-callers`](reference/commands/list-template-callers.md)
  - [`muxt list-template-calls`](reference/commands/list-template-calls.md)
  - [`muxt explore-module`](reference/commands/explore-module.md)
  - [`muxt generate-fake-server`](reference/commands/generate-fake-server.md)
  - [`muxt version`](reference/commands/version.md)
- [Template Name Syntax](reference/template-names.md): method, host, path, status, call
- [Call Parameters](reference/call-parameters.md): what each argument binds and how it is parsed
- [Call Results](reference/call-results.md): what a method may return and how it renders
- [Templates Variable](reference/templates-variable.md): how muxt finds templates
- [Type Checking](reference/type-checking.md): what `muxt check` verifies and what it cannot
- [Package Layout](reference/package-layout.md)
- [Known Issues](reference/known-issues.md)

## Explanation

- [Manifesto](explanation/manifesto.md): the principles
- [Motivation](explanation/motivation.md): why muxt exists
- [HTML is the API](explanation/html-is-the-api.md)
- [Package Structure](explanation/package-structure.md): why the templates variable and `embed` shape the package
- [Decisions](explanation/decisions/)

## Examples

- [Simple](examples/simple): edit-in-place table with htmx
- [htmx Counter](examples/htmx-counter): `--output-htmx` header helpers
- [htmx TodoMVC](examples/htmx-todo): `execute` on a GET route, `{{.StatusCode 404}}` errors, `hx-swap-oob` footer
- [fixi + SSE Clock](examples/fixiproject-clock): Server-Sent Events
- [Datastar Counter](examples/datastar-counter): `--output-datastar` patch elements
- [Datastar Todo](examples/datastar-todo): patches, form binding, per-item actions
- [Datastar Search](examples/datastar-search): signals in, SSE patches out, plus a `marshalJSON` API
