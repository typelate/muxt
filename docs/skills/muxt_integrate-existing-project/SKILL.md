---
name: muxt-integrate-existing-project
description: "Muxt: Use when adding Muxt to an existing Go web application. Covers incremental migration, package setup, and wiring Muxt routes alongside existing handlers."
---

# Integrate muxt into an existing project

Add Muxt to an existing Go web application incrementally. No big-bang rewrites.

## When to use this skill

- Existing Go web app uses raw `net/http` or another router and you want to add muxt for new routes.
- You want compile-time type checks on existing `html/template` usage before changing architecture.
- You're migrating routes one at a time alongside legacy handlers.

## Step 1: Add type safety to existing templates (optional, low risk)

If your service already uses `html/template` with `embed.FS`, run:

```bash
go install github.com/typelate/muxt/cmd/muxt@latest
muxt check
```

**Requirements:** package-level `var templates`, string literals in `ExecuteTemplate` calls, concrete types (not `any`/`interface{}`).

`muxt check` validates template names, field access, and call expressions. `--verbose` to see each endpoint. Note: `muxt check` does not accept `--use-receiver-type` — it discovers types from the generated code and template definitions.

This catches template errors at build time. Team learns muxt semantics before architectural changes.

## Step 2: Create an isolated package

```bash
mkdir -p internal/hypertext
```

`internal/hypertext/templates.go`:

```go
package hypertext

import (
    "embed"
    "html/template"
)

//go:embed *.gohtml
var fs embed.FS

//go:generate muxt generate --use-receiver-type=Server
var templates = template.Must(template.ParseFS(fs, "*.gohtml"))
```

If the receiver type lives in a different package, add `--use-receiver-type-package=github.com/yourorg/yourapp/internal/domain`. See [Templates Variable](../../reference/templates-variable.md) for embedding patterns (subdirectories, custom functions, multiple directories).

## Step 3: Write templates and receiver methods

```gotmpl
{{define "GET /dashboard Dashboard(ctx)"}}
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  <h1>{{.Result.Greeting}}, {{.Result.Username}}</h1>
{{end}}
{{end}}
```

```go
type DashboardData struct {
    Greeting string
    Username string
}

func (s *Server) Dashboard(ctx context.Context) (DashboardData, error) {
    user, err := s.db.GetCurrentUser(ctx)
    if err != nil {
        return DashboardData{}, err
    }
    return DashboardData{
        Greeting: getGreeting(time.Now()),
        Username: user.Name,
    }, nil
}
```

Use concrete return types (not `any`). Return `(T, error)` for fallible operations. Full TDD workflow: [Template-Driven Development](../muxt_test-driven-development/SKILL.md).

## Step 4: Generate and wire

```bash
go generate ./internal/hypertext/
```

```go
mux := http.NewServeMux()

// Existing routes continue working
api.RegisterRoutes(mux, srv)

// New muxt routes added incrementally
hypertext.TemplateRoutes(mux, srv)

http.ListenAndServe(":8080", mux)
```

Both systems share the same receiver. muxt routes use generated handlers; existing routes are unchanged.

## Step 5: Migrate routes incrementally

Path-based coexistence works because `http.ServeMux` routes by specificity:

```go
mux.HandleFunc("/admin/", legacyAdminHandler)  // not yet migrated
hypertext.TemplateRoutes(mux, srv)              // new paths via muxt
```

Each migrated route gets compile-time type checking. Unmigrated routes keep working.

## Common pitfalls

- **`any` in method signatures** — kills type analysis. Use concrete types.
- **Runtime template selection** — `templates.ExecuteTemplate(w, dynamicName, data)`. muxt can't analyze. Use static template names.
- **Mixing HTTP and business logic** — `func (s *Server) Dashboard(w http.ResponseWriter, r *http.Request)` is tightly coupled. Prefer pure domain methods returning `(T, error)`.

## Testing strategy

Unit-test domain methods without HTTP:

```go
func TestDashboard(t *testing.T) {
    srv := &Server{db: mockDB}
    data, err := srv.Dashboard(context.Background())
    require.NoError(t, err)
    assert.Equal(t, "Good morning", data.Greeting)
}
```

Integration-test generated routes via HTTP:

```go
func TestDashboardRoute(t *testing.T) {
    srv := &Server{db: testDB}
    mux := http.NewServeMux()
    paths := hypertext.TemplateRoutes(mux, srv)

    req := httptest.NewRequest("GET", paths.Dashboard(), nil)
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code)
    doc := domtest.ParseResponseDocument(t, rec.Result())
    assert.NotNil(t, doc.QuerySelector("h1"))
}
```

Full workflow: [Template-Driven Development](../muxt_test-driven-development/SKILL.md).

## External reference

- [CLI Commands](../../reference/cli.md), [Template Name Syntax](../../reference/template-names.md), [Call Parameters](../../reference/call-parameters.md), [Call Results](../../reference/call-results.md), [Templates Variable](../../reference/templates-variable.md), [Package Structure](../../explanation/package-structure.md)

### Test cases (`cmd/muxt/testdata/`)

- `reference_receiver_with_different_package.txt` — `--use-receiver-type-package` flag
- `reference_receiver_with_pointer.txt`, `reference_receiver_with_embedded_method.txt` — pointer / embedded receivers
- `reference_multiple_generated_routes.txt` — multiple templates variables in one package
- `reference_package_discovery.txt`, `reference_template_embed_gen_decl.txt` — package and embed discovery
