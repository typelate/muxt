# CLI Reference

Complete specification for `muxt` command-line interface. Use during setup and CI configuration.

## Quick Reference

| Command | Purpose | Common Flags |
|---------|---------|--------------|
| `generate` | Generate HTTP handlers from templates | `--use-receiver-type`, `--output-routes-func-with-logger-param`, `--output-file` |
| `check` | Type-check templates without generating | `--use-templates-variable`, `--verbose` |
| `list-template-callers` | List callers of a template | `--match`, `--format` |
| `list-template-calls` | List templates called by a template | `--match`, `--format` |
| `explore-module` | List every muxt package in the module | `--format` |
| `generate-fake-server` | Generate a fake-server `main.go` for exploring routes | `--output` |
| `version` | Print muxt version | `-v, --verbose` |
| _(no subcommand)_ | Print a routes overview for the working directory | `--format`, `--use-templates-variable`, `--use-receiver-type` |

## Flag Categories

Muxt flags fall into two categories:

- **Use Flags** (`--use-*`) — Specify what to use from your existing code
- **Output Flags** (`--output-*`) — Control the generated code

## Commands

### `muxt generate`

Generates type-safe HTTP handlers from HTML templates.

**Aliases:** `gen`, `g`

**Output:**
- Default (single-file mode): `template_routes.go` — All routes in one file
- With `--output-multiple-files`: `template_routes.go` + `*_template_routes_gen.go` per source file

```bash
muxt generate --use-receiver-type=App --output-routes-func-with-logger-param
```

Full flag tables (use, output, and deprecated flags), automatic file
cleanup, generated function signatures, and logging behavior:
[`muxt generate`](commands/generate.md). The command also accepts
`-v, --verbose` for debug output during generation.

---

### `muxt check`

Type-check templates without generating code. Use in CI or during development.

**Aliases:** `c`

```bash
muxt check --verbose
```

Flags, verbose output, and CI usage: [`muxt check`](commands/check.md).
[type-checking.md](type-checking.md) explains how type checking works.

---

### `muxt version`

Print muxt version. Use `-v` for verbose output including Go version.

**Aliases:** `v`

```bash
muxt version
muxt version -v  # Shows Go version used to compile muxt
```

#### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-v, --verbose` | bool | `false` | Show Go version used to compile muxt. |

---

### `muxt` (no subcommand)

Prints an overview of the working directory's package: its template routes (with method calls) and the registered template functions.

```bash
muxt
muxt --format json
```

**Flags:** `--format` (`text` or `json`), `--use-templates-variable`, `--use-receiver-type`, `--use-receiver-type-package`, `-v, --verbose`.

```
Template Routes:
  - GET /fruits/{id}/edit GetFormEditRow(id)
  - GET /{$} List(ctx)

Template Functions:
  - func printf(format string, a ...any) string
```

---

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-C, --change-directory` | string | _(current dir)_ | Change directory before running command. |

```bash
muxt -C ./web generate --use-receiver-type=Server
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| Non-zero | Error (message on stderr) |

---

## Command Reference Pages

- [`muxt generate`](commands/generate.md) — Full generate command reference
- [`muxt check`](commands/check.md) — Full check command reference
- [`muxt list-template-callers`](commands/list-template-callers.md) — List template callers
- [`muxt list-template-calls`](commands/list-template-calls.md) — List template call sites
- [`muxt explore-module`](commands/explore-module.md) — List every muxt package in the module
- [`muxt generate-fake-server`](commands/generate-fake-server.md) — Generate a fake server for exploring routes
- [`muxt version`](commands/version.md) — Version command reference

## Related

- [Template Name Syntax](template-names.md) — Route naming syntax
- [Call Parameters](call-parameters.md) — Method parameter parsing
- [Call Results](call-results.md) — Return value handling
- [Templates Variable](templates-variable.md) — Template variable requirements
- [Type Checking](type-checking.md) — Type checking behavior
- [Add Logging](../tutorials/add-logging.md) — Structured logging with `log/slog`
