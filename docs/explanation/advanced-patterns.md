# Advanced Patterns for Production Muxt Applications

This document explores advanced patterns and techniques used in real-world Muxt applications, drawn from production codebases.

## Custom Template Functions

Muxt works seamlessly with Go's template function 
maps. You can register custom functions to use within your templates.

### Registering Functions

```go
package hypertext

import (
	"embed"
	"html/template"

	"golang.org/x/text/message"
	"golang.org/x/text/language"
)

//go:embed *.gohtml
var templatesDir embed.FS

//go:generate muxt generate --receiver-type=Server
var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"percent":    percentFn,
	"dollars":    dollarsFn,
	"dateOnly":   dateOnly,
	"markdown":   markdownFn,
	"isLoggedIn": isLoggedIn,
}).ParseFS(templatesDir, "*.gohtml"))

func percentFn(multiply bool, value float64) string {
	if multiply {
		value *= 100
	}
	return fmt.Sprintf("%.2f%%", value)
}

func dollarsFn(value float64) string {
	return message.NewPrinter(language.English).Sprintf("$%0.2f", value)
}

func dateOnly(t time.Time) string {
	return t.Format(time.DateOnly)
}
```

### Using Custom Functions in Templates

```gotemplate
{{define "GET /report/{id} GetReport(ctx, id)"}}
<div class="report">
  <p>Return: {{.Result.Return | percent true}}</p>
  <p>Balance: {{.Result.Balance | dollars}}</p>
  <p>Date: {{.Result.Date | dateOnly}}</p>
</div>
{{end}}
```

## Custom Error Types with Status Codes

Create error types that implement `StatusCode() int` to control HTTP status codes based on error conditions.

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

func (r *ReadSecurityError) isNotFound() bool {
	return database.IsNotFoundError(r.err)
}

func (r *ReadSecurityError) Error() string {
	if r.isNotFound() {
		return "security not found"
	}
	return "failed to read security"
}

func (r *ReadSecurityError) Unwrap() error {
	return r.err
}

func (r *ReadSecurityError) StatusCode() int {
	if r.isNotFound() {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
```

Use in receiver methods:

```go
func (s *Server) ReadSecurity(ctx context.Context, id string) (security.Document, error) {
	doc, err := s.db.SecurityDocument(ctx, id, nil)
	if err != nil {
		s.Logger.ErrorContext(ctx, "failed to load security", slog.String("id", id))
		return security.Document{}, NewReadSecurityError(err)
	}
	return doc, nil
}
```

Benefits:
- Centralizes status code logic
- Makes error handling explicit
- Provides better debugging information
- Works seamlessly with Muxt's error handling

## Extending TemplateData

Add custom methods to the generated `TemplateData` type by defining them in the same package:

```go
// Custom authorization check
func (data TemplateData[T]) CanEditPortfolio(p database.PortfolioDocument) bool {
	ctx := data.Request().Context()
	session, ok := user.SessionClaimsFromContext(ctx)
	if !ok || session.Subject == "" || p.AuthorID == "" {
		return false
	}
	return p.AuthorID == session.Subject
}

// Automatic error status code handling
func (data *TemplateData[T]) ErrorStatusCode() *TemplateData[T] {
	if data.err == nil {
		return data
	}
	sc, ok := data.err.(interface{ StatusCode() int })
	if !ok {
		return data
	}
	return data.StatusCode(sc.StatusCode())
}

// HTMX request detection
func (data *TemplateData[T]) IsHXRequest() bool {
	if data.request == nil {
		return false
	}
	return data.request.Header.Get("HX-Request") == "true"
}
```

Use in templates:

```gotemplate
{{define "GET /portfolio/{id} GetPortfolio(ctx, id)"}}
{{- if .CanEditPortfolio .Result}}
  <button hx-get="/portfolio/{{.Result.ID}}/edit">Edit</button>
{{- end}}

{{- if .IsHXRequest}}
  {{/* Return partial for HTMX */}}
  <div class="portfolio-content">...</div>
{{- else}}
  {{/* Return full page */}}
  <!DOCTYPE html>
  <html>...</html>
{{- end}}
{{end}}
```

## Shared Template Partials

Organize reusable template fragments:

```gotemplate
{{/* _header.gohtml */}}
{{define "shared-header-content"}}
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<link rel="stylesheet" href="/static/styles.css">
<script src="https://unpkg.com/htmx.org@2.0.4"></script>
{{end}}

{{define "site-navigation"}}
<nav>
  <a href="/">Home</a>
  {{if .IsLoggedIn .Request.Context}}
    <a href="/profile">Profile</a>
    <a href="/logout">Logout</a>
  {{else}}
    <a href="/login">Login</a>
  {{end}}
</nav>
{{end}}
```

Use partials in route templates:

```gotemplate
{{define "GET /dashboard Dashboard(ctx)"}}
<!DOCTYPE html>
<html lang="en">
<head>
  <title>Dashboard</title>
  {{template "shared-header-content"}}
</head>
<body>
  {{template "site-navigation" .}}
  <main>
    <h1>Dashboard</h1>
    {{/* content here */}}
  </main>
</body>
</html>
{{end}}
```

## Complex Server Dependencies

Structure your server type with multiple collaborators for clean separation of concerns:

```go
type Database interface {
	Portfolio(ctx context.Context, id primitive.ObjectID) (PortfolioDocument, error)
	InsertPortfolio(ctx context.Context, meta Metadata) (PortfolioDocument, error)
	// ... more methods
}

type SecuritiesProvider interface {
	Returns(ctx context.Context, id string) (returns.List, error)
	Component(ctx context.Context, id string) (Component, error)
	Search(ctx context.Context, query string, limit int64) ([]Component, error)
}

type UsersService interface {
	Create(ctx context.Context) (User, error)
	SessionUserID(ctx context.Context) (int32, error)
}

type Server struct {
	Logger     *slog.Logger
	Database   Database
	Tables     Querier
	Pool       TXBeginner
	RiskFree   ReturnsProvider
	Securities SecuritiesProvider
	Users      UsersService
	Background BackgroundService
}
```

Benefits:
- Easy to test with fake implementations
- Clear dependencies
- Follows interface segregation principle
- Supports dependency injection

## Testing with Counterfeiter

Generate test doubles for your interfaces:

```go
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

package hypertext

//counterfeiter:generate -o fake/database.go --fake-name Database . Database
type Database interface {
	Portfolio(ctx context.Context, id string) (Portfolio, error)
}
```

Generate fakes:

```bash
go generate ./...
```

Use in tests:

```go
func TestPortfolio(t *testing.T) {
	fakeDB := &fake.Database{}
	fakeDB.PortfolioReturns(Portfolio{
		ID:   "123",
		Name: "Test Portfolio",
	}, nil)

	server := Server{Database: fakeDB}
	result, err := server.GetPortfolio(context.Background(), "123")

	require.NoError(t, err)
	require.Equal(t, "Test Portfolio", result.Name)
	require.Equal(t, 1, fakeDB.PortfolioCallCount())
}
```

## Multiple Route Sets in One Package

You can generate multiple independent route handlers in the same package for different template sets:

```go
package main

import (
	"embed"
	"html/template"
)

//go:embed public/*.gohtml shared/*.gohtml
var publicTemplates embed.FS

//go:embed admin/*.gohtml shared/*.gohtml
var adminTemplates embed.FS

//go:generate muxt generate --templates-variable=publicTmpl --routes-func=PublicRoutes --output-file=routes_public.go --receiver-interface=PublicHandler
var publicTmpl = template.Must(template.ParseFS(publicTemplates, "*"))

//go:generate muxt generate --templates-variable=adminTmpl --routes-func=AdminRoutes --output-file=routes_admin.go --receiver-interface=AdminHandler
var adminTmpl = template.Must(template.ParseFS(adminTemplates, "*"))
```

This generates two separate route registration functions with different receivers:

```go
func main() {
	mux := http.NewServeMux()

	PublicRoutes(mux, publicHandler)
	AdminRoutes(mux, adminHandler)

	http.ListenAndServe(":8080", mux)
}
```

**Use cases:**
- Separate public and admin interfaces
- Different authentication/middleware requirements
- API versioning (v1, v2 handlers)
- Multi-tenant applications

*[(See Muxt CLI Test/multiple_generated_routes_in_the_same_package)](../../cmd/muxt/testdata/multiple_generated_routes_in_the_same_package.txt)*

## Large-Scale Organization

For large applications with many routes:

### Directory Structure

```
internal/hypertext/
├── template.go              # Template configuration
├── server.go                # Server type and interfaces
├── functions.go             # Custom template functions
├── *.gohtml                 # Template files
├── portfolio.go             # Portfolio-related receivers
├── security.go              # Security-related receivers
├── user.go                  # User-related receivers
├── template_routes.go       # Muxt-Generated Handlers
├── template_routes_test.go  # Tests
└── fake/                    # Generated test doubles
    ├── database.go
    ├── securities_provider.go
    └── users_service.go
```

### Naming Conventions

Group related templates by prefix:

- `portfolio_*.gohtml` - Portfolio views
- `security_*.gohtml` - Security views
- `chart_*.gohtml` - Chart components
- `_*.gohtml` - Shared partials (prefix with underscore)

### Route Organization

Keep route definitions in templates, implementation in focused Go files:

```go
// portfolio.go
func (s *Server) ListPortfolios(ctx context.Context) ([]Portfolio, error) { /* ... */ }
func (s *Server) GetPortfolio(ctx context.Context, id string) (Portfolio, error) { /* ... */ }
func (s *Server) CreatePortfolio(ctx context.Context, form PortfolioForm) (Portfolio, error) { /* ... */ }

// security.go
func (s *Server) ListSecurities(ctx context.Context, query string) ([]Security, error) { /* ... */ }
func (s *Server) GetSecurity(ctx context.Context, id string) (Security, error) { /* ... */ }
```

## Production Tips

**Use structured logging** - `slog` with context gives you request tracing. Future debugging-you will thank present-you.

**Implement health checks** - Add a `/health` endpoint outside Muxt. Your monitoring system needs it.

**Version your APIs** - If you need versioning, put it in the path: `/v1/users`. Simple.

**Monitor generated code** - Occasionally read `template_routes.go`. It's your code. You should know what it does.

**Test edge cases** - Happy path is easy. Test the weird stuff. The nil pointers. The empty strings. The negative numbers.

**Document custom functions** - If you add template functions, keep them small try to do most of your complex code in your receiver methods.

**Use type-safe IDs** - `type UserID int` is better than `int`. The compiler can help you avoid mixing up user IDs and post IDs.

Production is where code meets reality. Reality doesn't care about clever. Reality only cares about working.

## Next Steps

- Review the [reference documentation](../reference/) for complete API details
- Explore [how-to guides](../how-to/) for specific tasks
- Check out [motivation](motivation.md) to understand Muxt's design philosophy
