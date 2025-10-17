# CLI Reference

Complete specification for `muxt` command-line interface. Use during setup and CI configuration.

## Quick Reference

| Command | Purpose | Common Flags |
|---------|---------|--------------|
| `generate` | Generate HTTP handlers from templates | `--use-receiver-type`, `--output-routes-func-with-logger-param`, `--output-file` |
| `check` | Type-check templates without generating | `--use-receiver-type`, `--verbose` |
| `documentation` | Generate markdown docs from templates | Same as `generate` |
| `version` | Print muxt version | `-v, --verbose` |

## Commands

### `muxt generate`

Generates type-safe HTTP handlers from HTML templates.

**Aliases:** `gen`, `g`

**Output:**
- `template_routes.go` — Main file with shared types and route registration function
- `*_template_routes_gen.go` — Per-file handlers for each `.gohtml` source

```bash
muxt generate --use-receiver-type=App --output-routes-func-with-logger-param
```

#### Core Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--use-receiver-type` | string | _(none)_ | Type name for method lookup. Enables type-safe parameter parsing. **Recommended for production.** |
| `--use-receiver-type-package` | string | _(current pkg)_ | Package path for `--use-receiver-type`. Only needed if receiver is in different package. |
| `--output-file` | string | `template_routes.go` | Main generated file name. Per-file route files use pattern `*_template_routes_gen.go`. |
| `--use-templates-variable` | string | `templates` | Global `*template.Template` variable name to search for. |

**Type resolution:**
- **Without** `--use-receiver-type`: Parameters are `string`, return types are `any`
- **With** `--use-receiver-type`: Muxt looks up actual method signatures, generates type parsers

[templates-variable.md](templates-variable.md) — Template variable requirements

#### Naming Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output-routes-func` | string | `TemplateRoutes` | Generated route registration function name. |
| `--output-receiver-interface` | string | `RoutesReceiver` | Generated receiver interface name. |
| `--output-template-data-type` | string | `TemplateData` | Template context type name (generic). |
| `--output-template-route-paths-type` | string | `TemplateRoutePaths` | Path helper methods type name. |

#### Feature Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output-routes-func-with-logger-param` | bool | `false` | Add `*slog.Logger` parameter. Logs requests (debug) and template errors (error). |
| `--output-routes-func-with-path-prefix-param` | bool | `false` | Add `pathPrefix string` parameter for mounting under subpaths. |
| ~~`--logger`~~ | bool | `false` | **Deprecated.** Use `--output-routes-func-with-logger-param` instead. |
| ~~`--path-prefix`~~ | bool | `false` | **Deprecated.** Use `--output-routes-func-with-path-prefix-param` instead. |

**With `--output-routes-func-with-logger-param`:**
```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, logger *slog.Logger) TemplateRoutePaths
```

**With `--output-routes-func-with-path-prefix-param`:**
```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, pathPrefix string) TemplateRoutePaths
```

**With both:**
```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, logger *slog.Logger, pathPrefix string) TemplateRoutePaths
```

[How to Add Logging](../how-to/add-logging.md)

### `muxt check`

Type-check templates without generating code. Use in CI or during development.

**Aliases:** `c`, `typelate`

```bash
muxt check --use-receiver-type=App --verbose
```

#### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--verbose`, `-v` | bool | `false` | Show each endpoint checked and success message. |

**Verbose output:**
```
checking endpoint GET /users/{id}
checking endpoint POST /users
OK
```

**Other flags:** Same as `muxt generate` except `--output-file` (no code generated)

[type-checking.md](type-checking.md) — How type checking works

---

### `muxt documentation`

Generate markdown API documentation from templates.

**Aliases:** `docs`, `d`

```bash
muxt documentation --use-receiver-type=App
```

**Flags:** Same as `muxt generate`

---

### `muxt version`

Print muxt version. Use `-v` for verbose output including Go version.

**Alias:** `v`

```bash
muxt version
muxt version -v  # Shows Go version used to compile muxt
```

#### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-v, --verbose` | bool | `false` | Show Go version used to compile muxt. |

---

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-C` | string | _(current dir)_ | Change directory before running command. |

```bash
muxt -C ./web generate --use-receiver-type=Server
```

---

## Common Patterns

**Production setup:**
```go
//go:embed *.gohtml
var templateFS embed.FS

//go:generate muxt generate --use-receiver-type=Server --output-routes-func-with-logger-param
var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))
```

**Run generation:**
```bash
go generate ./...
```

**CI type checking:**
```bash
muxt check --use-receiver-type=App --verbose
```

**Custom naming:**
```bash
muxt generate \
  --use-receiver-type=App \
  --output-routes-func=RegisterRoutes \
  --output-receiver-interface=Handler \
  --output-file=routes.go
```

**Mount under subpath:**
```bash
muxt generate --use-receiver-type=App --output-routes-func-with-path-prefix-param
```

Then use:
```go
Routes(mux, app, "/api/v1")
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| Non-zero | Error (message on stderr) |

---

## Related

- [templates-variable.md](templates-variable.md) — Template variable setup requirements
- [template-names.md](template-names.md) — Template naming syntax
- [type-checking.md](type-checking.md) — Type checking behavior
- [How to Add Logging](../how-to/add-logging.md) — Using `--output-routes-func-with-logger-param` flag
