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

- **[Quick Start](tutorials/quick-start.md)** - Your first Muxt server
- **[Add Logging](tutorials/add-logging.md)** - Structured logging with `log/slog`

## Examples

- **[Hypertext Example](examples/simple)** - Full application with tests
- **[HTMX Helpers](examples/htmx-counter)** - HTMX integration code

---

Organized by [Diátaxis](https://diataxis.fr/): Reference (specs), Explanation (concepts). Task-oriented workflows are in AI Assistant Skills.
