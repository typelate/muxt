# Muxt Complete Reference

The comprehensive guide to building production-ready web applications with Muxt. This reference includes all features, design principles, testing patterns, and real-world examples.

## Table of Contents

1. [Core Principles](#core-principles)
2. [Template Name Syntax](#template-name-syntax)
3. [Template Data Structure](#template-data-structure)
4. [Receiver Methods](#receiver-methods)
5. [Type System](#type-system)
6. [Package Structure](#package-structure)
7. [CLI Reference](#cli-reference)
8. [Testing](#testing)
9. [HTMX Integration](#htmx-integration)
10. [Advanced Patterns](#advanced-patterns)
11. [Real-World Examples](#real-world-examples)
12. [Troubleshooting](#troubleshooting)

---

## Template Name Syntax

### Complete BNF Specification

```bnf
<route> ::= [ <method> <space> ] [ <host> ] <path> [ <space> <http_status> ] [ <space> <call_expr> ]

<method> ::= "GET" | "POST" | "PUT" | "PATCH" | "DELETE"

<host> ::= <hostname> | <ip_address>

<hostname> ::= <label> { "." <label> }
<label> ::= <letter> { <letter> | <digit> | "-" }
<ip_address> ::= <digit>+ "." <digit>+ "." <digit>+ "." <digit>+

<path> ::= "/" [ <path_segment> { "/" <path_segment> } [ "/" ] ]
<path_segment> ::= <unreserved_characters>+ | "{" <identifier> "}" | "{" <identifier> "...}"

<http_status> ::= <integer> | <qualified_identifier>
<integer> ::= <digit> { <digit> }
<qualified_identifier> ::= <identifier> "." <identifier>

<call_expr> ::= <identifier> "(" [ <identifier> { "," <space> <identifier> } ] ")"

<identifier> ::= <letter> { <letter> | <digit> | "_" }

<space> ::= " "

<letter> ::= "a" | ... | "z" | "A" | ... | "Z"
<digit> ::= "0" | ... | "9"
<unreserved_characters> ::= <letter> | <digit> | "-" | "_" | "." | "~"
```

### All Syntax Variations

```gotemplate
{{/* Minimal: just a path */}}
{{define "/"}}...{{end}}

{{/* Method and path */}}
{{define "GET /"}}...{{end}}

{{/* Path parameters */}}
{{define "GET /user/{id}"}}...{{end}}
{{define "GET /user/{userID}/post/{postID}"}}...{{end}}

{{/* Wildcard path segment */}}
{{define "GET /files/{path...}"}}...{{end}}

{{/* Exact path match */}}
{{define "GET /{$}"}}...{{end}}

{{/* Prefix match */}}
{{define "GET /static/"}}...{{end}}

{{/* With status code */}}
{{define "POST /user 201"}}...{{end}}
{{define "GET /error 404"}}...{{end}}
{{define "GET /admin http.StatusUnauthorized"}}...{{end}}

{{/* With method call */}}
{{define "GET / Home()"}}...{{end}}
{{define "GET /user/{id} GetUser(ctx, id)"}}...{{end}}

{{/* Complete: method, path, status, call */}}
{{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}

{{/* With host */}}
{{define "GET example.com/"}}...{{end}}
{{define "GET api.example.com/v1/status Health()"}}...{{end}}

{{/* Most HTTP methods */}}
{{define "GET /resource"}}...{{end}}
{{define "POST /resource"}}...{{end}}
{{define "PUT /resource/{id}"}}...{{end}}
{{define "PATCH /resource/{id}"}}...{{end}}
{{define "DELETE /resource/{id}"}}...{{end}}
```

---

## Template Data Structure

### Full TemplateData Definition

```go
type TemplateData[T any] struct {
    // All fields are private
    receiver      RoutesReceiver
    response      http.ResponseWriter
    request       *http.Request
    result        T
    statusCode    int
    errStatusCode int
    okay          bool
    errList       []error
    redirectURL   string
    pathsPrefix   string
}

// Access data via methods (Go templates call these automatically)
func (d *TemplateData[T]) Result() T                                      { return d.result }
func (d *TemplateData[T]) Err() error                                     { return errors.Join(d.errList...) }
func (d *TemplateData[T]) Request() *http.Request                         { return d.request }
func (d *TemplateData[T]) MuxtVersion() string                            { return muxtVersion }
func (d *TemplateData[T]) Ok() bool                                       { return d.okay }  // true when method has no boolean second return
func (d *TemplateData[T]) StatusCode(code int) *TemplateData[T]           { d.statusCode = code; return d }
func (d *TemplateData[T]) Header(key, value string) *TemplateData[T]      { d.response.Header().Set(key, value); return d }
func (d *TemplateData[T]) Path() TemplateRoutePaths                       { return TemplateRoutePaths{pathsPrefix: d.pathsPrefix} }  // returns path helper for URL generation
```

### Accessing All Fields

In templates, use `.Result`, `.Err`, `.Request`, etc. - Go automatically calls the methods:

**Critical:** All TemplateData fields are private. Go templates automatically call zero-argument methods when referenced, so `.Result` calls `Result()`, `.Err` calls `Err()`, `.Request` calls `Request()`. This is standard Go template behavior, not Muxt magic.

```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
<!DOCTYPE html>
<html>
<head>
    <meta name="muxt-version" content="{{.MuxtVersion}}">
</head>
<body>
  {{/* Result: method return value */}}
  {{if not .Err}}
    <h1>{{.Result.Name}}</h1>
    <p>{{.Result.Email}}</p>
  {{end}}

  {{/* Err: method error */}}
  {{if .Err}}
    <div class="error">{{.Err.Error}}</div>
    <p>Error type: {{printf "%T" .Err}}</p>
  {{end}}

  {{/* Request: HTTP request data */}}
  <p>Path: {{.Request.URL.Path}}</p>
  <p>Method: {{.Request.Method}}</p>
  <p>Host: {{.Request.Host}}</p>

  {{/* Path values */}}
  <p>ID param: {{.Request.PathValue "id"}}</p>

  {{/* Query parameters */}}
  <p>Filter: {{.Request.URL.Query.Get "filter"}}</p>

  {{/* Headers */}}
  <p>User-Agent: {{.Request.Header.Get "User-Agent"}}</p>

  {{/* HTMX detection */}}
  {{if .Request.Header.Get "HX-Request"}}
    <p>This is an HTMX request</p>
  {{end}}

  {{/* Set status and headers from template */}}
  {{with and (.StatusCode 404) (.Header "X-Custom" "value")}}
    {{/* Both status and header set */}}
  {{end}}

  {{/* Check if template should be rendered (no boolean second return) */}}
  {{if .Ok}}
    <!-- This is shown only when handler doesn't return (T, bool) -->
  {{end}}

  {{/* Use path helper to generate type-safe URLs */}}
  <a href="{{.Path.GetUser 123}}">View User 123</a>
</body>
</html>
{{end}}
```

### Method Explanations

**`.Ok()` Method:**
- Returns `true` when the handler method does NOT have a boolean as the second return value
- Used internally to determine if the template should be executed
- For methods returning `(T, bool)`, if the bool is `false`, the template is NOT executed

**`.Path()` Method:**
- Returns a `TemplateRoutePaths` struct with methods for generating type-safe URLs
- Each route gets a corresponding method on the path helper
- Example: Route `GET /user/{id}` generates method `Path.GetUser(id int) string`

---

## Receiver Methods

### All Supported Signatures

```go
// Single return value
func (s Server) Method() T
func (s Server) Method(ctx context.Context) T
func (s Server) Method(ctx context.Context, id int) T

// Value and error
func (s Server) Method() (T, error)
func (s Server) Method(ctx context.Context) (T, error)
func (s Server) Method(ctx context.Context, id int) (T, error)

// Value and boolean (true = skip template)
func (s Server) Method() (T, bool)
func (s Server) Method(ctx context.Context) (T, bool)

// Error only
func (s Server) Method() error
func (s Server) Method(ctx context.Context) error

// With HTTP primitives
func (s Server) Method(response http.ResponseWriter, request *http.Request) error
func (s Server) Method(ctx context.Context, response http.ResponseWriter) error

// With form data
func (s Server) Method(form MyForm) (T, error)
func (s Server) Method(ctx context.Context, form MyForm) (T, error)

// Mixed parameters
func (s Server) Method(ctx context.Context, id int, form MyForm) (T, error)
```

### Special Parameter Names

| Parameter | Type | Description | Source |
|-----------|------|-------------|--------|
| `ctx` | `context.Context` | Request context | `request.Context()` |
| `request` | `*http.Request` | HTTP request | Direct |
| `response` | `http.ResponseWriter` | HTTP response writer | Direct |
| `form` | `url.Values` or struct | Form data | `request.Form` |
| Path params | Any supported type | URL path values | `request.PathValue(name)` |
| Form params | Any supported type | Form field values | `request.Form.Get(name)` |

### Status Code Control

**Method 1: Template name**
```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}
```

**Method 2: StatusCode() method**
```go
type Result struct {
    Data any
    code int
}

func (r Result) StatusCode() int {
    return r.code
}
```

**Method 3: StatusCode field**
```go
type Result struct {
    Data       any
    StatusCode int
}
```

**Method 4: Custom error with StatusCode()**
```go
type HTTPError struct {
    Message string
    Code    int
}

func (e HTTPError) Error() string { return e.Message }
func (e HTTPError) StatusCode() int { return e.Code }
```

**Method 5: From template**
```gotemplate
{{with .StatusCode 404}}
  <h1>Not Found</h1>
{{end}}
```

---

## Type System

### Supported Parameter Types

**Primitives:**
- `string` - No parsing
- `bool` - Uses `strconv.ParseBool` which accepts: `1`, `t`, `T`, `TRUE`, `true`, `True`, `0`, `f`, `F`, `FALSE`, `false`, `False`
- `int`, `int8`, `int16`, `int32`, `int64` (using `strconv.Atoi` or `strconv.ParseInt`)
- `uint`, `uint8`, `uint16`, `uint32`, `uint64` (using `strconv.ParseUint`)

**Special:**
- `context.Context` - From request context
- `*http.Request` - The request
- `http.ResponseWriter` - The response writer
- `url.Values` - Form values

**Custom:**
- Any type implementing `encoding.TextUnmarshaler`

### Custom Type Parsing

```go
// Implement TextUnmarshaler
type UserID string

func (id *UserID) UnmarshalText(text []byte) error {
    // Custom parsing logic
    normalized := strings.ToLower(string(text))
    if !isValidUserID(normalized) {
        return fmt.Errorf("invalid user ID: %s", text)
    }
    *id = UserID(normalized)
    return nil
}

// Use in method signature
func (s Server) GetUser(ctx context.Context, id UserID) (User, error) {
    return s.db.GetUser(ctx, string(id))
}
```

### Form Struct Field Types

**Supported field types:**
```go
type MyForm struct {
    // Strings
    Name     string
    Email    string

    // Numbers
    Age      int
    Count    uint

    // Booleans
    Subscribe bool
    Active    bool

    // Slices (multiple values with same name)
    Tags     []string
    IDs      []int

    // Nested (requires custom parsing)
    Address  Address  // Must implement TextUnmarshaler
}
```

**Field tags:**
```go
type LoginForm struct {
    Username string `name:"user_name"`   // Form field: user_name
    Password string `name:"user_pass"`   // Form field: user_pass
    Remember bool   `name:"remember_me"` // Form field: remember_me
    ApiKey   string `name:"api-key"`     // Form field: api-key
}
```

### Type Checking

Run `muxt check` to verify template types match Go types:

```bash
muxt check --receiver-type=Server
```

Type checking validates:
- Template actions reference fields that exist on Result type
- Method parameters match template call parameters
- Form struct fields match form field names
- Custom types implement required interfaces

---

## Package Structure

### Template Variable Discovery

Muxt requires a package-level variable of type `*html/template.Template`:

```go
//go:embed *.gohtml
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))
```

**Why package-level?** Muxt uses static analysis to find templates at generation time.

### Embed Patterns

**Single directory:**
```go
//go:embed *.gohtml
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))
```

**Multiple directories (explicit):**
```go
//go:embed pages/*.gohtml components/*.gohtml layouts/*.gohtml
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS,
    "pages/*.gohtml",
    "components/*.gohtml",
    "layouts/*.gohtml",
))
```

**Recursive (all subdirectories):**
```go
//go:embed **/*.gohtml
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "**/*.gohtml"))
```

**Note:** `go:embed` does NOT support double-star (`**`) glob patterns directly. You must enumerate each level:

```go
//go:embed *.gohtml */*.gohtml */*/*.gohtml
var templateFS embed.FS
```

### Template Configuration

**With custom functions:**
```go
//go:embed *.gohtml
var templateFS embed.FS

var templates = template.Must(
    template.New("").
        Funcs(template.FuncMap{
            "upper":      strings.ToUpper,
            "formatDate": formatDate,
        }).
        ParseFS(templateFS, "*.gohtml"),
)

func formatDate(t time.Time) string {
    return t.Format("January 2, 2006")
}
```

**With custom delimiters:**
```go
var templates = template.Must(
    template.New("").
        Delims("[[", "]]").
        ParseFS(templateFS, "*.gohtml"),
)
```

### Package Organization

**Recommended structure:**
```
myapp/
├── main.go
├── index.gohtml                    # Template file
├── index_template_routes_gen.go    # Generated routes for index.gohtml
├── users.gohtml                    # Template file
├── users_template_routes_gen.go    # Generated routes for users.gohtml
├── template_routes.go              # Main generated file with shared types
├── handlers.go                     # Receiver methods
└── handlers_test.go                # Tests
```

**Or with subdirectory:**
```
myapp/
├── main.go
└── internal/
    └── hypertext/
        ├── index.gohtml
        ├── index_template_routes_gen.go
        ├── admin.gohtml
        ├── admin_template_routes_gen.go
        ├── template_routes.go
        ├── handlers.go
        └── handlers_test.go
```

---

## CLI Reference

### Commands

**`muxt generate`** (aliases: `gen`, `g`)
Generate HTTP handlers from templates

**`muxt check`** (aliases: `c`, `typelate`)
Type-check templates without generating code

**`muxt documentation`** (aliases: `docs`, `d`)
Generate markdown documentation from templates

**`muxt version`** (alias: `v`)
Print muxt version

**`muxt help`**
Display help information

### Global Flags

**`-C <directory>`**
Change to directory before running command

```bash
muxt -C ./web generate --receiver-type=Server
```

### Generate Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output-file` | string | `template_routes.go` | Generated file name |
| `--templates-variable` | string | `templates` | Templates variable name |
| `--routes-func` | string | `TemplateRoutes` | Route registration function name |
| `--receiver-type` | string | _(none)_ | Type for method lookup |
| `--receiver-type-package` | string | _(current)_ | Package path for receiver type |
| `--receiver-interface` | string | `RoutesReceiver` | Interface name in generated file |
| `--template-data-type` | string | `TemplateData` | Template data type name |
| `--template-route-paths-type` | string | `TemplateRoutePaths` | Path helper type name |
| `--path-prefix` | bool | false | Add path prefix parameter |
| `--logger` | bool | false | Add slog.Logger parameter |

### Examples

**Basic:**
```bash
muxt generate --receiver-type=Server
```

**Custom names:**
```bash
muxt generate \
  --receiver-type=App \
  --routes-func=RegisterRoutes \
  --receiver-interface=Handler \
  --output-file=routes.go
```

**With logger:**
```bash
muxt generate --receiver-type=Server --logger
```

Generated signature:
```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, logger *slog.Logger) TemplateRoutePaths
```

**With path prefix:**
```bash
muxt generate --receiver-type=Server --path-prefix
```

Generated signature:
```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, pathPrefix string) TemplateRoutePaths
```

**Type checking:**
```bash
muxt check --receiver-type=Server
```

**From go:generate:**
```go
//go:generate muxt generate --receiver-type=Server --logger
```

---

## Testing

### Testing with Counterfeiter

Generate fakes for interfaces:

```bash
go install github.com/maxbrunsfeld/counterfeiter/v6@latest
```

**Generate fake:**
```go
//go:generate counterfeiter -generate
//counterfeiter:generate -o internal/fake/server.go --fake-name Server . RoutesReceiver
```

### Table-Driven Tests

**Pattern: Given-When-Then**

```go
func TestUserHandlers(t *testing.T) {
    type (
        Given struct {
            server *fake.Server
        }
        When struct {
            path TemplateRoutePaths
        }
        Then struct {
            server   *fake.Server
            response *http.Response
        }
        Case struct {
            Name  string
            Given func(*testing.T, Given)
            When  func(*testing.T, When) *http.Request
            Then  func(*testing.T, Then)
        }
    )

    run := func(t *testing.T, tc Case) {
        server := new(fake.Server)

        if tc.Given != nil {
            tc.Given(t, Given{server: server})
        }

        mux := http.NewServeMux()
        paths := TemplateRoutes(mux, server)

        req := tc.When(t, When{path: paths})
        rec := httptest.NewRecorder()
        mux.ServeHTTP(rec, req)

        tc.Then(t, Then{
            server:   server,
            response: rec.Result(),
        })
    }

    for _, tc := range []Case{
        {
            Name: "get user by id",
            Given: func(t *testing.T, g Given) {
                g.server.GetUserReturns(User{
                    Name:  "Alice",
                    Email: "alice@example.com",
                }, nil)
            },
            When: func(t *testing.T, w When) *http.Request {
                return httptest.NewRequest(http.MethodGet, w.path.GetUser(42), nil)
            },
            Then: func(t *testing.T, th Then) {
                require.Equal(t, 1, th.server.GetUserCallCount())
                require.Equal(t, 42, th.server.GetUserArgsForCall(0))
                require.Equal(t, http.StatusOK, th.response.StatusCode)
            },
        },
        {
            Name: "get user returns error",
            Given: func(t *testing.T, g Given) {
                g.server.GetUserReturns(User{}, errors.New("not found"))
            },
            When: func(t *testing.T, w When) *http.Request {
                return httptest.NewRequest(http.MethodGet, w.path.GetUser(42), nil)
            },
            Then: func(t *testing.T, th Then) {
                require.Equal(t, http.StatusOK, th.response.StatusCode)
                // Error is rendered in template, not returned as status
            },
        },
    } {
        t.Run(tc.Name, func(t *testing.T) {
            run(t, tc)
        })
    }
}
```

### DOM Testing with domtest

```bash
go get github.com/typelate/dom/domtest
```

**Test HTML structure:**
```go
import (
    "github.com/typelate/dom/domtest"
    "golang.org/x/net/html/atom"
)

func TestUserProfile(t *testing.T) {
    // ... setup ...

    mux.ServeHTTP(rec, req)

    doc := domtest.ParseResponseDocument(t, rec.Result())

    // Find elements
    if h1 := doc.QuerySelector("h1"); assert.NotNil(t, h1) {
        assert.Equal(t, "Alice", h1.TextContent())
    }

    // Check attributes
    if email := doc.QuerySelector(".email"); assert.NotNil(t, email) {
        assert.Equal(t, "alice@example.com", email.TextContent())
    }

    // Count elements
    items := doc.QuerySelectorAll(".user-item")
    assert.Equal(t, 5, len(items))
}
```

**Test fragments (HTMX partials):**
```go
func TestHTMXPartial(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/search?q=test", nil)
    req.Header.Set("HX-Request", "true")

    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    // Parse as fragment, not full document
    fragment := domtest.ParseResponseDocumentFragment(t, rec.Result(), atom.Body)

    el := fragment.FirstElementChild()
    assert.Equal(t, "div", el.TagName())
    assert.Equal(t, "results", el.GetAttribute("id"))
}
```

### Testing Path Helpers

Generated path helpers make URLs type-safe:

```go
func TestPathHelpers(t *testing.T) {
    mux := http.NewServeMux()
    paths := TemplateRoutes(mux, server)

    // Type-safe path construction
    userPath := paths.GetUser(42)
    assert.Equal(t, "/user/42", userPath)

    postPath := paths.GetPost(42, 100)
    assert.Equal(t, "/user/42/post/100", postPath)
}
```

---

## HTMX Integration

### Detecting HTMX Requests

```gotemplate
{{define "GET /search Search(ctx, query)"}}
{{if .Request.Header.Get "HX-Request"}}
  {{/* Return fragment for HTMX */}}
  <div id="results">
    {{range .Result}}
      <div class="result">{{.Title}}</div>
    {{end}}
  </div>
{{else}}
  {{/* Return full page */}}
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

### HTMX Response Headers

Set headers from template:

```gotemplate
{{define "POST /user CreateUser(ctx, form)"}}
{{with .Header "HX-Redirect" "/"}}
  {{/* HTMX will redirect */}}
{{end}}
<div>User created</div>
{{end}}
```

Or from Go method:

```go
func (s Server) CreateUser(ctx context.Context, response http.ResponseWriter, form UserForm) (User, error) {
    user, err := s.db.CreateUser(ctx, form)
    if err != nil {
        return User{}, err
    }

    // Redirect after successful creation
    response.Header().Set("HX-Redirect", "/users")
    return user, nil
}
```

### HTMX Patterns

**Infinite scroll:**
```gotemplate
{{define "GET /posts Posts(ctx, page)"}}
<div id="posts">
  {{range .Result}}
    <article>{{.Title}}</article>
  {{end}}

  {{if .Result}}
    <div hx-get="/posts?page={{add .Request.URL.Query.Get "page" 1}}"
         hx-trigger="revealed"
         hx-swap="outerHTML">
      Loading...
    </div>
  {{end}}
</div>
{{end}}
```

**Form validation:**
```gotemplate
{{define "POST /validate ValidateEmail(email)"}}
{{if .Err}}
  <span class="error">{{.Err.Error}}</span>
{{else}}
  <span class="success">✓ Valid</span>
{{end}}
{{end}}
```

```html
<input type="email"
       name="email"
       hx-post="/validate"
       hx-trigger="keyup changed delay:500ms"
       hx-target="next .validation">
<div class="validation"></div>
```

---

## Advanced Patterns

### Embedded Types

Methods from embedded fields are automatically discovered:

```go
type Database interface {
    GetUser(ctx context.Context, id int) (User, error)
}

type Auth struct {
    db Database
}

func (a Auth) Login(ctx context.Context, username, password string) (Session, error) {
    // ...
}

type Server struct {
    Auth      // Embedded - Login promoted to Server
    Analytics Analytics
}

// Template can call Login on Server
// {{define "POST /login Login(ctx, username, password)"}}
```

### Pointer vs Value Receivers

Both work identically:

```go
// Value receiver
func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    return s.db.GetUser(ctx, id)
}

// Pointer receiver
func (s *Server) GetUser(ctx context.Context, id int) (User, error) {
    return s.db.GetUser(ctx, id)
}
```

Use pointer receivers when you need to modify the receiver or avoid copying large structs.

### Multiple Packages

Receiver type can be in a different package:

```bash
muxt generate \
  --receiver-type=Server \
  --receiver-type-package=github.com/myapp/internal/server
```

### Input Validation Attributes

Muxt can read HTML5 validation attributes:

```gotemplate
{{define "POST /user CreateUser(ctx, form)"}}
<form method="post">
  <input type="text" name="Username"
         minlength="3"
         maxlength="20"
         pattern="[a-zA-Z0-9]+">

  <input type="email" name="Email">

  <input type="number" name="Age"
         min="18"
         max="120">

  <input type="url" name="Website"
         pattern="https?://.+">

  <button type="submit">Create</button>
</form>
{{end}}
```

Generated code validates based on these attributes: `minlength`, `maxlength`, `min`, `max`, and `pattern`.

### Custom Template Functions

Add helper functions to templates:

```go
var templates = template.Must(
    template.New("").
        Funcs(template.FuncMap{
            "formatDate": func(t time.Time) string {
                return t.Format("Jan 2, 2006")
            },
            "add": func(a, b int) int {
                return a + b
            },
            "contains": strings.Contains,
            "join": strings.Join,
        }).
        ParseFS(templateFS, "*.gohtml"),
)
```

Use in templates:

```gotemplate
{{define "GET /post/{id} GetPost(ctx, id)"}}
<article>
    <h1>{{.Result.Title}}</h1>
    <time>{{formatDate .Result.Published}}</time>
    <p>Next page: {{add .Result.Page 1}}</p>
    <p>Tags: {{join .Result.Tags ", "}}</p>
</article>
{{end}}
```

### Path Prefix Feature

Enable path prefix support:

```bash
muxt generate --receiver-type=Server --path-prefix
```

Generated signature:
```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, pathPrefix string) TemplateRoutePaths
```

Use:
```go
func main() {
    mux := http.NewServeMux()

    // All routes prefixed with /api/v1
    paths := TemplateRoutes(mux, server, "/api/v1")

    // /api/v1/users
    fmt.Println(paths.ListUsers())
}
```

### Logger Feature

Add structured logging:

```bash
muxt generate --receiver-type=Server --logger
```

Generated signature:
```go
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver, logger *slog.Logger) TemplateRoutePaths
```

Logs at:
- **Debug level**: Each request with pattern, path, and method
- **Error level**: Template execution failures

```go
import "log/slog"

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
    TemplateRoutes(mux, server, logger)
}
```

---

## Real-World Examples

### Example 1: Simple GET Route

**Template:**
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
```

**Go:**
```go
func (s Server) Home() string {
    return "Hello, world!"
}
```

### Example 2: Path Parameter with Type Parsing

**Template:**
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

**Go:**
```go
type User struct {
    Name  string
    Email string
}

func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    user, err := s.db.GetUser(ctx, id)
    if err != nil {
        return User{}, fmt.Errorf("user not found: %w", err)
    }
    return user, nil
}
```

### Example 3: Form Handling with Struct

**Template:**
```gotemplate
{{define "POST /login 200 Login(ctx, form)"}}
{{if .Err}}
  <div class="alert alert-error">
    {{.Err.Error}}
  </div>
  <a href="/login">Try again</a>
{{else}}
  <div class="alert alert-success">
    Welcome, {{.Result.Username}}!
  </div>
  <a href="/dashboard">Go to dashboard</a>
{{end}}
{{end}}
```

**Go:**
```go
type LoginForm struct {
    Username string `name:"username"`
    Password string `name:"password"`
}

type Session struct {
    Username string
    Token    string
}

func (s Server) Login(ctx context.Context, form LoginForm) (Session, error) {
    session, err := s.auth.Login(ctx, form.Username, form.Password)
    if err != nil {
        return Session{}, fmt.Errorf("invalid credentials")
    }
    return session, nil
}
```

### Example 4: HTMX Partial Rendering

**Template:**
```gotemplate
{{define "article-content"}}
  <h1>{{.Result.Title}}</h1>
  <p>{{.Result.Content}}</p>
{{end}}

{{define "GET /article/{id} Article(ctx, id)"}}
{{if .Request.Header.Get "HX-Request"}}
  {{/* HTMX request - return fragment */}}
  {{template "article-content" .}}
{{else}}
  {{/* Full page */}}
  <!DOCTYPE html>
  <html lang="en">
  <head>
      <meta charset="UTF-8">
      <title>{{.Result.Title}}</title>
      <script src="https://unpkg.com/htmx.org"></script>
  </head>
  <body>
      {{template "article-content" .}}
  </body>
  </html>
{{end}}
{{end}}
```

**Go:**
```go
type Article struct {
    Title   string
    Content string
}

func (s Server) Article(ctx context.Context, id int) (Article, error) {
    return s.db.GetArticle(ctx, id)
}
```

### Example 5: Error Handling with Custom Error Types

**Template:**
```gotemplate
{{define "error-page"}}
<!DOCTYPE html>
<html>
<head><title>Error</title></head>
<body>
  {{if eq (printf "%T" .Result.Error) "*main.NotFoundError"}}
    <h1>404 - Not Found</h1>
    <p>{{.Result.Error.Error}}</p>
  {{else if eq (printf "%T" .Result.Error) "*main.UnauthorizedError"}}
    <h1>401 - Unauthorized</h1>
    <p>{{.Result.Error.Error}}</p>
  {{else}}
    <h1>500 - Server Error</h1>
    <p>{{.Result.Error.Error}}</p>
  {{end}}
</body>
</html>
{{end}}

{{define "GET /article/{id} GetArticle(ctx, id)"}}
{{if .Result.Error}}
  {{template "error-page" .}}
{{else}}
  <h1>{{.Result.Title}}</h1>
{{end}}
{{end}}
```

**Go:**
```go
type NotFoundError struct {
    Message string
}

func (e NotFoundError) Error() string     { return e.Message }
func (e NotFoundError) StatusCode() int   { return http.StatusNotFound }

type Article struct {
    Title   string
    Content string
    Error   error
}

func (s Server) GetArticle(ctx context.Context, id int) Article {
    article, err := s.db.GetArticle(ctx, id)
    if err != nil {
        return Article{
            Error: NotFoundError{Message: "article not found"},
        }
    }
    return Article{
        Title:   article.Title,
        Content: article.Content,
    }
}
```

### Example 6: Multiple Form Fields (Slices)

**Template:**
```gotemplate
{{define "POST /filter FilterPosts(ctx, form)"}}
<div>
  <p>Filtered by tags: {{join .Result.FilteredTags ", "}}</p>
  <ul>
  {{range .Result.Posts}}
    <li>{{.Title}}</li>
  {{end}}
  </ul>
</div>
{{end}}
```

**Go:**
```go
type FilterForm struct {
    Tags []string
}

type FilterResult struct {
    FilteredTags []string
    Posts        []Post
}

func (s Server) FilterPosts(ctx context.Context, form FilterForm) (FilterResult, error) {
    posts, err := s.db.GetPostsByTags(ctx, form.Tags)
    return FilterResult{
        FilteredTags: form.Tags,
        Posts:        posts,
    }, err
}
```

**HTML:**
```html
<form method="post" action="/filter">
  <label><input type="checkbox" name="Tags" value="go"> Go</label>
  <label><input type="checkbox" name="Tags" value="web"> Web</label>
  <label><input type="checkbox" name="Tags" value="htmx"> HTMX</label>
  <button type="submit">Filter</button>
</form>
```

### Example 7: Status Code from Method

**Template:**
```gotemplate
{{define "GET /download/{id} Download(response, id)"}}
{{/* Response written by method */}}
{{end}}
```

**Go:**
```go
func (s Server) Download(response http.ResponseWriter, id string) error {
    file, err := s.storage.GetFile(id)
    if err != nil {
        return err
    }
    defer file.Close()

    response.Header().Set("Content-Disposition",
        fmt.Sprintf("attachment; filename=%q", file.Name))
    response.Header().Set("Content-Type", file.MimeType)
    response.WriteHeader(http.StatusOK)

    _, err = io.Copy(response, file)
    return err
}
```

---

## Troubleshooting

### Generation Issues

**"No templates found"**
- Ensure `templates` variable is at package level (not in a function)
- Verify `//go:embed` directive matches your template files
- Check that template files exist and have correct extension

**"Method not found on receiver"**
- Verify `--receiver-type=YourType` matches your actual type name
- Check method is exported (starts with capital letter)
- Ensure method signature matches template call exactly
- If using pointer receiver, generate with `--receiver-type=*YourType`

**"Template name does not match pattern"**
- Check template name follows syntax: `[METHOD ][HOST]/PATH[ STATUS][ CALL]`
- Ensure path starts with `/`
- Verify method name and parameters match exactly

### Type Checking Issues

**"Type mismatch in template action"**
- Run `muxt check --receiver-type=YourType` for specific errors
- Verify template actions reference fields that exist on Result type
- Check field names are capitalized (exported)

**"Parameter type mismatch"**
- Ensure template call parameters match Go method parameters
- Path parameter names must match `{param}` in template name
- Form parameter names must match struct field names (or `name` tag)

### Runtime Issues

**"400 Bad Request"**
- Path parameter failed to parse (e.g., "abc" to int)
- Form field has invalid type
- Check method signature accepts correct parameter types

**"Template execution error"**
- Template references field that doesn't exist on Result
- Template action has type error
- Check `.Err` is handled correctly in template

**"Panic: nil pointer"**
- Check `.Result` before accessing fields when `.Err` might be non-nil
- Use `{{if .Err}}` to handle error cases

### Common Mistakes

**Using `any` return type:**
```go
// Bad - type checker can't help
func (s Server) GetUser(ctx context.Context, id int) (any, error)

// Good - concrete type
func (s Server) GetUser(ctx context.Context, id int) (User, error)
```

**Not checking errors in template:**
```gotemplate
{{/* Bad - will panic if error */}}
{{define "GET /user/{id} GetUser(ctx, id)"}}
<h1>{{.Result.Name}}</h1>
{{end}}

{{/* Good - handles error */}}
{{define "GET /user/{id} GetUser(ctx, id)"}}
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  <h1>{{.Result.Name}}</h1>
{{end}}
{{end}}
```

**Template variable in wrong scope:**
```go
// Bad - function-local, Muxt can't find
func loadTemplates() *template.Template {
    return template.Must(template.ParseFS(...))
}

// Good - package-level
var templates = template.Must(template.ParseFS(...))
```

---

## Generated File Reference

### File Structure

Muxt generates multiple files to organize route handlers by source file:

#### Per-File Routes (`*_template_routes_gen.go`)

For each `.gohtml` file, a corresponding route file is generated. For example, `index.gohtml` generates `index_template_routes_gen.go`:

```go
// index_template_routes_gen.go
package yourpackage
import (...)

// File-scoped receiver interface
type IndexRoutesReceiver interface {
    Method1(...) ...
    Method2(...) ...
}

// File-scoped routes function
func IndexTemplateRoutes(mux *http.ServeMux, receiver IndexRoutesReceiver, pathsPrefix string) {
    // HTTP handler registrations for templates in index.gohtml
    mux.HandleFunc("GET /", func(...) { ... })
    mux.HandleFunc("POST /login", func(...) { ... })
}
```

**Note on Multiple Extensions:** Template files with multiple extensions (e.g., `index.html.gohtml`) will have all extensions stripped to form the base name.
This may lead to non-standard Go identifiers that require adjustment by `strcase.ToGoPascal()`. For example:
- `index.html.gohtml` → `IndexHtml` (not `Index`)
- `admin.tmpl.gohtml` → `AdminTmpl`
- `user.partial.gohtml` → `UserPartial`

#### Main Routes File (`template_routes.go`)

The main file contains shared types and orchestrates all per-file route functions:

```go
// template_routes.go
package yourpackage
import (...)

// 1. Unified RoutesReceiver interface (embeds per-file interfaces)
type RoutesReceiver interface {
    IndexRoutesReceiver
    UsersRoutesReceiver
    AdminRoutesReceiver
}

// 2. Main TemplateRoutes function
func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver) TemplateRoutePaths {
    pathsPrefix := ""

    // Call per-file route functions
    IndexTemplateRoutes(mux, receiver, pathsPrefix)
    UsersTemplateRoutes(mux, receiver, pathsPrefix)
    AdminTemplateRoutes(mux, receiver, pathsPrefix)

    // Handle templates defined via Parse() (not from files)
    mux.HandleFunc("GET /dynamic", func(...) { ... })

    return TemplateRoutePaths{pathsPrefix: pathsPrefix}
}

// 3. TemplateData type (shared by all routes)
type TemplateData[T any] struct {
    receiver      RoutesReceiver
    response      http.ResponseWriter
    request       *http.Request
    result        T
    // ... other fields
}

// 4. TemplateData methods
func (data *TemplateData[T]) MuxtVersion() string { ... }
func (data *TemplateData[T]) Path() TemplateRoutePaths { ... }
func (data *TemplateData[T]) Result() T { ... }
func (data *TemplateData[T]) Request() *http.Request { ... }
func (data *TemplateData[T]) StatusCode(int) *TemplateData[T] { ... }
func (data *TemplateData[T]) Header(key, value string) *TemplateData[T] { ... }
func (data *TemplateData[T]) Ok() bool { ... }
func (data *TemplateData[T]) Err() error { ... }
func (data *TemplateData[T]) Receiver() RoutesReceiver { ... }
func (data *TemplateData[T]) Redirect(url string, code int) (*TemplateData[T], error) { ... }

// 5. TemplateRoutePaths type
type TemplateRoutePaths struct {
    pathsPrefix string
}

// 6. TemplateRoutePaths methods (ALL path helpers)
func (routePaths TemplateRoutePaths) GetUser(id int) string { ... }
func (routePaths TemplateRoutePaths) CreateUser() string { ... }
func (routePaths TemplateRoutePaths) ListPosts() string { ... }
// ... one method per route from ALL files
```

### Finding Specific Parts

**Location strategies (no line numbers - they vary):**

| What to Find | Search Pattern | Notes |
|--------------|----------------|-------|
| TemplateRoutePaths type | `type TemplateRoutePaths struct` | Near end of file |
| Path helper methods | `func (routePaths TemplateRoutePaths)` | After TemplateRoutePaths type |
| TemplateData type | `type TemplateData\[T any\] struct` | After TemplateRoutes function |
| TemplateData methods | `func (data \*TemplateData\[T\])` | After TemplateData type |
| RoutesReceiver interface | `type RoutesReceiver interface` | Near top, after imports |
| TemplateRoutes function | `func TemplateRoutes\(` | After RoutesReceiver |
| Specific handler | `mux.HandleFunc\("GET /user"` | Inside TemplateRoutes |

**Using grep across generated files:**
```bash
# Find all generated route files
ls *_template_routes*.go

# Find TemplateRoutePaths type (in main file)
grep -n "type TemplateRoutePaths struct" template_routes.go

# List all path methods (in main file)
grep -n "^func (routePaths TemplateRoutePaths)" template_routes.go

# Find receiver interfaces across all files
grep "type.*RoutesReceiver interface" *_template_routes*.go

# Find a specific route handler (search all route files)
grep -r 'mux.HandleFunc("GET /user/{id}"' *_template_routes*.go

# List all per-file route functions
grep "func.*TemplateRoutes(" *_template_routes_gen.go
```

**Using MCP tools (for LLM agents):**
```
# Search for any symbol
go_search "TemplateRoutePaths"

# Find references to a method
go_symbol_references file:"template_routes.go" symbol:"GetUser"

# Get file context (may be large)
go_file_context file:"template_routes.go"
```

**Why line numbers are unreliable:**
- Number of routes affects file length
- Handler complexity varies (simple vs. form parsing)
- Configuration options (logger, path prefix) add code
- Each template adds ~15-50 lines to TemplateRoutes function

### What's in Each Section

**1. RoutesReceiver interface** - Shows required receiver methods
- Use this to verify your receiver type implements all methods
- Method signatures must match template calls exactly
- Example: `GetUser(ctx context.Context, id int) (User, error)`

**2. TemplateRoutes() function** - Route registration
- Contains all HTTP handler implementations
- Shows parameter parsing and validation logic
- Useful for debugging handler behavior
- Can be very long (bulk of file)

**3. TemplateData type** - Template data container
- Generic type: `TemplateData[T any]`
- Holds result, request, response, errors
- All fields are private (accessed via methods)

**4. TemplateData methods** - Template data accessors
- `.Result()` - Get method return value
- `.Err()` - Get error (if any)
- `.Request()` - Get HTTP request
- `.Path()` - Get path helper for URLs
- `.StatusCode(int)` - Set status code
- `.Header(k, v)` - Set response header
- `.Ok()` - Check if template should execute
- `.Receiver()` - Get receiver instance
- `.Redirect(url, code)` - Set redirect

**5. TemplateRoutePaths type** - Path helper container
- Simple struct with `pathsPrefix string` field
- Located near end of file
- Returned by TemplateRoutes()

**6. TemplateRoutePaths methods** - URL generators
- One method per route template
- All return `string` (the URL path)
- Take path parameters as arguments
- Example: `GetUser(id int) string` returns `"/user/42"`
- Located at very end of file
- Use these for type-safe URL generation in code

### Navigation Tips for LLM Agents

**When working with generated files:**

1. **Don't rely on line numbers** - File length varies with template count
2. **Use search patterns** - `grep` or `go_search` are more reliable
3. **Know the order** - Types appear in predictable sequence
4. **Path methods are last** - Always at end of file
5. **Use go_symbol_references** - Find where methods are called
6. **Method signatures matter** - Path methods always return `string`

**Common patterns:**
- Path method: `func (routePaths TemplateRoutePaths) MethodName(params...) string`
- TemplateData method: `func (data *TemplateData[T]) MethodName(...) ...`
- Handler registration: `mux.HandleFunc("METHOD /path", func(response, request) { ... })`

**Finding what you need:**
- **Need path URLs?** → Search for `TemplateRoutePaths)` at end of file
- **Need to understand handler?** → Search for `mux.HandleFunc` with route
- **Need receiver methods?** → Check `RoutesReceiver interface` at top
- **Need template data access?** → All TemplateData methods between TemplateRoutes and TemplateRoutePaths

## Summary

Muxt generates type-safe HTTP handlers from Go HTML templates:

1. **Templates define routes** - Template names specify HTTP method, path, status, and method call
2. **Methods return data** - Receiver methods implement business logic and return typed data
3. **Generated code connects them** - `muxt generate` creates the HTTP handler glue
4. **Standard library only** - Uses `net/http`, `html/template`, no external dependencies

**Workflow:**
```bash
# 1. Write templates
# {{define "GET /user/{id} GetUser(ctx, id)"}}...{{end}}

# 2. Implement methods
# func (s Server) GetUser(ctx context.Context, id int) (User, error)

# 3. Generate handlers
muxt generate --receiver-type=Server

# 4. Wire up and run
# TemplateRoutes(mux, server)
```

**Key Features:**
- Type-safe path and form parameter parsing
- Automatic error handling in templates
- HTMX-friendly partial rendering
- Custom status codes and headers
- Built-in input validation
- DOM-aware testing with domtest
- Zero runtime dependencies

**Philosophy:**
- Templates are contracts
- Methods implement behavior
- Generation ensures type safety
- Simplicity over cleverness
- HTML is a fine interface

Start simple. Add complexity only when needed. Ship fast. Iterate.
