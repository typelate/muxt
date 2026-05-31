# Call Results Reference

Receiver method return values control template data and HTTP status.
Use this reference when designing method signatures with team members.

## Return Patterns Quick Reference

| Pattern | `.Result` Type | `.Err` Type | Use When |
|---------|----------------|-------------|----------|
| `T` | `T` | Always `nil` | Infallible operations (static pages) |
| `(T, error)` | `T` (zero if error) | `error` or `nil` | Most endpoints (can fail) |
| `(T, bool)` | `T` | Always `nil` | Early exit/redirect (bool=false skips template) |
| `error` | `struct{}` | `error` or `nil` | No data needed (health checks) |

Use `(T, error)` for 90% of endpoints. It's the idiomatic Go pattern and enables proper error handling.

[howto_call_method.txt](../../cmd/muxt/testdata/howto_call_method.txt) · [howto_call_with_error.txt](../../cmd/muxt/testdata/howto_call_with_error.txt)

## Pattern 1: Single Value (Infallible)

**Use:** Static pages, configuration, data that can't fail

```go
func (s Server) About() AboutPage {
    return AboutPage{Version: s.version}
}
```
```gotmpl
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
```gotmpl
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
        return file, false  // I handled it, skip template
    }
    return file, true  // Continue to template execution
}
```

**Behavior:** If bool = `false`, handler returns immediately (you already wrote the response). If bool = `true`, execute template normally with `.Ok()` = true.

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
```gotmpl
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
| `.Ok()` | `bool` | True unless a `(T, bool)` method returned false |
| `.Request()` | `*http.Request` | HTTP request |
| `.Receiver()` | `R` | The receiver passed to `TemplateRoutes` |
| `.Path()` | `TemplateRoutePaths` | Route URL builders (one method per route); takes no argument |
| `.StatusCode(code)` | `*TemplateData` | Set HTTP status (returns the data for chaining) |
| `.Header(key, val)` | `*TemplateData` | Set response header (returns the data for chaining) |
| `.Redirect(url, code)` | `*TemplateData, error` | Redirect with custom status code |
| `.RedirectMultipleChoices(url)` | `*TemplateData, error` | Redirect with 300 status |
| `.RedirectMovedPermanently(url)` | `*TemplateData, error` | Redirect with 301 status |
| `.RedirectFound(url)` | `*TemplateData, error` | Redirect with 302 status |
| `.RedirectSeeOther(url)` | `*TemplateData, error` | Redirect with 303 status |
| `.String()` | `string` | Returns `""` (implements `fmt.Stringer`) |

**Why `{{.}}` outputs nothing:**

`TemplateData` implements `fmt.Stringer` returning an empty string. This allows methods that return `*TemplateData` (like `.Header()` and `.StatusCode()`) to be called directly in templates without outputting anything:

```gotmpl
{{.Header "HX-Trigger" "contact-sent"}}
```

Without `String()`, this would output the struct's internal fields. Previously you needed the workaround `{{with .Header "HX-Trigger" "contact-sent"}}{{end}}` to suppress output.

To access the receiver method's return value, use `{{.Result}}` explicitly.

[reference_template_data_stringer.txt](../../cmd/muxt/testdata/reference_template_data_stringer.txt)

**Chaining examples:**
```gotmpl
{{with and (.StatusCode 404) (.Header "X-Error" "not-found")}}
  <div>User not found</div>
{{end}}
```

Every entry is a method. In templates, call the zero-argument ones without arguments (`{{.Result}}`, `{{.Err}}`, `{{.Request}}`); pass arguments to the rest (`{{.StatusCode 404}}`, `{{.Header "X-Error" "not-found"}}`).

## Status Code Control

The generated handler resolves the status with `cmp.Or` — the first non-zero value in this list wins, highest to lowest:

| Priority | Source | Set by |
|----------|--------|--------|
| 1 | `.StatusCode(int)` template call | `{{.StatusCode 404}}` in the template |
| 2 | Error status | `400` on a parse/path/form error, `500` when the method returns a non-nil error |
| 3 | Result `StatusCode()` method, else result `StatusCode` field | the return type |
| 4 | Template-name code, else `200` — or `204` when the body is empty | `{{define "POST /user 201 ..."}}` |

There is no error-`StatusCode()` hook: a returned error is always `500`, regardless of the error's own methods. To return a status other than `500` for a failure, set it in the template with `.StatusCode`.

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

**Template status override** (priority 1 — overrides everything, including the `500` from an error):
```gotmpl
{{if .Err}}
  {{with .StatusCode 404}}
    <div>Not found</div>
  {{end}}
{{end}}
```

Prefer the template-name code for static codes (`201` for POST). Use a result `StatusCode` field or method for dynamic success codes, and `.StatusCode` in the template for dynamic error codes (`404` when not found, `422` for validation).

[reference_status_codes.txt](../../cmd/muxt/testdata/reference_status_codes.txt)

## Request Access in Templates

**Access headers, URL, cookies:**
```gotmpl
{{define "GET /profile Profile(ctx)"}}
{{if .Request.Header.Get "HX-Request"}}
  <div>{{.Result.Name}}</div>  <!-- HTMX partial -->
{{else}}
  <!DOCTYPE html><html>...</html>  <!-- Full page -->
{{end}}
{{end}}
```

**Path parameters:** the parsed value reaches the method as an argument, so read it from `.Result`. To read the raw string in the template, use `.Request.PathValue`:
```gotmpl
{{define "GET /user/{id} GetUser(ctx, id)"}}
<p>User ID from path: {{.Request.PathValue "id"}}</p>
<h1>{{.Result.Name}}</h1>
{{end}}
```

`.Path` is unrelated: it takes no argument and returns the generated route URL builders (e.g. `{{.Path.GetUser .Result.ID}}` to build a link).

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
- [reference_result_with_name_collision.txt](../../cmd/muxt/testdata/reference_result_with_name_collision.txt) — Handling name collisions
- [reference_call_with_complex_package.txt](../../cmd/muxt/testdata/reference_call_with_complex_package.txt) — Complex package paths

**Unsupported patterns:**
- [err_form_unsupported_return.txt](../../cmd/muxt/testdata/err_form_unsupported_return.txt) — Unsupported second return
- [err_form_unsupported_composite.txt](../../cmd/muxt/testdata/err_form_unsupported_composite.txt) — Composite types

**Browse all:** [cmd/muxt/testdata/](../../cmd/muxt/testdata/)
