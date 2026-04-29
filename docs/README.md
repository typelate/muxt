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
- **[HTMX Helpers](examples/counter-htmx)** - HTMX integration code

## AI Assistant Skills

Claude Code skills for working with Muxt:

- **[explore](skills/muxt_explore/SKILL.md)** - Trace through the template/method/route chain — pick a starting entry point (route, method, error, or fresh repo)
- **[template-driven-development](skills/muxt_test-driven-development/SKILL.md)** - Create new templates and methods using TDD
- **[forms](skills/muxt_forms/SKILL.md)** - Form creation, struct binding, validation, and accessible form HTML
- **[debug-generation-errors](skills/muxt_debug-generation-errors/SKILL.md)** - Diagnose and fix `muxt generate` / `muxt check` errors
- **[refactoring](skills/muxt_refactoring/SKILL.md)** - Rename methods, change patterns, move templates safely
- **[htmx](skills/muxt_htmx/SKILL.md)** - Explore, develop, and test HTMX interactions
- **[integrate-existing-project](skills/muxt_integrate-existing-project/SKILL.md)** - Add Muxt to an existing Go web application
- **[sqlc](skills/muxt_sqlc/SKILL.md)** - Use Muxt with sqlc for type-safe SQL + HTML
- **[goland-gotype](skills/muxt_goland-gotype/SKILL.md)** - Add gotype comments for GoLand IDE support (GoLand-only)
- **[maintain-tools](skills/muxt_maintain-tools/SKILL.md)** - Install and update muxt, gofumpt, counterfeiter, and other tools

---

Organized by [Diátaxis](https://diataxis.fr/): Reference (specs), Explanation (concepts). Task-oriented workflows are in AI Assistant Skills.
