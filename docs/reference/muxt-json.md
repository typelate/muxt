# muxt.json

`muxt.json` holds the `muxt generate` flags of each package. Each package's settings are an array of **entries**, the objects under its `packages` key, one per `muxt generate` run. The file is found by walking up from the working directory; the walk stops at the directory holding `go.mod`, and the file needs a `go.mod` at or above it. A module's `muxt.json` files do not nest. A command fails, naming both files, when it meets one below another in the same module: a run at or below the lower file, a run at the upper file for an entry whose directory is at or below the lower file, or `muxt generate config` with a file already below. Files in sibling directories, and in a nested module, are fine.

```json
{
	"package_defaults": {"output_htmx": true},
	"packages": {
		"example.com/m/workspace": [{"use_receiver_type": "T"}]
	}
}
```

- `packages` keys are import paths of packages at or below the file's directory. Two keys that reach one directory, such as through a symbolic link, are an error to a command that reads every entry (the table below).
- The value of a key is always an array, also for one entry. An empty array, an element that is not an object, and `null` are errors.
- Entry keys are the `generate` flag names without `--`, dashes written `_`: [flags](cli.md#flags). `verbose` is not a key.
- A boolean flag is a JSON boolean, a string flag a JSON string, `use_templates_variable` a list of strings, and `output_multipart_max_memory` a string such as `"64MB"`.
- `muxt generate package` rewrites the file in one sorted form, tab-indented; a hand-written order and spacing do not survive it.
- A setting resolves from the entry, then `package_defaults`, then the flag default.

| Command | Reads |
|---|---|
| `muxt generate`, no flags | every entry in the file's directory, dependencies first, and `route_paths` when present; the directory's entry elsewhere |
| `muxt check`, `list-template-calls`, `list-template-callers`, no flags | `use_templates_variable` of every entry in the file's directory, dependencies first; of the directory's entry elsewhere |
| `muxt test`, no flags | only the entries' directories: every entry in the file's directory, the directory's entry elsewhere |
| `muxt test-template-mutations`, no flags | `use_templates_variable` of the directory's entry; in the file's directory with no entry for it, an error naming the package directories |
| `muxt` | all of it, shown by source under `Configuration:` |
| `muxt generate def` | the directory's entry, or its entries when they agree on the receiver type, and `--output-file` may be a file any of them reads; in the file's directory with no entry and no templates variable, an error naming the package directories |
| `muxt explore-module`, `generate-fake-server` | only whether each package has an entry; settings come from the generated files' headers; with `route_paths` they show the hub's paths type for a listed package |

`muxt fmt` does not read the file.

In the file's directory, a package with a templates variable and no entry is neither generated nor checked.

Only a flagless `muxt generate` needs an entry. In a directory with none, the other commands use their own flags and defaults.

`muxt generate config` creates the file, `muxt generate package NAME` adds an entry, `muxt generate def NAME` adds a route template.

## Several entries in one package

Each entry is one `muxt generate` run, so one package holds several route sets ([steps](../how-to/multiple-route-sets.md)). `package_defaults` applies to each entry. A position in an error is always indexed, also for one entry: `packages["KEY"][0]`. Output names an entry `KEY`, or `KEY (FILE)` for a key with several, `FILE` being its `output_file`.

Two generated files share one package, so entries that share a name would write one file or declare one type twice. Muxt compares the resolved values, so a default counts: two entries that leave `output_file` unset both have `template_routes.go`. Set these in every entry of a key with several:

- `output_file`
- `output_routes_func`
- `output_receiver_interface`
- `output_template_data_type`
- `output_template_route_paths_type`
- `output_sse_template_data_type`, when both have `sse` routes; muxt does not compare it, and `go build` reports the redeclared type

A `package_defaults` that sets one of the first five gives every entry the same value, and the check fails.

The first of the first five names that two entries share, in the order above, fails the command and nothing is written. With `output_multiple_files`, two entries that would write one file fail too.

| Rule | Error |
|---|---|
| One value in two entries | `/home/me/app/muxt.json: packages["example.com/m"][0] and packages["example.com/m"][1] both have output_file "template_routes.go"; give each its own output_file` |
| `route_paths` with several entries | `/home/me/app/muxt.json: route_paths takes one entry per package, and packages["example.com/m"] has 2; keep one entry or remove route_paths` |
| `output_multiple_files` writes one file twice | `/home/me/app/b.go: packages["example.com/m"][0] and packages["example.com/m"][1] would both write it under output_multiple_files; give them different template files or turn output_multiple_files off` |

In a directory with several entries:

- `muxt generate`, `check`, `list-template-calls`, `list-template-callers` and bare `muxt` act on each entry.
- `muxt test` runs `go test` once, and its profile holds the template files of every entry.
- `muxt test-template-mutations` mutates the templates variables of every entry.
- `muxt generate def` needs the entries to agree on the receiver type, else it fails: `/home/me/app/muxt.json: packages["example.com/m"][0] and packages["example.com/m"][1] name different receiver types; generate def cannot choose one; give them the same use_receiver_type`.
- `muxt explore-module` has a row per generated file ([explore-module](commands/explore-module.md)); `generate-fake-server` fails, because it serves one routes function ([generate-fake-server](commands/generate-fake-server.md)).

## route_paths

A top-level `route_paths` key, beside `package_defaults` and `packages`, takes an object. With it, `muxt generate` writes one file into the directory that holds `muxt.json`, the **hub** package, with the path and route helpers of every package under `packages`. Each of those packages imports the hub. `"route_paths": {}` takes every default ([steps](../how-to/link-to-a-route-in-another-package.md)).

A package's **namespace** is its path below the hub, each element in Go exported form: `blog` is `Blog`, `admin/users` is `AdminUsers`. A template links to a route of any listed package through the namespace: `{{.Path.Blog.Get 42}}` is the URL of the `Get` route of the package `blog`. The package's own routes are reached the same way, as `{{.Path.Admin.Index}}` in `admin`, and a bare `{{.Path.Index}}` fails `muxt check` ([err_route_paths_bare_path.txt](../../cmd/muxt/testdata/err_route_paths_bare_path.txt)). Two packages with one namespace, or a namespace that is not an exported identifier, fail `muxt generate` and nothing is written.

| Member | Default | Rule |
|---|---|---|
| `output_file` | `route_paths.go` | A Go file name in the hub directory: it ends in `.go`, has a name before the extension, and holds no separator or white space. |
| `output_paths_type` | `TemplateRoutePaths` | An exported Go identifier. |
| `output_route_builder_type` | `TemplateRouteBuilder` | An exported Go identifier. |
| `output_route_type` | `TemplateRoute` | An exported Go identifier. |
| `output_package_paths_type_suffix` | `TemplateRoutePaths` | Letters, digits and underscores. A package's type is its namespace and the suffix: `Paths` gives `BlogPaths`. |
| `output_package_route_builder_type_suffix` | `TemplateRouteBuilder` | Letters, digits and underscores. |

A constructor is `New` and the type's name: `NewTemplateRoutePaths`, `NewTemplateRouteBuilder`.

For a package under `packages`:

- Its own `output_template_route_paths_type`, `output_template_route_type` and `output_template_route_builder_type` are ignored; the hub's names apply ([reference_route_paths_ignores_package_type_names.txt](../../cmd/muxt/testdata/reference_route_paths_ignores_package_type_names.txt)).
- Its routes file declares no path types.

The hub imports none of its packages. A **path parameter type** is the Go type of a path parameter of a route's call, such as `id` in `GET /blog/{id} Get(id)`. One declared in a listed package makes the hub import that package, and the package imports the hub: `muxt generate` succeeds and `go build` reports `import cycle` ([reference_route_paths_cycle.txt](../../cmd/muxt/testdata/reference_route_paths_cycle.txt)). A path parameter type and a template declared in the hub are shared by every package ([reference_route_paths_hub_shares_types_and_templates.txt](../../cmd/muxt/testdata/reference_route_paths_hub_shares_types_and_templates.txt), [steps](../how-to/link-to-a-route-in-another-package.md)).

`output_routes_func_with_path_prefix_param` makes each routes function take the prefix as its last argument. A link across packages carries the prefix once when every routes function receives the same value ([reference_route_paths_prefix.txt](../../cmd/muxt/testdata/reference_route_paths_prefix.txt)).

A run configured by flags, in a directory with no entry, keeps its own path types and does not write the hub file ([reference_route_paths_flags_run.txt](../../cmd/muxt/testdata/reference_route_paths_flags_run.txt)).

A package removed from `packages` is gone from the hub on the next `muxt generate`, and a template that still links to it fails `muxt check` ([reference_route_paths_removed_package.txt](../../cmd/muxt/testdata/reference_route_paths_removed_package.txt)).

Changing `output_file` removes the file written under the old name, so one file declares the hub's types. Hand-written files in the hub stay ([reference_route_paths_output_file_renamed.txt](../../cmd/muxt/testdata/reference_route_paths_output_file_renamed.txt)).

`muxt generate` in a listed package's directory writes that package and regenerates the hub file from every listed package ([reference_route_paths_child_directory.txt](../../cmd/muxt/testdata/reference_route_paths_child_directory.txt)).

Errors:

- `/home/me/app/muxt.json:1:17: route_paths must be a JSON object` ([read_test.go](../../internal/composite/read_test.go))
- `Error: /home/me/app/muxt.json: route_paths: unknown key "output_fil"; did you mean "output_file"?` ([err_route_paths_unknown_member.txt](../../cmd/muxt/testdata/err_route_paths_unknown_member.txt))
- `/home/me/app/muxt.json: route_paths: key "output_file" takes a JSON string` ([read_test.go](../../internal/composite/read_test.go))
- `Error: /home/me/app/muxt.json: route_paths: output_paths_type "paths" must be an exported Go identifier` ([err_route_paths_type_name.txt](../../cmd/muxt/testdata/err_route_paths_type_name.txt))
- `Error: package "example.com/m/_1" gets the route paths namespace "1", which is not an exported Go identifier; rename its directory` ([err_route_paths_namespace.txt](../../cmd/muxt/testdata/err_route_paths_namespace.txt))
- `Error: packages "example.com/m/admin/users" and "example.com/m/admin_users" both get the route paths namespace "AdminUsers"; rename one of them` ([err_route_paths_same_namespace.txt](../../cmd/muxt/testdata/err_route_paths_same_namespace.txt))
- `Error: route paths name "AdminPaths" is made by both output_paths_type and output_package_paths_type_suffix for package "example.com/m/admin"; change one of them` ([err_route_paths_type_names_clash.txt](../../cmd/muxt/testdata/err_route_paths_type_names_clash.txt))
- `TemplateRoutePaths method name collision: templates "GET /items list(ctx)" and "GET /items/{id} List(ctx, id)" both produce method "List"; rename one receiver method so the exported names differ by more than the case of the first letter, leading underscores, or characters that drop out` ([err_route_paths_method_collision.txt](../../cmd/muxt/testdata/err_route_paths_method_collision.txt))

Errors name the file and the fix: `packages key "workspace" is not an import path; write "example.com/m/workspace"`, `key "use-receiver-type" uses dashes; write "use_receiver_type"`, `unknown key "KEY"; did you mean "KNOWN"?`, `key "output_htmx" takes a JSON boolean`, `packages must be an object keyed by import path`, `packages["KEY"] must be an array of JSON objects with at least one entry; add an entry or delete the key`, `packages["KEY"][1] must be a JSON object, not null; delete the element`, `key "output_htmx" is repeated`, `packages["KEY"][0]: output_routes_func value must not be empty`, `no packages entry "KEY"; add one to generate this directory`.

Steps: [move generate flags](../how-to/muxt-json.md).
