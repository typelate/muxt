# muxt explore-module

Discover all muxt-generated packages in the current Go module. Shows configuration, commands, and external assets for each package.

**Aliases:** `explore`

```bash
muxt explore-module
muxt explore-module --format=json
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | `text` | Output format: `text` or `json`. |

## Output

For each muxt-generated package, shows:

- **Package path** and directory
- **Configuration** — routes function, receiver interface, receiver type, HTMX helpers, logger, path prefix
- **Commands** — ready-to-run `muxt` commands for listing routes, calls, callers, checking, and generating
- **External assets** — URLs found in `.gohtml` files (CDN links, external scripts)

## Examples

**Text overview:**
```bash
muxt explore-module
```

**Structured data for scripting:**
```bash
muxt explore-module --format=json
```

**List all template calls across packages:**
```bash
muxt explore-module --format=json | jq -r '.packages[].commands.calls' | sh
```

**Find HTMX-enabled packages:**
```bash
muxt explore-module --format=json | jq -r '.packages[] | select(.config.htmxHelpers) | .path'
```

**List external CDN assets:**
```bash
muxt explore-module --format=json | jq '.packages[].externalAssets[]'
```

## Related

- [muxt generate](generate.md) — Generate handlers from templates
- [muxt generate-fake-server](generate-fake-server.md) — Generate a fake server for interactive exploration
- [Explore](../../skills/muxt_explore/SKILL.md) — Skill for mapping an entire muxt codebase
