# Advanced Patterns

Production patterns for Muxt applications, extracted from production web services. Each pattern addresses specific architectural concerns in server-rendered applications.

## Extending TemplateData: Domain-Specific Template Logic

**Problem:** Templates need domain logic (authorization checks, conditional rendering) without coupling domain types to HTTP concerns. Standard approach mixes business rules into templates or pollutes domain models with presentation logic.

**Solution:** Extend generated `TemplateData[T]` with methods that operate on request context and result data. These methods access both HTTP primitives (request headers, context values) and domain data (result types, errors) without requiring domain types to know about HTTP.

```go
// Authorization using request context and domain data
func (data TemplateData[T]) CanEditPortfolio(p Portfolio) bool {
    ctx := data.Request().Context()
    session, ok := user.SessionFromContext(ctx)
    if !ok || session.UserID == "" || p.AuthorID == "" {
        return false
    }
    return p.AuthorID == session.UserID
}

// Protocol detection for progressive enhancement
func (data TemplateData[T]) IsHXRequest() bool {
    return data.Request().Header.Get("HX-Request") == "true"
}

// Automatic error status codes from domain errors
func (data *TemplateData[T]) ErrorStatusCode() *TemplateData[T] {
    if data.Err == nil {
        return data
    }
    if sc, ok := data.Err.(interface{ StatusCode() int }); ok {
        return data.StatusCode(sc.StatusCode())
    }
    return data
}
```

**Usage in templates:**
```gotemplate
{{define "GET /portfolio/{id} GetPortfolio(ctx, id)"}}
{{if .CanEditPortfolio .Result}}
  <button hx-get="/portfolio/{{.Result.ID}}/edit">Edit</button>
{{end}}

{{if .IsHXRequest}}
  <div class="portfolio-content">...</div>  <!-- HTMX partial -->
{{else}}
  <!DOCTYPE html><html>...</html>  <!-- Full page for direct navigation -->
{{end}}
{{end}}
```

**Why this works:** `TemplateData[T]` methods have access to request context (authentication, headers) and domain data (result values, errors). Domain types stay pure. Templates get domain-aware helpers. No coupling between layers.

**When to use:** Authorization checks, protocol detection (HTMX vs browser), feature flags from context, locale-aware rendering. Any logic that requires both HTTP primitives and domain data.

## Domain Errors with HTTP Semantics

**Problem:** Domain errors need HTTP status codes without domain layer knowing about HTTP. Standard approach either couples domain to `net/http` or forces handlers to maintain error-to-status mappings.

**Solution:** Domain errors implement `StatusCode() int`. Muxt inspects error for method presence. Domain layer defines semantic errors, HTTP layer uses status codes automatically.

```go
type ReadSecurityError struct {
    err error
}

func NewReadSecurityError(err error) *ReadSecurityError {
    if err == nil {
        return nil
    }
    return &ReadSecurityError{err: err}
}

func (r *ReadSecurityError) Error() string {
    if database.IsNotFoundError(r.err) {
        return "security not found"
    }
    return "failed to read security"
}

func (r *ReadSecurityError) StatusCode() int {
    if database.IsNotFoundError(r.err) {
        return http.StatusNotFound
    }
    return http.StatusInternalServerError
}

func (r *ReadSecurityError) Unwrap() error {
    return r.err
}
```

**Usage in receiver methods:**
```go
func (s *Server) ReadSecurity(ctx context.Context, id string) (security.Document, error) {
    doc, err := s.db.SecurityDocument(ctx, id)
    if err != nil {
        s.Logger.ErrorContext(ctx, "failed to load security", slog.String("id", id))
        return security.Document{}, NewReadSecurityError(err)
    }
    return doc, nil
}
```

**Why this works:** Error type encapsulates domain semantics (not found, unauthorized, validation failure). `StatusCode()` method maps domain error to HTTP status. Muxt generated handlers call `StatusCode()` if present, fall back to defaults otherwise. Domain code never imports `net/http`.

**Pattern:** Wrap database/service errors at domain boundary. Error constructors return `nil` for `nil` input (ergonomic unwrapping). Error messages are user-facing (logged separately with context). Status codes map domain states to HTTP semantics. Be careful not to leak errors that might expose app internals.

**When to use:** Multi-error-state operations (not found vs permission denied vs internal error), domain-specific error types (validation errors with field details), services consumed by both HTTP and non-HTTP callers.

## Multi-Receiver Architecture: Separating Public and Admin Routes

**Problem:** Single codebase serves multiple audiences (public users, administrators, API consumers) with different authentication, rate limiting, and feature sets. Monolithic handler registration creates tangled middleware chains.

**Solution:** Multiple template variables generate separate route registration functions with distinct receiver interfaces. Each route set has dedicated receiver, middleware, and configuration.

```go
package hypertext

import (
    "embed"
    "html/template"
)


//go:embed admin/ public/ shared/
var templateSource embed.FS

//go:generate muxt generate --find-templates-variable=publicTmpl --output-routes-func=PublicRoutes --output-file=routes_public.go --output-receiver-interface=PublicHandler
var publicTmpl = template.Must(template.Must(template.ParseFS(templateSource, "shared/*.gohtml")).ParseFS(templateSource, "public/*.gohtml"))

//go:generate muxt generate --find-templates-variable=adminTmpl --output-routes-func=AdminRoutes --output-file=routes_admin.go --output-receiver-interface=AdminHandler
var adminTmpl = template.Must(template.Must(template.ParseFS(templateSource, "shared/*.gohtml")).ParseFS(templateSource, "admin/*.gohtml"))
```

**Wire differently:**
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

**Why this works:** Separate template sets produce separate generated code. Each receiver interface lists only methods required for that route set. Type system enforces separation. Middleware wraps different handler sets independently.

**Architecture implications:** Public handler might embed read-only services. Admin handler embeds write services. Different logging levels, different authentication, different databases (read replicas vs primary). Compile-time enforcement of capability separation.

**When to use:** Public vs admin interfaces, API versioning (v1 vs v2 receivers), multi-tenant apps (different template sets per tenant), different authentication realms, staged rollouts (experimental routes in separate receiver).

[multiple_generated_routes_in_the_same_package.txt](../../cmd/muxt/testdata/multiple_generated_routes_in_the_same_package.txt)

## Interface Segregation: Composable Dependencies

**Problem:** Large server types with many dependencies become hard to test, hard to reason about, and violate interface segregation principle. Mocking entire server for unit tests requires comprehensive fakes.

**Solution:** Server composes focused interfaces. Each interface represents a capability. Receiver methods depend on specific interfaces, not entire server.

```go
type Database interface {
    Portfolio(ctx context.Context, id string) (Portfolio, error)
    InsertPortfolio(ctx context.Context, meta Metadata) (Portfolio, error)
}

type SecuritiesProvider interface {
    Returns(ctx context.Context, id string) (returns.List, error)
    Search(ctx context.Context, query string, limit int) ([]Security, error)
}

type UsersService interface {
    SessionUserID(ctx context.Context) (string, error)
}

type Server struct {
    Logger     *slog.Logger
    Database   Database
    Securities SecuritiesProvider
    Users      UsersService
}
```

**Mock service interfaces, not RoutesReceiver:**
```go
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o internal/fake/database.go . Database
//counterfeiter:generate -o internal/fake/securities.go . SecuritiesProvider

func TestGetPortfolio(t *testing.T) {
    fakeDB := &fake.Database{}
    fakeDB.PortfolioReturns(Portfolio{ID: "123", Name: "Growth"}, nil)

    server := Server{Database: fakeDB}
    result, err := server.GetPortfolio(context.Background(), "123")

    require.NoError(t, err)
    assert.Equal(t, "Growth", result.Name)
    assert.Equal(t, 1, fakeDB.PortfolioCallCount())
}
```

**Why this works:** Mocking service interfaces (Database, SecuritiesProvider) instead of RoutesReceiver creates a thicker testable application layer. Tests exercise receiver methods, TemplateData[T] extensions, and error handling logic—more business logic per test. Mocking RoutesReceiver creates a thin boundary that only tests generated handler code (routing, parameter parsing), which already has some coverage from Muxt's own tests.

**Test layer thickness tradeoffs:**

1. **Thin (mock RoutesReceiver):** Tests only verify generated handlers call correct receiver methods. Fast but minimal business logic coverage.

2. **Medium (mock service interfaces):** Tests exercise receiver methods, domain errors, TemplateData extensions. Still fast (no I/O), covers most business logic. **Recommended for apps with well-tested existing business logic.**

3. **Thick (mock service collaborator interfaces):** Tests exercise receiver methods plus services. Mock collaborators that wrap network calls (for example use mock Querier interface from [sqlc](https://docs.sqlc.dev/en/latest/reference/config.html#go) and set `emit_interface` to `true`). Covers business logic but each test effectively tests a whole call path, more verbose. Use when Go-level business logic justifies it. Make sure to also [test your SQL queries](https://github.com/peterldowns/pgtestdb).

**When to use:** When you value well-tested behavior. Avoid mocking RoutesReceiver unless specifically testing handler generation behavior.

## Custom Template Functions: Domain-Specific Formatting

**Problem:** Templates need domain-specific formatting (currency, percentages, dates) that standard Go template functions don't provide. Formatting logic duplicated across templates or done in receiver methods (mixing presentation with business logic).

**Solution:** Register custom functions in template initialization. Functions are pure, stateless formatters. Templates call them in pipelines.

```go
//go:embed *.gohtml
var fs embed.FS

//go:generate muxt generate --find-receiver-type=Server
var templates = template.Must(
    template.New("").
        Funcs(template.FuncMap{
            "percent":  percentFn,
            "dollars":  dollarsFn,
            "dateOnly": dateOnlyFn,
        }).
        ParseFS(fs, "*.gohtml"),
)

func percentFn(multiply bool, value float64) string {
    if multiply {
        value *= 100
    }
    return fmt.Sprintf("%.2f%%", value)
}

func dollarsFn(value float64) string {
    return message.NewPrinter(language.English).Sprintf("$%0.2f", value)
}

func dateOnlyFn(t time.Time) string {
    return t.Format(time.DateOnly)
}
```

**Usage:**
```gotemplate
{{define "GET /report/{id} GetReport(ctx, id)"}}
<p>Return: {{.Result.Return | percent true}}</p>
<p>Balance: {{.Result.Balance | dollars}}</p>
<p>Date: {{.Result.Date | dateOnly}}</p>
{{end}}
```

**Why this works:** Functions registered before `ParseFS` are available in all templates. Functions are pure transformations (input → output, no side effects). Pipelines compose functions naturally. Formatting stays in presentation layer.

**Design principle:** Keep functions simple. Complex logic belongs in receiver methods (business logic) or TemplateData methods (request-aware logic). Template functions are formatters and converters only.

**When to use:** Locale-aware number formatting, date/time display, currency conversion, percentage calculation, markdown rendering, string truncation, URL encoding. Any pure transformation from domain data to display string.

## Production Architecture

Large Muxt applications converge on this structure:

```
internal/hypertext/
├── server.go                     # Server type, interface definitions
├── template.go                   # Template config, custom functions, go:generate directive
├── functions.go                  # Template function implementations
├── template_data.go              # TemplateData[T] method extensions
├── errors.go                     # Domain errors with StatusCode() methods
├── {domain}_*.go                 # Receiver method implementations (portfolio.go, security.go)
├── {domain}_*.gohtml             # Route templates grouped by domain
├── _*.gohtml                     # Shared partials (prefix convention)
├── template_routes.go            # Generated: main orchestration, shared types
├── *_template_routes_gen.go      # Generated: per-source-file handlers
├── *_test.go                     # Table-driven tests using counterfeiter fakes
└── internal/fake/                # Generated: counterfeiter test doubles
    ├── database.go
    └── securities_provider.go
```

**File naming conventions:**
- `portfolio_list.gohtml`, `portfolio_edit.gohtml` — Domain-grouped templates
- `_header.gohtml`, `_navigation.gohtml` — Shared partials (underscore prefix)
- `portfolio.go` — Receiver methods for portfolio routes
- `portfolio_test.go` — Tests for portfolio receivers

**Generated code:** Muxt produces `template_routes.go` (shared types, route registration orchestration) and one `*_template_routes_gen.go` per source template file. Check these into version control. Review during code review. They're your code.

**Separation of concerns:**
- Domain methods (`{domain}.go`) — Business logic, return domain types
- Template extensions (`template_data.go`) — Request-aware presentation logic
- Templates (`*.gohtml`) — HTML structure, calls to domain methods and template functions
- Custom functions (`functions.go`) — Pure formatters and converters
- Error types (`errors.go`) — Domain errors with HTTP status semantics

**Testing strategy:** Unit test domain methods with counterfeiter fakes (fast, isolated). Integration test generated routes with httptest (verify routing, status codes, HTML structure). Use domtest for HTML assertions.

## Related

- [How to Integrate](../how-to/integrate-existing-project.md) — Production migration patterns
- [How to Test](../how-to/test-handlers.md) — Testing patterns with counterfeiter and domtest
- [CLI Reference](../reference/cli.md) — Generation flags and options
