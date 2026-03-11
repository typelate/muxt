# muxt list-template-calls

For a given template (or all templates), list all other templates called. It shows `{{template "Y" .}}` actions within each template.

**Aliases:** `calls`

```bash
muxt list-template-calls
muxt list-template-calls --match "header"
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--use-templates-variable` | string | `templates` | Global `*template.Template` variable name to search for. |
| `--match` | string[] | _(all)_ | Filter by template name. Can specify multiple regular expressions. Omit to list calls for all templates. |
| `--format` | string | `text` | Output format: `text` or `json`. |

## Examples

**List calls for all templates:**
```bash
muxt list-template-calls
```

**Filter by pattern:**
```bash
muxt list-template-calls --match "header"
muxt list-template-calls --match "nav" --match "footer"
```

**JSON output:**
```bash
muxt list-template-calls --format json
```

## Related

- [muxt list-template-callers](list-template-callers.md) — List places that call a template
- [muxt generate](generate.md) — Generate handlers from templates
