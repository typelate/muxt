# Muxt Quick Reference

Generate type-safe HTTP handlers from Go HTML template names.

## Template Name Syntax

```
[METHOD ][HOST]/[PATH][ HTTP_STATUS][ CALL]
```

Examples:
```gotemplate
{{define "GET /"}}...{{end}}
{{define "GET /user/{id}"}}...{{end}}
{{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}
{{define "GET /article/{id} GetArticle(ctx, id)"}}...{{end}}
```

Components:
- **METHOD** (optional): GET, POST, PATCH, DELETE, PUT
- **PATH** (required): `/path`, `/path/{param}`
- **HTTP_STATUS** (optional): 200, 201, 404 or http.StatusOK
- **CALL** (optional): MethodName(param1, param2)

## Template Data Access

Templates receive `TemplateData[T]`. Access via methods (Go calls them automatically):

```gotemplate
{{.Result}}         {{/* Method return value */}}
{{.Result.Field}}   {{/* Access field on result */}}
{{.Err}}           {{/* Method error */}}
{{.Request}}       {{/* *http.Request */}}
```

Always check `.Err`:
```gotemplate
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  <h1>{{.Result.Name}}</h1>
{{end}}
```

## Special Parameters

Call parameters map to:
- `ctx` → `context.Context`
- `request` → `*http.Request`
- `response` → `http.ResponseWriter`
- `form` → struct parsed from request.Form
- `{pathParam}` → parsed from URL to method parameter type

## Return Types

```go
func (Reciever) Value() T                  // .Result=T, .Err=nil
func (Reciever) ValueErr() (T, error)         // .Result=T, .Err=error
func (Reciever) ValueOk() (T, bool)          // .Result=T, bool=true skips template
func (Reciever) JustErr() error              // .Result=zero, .Err=error
```

## Complete Example

**template.gohtml:**
```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
<!DOCTYPE html>
<html>
<body>
{{if .Err}}
  <div>Error: {{.Err.Error}}</div>
{{else}}
  <h1>{{.Result.Name}}</h1>
  <p>{{.Result.Email}}</p>
{{end}}
</body>
</html>
{{end}}
```

**main.go:**
```go
package main

import (
	"context"
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

type User struct {
	Name  string
	Email string
}

func (s Server) GetUser(ctx context.Context, id int) (User, error) {
	return s.db.GetUser(ctx, id) // id parsed from path string
}

func main() {
	mux := http.NewServeMux()
	srv := Server{db: NewDatabase()}
	TemplateRoutes(mux, srv)
	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

Run: `go generate && go run .`


## Form Handling

**template.gohtml:**
```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}
<div>Created: {{.Result.Username}}</div>
{{end}}
```

**main.go:**
```go
type UserForm struct {
	Username string
	Email    string
}

func (s Server) CreateUser(ctx context.Context, form UserForm) (User, error) {
	return s.db.CreateUser(ctx, form.Username, form.Email)
}
```

Form fields map to struct fields. Supports: `string`, `int*`, `uint*`, `bool`, `encoding.TextUnmarshaler`.

## Status Codes

**1. Template name:**
```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}
  <!-- Returns 201 -->
{{end}}
```

**2. StatusCode field:**
```go
type Result struct {
	Data       User
	StatusCode int
}
```

**3. StatusCode() method:**
```go
func (r Result) StatusCode() int { return r.code }
```

## Commands

```bash
muxt generate --receiver-type=Server    # Generate handlers
muxt check --receiver-type=Server       # Type check only
muxt version                            # Show version
```

## Generated Files

- `template_routes.go` - Main file with `TemplateRoutes()` function
- `*_template_routes_gen.go` - Per-source-file handlers

## More Examples

See test cases: [cmd/muxt/testdata/](../../cmd/muxt/testdata/)
- [simple_get.txt](../../cmd/muxt/testdata/simple_get.txt) - Basic GET
- [path_param.txt](../../cmd/muxt/testdata/path_param.txt) - Path parameters
- [form.txt](../../cmd/muxt/testdata/form.txt) - Form parsing
- [status_codes.txt](../../cmd/muxt/testdata/status_codes.txt) - Status codes
- [blog.txt](../../cmd/muxt/testdata/blog.txt) - Complete app

Browse all: `ls cmd/muxt/testdata/*.txt`
