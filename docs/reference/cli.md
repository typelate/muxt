# CLI Reference

Complete specification for `muxt` command-line interface. Use during setup and CI configuration.

## Commands

| Command | Purpose | Docs |
|---------|---------|------|
| [`generate`](commands/generate.md) | Generate HTTP handlers from templates | [details](commands/generate.md) |
| [`check`](commands/check.md) | Type-check templates without generating | [details](commands/check.md) |
| [`version`](commands/version.md) | Print muxt version | [details](commands/version.md) |
| [`list-template-callers`](commands/list-template-callers.md) | List template callers | [details](commands/list-template-callers.md) |
| [`list-template-calls`](commands/list-template-calls.md) | List template call sites | [details](commands/list-template-calls.md) |

## Flag Categories

Muxt flags are organized into three clear categories:

- **Use Flags** (`--use-*`) — Specify what to use from your existing code
- **Output Flags** (`--output-*`) — Control names in generated code
- **Feature Flags** — Enable optional features

See [`muxt generate`](commands/generate.md) for the full flag reference.

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-C` | string | _(current dir)_ | Change directory before running command. |

```bash
muxt -C ./web generate --use-receiver-type=Server
```

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| Non-zero | Error (message on stderr) |

## Related

- [Template Name Syntax](template-names.md) — Route naming syntax
- [Call Parameters](call-parameters.md) — Method parameter parsing
- [Call Results](call-results.md) — Return value handling
- [Templates Variable](templates-variable.md) — Template variable requirements
- [Type Checking](type-checking.md) — Type checking behavior
- [How to Add Logging](../how-to/add-logging.md)
