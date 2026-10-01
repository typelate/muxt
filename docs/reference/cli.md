# CLI Reference

`muxt` loads the working directory's package like the `go` command, honoring `GOWORK`, `GOFLAGS`, and `GOROOT`. Failures print to stderr and exit `1`.

## Commands

| Command | Aliases | Does |
|---|---|---|
| [`generate`](commands/generate.md) | `gen`, `g` | Write the routes file. |
| [`check`](commands/check.md) | `c` | Type-check each template against the data it receives. |
| [`list-template-callers`](commands/list-template-callers.md) | `callers` | List what renders each template. |
| [`list-template-calls`](commands/list-template-calls.md) | `calls` | List the `{{template}}` calls each template makes. |
| [`explore-module`](commands/explore-module.md) | `explore` | List every muxt package in the module. |
| [`generate-fake-server`](commands/generate-fake-server.md) | | Write a fake server for browsing routes. |
| [`test-template-mutations`](commands/test-template-mutations.md) | | Report template actions no test catches. |
| [`version`](commands/version.md) | `v` | Print the version. |
| `completion` | | Print a shell completion script for `bash`, `fish`, `powershell`, or `zsh`. |

`muxt` with no subcommand prints the package's routes and the functions registered with `Funcs`; `--use-receiver-type` adds the receiver's methods:

```text
Template Routes:
  - GET /{$} List(ctx)

Template Functions:
```

## Flags

`--use-*` flags name code muxt reads. `--output-*` flags shape code `generate` writes. Single-command flags (`--match`, `-o`, the mutation flags) are on that command's page.

| Flag | Default | Description |
|---|---|---|
| `-C, --change-directory` | | Change directory before running. Accepted anywhere on the line. |
| `--use-templates-variable` | `templates` | Package-level `*template.Template` variable. Repeatable ([templates-variable.md](templates-variable.md#multiple-template-variables)). Not accepted by `explore-module`, `generate-fake-server`, or `version`. |
| `--use-receiver-type` | | Type whose method signatures give each call its parameter and result types ([type resolution](call-parameters.md#type-resolution)). Accepted by the root command and `generate`; `check` reads it from the generated file. |
| `--use-receiver-type-package` | current package | Import path that holds `--use-receiver-type`. Accepted by the root command and `generate`. |
| `--format` | `text` | `text` or `json`. Accepted by the root command, `explore-module`, the list commands, and `test-template-mutations`. |
| `-v, --verbose` | `false` | More output; each command page says what. Accepted by the root command, `generate`, `check`, `version`, and `test-template-mutations`. |

```bash
muxt -C ./web generate --use-receiver-type=Server
```

### `generate` output flags

| Flag | Default | Description |
|---|---|---|
| `--output-file` | `template_routes.go` | File to write. |
| `--output-routes-func` | `TemplateRoutes` | Function that registers handlers on an `*http.ServeMux`. |
| `--output-receiver-interface` | `RoutesReceiver` | Interface listing the methods the routes call. |
| `--output-template-data-type` | `TemplateData` | Type passed to route templates. |
| `--output-sse-template-data-type` | `SSETemplateData` | Type passed to Server-Sent Events route templates. |
| `--output-template-route-paths-type` | `TemplateRoutePaths` | Type whose methods build route paths. |
| `--output-template-route-type` | `TemplateRoute` | Type those methods return. Name it differently in each file generated into one package ([reference_multiple_generated_routes.txt](../../cmd/muxt/testdata/reference_multiple_generated_routes.txt)). |
| `--output-routes-func-with-logger-param` | `false` | Add a `*slog.Logger` parameter. |
| `--output-routes-func-with-path-prefix-param` | `false` | Add a `pathsPrefix string` parameter. |
| `--output-routes-func-with-middleware-param` | `false` | Add a `middleware func(http.Handler) http.Handler` parameter. |
| `--output-multiple-files` | `false` | Write one generated file per template source file. |
| `--output-multipart-max-memory` | `32 MiB` | Memory limit passed to `ParseMultipartForm` for `multipart` arguments. Accepts `32MB`, `64MiB`, `1GB`. |
| `--output-htmx` | `false` | Add HTMX request and response header helpers to the template data type ([reference_output_htmx.txt](../../cmd/muxt/testdata/reference_output_htmx.txt)), and `HTMX()` to `TemplateRoute` to build `hx-*` attributes: `{{((.Path.Delete 7).HTMX.Target "#row").Attributes}}` ([reference_output_htmx_template_route.txt](../../cmd/muxt/testdata/reference_output_htmx_template_route.txt)). |
| `--output-datastar` | `false` | Frame Server-Sent Events as Datastar `datastar-patch-elements` events and enable `signals` ([Server-Sent Events](call-parameters.md#server-sent-events)). Excludes `--output-htmx`. |
| `--output-exported-default-identifiers` | `true` | `false` makes the default names above unexported. Explicit `--output-*` names are used as given. |
| `--output-muxt-version` | `true` | `false` omits the `// muxt version:` header and the `MuxtVersion` method. |

### Deprecated flags

Each still works and prints `Flag --old has been deprecated, use --new instead`.

| Old | New |
|---|---|
| `--templates-variable`, `--find-templates-variable` | `--use-templates-variable` |
| `--receiver-type`, `--find-receiver-type` | `--use-receiver-type` |
| `--receiver-type-package`, `--find-receiver-type-package` | `--use-receiver-type-package` |
| `--receiver-interface` | `--output-receiver-interface` |
| `--routes-func` | `--output-routes-func` |
| `--template-data-type` | `--output-template-data-type` |
| `--template-route-paths-type` | `--output-template-route-paths-type` |
| `--logger` | `--output-routes-func-with-logger-param` |
| `--path-prefix` | `--output-routes-func-with-path-prefix-param` |
| `--output-htmx-helpers` | `--output-htmx` |
