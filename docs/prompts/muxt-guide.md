# Muxt Practical Guide

A comprehensive guide for building type-safe HTTP handlers with Muxt. This guide covers everything you need to build production-ready web applications using Go HTML templates.

## What is Muxt?

Muxt generates HTTP handlers from Go HTML templates. Templates are the single source of truth—they define routes, parameters, and method calls. Generated code uses only Go's standard library.

**Core principle:** Templates declare the interface. Go methods implement the behavior. Muxt generates the glue.

## Template Name Syntax

Template names follow this exact pattern:

```
[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]
```

### Full Syntax Components

| Component | Required | Description | Examples |
|-----------|----------|-------------|----------|
| METHOD | No | HTTP method | GET, POST, PATCH, DELETE, PUT |
| HOST | No | Host matcher | example.com, api.example.com, 192.168.1.1 |
| PATH | Yes | URL path pattern | /, /user/{id}, /api/v1/posts |
| HTTP_STATUS | No | Success status code | 200, 201, 404, http.StatusCreated |
| CALL | No | Method invocation | Home(), GetUser(ctx, id) |

### Complete Examples

```gotemplate
{{/* Simple static route */}}
{{define "GET /"}}...{{end}}

{{/* Path parameter */}}
{{define "GET /user/{id}"}}...{{end}}

{{/* With status code */}}
{{define "POST /user 201"}}...{{end}}

{{/* With method call */}}
{{define "GET /user/{id} GetUser(ctx, id)"}}...{{end}}

{{/* Complete: method, path, status, call */}}
{{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}

{{/* With host */}}
{{define "GET api.example.com/status Health()"}}...{{end}}

{{/* Complex path with multiple params */}}
{{define "GET /user/{userID}/post/{postID} GetPost(ctx, userID, postID)"}}...{{end}}

{{/* HTTP method variations */}}
{{define "PATCH /user/{id} UpdateUser(ctx, id, form)"}}...{{end}}
{{define "DELETE /user/{id} DeleteUser(ctx, id)"}}...{{end}}
```

### Path Patterns

**Exact match with `{$}`:**
```gotemplate
{{define "GET /{$}"}}
<!-- Matches only "/" exactly, not "/foo" -->
{{end}}
```

**Prefix match with trailing slash:**
```gotemplate
{{define "GET /static/"}}
<!-- Matches "/static/" and all sub-paths like "/static/css/main.css" -->
{{end}}
```

**Wildcard path segments:**
```gotemplate
{{define "GET /files/{path...}"}}
<!-- path captures the rest of the URL -->
{{end}}
```

## TemplateData Structure

Every template receives `TemplateData[T]` with private fields accessed via methods:

```go
type TemplateData[T any] struct {
    // All fields are private
    result        T
    errList       []error
    request       *http.Request
    response      http.ResponseWriter
    statusCode    int
    // ... other internal fields
}

// Access data via methods (Go templates call these automatically)
func (d *TemplateData[T]) Result() T
func (d *TemplateData[T]) Err() error
func (d *TemplateData[T]) Request() *http.Request
func (d *TemplateData[T]) MuxtVersion() string
func (d *TemplateData[T]) StatusCode(int) *TemplateData[T]
func (d *TemplateData[T]) Header(key, value string) *TemplateData[T]
```

### Accessing Fields

In templates, use `.Result`, `.Err`, `.Request` - Go automatically calls the methods:

**Important:** Go templates automatically call zero-argument methods when you reference them. Writing `.Result` in a template calls the `Result()` method. All TemplateData fields are private and accessed this way.

```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
<!DOCTYPE html>
<html>
<body>
  {{/* Check for errors first */}}
  {{if .Err}}
    <div class="error">{{.Err.Error}}</div>
  {{else}}
    {{/* Access result fields */}}
    <h1>{{.Result.Name}}</h1>
    <p>Email: {{.Result.Email}}</p>
    <p>Active: {{.Result.Active}}</p>
  {{end}}

  {{/* Access request data */}}
  <p>Request path: {{.Request.URL.Path}}</p>
  <p>Query param: {{.Request.URL.Query.Get "filter"}}</p>
  <p>User agent: {{.Request.Header.Get "User-Agent"}}</p>

  {{/* Path parameters via Request */}}
  <p>User ID from path: {{.Request.PathValue "id"}}</p>

  {{/* Check for HTMX requests */}}
  {{if .Request.Header.Get "HX-Request"}}
    <!-- Render partial for HTMX -->
  {{else}}
    <!-- Render full page -->
  {{end}}
</body>
</html>
{{end}}
```

## HTTP Methods

### GET Requests

```gotemplate
{{define "GET /users ListUsers(ctx)"}}
<ul>
{{range .Result}}
  <li>{{.Name}} - {{.Email}}</li>
{{end}}
</ul>
{{end}}
```

```go
type User struct {
    Name  string
    Email string
}

func (s Server) ListUsers(ctx context.Context) ([]User, error) {
    return s.db.GetAllUsers(ctx)
}
```

### POST Requests

```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}
<div>User {{.Result.Name}} created with ID {{.Result.ID}}</div>
{{end}}
```

```go
type UserForm struct {
    Name     string
    Email    string
    Password string
}

type User struct {
    ID    int
    Name  string
    Email string
}

func (s Server) CreateUser(ctx context.Context, form UserForm) (User, error) {
    user, err := s.db.CreateUser(ctx, form.Name, form.Email, form.Password)
    return user, err
}
```

### PATCH Requests

```gotemplate
{{define "PATCH /user/{id} UpdateUser(ctx, id, form)"}}
<div>User updated successfully</div>
{{end}}
```

```go
type UpdateUserForm struct {
    Name  string
    Email string
}

func (s Server) UpdateUser(ctx context.Context, id int, form UpdateUserForm) error {
    return s.db.UpdateUser(ctx, id, form.Name, form.Email)
}
```

### DELETE Requests

```gotemplate
{{define "DELETE /user/{id} 204 DeleteUser(ctx, id)"}}
{{/* 204 No Content - template can be empty */}}
{{end}}
```

```go
func (s Server) DeleteUser(ctx context.Context, id int) error {
    return s.db.DeleteUser(ctx, id)
}
```

## Call Parameters

### Special Parameter Names

Muxt recognizes these special names in method signatures:

- **`ctx`** → `context.Context` from `request.Context()`
- **`request`** → `*http.Request`
- **`response`** → `http.ResponseWriter`
- **`form`** → `url.Values` from `request.Form`

### Path Parameters

Path parameters like `{id}` are extracted from the URL:

```gotemplate
{{define "GET /user/{userID}/post/{postID} GetPost(ctx, userID, postID)"}}
```

```go
func (s Server) GetPost(ctx context.Context, userID int, postID int) (Post, error) {
    // userID and postID parsed from path
    return s.db.GetPost(ctx, userID, postID)
}
```

**Without `--receiver-type`:** Path params are always `string`

**With `--receiver-type=Server`:** Path params are parsed to match method signature types (int, bool, etc.)

### Form Parameters

Parameters not in the path or special names come from form data:

```gotemplate
{{define "POST /login Login(ctx, username, password)"}}
```

```go
func (s Server) Login(ctx context.Context, username, password string) (Session, error) {
    // username and password from form fields
    session, err := s.auth.Login(ctx, username, password)
    return session, err
}
```

### Form Structs

Use a struct to group form parameters:

```go
type LoginForm struct {
    Username string
    Password string
    Remember bool
}

func (s Server) Login(ctx context.Context, form LoginForm) (Session, error) {
    // form fields automatically populated
    if form.Remember {
        // ...
    }
    return s.auth.Login(ctx, form.Username, form.Password)
}
```

Template:
```gotemplate
{{define "POST /login Login(ctx, form)"}}
```

**Struct field tags:**
```go
type LoginForm struct {
    Username string `name:"user-name"`   // Maps from "user-name" form field
    Password string `name:"user-pass"`   // Maps from "user-pass" form field
    Remember bool   `name:"remember-me"` // Maps from "remember-me" checkbox
}
```

### Mixing Parameter Types

Combine path params, form params, and special params:

```gotemplate
{{define "POST /user/{id}/update UpdateUser(ctx, id, form)"}}
```

```go
type UpdateUserForm struct {
    Name  string
    Email string
}

func (s Server) UpdateUser(ctx context.Context, id int, form UpdateUserForm) (User, error) {
    // id from path, form fields from request body
    return s.db.UpdateUser(ctx, id, form.Name, form.Email)
}
```

### Using http.ResponseWriter

For special cases like file downloads:

```gotemplate
{{define "GET /download/{id} Download(response, id)"}}
{{end}}
```

```go
func (s Server) Download(response http.ResponseWriter, id string) error {
    file, err := s.storage.Get(id)
    if err != nil {
        return err
    }

    response.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file.Name))
    response.Header().Set("Content-Type", file.MimeType)
    response.WriteHeader(http.StatusOK)
    io.Copy(response, file.Reader)
    return nil
}
```

## Type Parsing

Muxt automatically parses these types from strings using Go's `strconv` package:

### Numeric Types
- `int`, `int8`, `int16`, `int32`, `int64` (using `strconv.Atoi` or `strconv.ParseInt`)
- `uint`, `uint8`, `uint16`, `uint32`, `uint64` (using `strconv.ParseUint`)

### Boolean
- `bool` - Uses `strconv.ParseBool` which accepts:
  - True: `1`, `t`, `T`, `TRUE`, `true`, `True`
  - False: `0`, `f`, `F`, `FALSE`, `false`, `False`

### String
- `string` - No parsing, passed through directly

### Custom Types with TextUnmarshaler

Implement `encoding.TextUnmarshaler`:

```go
type UserID string

func (id *UserID) UnmarshalText(text []byte) error {
    *id = UserID(strings.ToLower(string(text)))
    return nil
}

func (s Server) GetUser(ctx context.Context, id UserID) (User, error) {
    // id is custom type, parsed via UnmarshalText
    return s.db.GetUser(ctx, string(id))
}
```

### Type Parsing Errors

If a parameter can't be parsed, Muxt returns `400 Bad Request`:

```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
```

```go
func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    // ...
}
```

Request to `/user/abc` → 400 Bad Request (can't parse "abc" to int)

## Return Types

### Supported Return Signatures

```go
// Single value
func (s Server) Method() T

// Value and error
func (s Server) Method() (T, error)

// Value and boolean (true = skip template execution)
func (s Server) Method() (T, bool)

// Error only
func (s Server) Method() error
```

### Single Value

```go
func (s Server) GetGreeting() string {
    return "Hello, world!"
}
```

Template:
```gotemplate
{{define "GET / GetGreeting()"}}
<h1>{{.Result}}</h1>
{{/* .Err is always nil */}}
{{end}}
```

### Value and Error

```go
func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    user, err := s.db.FindUser(ctx, id)
    if err != nil {
        return User{}, fmt.Errorf("user not found: %w", err)
    }
    return user, nil
}
```

Template:
```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  <h1>{{.Result.Name}}</h1>
  <p>{{.Result.Email}}</p>
{{end}}
{{end}}
```

### Value and Boolean

```go
func (s Server) GetCachedUser(ctx context.Context, id int) (User, bool) {
    user, ok := s.cache.Get(id)
    if ok {
        // true means: I've handled the response, skip template
        return user, true
    }
    // false means: execute template with user data
    return user, false
}
```

**Use case:** Cache hits, redirects, or when you've already written the response.

### Error Only

```go
func (s Server) Healthcheck() error {
    if err := s.db.Ping(); err != nil {
        return err
    }
    return nil
}
```

Template:
```gotemplate
{{define "GET /health Healthcheck()"}}
{{if .Err}}
  <div class="error">Unhealthy: {{.Err.Error}}</div>
{{else}}
  <div class="success">Healthy</div>
{{end}}
{{/* .Result is struct{} (empty) */}}
{{end}}
```

## Status Codes

### Method 1: Specify in Template Name

```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}
<!-- Returns 201 Created on success -->
{{end}}

{{define "GET /admin 401"}}
<!-- Returns 401 Unauthorized -->
{{end}}
```

### Method 2: StatusCode() Method on Result

```go
type UserResult struct {
    User User
    code int
}

func (r UserResult) StatusCode() int {
    return r.code
}

func (s Server) GetUser(ctx context.Context, id int) (UserResult, error) {
    user, err := s.db.GetUser(ctx, id)
    if err != nil {
        return UserResult{code: 404}, err
    }
    return UserResult{User: user, code: 200}, nil
}
```

### Method 3: StatusCode Field on Result

```go
type UserResult struct {
    User       User
    StatusCode int
}

func (s Server) GetUser(ctx context.Context, id int) (UserResult, error) {
    user, err := s.db.GetUser(ctx, id)
    if err != nil {
        return UserResult{StatusCode: 404}, err
    }
    return UserResult{User: user, StatusCode: 200}, nil
}
```

### Method 4: Custom Error Types

```go
type NotFoundError struct {
    Message string
}

func (e NotFoundError) Error() string {
    return e.Message
}

func (e NotFoundError) StatusCode() int {
    return http.StatusNotFound
}

func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    user, err := s.db.FindUser(ctx, id)
    if err != nil {
        return User{}, NotFoundError{Message: "user not found"}
    }
    return user, nil
}
```

The handler automatically uses `404 Not Found` when this error is returned.

## Form Validation and Input Attributes

Muxt can read HTML input validation attributes to generate better error messages:

```gotemplate
{{define "POST /user CreateUser(ctx, form)"}}
<form method="post">
  <input type="text" name="Username" minlength="3" maxlength="20">
  <input type="email" name="Email">
  <input type="number" name="Age" min="18" max="120">
  <input type="text" name="Website" pattern="https?://.+">
  <button type="submit">Create User</button>
</form>
{{end}}
```

Muxt reads these attributes and generates validation for:
- `minlength`/`maxlength` - String length constraints
- `min`/`max` - Numeric range constraints
- `pattern` - Regex validation

## Error Handling Patterns

### Pattern 1: Inline Error Messages

```gotemplate
{{define "POST /user CreateUser(ctx, form)"}}
{{if .Err}}
  <div class="alert alert-error">
    {{.Err.Error}}
  </div>
{{else}}
  <div class="alert alert-success">
    User {{.Result.Username}} created!
  </div>
{{end}}
{{end}}
```

### Pattern 2: Error Template Reuse

```gotemplate
{{define "error-message"}}
  <div class="error" id="error-{{.Type}}">{{.Error}}</div>
{{end}}

{{define "GET /user/{id} GetUser(ctx, id)"}}
{{if .Err}}
  {{template "error-message" .Err}}
{{else}}
  <h1>{{.Result.Name}}</h1>
{{end}}
{{end}}
```

### Pattern 3: Conditional Rendering Based on Error Type

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
{{if .Err}}
  {{if eq (printf "%T" .Err) "*errors.NotFoundError"}}
    <h1>404 - Article Not Found</h1>
  {{else}}
    <h1>500 - Server Error</h1>
    <p>{{.Err.Error}}</p>
  {{end}}
{{else}}
  <article>
    <h1>{{.Result.Title}}</h1>
    <div>{{.Result.Content}}</div>
  </article>
{{end}}
{{end}}
```

## CLI Commands

### Generate Handlers

```bash
# Basic generation (creates multiple files)
muxt generate

# With receiver type
muxt generate --receiver-type=Server

# With custom output file prefix
muxt generate --receiver-type=Server --output-file=routes.go

# With custom function name
muxt generate --receiver-type=Server --routes-func=RegisterRoutes

# With multiple options
muxt generate \
  --receiver-type=Server \
  --receiver-type-package=github.com/myapp/internal/server \
  --routes-func=Routes \
  --receiver-interface=Handler \
  --output-file=routes.go
```

### Type Checking

```bash
# Check types without generating
muxt check --receiver-type=Server

# Check with all options
muxt check \
  --receiver-type=Server \
  --receiver-type-package=github.com/myapp/internal/server
```

### Other Commands

```bash
# View version
muxt version

# View help
muxt help

# Generate from different directory
muxt -C ./web generate --receiver-type=Server
```

### Common Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--receiver-type` | Type name for method lookup | _(inferred)_ |
| `--receiver-type-package` | Package path for receiver type | _(current package)_ |
| `--output-file` | Generated file name | `template_routes.go` |
| `--routes-func` | Function name for route registration | `TemplateRoutes` |
| `--receiver-interface` | Interface name in generated file | `RoutesReceiver` |
| `--template-data-type` | Type name for template data | `TemplateData` |
| `--templates-variable` | Templates variable name | `templates` |

## Setup Workflow

### 1. Create Template Files

**pages.gohtml:**
```gotemplate
{{define "GET / Home()"}}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Home</title>
</head>
<body>
    <h1>{{.Result}}</h1>
</body>
</html>
{{end}}

{{define "GET /about"}}
<h1>About Us</h1>
<p>We build great software.</p>
{{end}}
```

### 2. Create Go Package

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

type Server struct {
    db Database
}

func (s Server) Home() string {
    return "Welcome to our site!"
}

func main() {
    db := connectDB()
    srv := Server{db: db}

    mux := http.NewServeMux()
    TemplateRoutes(mux, srv)

    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

### 3. Generate and Run

```bash
# Generate handlers
go generate

# Run the server
go run .
```

## Advanced Patterns

### Embedded Methods

```go
type Auth struct{}

func (Auth) Login(ctx context.Context, username, password string) (Session, error) {
    // ...
}

type Server struct {
    Auth  // Embedded field
    db Database
}
```

Templates can call `Login` on `Server` because it's promoted from `Auth`.

### Pointer Receivers

Both value and pointer receivers work:

```go
func (s Server) GetUser(ctx context.Context, id int) (User, error)   // Value
func (s *Server) GetUser(ctx context.Context, id int) (User, error)  // Pointer
```

### Multiple Form Fields of Same Name (Slices)

```go
type FilterForm struct {
    Tags []string  // Multiple "tags" form fields
}

func (s Server) FilterPosts(ctx context.Context, form FilterForm) ([]Post, error) {
    return s.db.GetPostsByTags(ctx, form.Tags)
}
```

HTML:
```html
<form method="post" action="/filter">
  <input type="checkbox" name="Tags" value="go"> Go
  <input type="checkbox" name="Tags" value="web"> Web
  <input type="checkbox" name="Tags" value="htmx"> HTMX
  <button type="submit">Filter</button>
</form>
```

## Best Practices

### 1. Keep Templates Focused

Each template should handle one route and one concern:

**Good:**
```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
<div>{{.Result.Name}}</div>
{{end}}

{{define "GET /user/{id}/posts GetUserPosts(ctx, id)"}}
<div>{{range .Result}}...{{end}}</div>
{{end}}
```

**Avoid:**
```gotemplate
{{/* Don't try to handle multiple routes in one template */}}
```

### 2. Return Static Types

**Good:**
```go
func (s Server) GetUser(ctx context.Context, id int) (User, error)
func (s Server) GetUsers(ctx context.Context) ([]User, error)
```

**Avoid:**
```go
func (s Server) GetUser(ctx context.Context, id int) (any, error)          // Type checker can't help
func (s Server) GetUser(ctx context.Context, id int) (interface{}, error)  // Type checker can't help
```

### 3. Use Semantic HTML

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>{{.Result.Title}}</title>
</head>
<body>
    <main>
        <article>
            <header>
                <h1>{{.Result.Title}}</h1>
                <time datetime="{{.Result.Published.Format "2006-01-02"}}">
                    {{.Result.Published.Format "January 2, 2006"}}
                </time>
            </header>
            <section>
                {{.Result.Content}}
            </section>
        </article>
    </main>
</body>
</html>
{{end}}
```

### 4. Handle Errors Explicitly

Always check `.Err` in templates and provide user-friendly messages:

```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
{{if .Err}}
  <div role="alert" class="error">
    <h2>Unable to Load User</h2>
    <p>{{.Err.Error}}</p>
  </div>
{{else}}
  <!-- Success state -->
{{end}}
{{end}}
```

### 5. Leverage HTMX for Interactivity

```gotemplate
{{define "GET /search Search(ctx, query)"}}
{{if .Request.Header.Get "HX-Request"}}
  {{/* Partial for HTMX */}}
  <div id="results">
    {{range .Result}}
      <div class="result">{{.Title}}</div>
    {{end}}
  </div>
{{else}}
  {{/* Full page */}}
  <!DOCTYPE html>
  <html>
  <head>
      <script src="https://unpkg.com/htmx.org"></script>
  </head>
  <body>
      <input type="search"
             name="query"
             hx-get="/search"
             hx-trigger="keyup changed delay:500ms"
             hx-target="#results">
      <div id="results"></div>
  </body>
  </html>
{{end}}
{{end}}
```

## Troubleshooting

### "No templates found"
- Ensure `templates` variable is package-level
- Check that `//go:embed` directive is correct
- Verify template files exist and match the glob pattern

### "Method not found on receiver"
- Check method signature matches template call exactly
- Ensure you're using `--receiver-type=YourType`
- Verify the method is exported (capitalized)

### "Type mismatch" errors
- Run `muxt check` to see specific type errors
- Ensure template actions reference fields that exist on Result type
- Check parameter types match method signature

### "400 Bad Request" at runtime
- Path parameter failed to parse (e.g., "abc" to int)
- Form field missing or invalid type
- Check method signature accepts the correct types

## Working with Generated Code

### File Structure

Muxt generates multiple files based on your template structure:

#### Main `template_routes.go` contains:

1. **`RoutesReceiver` interface** - Interface that embeds per-file interfaces
2. **`TemplateRoutes()` function** - Main orchestration function (calls per-file functions)
3. **`TemplateData[T]` type** - Template data structure
4. **`TemplateData` methods** - Methods like `Result()`, `Err()`, `Request()`, `Path()`
5. **`TemplateRoutePaths` type** - Path helper struct (near end of file)
6. **`TemplateRoutePaths` methods** - Path generation methods (at end of file)

#### Per-file `*_template_routes_gen.go` contains:

Each `.gohtml` file gets its own generated file (e.g., `index_template_routes_gen.go` for `index.gohtml`):

1. **File-specific interface** - e.g., `IndexRoutesReceiver` for methods used in that file
2. **File-specific function** - e.g., `IndexTemplateRoutes()` that registers routes from that file
3. **HTTP handlers** - All handlers for templates defined in that `.gohtml` file

### Finding Types and Methods

**Using grep/search (across multiple files):**
```bash
# Find TemplateRoutePaths type definition
grep "type TemplateRoutePaths struct" template_routes.go

# Find all path helper methods
grep "^func (routePaths TemplateRoutePaths)" template_routes.go

# Find all TemplateData methods
grep "^func (data \*TemplateData" template_routes.go

# Find RoutesReceiver interface (main)
grep "type RoutesReceiver interface" template_routes.go

# Find file-specific interfaces
grep "type .*RoutesReceiver interface" *_template_routes_gen.go

# Find all HTTP handlers across files
grep "mux.HandleFunc" *_template_routes_gen.go
```

**Using MCP (for LLM agents):**
```
# Search for types across workspace
go_search "TemplateRoutePaths"

# Find where a method is used
go_symbol_references file:"template_routes.go" symbol:"GetUser"

# Understand file structure
go_file_context file:"template_routes.go"
```

**Common patterns:**
- Path methods always return `string`: `func (routePaths TemplateRoutePaths) MethodName(...) string`
- TemplateData methods use pointer receiver: `func (data *TemplateData[T])`
- Receiver methods are in the interface: `type RoutesReceiver interface { ... }`

### What's Useful in Each Section

**RoutesReceiver interface:**
- Shows all methods your receiver type must implement
- Method signatures must match template calls exactly

**TemplateRoutes() function:**
- Contains all HTTP handler implementations
- Shows how parameters are parsed and validated
- Useful for debugging handler behavior

**TemplateData type and methods:**
- `.Result()` - Access method return value
- `.Err()` - Check for errors
- `.Request()` - Access HTTP request
- `.Path()` - Get path helper for URL generation

**TemplateRoutePaths type and methods:**
- One method per route for type-safe URL generation
- Example: `paths.GetUser(42)` → `"/user/42"`
- Located at the end of the file

## Summary

Muxt turns templates into HTTP handlers:
1. Write templates with route names: `{{define "GET /path Method()"}}`
2. Implement receiver methods with matching signatures
3. Run `muxt generate` to create handlers
4. Wire up with `TemplateRoutes(mux, receiver)`

Templates define the interface. Methods implement behavior. Muxt generates the glue. Everything else is standard Go.
