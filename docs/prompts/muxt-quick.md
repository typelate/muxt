# Muxt Quick Reference

A concise guide for generating type-safe HTTP handlers from Go HTML templates using Muxt.

## Core Concept

Muxt generates HTTP handlers by reading template names. Templates are the single source of truth for routes and parameters. The generated code uses only Go's standard library.

## Template Name Syntax

Template names follow this pattern:

```
[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]
```

**Examples:**
```gotemplate
{{define "GET /"}}...{{end}}
{{define "GET /user/{id}"}}...{{end}}
{{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}
{{define "GET /article/{id} GetArticle(ctx, id)"}}...{{end}}
```

### Components

- **METHOD** (optional): GET, POST, PATCH, DELETE, PUT
- **HOST** (optional): example.com or 192.168.1.1
- **PATH** (required): Must start with `/`, can include `{param}` placeholders
- **HTTP_STATUS** (optional): 200, 201, 404, etc. or http.StatusOK
- **CALL** (optional): MethodName(param1, param2, ...)

## Template Data Structure

Every template receives a `TemplateData[T]` struct with private fields accessed via methods:

```go
type TemplateData[T any] struct {
    // All fields are private
    result  T
    errList []error
    request *http.Request
    // ... other internal fields
}

// Access data via methods (Go templates call these automatically)
func (d *TemplateData[T]) Result() T
func (d *TemplateData[T]) Err() error
func (d *TemplateData[T]) Request() *http.Request
func (d *TemplateData[T]) MuxtVersion() string
```

### Accessing Data in Templates

In templates, use `.Result`, `.Err`, `.Request` - Go automatically calls the methods:

**Note:** Go templates automatically call zero-argument methods, so `.Result` calls `Result()`, `.Err` calls `Err()`, etc.

```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
<!DOCTYPE html>
<html>
<body>
  {{if .Err}}
    <div class="error">{{.Err.Error}}</div>
  {{else}}
    <h1>{{.Result.Name}}</h1>
    <p>{{.Result.Email}}</p>
  {{end}}

  <!-- Access request data -->
  <p>Path: {{.Request.URL.Path}}</p>
  <p>User-Agent: {{.Request.Header.Get "User-Agent"}}</p>
</body>
</html>
{{end}}
```

## Basic Receiver Methods

### Simple GET Handler

Template:
```gotemplate
{{define "GET / Home()"}}
<h1>{{.Result}}</h1>
{{end}}
```

Go method:
```go
type Server struct{}

func (Server) Home() string {
    return "Hello, world!"
}
```

### With Path Parameters

Template:
```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
<h1>{{.Result.Name}}</h1>
{{end}}
```

Go method:
```go
type User struct {
    Name  string
    Email string
}

func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    // id is automatically parsed from string to int
    user, err := s.db.GetUser(ctx, id)
    return user, err
}
```

### With Form Data

Template:
```gotemplate
{{define "POST /login 200 Login(ctx, username, password)"}}
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  <div>Welcome, {{.Result.Username}}!</div>
{{end}}
{{end}}
```

Go method:
```go
type Session struct {
    Username string
    Token    string
}

func (s Server) Login(ctx context.Context, username, password string) (Session, error) {
    // username and password come from form fields
    session, err := s.auth.Login(ctx, username, password)
    return session, err
}
```

## Special Parameter Names

Muxt recognizes these special parameter names in method calls:

- **`ctx`** → `context.Context` from `request.Context()`
- **`request`** → `*http.Request`
- **`response`** → `http.ResponseWriter`
- **`form`** → `url.Values` from `request.Form`
- **Path parameters** (like `{id}`) → parsed to the method's parameter type

Example:
```gotemplate
{{define "POST /upload Upload(ctx, response, request)"}}
{{end}}
```

```go
func (s Server) Upload(ctx context.Context, response http.ResponseWriter, request *http.Request) error {
    // Full access to HTTP primitives when needed
    file, _, err := request.FormFile("file")
    if err != nil {
        return err
    }
    defer file.Close()

    response.Header().Set("Content-Type", "application/json")
    // ...
    return nil
}
```

## Return Types

Muxt supports these return patterns:

```go
func Method() T                  // Single value (.Result = T, .Err = nil)
func Method() (T, error)         // Value and error (.Result = T, .Err = error)
func Method() (T, bool)          // Value and boolean (if bool is true, skip template)
func Method() error              // Error only (.Result = struct{}, .Err = error)
```

### Error Handling

Always check `.Err` in templates:

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
{{if .Err}}
  <div class="error">Article not found</div>
{{else}}
  <h1>{{.Result.Title}}</h1>
  <p>{{.Result.Content}}</p>
{{end}}
{{end}}
```

## HTML Requirements

Templates must:
- Be valid, well-formed HTML5
- Use Go template actions ({{.Field}}, {{if}}, {{range}}, etc.)
- Reference only fields that exist on the Result type
- Prefer semantic HTML elements

**Good:**
```gotemplate
{{define "GET /posts ListPosts()"}}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Posts</title>
</head>
<body>
    <main>
        {{range .Result}}
        <article>
            <h2>{{.Title}}</h2>
            <p>{{.Summary}}</p>
        </article>
        {{end}}
    </main>
</body>
</html>
{{end}}
```

## Setup and Generation

### 1. Create Template Variable

```go
package main

import (
    "embed"
    "html/template"
)

//go:embed *.gohtml
var templateFS embed.FS

//go:generate muxt generate --receiver-type=Server
var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))
```

### 2. Define Your Receiver Type

```go
type Server struct {
    db Database
}

func (s Server) Home() string {
    return "Hello, world!"
}
```

### 3. Generate Handlers

```bash
go generate
```

This creates `template_routes.go` with the `TemplateRoutes` function.

### 4. Wire Up Routes

```go
func main() {
    mux := http.NewServeMux()
    srv := Server{db: db}
    TemplateRoutes(mux, srv)
    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

## Essential Rules

1. **Template names are routes** - The template name defines the HTTP route and method
2. **Method signatures must match** - Template call parameters must match Go method parameters exactly
3. **Types must be compatible** - Template actions must reference fields that exist on Result type
4. **Use .Result for data** - Access method return value via `.Result` in templates
5. **Check .Err for errors** - Always handle errors in templates with `{{if .Err}}`
6. **Path params are typed** - With `--receiver-type`, path parameters are parsed to match method signature types

## Common Patterns

### Static Page
```gotemplate
{{define "GET /about"}}
<h1>About Us</h1>
<p>This is the about page.</p>
{{end}}
```

No method call needed. Template renders as-is.

### Dynamic Page with Data
```gotemplate
{{define "GET /dashboard Dashboard(ctx)"}}
<h1>Dashboard</h1>
<p>Total users: {{.Result.UserCount}}</p>
{{end}}
```

### Form Submission
```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}
<div>User {{.Result.Username}} created!</div>
{{end}}
```

```go
type UserForm struct {
    Username string
    Email    string
}

func (s Server) CreateUser(ctx context.Context, form UserForm) (User, error) {
    return s.db.CreateUser(ctx, form.Username, form.Email)
}
```

### Status Codes

Specify status code in template name:
```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}
<!-- Returns 201 Created on success -->
{{end}}
```

Or use StatusCode field/method on result type:
```go
type Result struct {
    Data       any
    StatusCode int
}

func (s Server) GetUser(ctx context.Context, id int) (Result, error) {
    user, err := s.db.GetUser(ctx, id)
    if err != nil {
        return Result{StatusCode: 404}, err
    }
    return Result{Data: user, StatusCode: 200}, nil
}
```

## Type Parsing

Muxt automatically parses these types from path parameters and form fields using Go's `strconv` package:

- **Strings**: `string` (no parsing)
- **Integers**: `int`, `int8`, `int16`, `int32`, `int64`, `uint`, `uint8`, `uint16`, `uint32`, `uint64`
- **Boolean**: `bool` - Uses `strconv.ParseBool` (accepts: `1`, `t`, `T`, `TRUE`, `true`, `True`, `0`, `f`, `F`, `FALSE`, `false`, `False`)
- **Custom**: Any type implementing `encoding.TextUnmarshaler`

If parsing fails, Muxt returns `400 Bad Request`.

## Quick Command Reference

```bash
# Generate handlers
muxt generate --receiver-type=Server

# Check types without generating
muxt check --receiver-type=Server

# Generate with custom names
muxt generate \
  --receiver-type=Server \
  --routes-func=Routes \
  --output-file=routes.go

# View help
muxt help
```

## Minimal Complete Example

**index.gohtml:**
```gotemplate
{{define "GET / Home()"}}
<!DOCTYPE html>
<html>
<head><title>Hello</title></head>
<body><h1>{{.Result}}</h1></body>
</html>
{{end}}
```

**main.go:**
```go
package main

import (
    "embed"
    "html/template"
    "log"
    "net/http"
)

//go:embed *.gohtml
var templateFS embed.FS

//go:generate muxt generate --receiver-type=Server
var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))

type Server struct{}

func (Server) Home() string {
    return "Hello, world!"
}

func main() {
    mux := http.NewServeMux()
    TemplateRoutes(mux, Server{})
    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

**Run:**
```bash
go generate
go run .
```

Visit `http://localhost:8080` to see "Hello, world!"

## Navigating Generated Code

The generated `template_routes.go` file contains (in order):
1. `RoutesReceiver` interface
2. `TemplateRoutes()` function (bulk of file)
3. `TemplateData[T]` type and methods
4. `TemplateRoutePaths` type and path methods (at the end)

**Finding types:**
- Search: `type TemplateRoutePaths` or `type TemplateData`
- Pattern: `func (routePaths TemplateRoutePaths)` for path methods
- MCP: Use `go_search "TemplateRoutePaths"` or `go_symbol_references`
