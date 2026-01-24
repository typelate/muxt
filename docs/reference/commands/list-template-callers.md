# muxt list-template-callers

For a given template, list all places that call it. Shows both Go code call sites (`ExecuteTemplate`) and template call sites (`{{template "X" .}}`).

**Aliases:** `callers`

```bash
muxt list-template-callers
muxt list-template-callers --match "GET /users"
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--use-templates-variable` | string[] | `templates` | Global `*template.Template` variable name(s) to search for. Pass multiple times to combine results across template sets. |
| `--match` | string[] | _(all)_ | Filter by template name. Can specify multiple regular expressions. Omit to list callers for all templates. |
| `--format` | string | `text` | Output format: `text` or `json`. |

## Examples

**List callers for all templates:**
```bash
muxt list-template-callers
```

**Filter by pattern:**
```bash
muxt list-template-callers --match "GET /users"
muxt list-template-callers --match "POST" --match "DELETE"
```

**JSON output:**
```bash
muxt list-template-callers --format json
```

## Related

- [muxt list-template-calls](list-template-calls.md) — List templates called by a template
- [muxt generate](generate.md) — Generate handlers from templates
