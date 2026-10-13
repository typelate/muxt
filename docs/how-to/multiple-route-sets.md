# How to serve public and admin routes from one package

Give each audience its own templates variable and generate a route set per variable. Each set gets its own routes function, receiver interface, and template data type, so the compiler enforces the separation.

```go
//go:embed admin/*.gohtml public/*.gohtml shared/*.gohtml
var templateSource embed.FS

//go:generate muxt generate --use-templates-variable=publicTmpl --output-routes-func=PublicRoutes --output-file=routes_public.go --output-receiver-interface=PublicHandler --output-template-data-type=PublicData --output-template-route-paths-type=PublicPaths --output-template-route-type=PublicRoute
var publicTmpl = template.Must(template.Must(template.ParseFS(templateSource, "shared/*.gohtml")).ParseFS(templateSource, "public/*.gohtml"))

//go:generate muxt generate --use-templates-variable=adminTmpl --output-routes-func=AdminRoutes --output-file=routes_admin.go --output-receiver-interface=AdminHandler --output-template-data-type=AdminData --output-template-route-paths-type=AdminPaths --output-template-route-type=AdminRoute
var adminTmpl = template.Must(template.Must(template.ParseFS(templateSource, "shared/*.gohtml")).ParseFS(templateSource, "admin/*.gohtml"))
```

Both generated files share the package, so every default identifier is renamed with its `--output-*` flag ([flags](../reference/cli.md#flags)): the routes function, file, receiver interface, template data type, paths type, and route type (`PublicRoute` and `AdminRoute` here, with `PublicRouteBuilder` and `AdminRouteBuilder` derived from them); add `--output-sse-template-data-type` when both sets have `sse` routes. Check each set separately: `muxt check --use-templates-variable=publicTmpl` and again for `adminTmpl`. One run with two `--use-templates-variable` flags merges both sets into one routes function ([multiple templates variables](../reference/templates-variable.md#multiple-template-variables)); two runs keep them apart.

Register each set with its own receiver, with your own constructors:

```go
PublicRoutes(mux, NewPublicHandler(db, cache))
AdminRoutes(mux, NewAdminHandler(db, adminLogger))
```

Each receiver interface lists only the methods its templates call.

The full test is [reference_multiple_generated_routes.txt](../../cmd/muxt/testdata/reference_multiple_generated_routes.txt).
