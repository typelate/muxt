# Muxt Advanced Reference

Supplements [muxt-guide.md](muxt-guide.md) with edge cases and detailed patterns.

## Template Name BNF

```bnf
<route>  ::= [ <method> " " ] [ <host> ] <path> [ " " <status> ] [ " " <call> ]
<method> ::= "GET" | "POST" | "PATCH" | "PUT" | "DELETE"
<path>   ::= "/" { <segment> "/" } [ <segment> ]
<segment>::= <literal> | "{" <param> "}" | "{" <param> "...}"
<status> ::= <number> | "http.Status" <ident>
<call>   ::= <ident> "(" [ <args> ] ")"
```

**Path patterns:**
- `/{$}` — Exact match only `/`
- `/static/` — Prefix match `/static/*`
- `/files/{path...}` — Wildcard captures rest

## All TemplateData Methods

```go
func (d TemplateData[T]) Result() T              // Method return value
func (d TemplateData[T]) Err() error             // Joined errors
func (d TemplateData[T]) Request() *http.Request // Original request
func (d TemplateData[T]) Path() TemplateRoutePaths // URL generators
func (d TemplateData[T]) Ok() bool               // False skips template
func (d TemplateData[T]) StatusCode(int) TemplateData[T] // Set status
func (d TemplateData[T]) Header(k, v string) TemplateData[T] // Set header
```

## All CLI Flags

```bash
muxt generate \
  --use-receiver-type=Server \
  --use-receiver-type-package=github.com/app/server \
  --output-file=routes.go \
  --output-routes-func=TemplateRoutes \
  --output-receiver-interface=RoutesReceiver \
  --output-routes-func-with-logger-param \
  --output-routes-func-with-path-prefix-param
```

With `--output-routes-func-with-logger-param`: `func TemplateRoutes(mux, receiver, logger *slog.Logger)`
With `--output-routes-func-with-path-prefix-param`: `func TemplateRoutes(mux, receiver, prefix string)`

## Generated File Structure

**`template_routes.go`:**
```
1. RoutesReceiver interface (embeds per-file interfaces)
2. TemplateRoutes() function
3. TemplateData[T] type and methods
4. TemplateRoutePaths type (near end)
5. Path helper methods (at end)
```

**`*_template_routes_gen.go`** (one per .gohtml):
- File-scoped interface (e.g., `IndexRoutesReceiver`)
- File-scoped route function
- HTTP handlers

**Find code:**
```bash
grep "type TemplateRoutePaths" template_routes.go
grep "type .*RoutesReceiver interface" *_template_routes_gen.go
```

## Testing Patterns

**Generate fakes:**
```go
//go:generate counterfeiter -generate
//counterfeiter:generate -o internal/fake/server.go --fake-name Server . RoutesReceiver
```

**Full test pattern:**
```go
func TestHandler(t *testing.T) {
    // Setup
    server := new(fake.Server)
    server.GetUserReturns(User{Name: "Alice"}, nil)
    mux := http.NewServeMux()
    paths := TemplateRoutes(mux, server)

    // Execute
    req := httptest.NewRequest(http.MethodGet, paths.GetUser(42), nil)
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    // Verify call
    require.Equal(t, 1, server.GetUserCallCount())
    ctx, id := server.GetUserArgsForCall(0)
    assert.Equal(t, 42, id)

    // Verify response
    assert.Equal(t, http.StatusOK, rec.Code)

    // Verify DOM
    doc := domtest.ParseResponseDocument(t, rec.Result())
    assert.Equal(t, "Alice", doc.QuerySelector("h1").TextContent())
}
```

**HTMX partial test:**
```go
req.Header.Set("HX-Request", "true")
fragment := domtest.ParseResponseDocumentFragment(t, rec.Result(), atom.Body)
```

## Form Parsing Details

**Field tags:**
```go
type Form struct {
    Name string `name:"user-name"` // Maps to form field "user-name"
}
```

**HTML validation attributes generate checks:**
- `minlength`, `maxlength` — String length
- `min`, `max` — Numeric bounds
- `pattern` — Regex match

## Error Handling

**Custom error with status:**
```go
type NotFoundError struct{ ID int }
func (e NotFoundError) Error() string { return fmt.Sprintf("not found: %d", e.ID) }
func (e NotFoundError) StatusCode() int { return 404 }
```

**Multiple errors:**
```go
func (s Server) Validate(ctx context.Context, form Form) (Result, error) {
    var errs []error
    if form.Name == "" { errs = append(errs, errors.New("name required")) }
    if form.Age < 0 { errs = append(errs, errors.New("age invalid")) }
    return Result{}, errors.Join(errs...)
}
```

## HTMX Patterns

**Conditional rendering:**
```gotemplate
{{if .Request.Header.Get "HX-Request"}}
  {{/* Partial */}}
{{else}}
  {{/* Full page */}}
{{end}}
```

**Response headers:**
```gotemplate
{{with .Header "HX-Redirect" "/"}}{{end}}
{{with .Header "HX-Trigger" "userCreated"}}{{end}}
{{with .Header "HX-Reswap" "outerHTML"}}{{end}}
```

**From Go:**
```go
func (s Server) Create(ctx context.Context, response http.ResponseWriter, form Form) (User, error) {
    response.Header().Set("HX-Redirect", "/users")
    return s.db.Create(ctx, form)
}
```

## Edge Cases

**Pointer receiver:**
```go
func (s *Server) Mutate(...) error { s.count++; return nil }
```

**Embedded methods:**
```go
type Server struct {
    Auth      // Auth.Login() promoted to Server
    *Database // Database.Query() promoted
}
```

**Cross-package receiver:**
```bash
muxt generate \
  --use-receiver-type=Server \
  --use-receiver-type-package=github.com/app/internal/server
```

**url.Values parameter:**
```go
func (s Server) Search(ctx context.Context, query url.Values) ([]Result, error)
```

## Common Mistakes

```go
// Bad: any loses type safety
func (s Server) Get(...) (any, error)

// Good: concrete types
func (s Server) Get(...) (User, error)
```

```gotemplate
{{/* Bad: panics if error */}}
<h1>{{.Result.Name}}</h1>

{{/* Good: check error */}}
{{if .Err}}<div>{{.Err.Error}}</div>{{else}}<h1>{{.Result.Name}}</h1>{{end}}
```

```go
// Bad: templates inside function
func main() {
    var templates = template.Must(...)  // Won't be found
}

// Good: package-level
var templates = template.Must(...)
```