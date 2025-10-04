# CLI Reference

Complete reference for the `muxt` command-line interface.

## Commands

### `muxt generate`

Generates Go code from HTML templates.

**Aliases**: `gen`, `g`

**Usage**:
```bash
muxt generate [flags]
```

**Example**:
```bash
muxt generate --receiver-type=App --logger --output-file=routes.go
```

#### Flags

##### `--output-file`
**Type**: `string`
**Default**: `template_routes.go`

The generated file name containing the routes function and receiver interface.

```bash
muxt generate --output-file=routes.go
```

##### `--templates-variable`
**Type**: `string`
**Default**: `templates`

The name of the global variable with type `*html/template.Template` in the working directory package.

```bash
muxt generate --templates-variable=myTemplates
```

##### `--routes-func`
**Type**: `string`
**Default**: `TemplateRoutes`

The function name for the package registering handler functions on an `*http.ServeMux`.

```bash
muxt generate --routes-func=RegisterRoutes
```

##### `--receiver-type`
**Type**: `string`
**Default**: _(none)_

The type name for a named type to use for looking up method signatures. If not set, all methods added to the receiver interface will have inferred signatures with argument types based on the argument identifier names.

```bash
muxt generate --receiver-type=App
```

##### `--receiver-type-package`
**Type**: `string`
**Default**: _(current directory package)_

The package path to use when looking for `receiver-type`. If not set, the package in the current directory is used.

```bash
muxt generate --receiver-type=App --receiver-type-package=github.com/user/app/internal
```

##### `--receiver-interface`
**Type**: `string`
**Default**: `RoutesReceiver`

The interface name in the generated output file listing the methods used by the handler routes in routes-func.

```bash
muxt generate --receiver-interface=Handler
```

##### `--template-data-type`
**Type**: `string`
**Default**: `TemplateData`

The type name for the template data passed to root route templates.

```bash
muxt generate --template-data-type=PageData
```

##### `--template-route-paths-type`
**Type**: `string`
**Default**: `TemplateRoutePaths`

The type name for the type with path constructor helper methods.

```bash
muxt generate --template-route-paths-type=Paths
```

##### `--path-prefix`
**Type**: `bool`
**Default**: `false`

Adds a `path-prefix` parameter to the TemplateRoutes function and uses it in each path generator method.

```bash
muxt generate --path-prefix
```

**Generated signature**:
```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, pathsPrefix string) TemplateRoutePaths
```

##### `--logger`
**Type**: `bool`
**Default**: `false`

Adds a `*slog.Logger` parameter to the TemplateRoutes function and uses it to log ExecuteTemplate errors and debug information in handlers.

```bash
muxt generate --logger
```

**Generated signature**:
```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, logger *slog.Logger) TemplateRoutePaths
```

**Logging behavior**:
- **Debug level**: Logs each request with pattern, path, and method
- **Error level**: Logs template execution failures with error details

See [How to Add Logging](../how-to/add-logging.md) for usage examples.

---

### `muxt check`

Runs type checking on templates without generating code.

**Aliases**: `c`, `typelate`

**Usage**:
```bash
muxt check [flags]
```

**Examples**:
```bash
muxt check --receiver-type=App
muxt check --verbose
muxt check -v
```

#### Flags

##### `--verbose` / `-v`
**Type**: `bool`
**Default**: `false`

Enable verbose output showing each endpoint being checked and a success message when complete.

```bash
muxt check --verbose
muxt check -v
```

**Output**:
```
checking endpoint GET /users/{id}
checking endpoint POST /users
OK
```

**Other flags**: Same as `muxt generate` (except `--output-file`)

---

### `muxt documentation`

Generates markdown documentation from templates.

**Aliases**: `docs`, `d`

**Usage**:
```bash
muxt documentation [flags]
```

**Flags**: Same as `muxt generate`

---

### `muxt version`

Prints the muxt version.

**Alias**: `v`

**Usage**:
```bash
muxt version
```

---

### `muxt help`

Displays help information.

**Usage**:
```bash
muxt help
```

---

## Global Flags

### `-C`
**Type**: `string`
**Default**: _(current directory)_

Change root directory before running the command.

```bash
muxt -C ./myapp generate --receiver-type=App
```

---

## Examples

### Basic Generation

```bash
muxt generate
```

Generates `template_routes.go` with:
- Routes function: `TemplateRoutes`
- Templates variable: `templates`
- Receiver interface: `RoutesReceiver`

### With Receiver Type

```bash
muxt generate --receiver-type=App
```

Looks up method signatures on the `App` type.

### With Logging

```bash
muxt generate --receiver-type=App --logger
```

Adds structured logging to generated handlers.

### Custom Names

```bash
muxt generate \
  --routes-func=RegisterRoutes \
  --receiver-interface=Handler \
  --template-data-type=PageData \
  --output-file=routes.go
```

### From Different Directory

```bash
muxt -C ./web generate --receiver-type=Server
```

Runs generation in the `./web` directory.

### Multiple Flags

```bash
muxt generate \
  --receiver-type=App \
  --logger \
  --path-prefix \
  --output-file=routes.go
```

---

## Using with `go:generate`

Add a directive to your Go file:

```go
//go:generate muxt generate --receiver-type=App --logger
```

Run generation:

```bash
go generate ./...
```

---

## Exit Codes

- `0` - Success
- Non-zero - Error (message written to stderr)

---

## Related

- [Template Name Syntax](template-names.md) - Template naming conventions
- [How to Add Logging](../how-to/add-logging.md) - Using the `--logger` flag
- [How to Integrate Muxt](../how-to/integrate-existing-project.md) - Setting up generation
