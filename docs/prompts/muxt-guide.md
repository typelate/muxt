# Muxt Practical Guide

Comprehensive guide for building type-safe HTTP handlers with Muxt.

## Core Concept

Templates define routes and methods. Go implements behavior. Muxt generates handlers using only standard library.

## Template Name Syntax

```
[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]
```

Components:
- **METHOD** (optional): GET, POST, PATCH, DELETE, PUT
- **HOST** (optional): example.com, api.example.com
- **PATH** (required): `/`, `/user/{id}`, `/api/v1/posts`
- **HTTP_STATUS** (optional): 200, 201, 404, http.StatusCreated
- **CALL** (optional): Home(), GetUser(ctx, id)

Examples:
```gotemplate
{{define "GET /"}}...{{end}}
{{define "GET /user/{id}"}}...{{end}}
{{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}
{{define "GET /user/{userID}/post/{postID} GetPost(ctx, userID, postID)"}}...{{end}}
{{define "PATCH /user/{id} UpdateUser(ctx, id, form)"}}...{{end}}
{{define "DELETE /user/{id} DeleteUser(ctx, id)"}}...{{end}}
```

Path patterns:
- `GET /{$}` - Exact match only `/`
- `GET /static/` - Prefix match `/static/*`
- `GET /files/{path...}` - Wildcard captures rest of path

See [reference_path_exact_match.txt](../../cmd/muxt/testdata/reference_path_exact_match.txt), [reference_path_prefix.txt](../../cmd/muxt/testdata/reference_path_prefix.txt)

## TemplateData Access

Templates receive `TemplateData[T]`. Access via methods (Go calls them automatically):

```gotemplate
{{.Result}}                                {{/* Method return value */}}
{{.Err}}                                   {{/* Method error */}}
{{.Request.URL.Path}}                      {{/* Request data */}}
{{.Request.PathValue "id"}}                {{/* Path parameter */}}
{{.Request.Header.Get "HX-Request"}}       {{/* Check HTMX */}}
```

Always check errors:
```gotemplate
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  <h1>{{.Result.Name}}</h1>
{{end}}
```

## HTTP Methods

All HTTP methods supported: GET, POST, PATCH, PUT, DELETE

Examples:
```gotemplate
{{define "GET /users ListUsers(ctx)"}}
  <ul>{{range .Result}}<li>{{.Name}}</li>{{end}}</ul>
{{end}}

{{define "POST /user 201 CreateUser(ctx, form)"}}
  <div>Created: {{.Result.Name}}</div>
{{end}}

{{define "PATCH /user/{id} UpdateUser(ctx, id, form)"}}
  <div>Updated</div>
{{end}}

{{define "DELETE /user/{id} 204 DeleteUser(ctx, id)"}}
  {{/* 204 No Content */}}
{{end}}
```

See [tutorial_basic_route.txt](../../cmd/muxt/testdata/tutorial_basic_route.txt), [howto_patch_method.txt](../../cmd/muxt/testdata/howto_patch_method.txt)

## Call Parameters

Special parameter names:
- `ctx` → `context.Context`
- `request` → `*http.Request`
- `response` → `http.ResponseWriter`
- `form` → struct from request.Form
- `{pathParam}` → parsed from URL

Path parameters:
```gotemplate
{{define "GET /user/{userID}/post/{postID} GetPost(ctx, userID, postID)"}}
```
```go
func (s Server) GetPost(ctx context.Context, userID, postID int) (Post, error) {
    return s.db.GetPost(ctx, userID, postID) // params auto-parsed to int
}
```

Form structs:
```go
type LoginForm struct {
    Username string `name:"user-name"`  // Field tag maps to form field name
    Password string
}

func (s Server) Login(ctx context.Context, form LoginForm) (Session, error) {
    return s.auth.Login(ctx, form.Username, form.Password)
}
```

See [howto_arg_context.txt](../../cmd/muxt/testdata/howto_arg_context.txt), [howto_form_basic.txt](../../cmd/muxt/testdata/howto_form_basic.txt), [howto_arg_path_param.txt](../../cmd/muxt/testdata/howto_arg_path_param.txt)

## Type Parsing

Muxt parses: `string`, `int*`, `uint*`, `bool`, `encoding.TextUnmarshaler`

Parse error → 400 Bad Request

Custom types via `UnmarshalText`:
```go
type UserID string

func (id *UserID) UnmarshalText(text []byte) error {
    *id = UserID(strings.ToLower(string(text)))
    return nil
}
```

## Return Types

Supported signatures:
```go
func () T                  // .Result=T, .Err=nil
func () (T, error)         // .Result=T, .Err=error
func () (T, bool)          // .Result=T, bool=true skips template
func () error              // .Result=zero, .Err=error
```

Always check `.Err` in templates.

See [howto_call_method.txt](../../cmd/muxt/testdata/howto_call_method.txt), [explanation_call_two_returns.txt](../../cmd/muxt/testdata/explanation_call_two_returns.txt)

## Status Codes

Four methods:

**1. Template name:**
```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}
```

**2. StatusCode() method:**
```go
func (r Result) StatusCode() int { return r.code }
```

**3. StatusCode field:**
```go
type Result struct { StatusCode int }
```

**4. Error with StatusCode():**
```go
func (e NotFoundError) StatusCode() int { return 404 }
```

See [reference_status_codes.txt](../../cmd/muxt/testdata/reference_status_codes.txt)

## Form Validation

Muxt reads HTML validation attributes: `minlength`, `maxlength`, `min`, `max`, `pattern`

See [reference_validation_min_max.txt](../../cmd/muxt/testdata/reference_validation_min_max.txt)

## CLI Commands

```bash
muxt generate --find-receiver-type=Server           # Generate handlers
muxt check --find-receiver-type=Server              # Type check only
muxt version                                   # Show version
muxt help                                      # Show help
muxt -C ./web generate --find-receiver-type=Server  # Generate from dir
```

Key flags: `--find-receiver-type`, `--output-file`, `--output-routes-func`, `--output-receiver-interface`

## Setup Workflow

1. Create `.gohtml` files with templates
2. Add `//go:embed` and `//go:generate muxt generate --find-receiver-type=Server`
3. Define receiver type with methods
4. Run `go generate` then `go run .`

See muxt-quick.md for complete example.

## Advanced Patterns

- Embedded methods work via field promotion
- Both value and pointer receivers supported
- Form slices: `type Form struct { Tags []string }` parses multiple values

See [explanation_receiver_embedded_method.txt](../../cmd/muxt/testdata/explanation_receiver_embedded_method.txt), [explanation_receiver_pointer.txt](../../cmd/muxt/testdata/explanation_receiver_pointer.txt)

## Best Practices

1. **One route per template** - Keep templates focused
2. **Return static types** - Avoid `any`/`interface{}`  for type safety
3. **Use semantic HTML** - `<article>`, `<main>`, `<time datetime=...>`
4. **Check `.Err` in templates** - Always handle errors explicitly
5. **HTMX pattern** - Check `HX-Request` header for partials vs full pages

See [use-htmx.md](../how-to/use-htmx.md) for HTMX patterns.

## Troubleshooting

- **"No templates found"** - Check `//go:embed` directive, verify templates variable is package-level
- **"Method not found"** - Use `--find-receiver-type`, verify method is exported and signature matches
- **"Type mismatch"** - Run `muxt check` for details
- **"400 Bad Request"** - Path param parse failure (e.g., "abc" → int)

## Generated Code Structure

**`template_routes.go`:**
- `RoutesReceiver` interface
- `TemplateRoutes()` function
- `TemplateData[T]` type and methods
- `TemplateRoutePaths` type (path helpers at end of file)

**`*_template_routes_gen.go`** (one per .gohtml file):
- File-specific interface (e.g., `IndexRoutesReceiver`)
- File-specific route function
- HTTP handlers for that file's templates

**Finding generated code:**
```bash
grep "type TemplateRoutePaths" template_routes.go
grep "type .*RoutesReceiver interface" *_template_routes_gen.go
```

**MCP tools:**
```
go_search "TemplateRoutePaths"
go_file_context file:"template_routes.go"
```

## Summary

Muxt turns templates into HTTP handlers:
1. Write templates with route names: `{{define "GET /path Method()"}}`
2. Implement receiver methods with matching signatures
3. Run `muxt generate` to create handlers
4. Wire up with `TemplateRoutes(mux, receiver)`

Templates define the interface. Methods implement behavior. Muxt generates the glue. Everything else is standard Go.
