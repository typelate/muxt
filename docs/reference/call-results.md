# Call Results Reference

Receiver method return values control template data and HTTP status.
Use this reference when designing method signatures with team members.

## Return Patterns Quick Reference

| Pattern | `.Result` Type | `.Err` Type | Use When |
|---------|----------------|-------------|----------|
| `T` | `T` | Always `nil` | Infallible operations (static pages) |
| `(T, error)` | `T` (zero if error) | `error` or `nil` | Most endpoints (can fail) |
| `(T, bool)` | `T` | Always `nil` | Early exit/redirect (bool=true skips template) |
| `error` | `struct{}` | `error` or `nil` | No data needed (health checks) |

Use `(T, error)` for 90% of endpoints. It's the idiomatic Go pattern and enables proper error handling.

[howto_call_with_error.txt](../../cmd/muxt/testdata/howto_call_with_error.txt)

## Pattern 1: Single Value (Infallible)

**Use:** Static pages, configuration, data that can't fail

```go
func (s Server) About() AboutPage {
    return AboutPage{Version: s.version}
}
```
```gotemplate
{{define "GET /about About()"}}
<h1>{{.Result.Version}}</h1>
{{end}}
```

**Behavior:** `.Result` = value, `.Err` = nil (always)

## Pattern 2: Value and Error (Standard)

**Use:** Most endpoints (database queries, API calls, anything fallible)

```go
func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    user, err := s.db.FindUser(ctx, id)
    if err != nil {
        return User{}, fmt.Errorf("user not found: %w", err)
    }
    return user, nil
}
```
```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
{{with $err := .Err}}
  <div class="error">{{$err.Error}}</div>
{{else}}
  <h1>{{.Result.Name}}</h1>
{{end}}
{{end}}
```

**Behavior:** `.Result` = value (zero value if error), `.Err` = error (nil if success)

Always check `{{if .Err}}` or `{{with .Err}}` in templates when method returns error. Template executes even on error.

[reference_call_with_error_return.txt](../../cmd/muxt/testdata/reference_call_with_error_return.txt)

## Pattern 3: Value and Boolean (Early Exit)

**Use:** Custom response already written (redirects, cache hits, streaming)

```go
func (s Server) Download(response http.ResponseWriter, request *http.Request, id int) (File, bool) {
    file, ok := s.cache.Get(id)
    if ok {
        http.ServeContent(response, request, file.Name, file.ModTime, file.Reader)
        return file, true  // Skip template execution
    }
    return file, false  // Execute template
}
```

**Behavior:** If bool = `true`, handler returns immediately (skip template). If bool = `false`, execute template normally.

[reference_call_with_bool_return.txt](../../cmd/muxt/testdata/reference_call_with_bool_return.txt)

## Pattern 4: Error Only (No Data)

**Use:** Health checks, webhooks, operations that return no data

```go
func (s Server) Healthcheck() error {
    if err := s.db.Ping(); err != nil {
        return err
    }
    return nil
}
```
```gotemplate
{{define "GET /health Healthcheck()"}}
{{if .Err}}Unhealthy: {{.Err.Error}}{{else}}OK{{end}}
{{end}}
```

**Behavior:** `.Result` = `struct{}` (empty), `.Err` = error (nil if healthy)

## TemplateData[T] API

Templates receive `TemplateData[T]` where `T` is the method's first return value:

| Method/Field | Type | Description |
|--------------|------|-------------|
| `.Result` | `T` | Returned value (zero value if error) |
| `.Err` | `error` | Returned error (nil if success) |
| `.Request()` | `*http.Request` | HTTP request |
| `.Path(name)` | `string` | Path parameter value |
| `.StatusCode(code)` | `int` | Set HTTP status (returns code for chaining) |
| `.Header(key, val)` | `string` | Set response header (returns val for chaining) |

**Chaining examples:**
```gotemplate
{{with and (.StatusCode 404) (.Header "X-Error" "not-found")}}
  <div>User not found</div>
{{end}}
```

`.Result` and `.Err` are fields (no parens). `.Request()`, `.Path()`, `.StatusCode()`, `.Header()` are methods (need parens).

## Status Code Control

**Precedence (highest to lowest):**
1. Template name: `{{define "POST /user 201 ..."}}`
2. Result type `StatusCode()` method
3. Result type `StatusCode` field
4. Error type `StatusCode()` method
5. Template `.StatusCode(int)` call
6. Default (200 success, 500 error)

**Error with StatusCode() method:**
```go
type NotFoundError struct{ Message string }

func (e NotFoundError) Error() string { return e.Message }
func (e NotFoundError) StatusCode() int { return 404 }

func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    user, err := s.db.FindUser(ctx, id)
    if err != nil {
        return User{}, NotFoundError{Message: "user not found"}
    }
    return user, nil
}
```

**Result with StatusCode() method:**
```go
type UserResult struct {
    User User
    code int
}

func (r UserResult) StatusCode() int { return r.code }
```

**Result with StatusCode field:**
```go
type UserResult struct {
    User       User
    StatusCode int
}
```

**Template status override:**
```gotemplate
{{if .Err}}
  {{with .StatusCode 404}}
    <div>Not found</div>
  {{end}}
{{end}}
```

Prefer template name for static codes (201 for POST). Use error/result types for dynamic codes (404 when not found, 422 for validation).

## Request Access in Templates

**Access headers, URL, cookies:**
```gotemplate
{{define "GET /profile Profile(ctx)"}}
{{if .Request.Header.Get "HX-Request"}}
  <div>{{.Result.Name}}</div>  <!-- HTMX partial -->
{{else}}
  <!DOCTYPE html><html>...</html>  <!-- Full page -->
{{end}}
{{end}}
```

**Path parameters:**
```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
<p>User ID from path: {{.Path "id"}}</p>
<h1>{{.Result.Name}}</h1>
{{end}}
```

## Type Safety Requirements

**Use concrete types for compile-time checking:**

**Good:**
```go
func (s Server) GetUser(ctx context.Context) (User, error)
func (s Server) GetUsers(ctx context.Context) ([]User, error)
func (s Server) GetStats(ctx context.Context) (map[string]int, error)
```

**Avoid:**
```go
func (s Server) GetUser(ctx context.Context) (any, error)          // No type checking
func (s Server) GetUser(ctx context.Context) (interface{}, error)  // No type checking
```

Concrete types enable `muxt check` to catch template errors at build time. Always return specific types.

## Constraints

**Second return value must be `error` or `bool`:**

**Allowed:**
```go
func (s Server) Method() (T, error)
func (s Server) Method() (T, bool)
```

**Not allowed:**
```go
func (s Server) Method() (T, chan error)         // Channels unsupported
func (s Server) Method() (T, []error)            // Slices unsupported
func (s Server) Method() (T, map[string]error)   // Maps unsupported
```

[err_form_unsupported_return.txt](../../cmd/muxt/testdata/err_form_unsupported_return.txt) · [err_form_unsupported_composite.txt](../../cmd/muxt/testdata/err_form_unsupported_composite.txt)

## Test Files by Category

**Return patterns:**
- [howto_call_with_error.txt](../../cmd/muxt/testdata/howto_call_with_error.txt) — `(T, error)` pattern
- [reference_call_with_error_return.txt](../../cmd/muxt/testdata/reference_call_with_error_return.txt) — Error handling
- [reference_call_with_bool_return.txt](../../cmd/muxt/testdata/reference_call_with_bool_return.txt) — Early exit with bool
- [err_form_bool_return.txt](../../cmd/muxt/testdata/err_form_bool_return.txt) — Boolean returns

**Result types:**
- [reference_result_with_import_type.txt](../../cmd/muxt/testdata/reference_result_with_import_type.txt) — Imported result types
- [reference_result_with_named_type.txt](../../cmd/muxt/testdata/reference_result_with_named_type.txt) — Named return values

**Unsupported patterns:**
- [err_form_unsupported_return.txt](../../cmd/muxt/testdata/err_form_unsupported_return.txt) — Unsupported second return
- [err_form_unsupported_composite.txt](../../cmd/muxt/testdata/err_form_unsupported_composite.txt) — Composite types

**Browse all:** [cmd/muxt/testdata/](../../cmd/muxt/testdata/)
