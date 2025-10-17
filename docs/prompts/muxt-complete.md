# Muxt Complete Reference

Comprehensive reference for production-ready web applications with Muxt. Includes all features, testing patterns, and advanced usage.

**For quick reference:** See [muxt-quick.md](muxt-quick.md)
**For practical usage:** See [muxt-guide.md](muxt-guide.md)

---

## Template Name Syntax

```
[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]
```

**BNF:** `<route> ::= [ <method> <space> ] [ <host> ] <path> [ <space> <http_status> ] [ <space> <call_expr> ]`

See [template-names.md](../reference/template-names.md) for complete BNF and examples.

Common patterns:
```gotemplate
{{define "GET /"}}...{{end}}
{{define "GET /user/{id}"}}...{{end}}
{{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}
{{define "GET /user/{userID}/post/{postID} GetPost(ctx, userID, postID)"}}...{{end}}
{{define "GET /{$}"}}...{{end}}                  {{/* Exact match */}}
{{define "GET /static/"}}...{{end}}              {{/* Prefix match */}}
{{define "GET /files/{path...}"}}...{{end}}      {{/* Wildcard */}}
```

---

## Template Data Structure

Templates receive `TemplateData[T]`. All fields private, accessed via methods:

```gotemplate
{{.Result}}                         {{/* Method return value */}}
{{.Err}}                            {{/* Error (if any) */}}
{{.Request.URL.Path}}               {{/* *http.Request */}}
{{.Request.PathValue "id"}}         {{/* Path parameters */}}
{{.Request.Header.Get "HX-Request"}} {{/* Headers */}}
{{.Path.GetUser 123}}               {{/* Type-safe URL generation */}}
{{with .StatusCode 404}}...{{end}}  {{/* Set status code */}}
{{with .Header "X-Custom" "val"}}...{{end}} {{/* Set headers */}}
{{.Ok}}                             {{/* true if template executes */}}
```

**Key methods:**
- `.Result()` → method return value (type T)
- `.Err()` → error joined from error list
- `.Request()` → `*http.Request`
- `.Path()` → `TemplateRoutePaths` for URL generation
- `.StatusCode(int)` → set HTTP status, returns self for chaining
- `.Header(k, v)` → set response header, returns self
- `.Ok()` → `false` when method returns `(T, bool)` with `bool=false`

Always check `.Err`:
```gotemplate
{{if .Err}}<div>Error: {{.Err.Error}}</div>{{else}}<h1>{{.Result.Name}}</h1>{{end}}
```

---

## Receiver Methods

**Return types:**
```go
func (s Server) M() T                   // .Result=T, .Err=nil
func (s Server) M() (T, error)          // .Result=T, .Err=error
func (s Server) M() (T, bool)           // .Result=T, bool=false skips template
func (s Server) M() error               // .Result=zero, .Err=error
```

**Parameter names:**
- `ctx` → `context.Context` (from `request.Context()`)
- `request` → `*http.Request`
- `response` → `http.ResponseWriter`
- `form` → struct parsed from `request.Form`
- `{pathParam}` → parsed from `request.PathValue(name)`

**Status codes (5 methods):**
1. Template name: `{{define "POST /user 201 CreateUser(ctx, form)"}}`
2. Result method: `func (r Result) StatusCode() int { return r.code }`
3. Result field: `type Result struct { StatusCode int }`
4. Error method: `func (e HTTPError) StatusCode() int { return 404 }`
5. Template: `{{with .StatusCode 404}}<h1>Not Found</h1>{{end}}`

---

## Type System

**Parsed types:** `string`, `bool`, `int*`, `uint*`, `encoding.TextUnmarshaler`

**Special types:** `context.Context`, `*http.Request`, `http.ResponseWriter`, `url.Values`

**Custom parsing via TextUnmarshaler:**
```go
type UserID string
func (id *UserID) UnmarshalText(text []byte) error {
    *id = UserID(strings.ToLower(string(text)))
    return nil
}
```

**Form struct fields:**
```go
type MyForm struct {
    Name     string   `name:"user_name"`  // Field tag maps form field name
    Age      int
    Tags     []string                      // Multiple values
    Subscribe bool
}
```

**Type checking:**
```bash
muxt check --find-receiver-type=Server  # Validates templates match Go types
```

---

## Package Structure

**Package-level template variable required:**
```go
//go:embed *.gohtml
var templateFS embed.FS
var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))
```

**Multiple directories (enumerate each level):**
```go
//go:embed *.gohtml */*.gohtml */*/*.gohtml
var templateFS embed.FS
var templates = template.Must(template.ParseFS(templateFS, "**/*.gohtml"))
```

**Custom functions:**
```go
var templates = template.Must(
    template.New("").
        Funcs(template.FuncMap{"upper": strings.ToUpper}).
        ParseFS(templateFS, "*.gohtml"),
)
```

**Generated files:**
- `template_routes.go` - Main file with `TemplateRoutes()`, `TemplateData[T]`, `TemplateRoutePaths`
- `*_template_routes_gen.go` - Per-source-file handlers and interfaces

---

## CLI Reference

**Commands:**
- `muxt generate` (aliases: `gen`, `g`) - Generate handlers
- `muxt check` (aliases: `c`, `typelate`) - Type-check only
- `muxt documentation` (aliases: `docs`, `d`) - Generate markdown docs
- `muxt version` (alias: `v`) - Print version

**Key flags:**
- `--find-receiver-type=Server` - Type for method lookup (required)
- `--output-file=routes.go` - Generated file name
- `--output-routes-func=TemplateRoutes` - Route function name
- `--output-receiver-interface=RoutesReceiver` - Interface name
- `--path-prefix` - Add path prefix parameter
- `--logger` - Add `*slog.Logger` parameter
- `-C ./web` - Change directory before running

**Examples:**
```bash
muxt generate --find-receiver-type=Server
muxt check --find-receiver-type=Server
muxt generate --find-receiver-type=Server --logger --path-prefix
muxt -C ./web generate --find-receiver-type=App --output-routes-func=RegisterRoutes
```

**From go:generate:**
```go
//go:generate muxt generate --find-receiver-type=Server
```

---

## Testing

**Generate fakes with counterfeiter:**
```go
//go:generate counterfeiter -generate
//counterfeiter:generate -o internal/fake/server.go --fake-name Server . RoutesReceiver
```

**Table-driven tests (Given-When-Then):**
```go
func TestUserHandlers(t *testing.T) {
    server := new(fake.Server)
    server.GetUserReturns(User{Name: "Alice"}, nil)

    mux := http.NewServeMux()
    paths := TemplateRoutes(mux, server)

    req := httptest.NewRequest(http.MethodGet, paths.GetUser(42), nil)
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    assert.Equal(t, 1, server.GetUserCallCount())
    assert.Equal(t, 42, server.GetUserArgsForCall(0))
    assert.Equal(t, http.StatusOK, rec.Result().StatusCode)
}
```

**DOM testing with domtest:**
```go
import "github.com/typelate/dom/domtest"

doc := domtest.ParseResponseDocument(t, rec.Result())
h1 := doc.QuerySelector("h1")
assert.Equal(t, "Alice", h1.TextContent())

// For HTMX partials
fragment := domtest.ParseResponseDocumentFragment(t, rec.Result(), atom.Body)
```

**Path helpers are type-safe:**
```go
paths.GetUser(42)             // "/user/42"
paths.GetPost(42, 100)        // "/user/42/post/100"
```

See [test-handlers.md](../how-to/test-handlers.md) for complete patterns.

---

## HTMX Integration

**Detect HTMX requests:**
```gotemplate
{{if .Request.Header.Get "HX-Request"}}
  <div id="results">{{range .Result}}<div>{{.Title}}</div>{{end}}</div>
{{else}}
  <!DOCTYPE html><html>...full page...</html>
{{end}}
```

**Set response headers:**
```gotemplate
{{with .Header "HX-Redirect" "/"}}{{end}}  {{/* Redirect */}}
```

Or from Go:
```go
func (s Server) CreateUser(ctx context.Context, response http.ResponseWriter, form UserForm) (User, error) {
    response.Header().Set("HX-Redirect", "/users")
    return s.db.CreateUser(ctx, form)
}
```

**Common patterns:**
- Infinite scroll: `hx-get="/posts?page={{add .Page 1}}" hx-trigger="revealed"`
- Form validation: `hx-post="/validate" hx-trigger="keyup changed delay:500ms"`
- Partial rendering: Check `HX-Request` header, return fragment vs full page

See [use-htmx.md](../how-to/use-htmx.md) for complete examples.

---

## Advanced Patterns

**Embedded methods promoted:**
```go
type Server struct {
    Auth  // Login() promoted to Server
}
```

**Pointer and value receivers both work:**
```go
func (s Server) GetUser(...) (User, error)   // Value receiver
func (s *Server) GetUser(...) (User, error)  // Pointer receiver (for mutation/large structs)
```

**Cross-package receiver:**
```bash
muxt generate --find-receiver-type=Server --find-receiver-type-package=github.com/myapp/internal/server
```

**HTML5 validation attributes:**
Input attributes `minlength`, `maxlength`, `min`, `max`, `pattern` generate validation code.
See [templates_input_validation_min_max.txt](../../cmd/muxt/testdata/templates_input_validation_min_max.txt)

**Custom template functions:**
```go
var templates = template.Must(
    template.New("").
        Funcs(template.FuncMap{"formatDate": formatDate, "add": add}).
        ParseFS(templateFS, "*.gohtml"),
)
```

**Path prefix flag:**
```bash
muxt generate --find-receiver-type=Server --path-prefix
```
Generated signature: `func TemplateRoutes(mux, receiver, pathPrefix string) TemplateRoutePaths`

**Logger flag:**
```bash
muxt generate --find-receiver-type=Server --logger
```
Generated signature: `func TemplateRoutes(mux, receiver, logger *slog.Logger) TemplateRoutePaths`
Logs debug (each request) and error (template failures).

---

## Examples

See test files in [cmd/muxt/testdata/](../../cmd/muxt/testdata/) for working examples:

**Basic patterns:**
- [simple_get.txt](../../cmd/muxt/testdata/simple_get.txt) - Basic GET handler
- [argument_path_param.txt](../../cmd/muxt/testdata/argument_path_param.txt) - Path parameters with type parsing
- [form.txt](../../cmd/muxt/testdata/form.txt) - Form handling with structs
- [status_codes.txt](../../cmd/muxt/testdata/status_codes.txt) - All status code methods

**Advanced patterns:**
- [blog.txt](../../cmd/muxt/testdata/blog.txt) - Complete blog application
- [receiver_embedded_field_method.txt](../../cmd/muxt/testdata/receiver_embedded_field_method.txt) - Embedded methods
- [method_receiver_is_a_pointer.txt](../../cmd/muxt/testdata/method_receiver_is_a_pointer.txt) - Pointer receivers
- [call_method_with_two_returns.txt](../../cmd/muxt/testdata/call_method_with_two_returns.txt) - Error handling

Browse all: `ls cmd/muxt/testdata/*.txt`

---

## Troubleshooting

**Generation errors:**
- "No templates found" → `templates` variable must be package-level (not in function)
- "Method not found" → Verify `--find-receiver-type=YourType` matches, method is exported
- "Template name invalid" → Must follow `[METHOD ][HOST]/PATH[ STATUS][ CALL]`

**Type errors:**
- Run `muxt check --find-receiver-type=Server` for details
- Template actions must reference fields that exist on Result type
- Path/form parameter names must match method signature

**Runtime errors:**
- 400 Bad Request → Path parameter parse failure (e.g., "abc" → int)
- Template execution error → Missing field on Result or unhandled `.Err`
- Panic → Always check `.Err` before accessing `.Result` fields

**Common mistakes:**
```go
// Bad - use concrete types
func (s Server) GetUser(...) (any, error)

// Good
func (s Server) GetUser(...) (User, error)
```

```gotemplate
{{/* Bad - will panic if error */}}
<h1>{{.Result.Name}}</h1>

{{/* Good - check error */}}
{{if .Err}}<div>{{.Err.Error}}</div>{{else}}<h1>{{.Result.Name}}</h1>{{end}}
```

---

## Generated File Reference

**File structure:**
- `template_routes.go` - Main file with:
  - `RoutesReceiver` interface (embeds per-file interfaces)
  - `TemplateRoutes()` function
  - `TemplateData[T]` type and methods
  - `TemplateRoutePaths` type and methods (at end)
- `*_template_routes_gen.go` - Per-source-file:
  - File-scoped interface (e.g., `IndexRoutesReceiver`)
  - File-scoped route function (e.g., `IndexTemplateRoutes()`)
  - HTTP handlers for that file's templates

**Finding code in generated files:**

```bash
# List generated files
ls *_template_routes*.go

# Find TemplateRoutePaths type (near end of template_routes.go)
grep "type TemplateRoutePaths struct" template_routes.go

# Find path helper methods (at end of template_routes.go)
grep "^func (routePaths TemplateRoutePaths)" template_routes.go

# Find RoutesReceiver interface (near top of template_routes.go)
grep "type RoutesReceiver interface" template_routes.go

# Find specific route handler
grep 'mux.HandleFunc("GET /user/{id}"' *_template_routes*.go
```

**Using MCP tools:**
```
go_search "TemplateRoutePaths"
go_symbol_references file:"template_routes.go" symbol:"GetUser"
```

**Key sections:**
1. `RoutesReceiver` interface - Required methods for receiver type
2. `TemplateRoutes()` function - Route registration and handlers (bulk of file)
3. `TemplateData[T]` type - Template context container
4. `TemplateData` methods - `.Result()`, `.Err()`, `.Request()`, `.Path()`, etc.
5. `TemplateRoutePaths` type - Simple struct with `pathsPrefix` field (near end)
6. `TemplateRoutePaths` methods - Type-safe URL generators (at end)

**Note:** Line numbers vary with route count. Use search patterns, not line numbers.

## Summary

Templates define routes → Methods implement behavior → `muxt generate` creates handlers

**Workflow:**
```bash
# 1. Write templates: {{define "GET /user/{id} GetUser(ctx, id)"}}...{{end}}
# 2. Implement methods: func (s Server) GetUser(ctx context.Context, id int) (User, error)
# 3. Generate: muxt generate --find-receiver-type=Server
# 4. Run: TemplateRoutes(mux, server)
```

**Key features:** Type-safe parsing, error handling, HTMX support, status codes, validation, domtest, zero runtime deps

**Philosophy:** Templates are contracts. Methods implement behavior. Generation ensures type safety. HTML is a fine interface.
