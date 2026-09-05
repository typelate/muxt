# Muxt Documentation

Type-safe HTTP handlers from Go HTML templates.

## How-to guides

- **[Structure a project for testing](how-to/receiver-package-and-testing.md)** - Receiver in a library package, sqlc storage, httptest through the generated routes, fake the service layer
- **[Build a Datastar live view](how-to/datastar-live-view.md)** - Element patches, signals both directions, .Path inside Datastar attributes
- **[Extend TemplateData](how-to/extend-template-data.md)** - Request-aware helpers: authorization checks, protocol detection
- **[Map domain errors to status codes](how-to/domain-error-status-codes.md)** - StatusCode() on domain errors, applied at render time
- **[Serve public and admin route sets](how-to/multiple-route-sets.md)** - Separate receivers, interfaces, and middleware per audience
- **[Add custom template functions](how-to/template-functions.md)** - Pure formatters registered before ParseFS

## Reference

- **[CLI Overview](reference/cli.md)** - Commands and flags
  - [`muxt generate`](reference/commands/generate.md) - Generate HTTP handlers
  - [`muxt check`](reference/commands/check.md) - Type-check templates
  - [`muxt version`](reference/commands/version.md) - Print version
  - [`muxt list-template-callers`](reference/commands/list-template-callers.md) - List callers
  - [`muxt list-template-calls`](reference/commands/list-template-calls.md) - List call sites
  - [`muxt explore-module`](reference/commands/explore-module.md) - List muxt packages
  - [`muxt generate-fake-server`](reference/commands/generate-fake-server.md) - Fake server for exploring routes
- **[Template Name Syntax](reference/template-names.md)** - Route naming syntax
- **[Call Parameters](reference/call-parameters.md)** - Method parameter parsing
- **[Call Results](reference/call-results.md)** - Return value handling
- **[Templates Variable](reference/templates-variable.md)** - Code generation discovery
- **[Type Checking](reference/type-checking.md)** - Static analysis
- **[Known Issues](reference/known-issues.md)** - Limitations and workarounds
- **[Package Layout](reference/package-layout.md)** - Where files live in a production muxt package

## Explanation

- **[Manifesto](explanation/manifesto.md)** - Core principles
- **[Motivation](explanation/motivation.md)** - Why Muxt exists
- **[Complexity is the Enemy](explanation/complexity-is-the-enemy.md)**
- **[Package Structure](explanation/package-structure.md)**
- **[Architecture Decisions](explanation/decisions/)**

## Tutorials

- **[Quick Start](tutorials/quick-start.md)** - Your first Muxt server
- **[Add Logging](tutorials/add-logging.md)** - Structured logging with `log/slog`
- **[Hot Reload with Air](tutorials/hot-reload-with-air.md)** - Regenerate, rebuild, and restart on save

## Examples

- **[Hypertext Example](examples/simple)** - Full application with tests
- **[HTMX Helpers](examples/htmx-counter)** - HTMX integration code
- **[HTMX TodoMVC](examples/htmx-todo)** - TodoMVC with JSON persistence
- **[fixi + SSE Clock](examples/fixiproject-clock)** - Server-Sent Events streaming
- **[Datastar Counter](examples/datastar-counter)** - Datastar patch-elements over SSE
- **[Datastar Todo](examples/datastar-todo)** - Datastar with form binding and per-item actions
- **[Datastar Search](examples/datastar-search)** - Datastar signals decoded with unmarshalJSON(body), plus a marshalJSON API

---

Organized by [Diátaxis](https://diataxis.fr/): Tutorials (walkthroughs), How-to guides (task recipes), Reference (specs), and Explanation (concepts). The examples are runnable companions to the how-to guides.
