# Muxt Guide

Build hypermedia apps with Go. Templates define routes, Go implements behavior, muxt generates handlers.

## Template Name Syntax

```
[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]
```

| Component | Required | Description |
|-----------|----------|-------------|
| METHOD | No | GET, POST, PATCH, PUT, DELETE |
| HOST | No | api.example.com |
| PATH | Yes | `/user/{id}`, `/{$}` (exact), `/static/` (prefix) |
| STATUS | No | 200, 201, http.StatusCreated |
| CALL | No | MethodName(ctx, id, form) |

```gotemplate
{{define "GET /"}}...{{end}}
{{define "GET /user/{id} GetUser(ctx, id)"}}...{{end}}
{{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}
{{define "GET /user/{uid}/post/{pid} GetPost(ctx, uid, pid)"}}...{{end}}
{{define "DELETE /user/{id} 204 DeleteUser(ctx, id)"}}...{{end}}
```

## Call Parameters

| Name | Type | Source |
|------|------|--------|
| `ctx` | `context.Context` | `request.Context()` |
| `request` | `*http.Request` | Full request |
| `response` | `http.ResponseWriter` | For headers/streaming |
| `form` | struct | Parsed from `request.Form` |
| `{param}` | string/int/custom | `request.PathValue()` |

**Form struct example:**
```go
type UserForm struct {
    Name  string   `name:"user_name"`  // Maps form field
    Age   int
    Tags  []string                      // Multiple values
}
```

Supported types: `string`, `int*`, `uint*`, `bool`, `encoding.TextUnmarshaler`

## Template Data

Templates receive `TemplateData[T]`:

```gotemplate
{{.Result}}                          {{/* Method return value */}}
{{.Err}}                             {{/* Error (always check!) */}}
{{.Request.URL.Path}}                {{/* Request data */}}
{{.Request.Header.Get "HX-Request"}} {{/* HTMX detection */}}
{{.Path.GetUser 42}}                 {{/* Type-safe URL */}}
{{with .StatusCode 404}}...{{end}}   {{/* Set status */}}
{{with .Header "X-Custom" "v"}}...{{end}} {{/* Set header */}}
```

**Always check errors:**
```gotemplate
{{if .Err}}<div>{{.Err.Error}}</div>{{else}}<h1>{{.Result.Name}}</h1>{{end}}
```

## Return Types

```go
func (s Server) M() T              // .Result=T
func (s Server) M() (T, error)     // .Result=T, .Err=error
func (s Server) M() (T, bool)      // bool=false skips template
func (s Server) M() error          // .Err=error
```

## Status Codes

1. **Template name:** `{{define "POST /user 201 Create(ctx, form)"}}`
2. **Result method:** `func (r R) StatusCode() int { return 201 }`
3. **Result field:** `type R struct { StatusCode int }`
4. **Error method:** `func (e Err) StatusCode() int { return 404 }`
5. **Template:** `{{with .StatusCode 404}}...{{end}}`

## Setup

**main.go:**
```go
package main

import (
    "context"
    "embed"
    "html/template"
    "net/http"
)

//go:embed *.gohtml
var templateFS embed.FS

//go:generate muxt generate --use-receiver-type=Server
var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))

type Server struct{ db Database }

func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    return s.db.GetUser(ctx, id)
}

func main() {
    mux := http.NewServeMux()
    TemplateRoutes(mux, Server{db: NewDB()})
    http.ListenAndServe(":8080", mux)
}
```

Run: `go generate && go run .`

## CLI

```bash
muxt generate --use-receiver-type=Server   # Generate handlers
muxt check --use-receiver-type=Server      # Type check only
muxt generate --use-receiver-type=Server --output-routes-func-with-logger-param --output-routes-func-with-path-prefix-param
```

Key flags:
- `--use-receiver-type=T` — Type with handler methods
- `--output-file=routes.go` — Output filename
- `--output-routes-func-with-logger-param` — Add `*slog.Logger` parameter
- `--output-routes-func-with-path-prefix-param` — Add path prefix parameter
- `-C ./web` — Run from directory

## Generated Files

- `template_routes.go` — `TemplateRoutes()`, `TemplateData[T]`, `TemplateRoutePaths`
- `*_template_routes_gen.go` — Per-source handlers and interfaces

## HTMX

```gotemplate
{{if .Request.Header.Get "HX-Request"}}
  <div id="result">{{.Result.Name}}</div>
{{else}}
  <!DOCTYPE html><html>...full page...</html>
{{end}}
```

Set response headers:
```gotemplate
{{with .Header "HX-Redirect" "/"}}{{end}}
```

## Testing

```go
func TestGetUser(t *testing.T) {
    server := new(fake.Server)
    server.GetUserReturns(User{Name: "Alice"}, nil)

    mux := http.NewServeMux()
    paths := TemplateRoutes(mux, server)

    req := httptest.NewRequest(http.MethodGet, paths.GetUser(42), nil)
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code)
}
```

DOM testing with `github.com/typelate/dom/domtest`:
```go
doc := domtest.ParseResponseDocument(t, rec.Result())
h1 := doc.QuerySelector("h1")
assert.Equal(t, "Alice", h1.TextContent())
```

## Common Patterns

**Embedded methods work:**
```go
type Server struct {
    Auth  // Login() promoted
}
```

**Multiple template directories:**
```go
//go:embed *.gohtml */*.gohtml */*/*.gohtml
var templateFS embed.FS
```

**Custom template functions:**
```go
var templates = template.Must(
    template.New("").
        Funcs(template.FuncMap{"upper": strings.ToUpper}).
        ParseFS(templateFS, "*.gohtml"),
)
```

**Custom type parsing:**
```go
type UserID string
func (id *UserID) UnmarshalText(text []byte) error {
    *id = UserID(strings.ToLower(string(text)))
    return nil
}
```

## Troubleshooting

| Error | Solution |
|-------|----------|
| "No templates found" | `templates` must be package-level |
| "Method not found" | Check `--find-receiver-type`, method exported |
| 400 Bad Request | Path param parse failure |
| Template panic | Check `.Err` before `.Result` fields |

## Summary

1. Write template: `{{define "GET /user/{id} GetUser(ctx, id)"}}`
2. Implement method: `func (s Server) GetUser(ctx context.Context, id int) (User, error)`
3. Generate: `muxt generate --find-receiver-type=Server`
4. Wire: `TemplateRoutes(mux, server)`

Templates are contracts. Methods implement behavior. Muxt generates the glue.