# muxt check

Type-check templates without running them. Use in CI or during development to catch template errors early.

**Aliases:** `c`

```bash
muxt check --use-templates-variable=templates --verbose
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--use-templates-variable` | string | `templates` | Global `*template.Template` variable name to search for. |
| `--verbose`, `-v` | bool | `false` | Show each endpoint checked and success message. |

## Verbose Output

```
checking endpoint GET /users/{id}
checking endpoint POST /users
OK
```

## CI Usage

Add to your CI pipeline to catch template errors early:

```bash
muxt check --verbose
```

It verifies:
- Template names parse correctly as route patterns
- Method calls in template names reference valid methods
- Parameter types are compatible
- Template body field access is valid for method return types
- No unused templates or dead code outside `{{define}}` blocks

## Related

- [Type Checking](../type-checking.md) — How type checking works
- [muxt generate](generate.md) — Generate handlers from templates
- [Debug Generation Errors](../../skills/muxt_debug-generation-errors/SKILL.md) — Error reference table with all error categories and test files
