# Muxt Documentation

Type-safe HTTP handlers from Go HTML templates.

## Reference

- **[CLI Overview](reference/cli.md)** - Commands and flags
  - [`muxt generate`](reference/commands/generate.md) - Generate HTTP handlers
  - [`muxt check`](reference/commands/check.md) - Type-check templates
  - [`muxt version`](reference/commands/version.md) - Print version
  - [`muxt list-template-callers`](reference/commands/list-template-callers.md) - List callers
  - [`muxt list-template-calls`](reference/commands/list-template-calls.md) - List call sites
- **[Template Name Syntax](reference/template-names.md)** - Route naming syntax
- **[Call Parameters](reference/call-parameters.md)** - Method parameter parsing
- **[Call Results](reference/call-results.md)** - Return value handling
- **[Templates Variable](reference/templates-variable.md)** - Code generation discovery
- **[Type Checking](reference/type-checking.md)** - Static analysis
- **[Known Issues](reference/known-issues.md)** - Limitations and workarounds

## Explanation

- **[Manifesto](explanation/manifesto.md)** - Core principles
- **[Motivation](explanation/motivation.md)** - Why Muxt exists
- **[Complexity is the Enemy](explanation/complexity-is-the-enemy.md)**
- **[Go Proverbs and Muxt](explanation/go-proverbs-and-muxt.md)**
- **[Advanced Patterns](explanation/advanced-patterns.md)**
- **[Package Structure](explanation/package-structure.md)**
- **[Architecture Decisions](explanation/decisions/)**

## Tutorials

- **[Add Logging](tutorials/add-logging.md)** - Structured logging with `log/slog`

## Examples

- **[Hypertext Example](examples/simple)** - Full application with tests
- **[HTMX Helpers](examples/htmx)** - HTMX integration code

## AI Assistant Skills

Claude Code skills for working with Muxt:

- **[explore-from-route.md](skills/explore-from-route.md)** - Trace from a URL path to its template and receiver method
- **[explore-from-method.md](skills/explore-from-method.md)** - Find which routes and templates use a receiver method
- **[explore-from-error.md](skills/explore-from-error.md)** - Trace an error message back to its handler and template
- **[explore-repo-overview.md](skills/explore-repo-overview.md)** - Map all routes, templates, and the receiver type
- **[template-driven-development.md](skills/template-driven-development.md)** - Create new templates and methods using TDD
- **[forms.md](skills/forms.md)** - Form creation, struct binding, validation, and accessible form HTML
- **[debug-generation-errors.md](skills/debug-generation-errors.md)** - Diagnose and fix `muxt generate` / `muxt check` errors
- **[refactoring.md](skills/refactoring.md)** - Rename methods, change patterns, move templates safely
- **[htmx.md](skills/htmx.md)** - Explore, develop, and test HTMX interactions
- **[integrate-existing-project.md](skills/integrate-existing-project.md)** - Add Muxt to an existing Go web application
- **[sqlc.md](skills/sqlc.md)** - Use Muxt with sqlc for type-safe SQL + HTML
- **[goland-gotype.md](skills/goland-gotype.md)** - Add gotype comments for GoLand IDE support (GoLand-only)

---

Organized by [Diátaxis](https://diataxis.fr/): Reference (specs), Explanation (concepts). Task-oriented workflows are in AI Assistant Skills.
