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

Add to your CI pipeline to catch template-to-method mismatches before deployment:

```bash
muxt check --verbose
```

The check command validates:
- Template names parse correctly as route patterns
- Method calls in template names reference methods that exist on the receiver type (if `--use-receiver-type` is set on the `generate` command's `go:generate` directive)
- Parameter types are compatible

## Related

- [Type Checking](../type-checking.md) — How type checking works
- [muxt generate](generate.md) — Generate handlers from templates
