# How to serve public and admin routes from one package

One codebase often serves audiences with different authentication, rate limits, and capabilities. Generate a separate route set per audience: each template variable gets its own routes function, receiver interface, and template data type, so the type system enforces the separation.

```go
package hypertext

import (
    "embed"
    "html/template"
)

//go:embed admin/ public/ shared/
var templateSource embed.FS

//go:generate muxt generate --use-templates-variable=publicTmpl --output-routes-func=PublicRoutes --output-file=routes_public.go --output-receiver-interface=PublicHandler --output-template-data-type=PublicData --output-template-route-paths-type=PublicPaths
var publicTmpl = template.Must(template.Must(template.ParseFS(templateSource, "shared/*.gohtml")).ParseFS(templateSource, "public/*.gohtml"))

//go:generate muxt generate --use-templates-variable=adminTmpl --output-routes-func=AdminRoutes --output-file=routes_admin.go --output-receiver-interface=AdminHandler --output-template-data-type=AdminData --output-template-route-paths-type=AdminPaths
var adminTmpl = template.Must(template.Must(template.ParseFS(templateSource, "shared/*.gohtml")).ParseFS(templateSource, "admin/*.gohtml"))
```

Both generated files live in one package, so every default identifier that would collide — routes function, output file, receiver interface, data type, route-paths type — is renamed with its `--output-*` flag.

Wire each set with its own receiver and middleware:

```go
func main() {
    mux := http.NewServeMux()

    // Public routes: rate limited, public auth
    publicHandler := NewPublicHandler(db, cache)
    PublicRoutes(mux, publicHandler)

    // Admin routes: admin auth, no rate limit, different logging
    adminHandler := NewAdminHandler(db, adminLogger)
    AdminRoutes(mux, adminHandler)

    http.ListenAndServe(":8080", mux)
}
```

Each receiver interface lists only the methods its route set calls, so capability separation is compile-time checked: the public handler can embed read-only services while the admin handler embeds write services, different loggers, or a primary database instead of a replica.

The same shape covers API versioning (a v1 and a v2 set), per-tenant template sets, separate authentication realms, and staged rollouts (experimental routes behind their own receiver).

[reference_multiple_generated_routes.txt](../../cmd/muxt/testdata/reference_multiple_generated_routes.txt)
