---
name: muxt-integrate-existing-project
description: "Muxt: Use when adding Muxt to an existing Go web application. Covers incremental migration, package setup, and wiring Muxt routes alongside existing handlers."
---

# Integrate into an Existing Project

Add Muxt to an existing Go web application incrementally. No big-bang rewrites.

## Step 1: Add Type Safety to Existing Templates (Optional, Low Risk)

If your service already uses `html/template` with `embed.FS`, add compile-time checks without changing handler code:

```bash
go install github.com/typelate/muxt/cmd/muxt@latest
muxt check
```

**Requirements:**
- Package-level `var templates` with `embed.FS`
- String literals in `ExecuteTemplate` calls
- Concrete types (not `any`/`interface{}`)

`muxt check` validates template names, field access, and call expressions. Use `--verbose` to see each endpoint checked. Note: `muxt check` does not accept `--use-receiver-type` — it discovers types from the generated code and template definitions.

This catches template errors at build time. Team learns Muxt semantics before changing architecture.

## Step 2: Create an Isolated Package

Create a new package for Muxt-generated code. Keep existing handlers untouched.

```bash
mkdir -p internal/hypertext
```

Create `internal/hypertext/templates.go`:

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

If the receiver type lives in a different package:

```go
//go:generate muxt generate --use-receiver-type=Server --use-receiver-type-package=github.com/yourorg/yourapp/internal/domain
```

See [Templates Variable](../reference/templates-variable.md) for embedding patterns (subdirectories, custom functions, multiple directories).

## Step 3: Write Templates and Receiver Methods

Create a route template (`internal/hypertext/dashboard.gohtml`):

```gotemplate
{{define "GET /dashboard Dashboard(ctx)"}}
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  <h1>{{.Result.Greeting}}, {{.Result.Username}}</h1>
{{end}}
{{end}}
```

Implement the receiver method:

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

Use concrete return types (not `any`). Return `(T, error)` for fallible operations.

See [Template-Driven Development](template-driven-development.md) for the full TDD workflow.

## Step 4: Generate and Wire

```bash
go generate ./internal/hypertext/
```

Wire both routing systems in your main function:

```go
mux := http.NewServeMux()

// Existing routes continue working
api.RegisterRoutes(mux, srv)

// New Muxt routes added incrementally
hypertext.TemplateRoutes(mux, srv)

http.ListenAndServe(":8080", mux)
```

Both systems share the same receiver. Muxt routes use generated handlers, existing routes are unchanged.

## Step 5: Migrate Routes Incrementally

Move routes one at a time. Path-based coexistence works because `http.ServeMux` routes by specificity:

```go
mux.HandleFunc("/admin/", legacyAdminHandler)  // Not yet migrated
hypertext.TemplateRoutes(mux, srv)              // New paths via Muxt
```

Each migrated route gets compile-time type checking. Unmigrated routes keep working.

## Common Pitfalls

**Avoid `any` in method signatures:**
```go
// Bad - type checker can't help
func (s *Server) Dashboard(ctx context.Context) (any, error)

// Good - concrete types enable static analysis
func (s *Server) Dashboard(ctx context.Context) (DashboardData, error)
```

**Avoid runtime template selection:**
```go
// Bad - Muxt can't analyze
templateName := getTemplateName(r)
templates.ExecuteTemplate(w, templateName, data)

// Good - static template names
templates.ExecuteTemplate(w, "user-profile", data)
```

**Avoid mixing HTTP and business logic:**
```go
// Bad - tightly coupled to HTTP
func (s *Server) Dashboard(w http.ResponseWriter, r *http.Request) { ... }

// Good - pure domain logic
func (s *Server) Dashboard(ctx context.Context) (DashboardData, error) { ... }
```

## Testing Strategy

Unit test domain methods without HTTP:

```go
func TestDashboard(t *testing.T) {
    srv := &Server{db: mockDB}
    data, err := srv.Dashboard(context.Background())
    require.NoError(t, err)
    assert.Equal(t, "Good morning", data.Greeting)
}
```

Integration test generated routes with HTTP:

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

See [Template-Driven Development](template-driven-development.md) for the full testing workflow.

## Reference

- [CLI Commands](../reference/cli.md)
- [Template Name Syntax](../reference/template-names.md)
- [Call Parameters](../reference/call-parameters.md)
- [Call Results](../reference/call-results.md)
- [Templates Variable](../reference/templates-variable.md)
- [Package Structure](../explanation/package-structure.md)

### Test Cases (`cmd/muxt/testdata/`)

- `reference_receiver_with_different_package.txt` — `--use-receiver-type-package` flag
- `reference_receiver_with_pointer.txt` — Pointer receiver types
- `reference_receiver_with_embedded_method.txt` — Embedded method promotion
- `reference_multiple_generated_routes.txt` — Multiple templates variables in one package
- `reference_package_discovery.txt` — How muxt discovers the package
- `reference_template_embed_gen_decl.txt` — `//go:embed` with `var` declaration
