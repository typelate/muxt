# How to Integrate Muxt into an Existing Project

Migrate production services incrementally. Start small, prove value, scale adoption.

## Strategy

**Port small services first.** Get team reps on non-critical paths. Learn migration patterns. Apply to larger systems.

**Run alongside existing code.** Zero-downtime migration. Both architectures coexist during transition.

**Key insight:** Muxt isn't a framework. It's static analysis + code generation. Your domain logic stays pure Go.

## Path 1: Add Type Safety to Existing Templates (Low Risk)

If your service already uses `html/template`, add compile-time checks without changing handler code. Safe first step.

**Requirements:**
- Package-level `var templates` with `embed.FS`
- String literals in `ExecuteTemplate` calls
- Concrete types (not `any`/`interface{}`)

**Example:**
```go
//go:embed *.gohtml
var templateSource embed.FS
var templates = template.Must(template.ParseFS(templateSource, "*.gohtml"))

// Handler stays unchanged
func (s *Server) UserProfile(w http.ResponseWriter, r *http.Request) {
    user := s.db.GetUser(r.Context(), getUserID(r))  // Concrete type
    templates.ExecuteTemplate(w, "user-profile", user)  // String literal
}
```

**Validate:**
```bash
go install github.com/typelate/muxt/cmd/muxt@latest
muxt check --use-receiver-type=Server
```

**Value:** Catch template errors at build time. Team learns Muxt semantics before changing architecture.

## Path 2: Generate Handlers in New Package (Production Ready)

Recommended for incremental migration. Isolate Muxt-generated code from existing handlers.

**Architecture pattern:** `internal/hypertext` for templates + generated code, `internal/domain` for business logic.

### Step-by-Step

**1. Create isolated package:**
```bash
mkdir -p internal/hypertext
```

**2. Define templates with package-level var (`internal/hypertext/templates.go`):**
```go
package hypertext

import ("embed"; "html/template")

//go:embed *.gohtml */*.gohtml
var fs embed.FS

//go:generate muxt generate --use-receiver-type=Server --use-receiver-type-package=github.com/yourorg/yourapp/internal/domain --output-routes-func=Routes
var templates = template.Must(template.ParseFS(fs, "*.gohtml", "*/*.gohtml"))
```

**Critical:**
- `templates` must be package-level (Muxt finds via static analysis)
- `--use-receiver-type-package` points to your domain package
- `--output-routes-func` names the registration function (default: `TemplateRoutes`)

**For complex layouts:**
```go
//go:embed pages/*.gohtml components/*.gohtml layouts/*.gohtml
var fs embed.FS

var templates = template.Must(template.ParseFS(fs,
    "pages/*.gohtml",
    "components/*.gohtml",
    "layouts/*.gohtml",
))
```

**3. Create route templates (`internal/hypertext/dashboard.gohtml`):**
```gotemplate
{{define "GET /dashboard Dashboard(ctx)"}}
<!DOCTYPE html>
<html>
<head><title>Dashboard</title></head>
<body>
  {{if .Err}}<div class="error">{{.Err.Error}}</div>{{end}}
  <h1>{{.Result.Greeting}}, {{.Result.Username}}</h1>
  <p>Last login: {{.Result.LastLogin.Format "2006-01-02"}}</p>
</body>
</html>
{{end}}
```

**4. Implement domain methods (`internal/domain/server.go`):**
```go
package domain

type Server struct {
    db *sql.DB
}

type DashboardData struct {
    Greeting  string
    Username  string
    LastLogin time.Time
}

func (s *Server) Dashboard(ctx context.Context) (DashboardData, error) {
    // Pure business logic - easily tested
    user, err := s.db.GetCurrentUser(ctx)
    if err != nil {
        return DashboardData{}, err
    }
    return DashboardData{
        Greeting:  getGreeting(time.Now()),
        Username:  user.Name,
        LastLogin: user.LastLogin,
    }, nil
}
```

**5. Generate handlers:**
```bash
cd internal/hypertext
go generate  # Creates template_routes.go + dashboard_template_routes_gen.go
```

**6. Wire both routing systems (`cmd/server/main.go`):**
```go
package main

import (
    "log"
    "net/http"

    "github.com/yourorg/yourapp/internal/api"       // Existing JSON API
    "github.com/yourorg/yourapp/internal/domain"
    "github.com/yourorg/yourapp/internal/hypertext" // Muxt routes
)

func main() {
    db := setupDatabase()
    srv := domain.NewServer(db)

    mux := http.NewServeMux()

    // Existing routes continue working
    api.RegisterRoutes(mux, srv)

    // New Muxt routes added incrementally
    hypertext.Routes(mux, srv)

    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

**Result:** Both systems share same `domain.Server`. Muxt routes use generated handlers, existing routes unchanged.

## Migration Patterns for Large Services

**Pattern 1: Feature flags**
```go
if featureFlags.UseMuxt(r.Context()) {
    hypertext.Routes(mux, srv)
} else {
    legacy.Routes(mux, srv)
}
```

**Pattern 2: Shadow mode**
```go
mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
    // Serve from Muxt, log comparison with legacy
    muxtResult := captureResponse(hypertext.Dashboard)
    legacyResult := captureResponse(legacy.Dashboard)
    if !bytes.Equal(muxtResult, legacyResult) {
        log.Warn("divergence detected", "path", r.URL.Path)
    }
    w.Write(muxtResult)
})
```

**Pattern 3: Path-based migration**
```go
// Migrate paths incrementally
mux.HandleFunc("/admin/", legacyAdminHandler)   // Not yet migrated
hypertext.Routes(mux, srv)                       // New paths via Muxt
```

## Performance Characteristics

**Compile time:** Muxt analysis runs during `go generate`. Zero runtime overhead from code generation.

**Runtime:** Generated handlers are thin wrappers around your domain methods. Same performance as hand-written handlers.

**Binary size:** Generated code is verbose but compresses well. Expect 10-20KB per route in debug builds, <5KB in production (stripped + compressed).

**Memory:** Template parsing happens once at startup via `template.Must`. After that, zero allocations for route dispatch.

## Testing Strategy

**Unit test domain methods** (no HTTP):
```go
func TestDashboard(t *testing.T) {
    srv := &domain.Server{db: mockDB}
    data, err := srv.Dashboard(context.Background())
    require.NoError(t, err)
    assert.Equal(t, "Good morning", data.Greeting)
}
```

**Integration test generated routes** (with HTTP):
```go
func TestDashboardRoute(t *testing.T) {
    srv := &domain.Server{db: testDB}
    mux := http.NewServeMux()
    hypertext.Routes(mux, srv)

    req := httptest.NewRequest("GET", "/dashboard", nil)
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code)
    doc := domtest.ParseResponseDocument(t, rec.Result())
    assert.NotNil(t, doc.QuerySelector("h1"))
}
```

**Critical:** Domain logic has zero HTTP dependencies. Test without `httptest`. Generated handlers test routing + template rendering.

## Common Pitfalls

**Avoid `any` in method signatures**
```go
// Bad - type checker can't help
func (s *Server) Dashboard(ctx context.Context) (any, error)

// Good - concrete types enable static analysis
func (s *Server) Dashboard(ctx context.Context) (DashboardData, error)
```

**Avoid runtime template selection**
```go
// Bad - Muxt can't analyze
templateName := getTemplateName(r)
templates.ExecuteTemplate(w, templateName, data)

// Good - static template names
templates.ExecuteTemplate(w, "user-profile", data)
```

**Avoid mixing HTTP and business logic**
```go
// Bad - tightly coupled to HTTP
func (s *Server) Dashboard(w http.ResponseWriter, r *http.Request) {
    data := s.db.Get()
    json.NewEncoder(w).Encode(data)
}

// Good - pure domain logic
func (s *Server) Dashboard(ctx context.Context) (DashboardData, error) {
    return s.db.Get(ctx)
}
```

## Team Onboarding Checklist

- [ ] Install `muxt` CLI globally
- [ ] Port one low-traffic route to learn workflow
- [ ] Run `muxt check` in CI to catch template errors
- [ ] Review generated `template_routes.go` to understand handler structure
- [ ] Write domain methods that return concrete types
- [ ] Test domain methods without HTTP dependencies
- [ ] Use `domtest` for HTML assertions in integration tests
- [ ] Read [template name syntax](../reference/template-names.md) for path parameters
- [ ] Review [receiver methods guide](write-receiver-methods.md) for patterns

## When to Use Muxt

**Use Muxt for:**
- Server-rendered HTML responses
- Type-safe template rendering
- Services where HTML is the primary interface
- Teams that value compile-time safety

**Don't use Muxt for:**
- JSON APIs (use standard handlers)
- WebSocket connections (use standard handlers)
- File downloads/streaming (use standard handlers)
- Services with zero HTML output

Muxt is a scalpel, not a hammer. Use it where it adds value: type-safe HTML generation.

## Next Steps

- [Write receiver methods](write-receiver-methods.md) - Domain-oriented patterns
- [Test handlers](test-handlers.md) - Given-When-Then testing
- [Template name syntax](../reference/template-names.md) - Complete routing syntax
- [Type checking reference](../reference/type-checking.md) - Static analysis details
