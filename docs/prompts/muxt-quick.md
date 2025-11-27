# Muxt Quick Reference

Generate type-safe HTTP handlers from Go HTML template names.

## Syntax

```
[METHOD ][HOST]/PATH[ HTTP_STATUS][ CALL]
```

```gotemplate
{{define "GET /"}}...{{end}}
{{define "GET /user/{id} GetUser(ctx, id)"}}...{{end}}
{{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}
{{define "DELETE /user/{id} 204 DeleteUser(ctx, id)"}}...{{end}}
```

## Call Parameters

| Name | Type | Source |
|------|------|--------|
| `ctx` | `context.Context` | `request.Context()` |
| `request` | `*http.Request` | Request object |
| `response` | `http.ResponseWriter` | Response writer |
| `form` | struct | `request.Form` parsed to struct |
| `{param}` | string/int/etc | `request.PathValue()` |

## Template Data

```gotemplate
{{.Result}}              {{/* Method return value */}}
{{.Err}}                 {{/* Error if any */}}
{{.Request}}             {{/* *http.Request */}}
{{.Path.GetUser 42}}     {{/* Type-safe URL: /user/42 */}}
```

## Return Types

```go
func (s Server) M() T              // .Result=T
func (s Server) M() (T, error)     // .Result=T, .Err=error
func (s Server) M() error          // .Err=error
```

## Example

**template.gohtml:**
```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
{{if .Err}}<div>Error: {{.Err.Error}}</div>
{{else}}<h1>{{.Result.Name}}</h1>{{end}}
{{end}}
```

**main.go:**
```go
//go:embed *.gohtml
var templateFS embed.FS

//go:generate muxt generate --find-receiver-type=Server
var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))

type Server struct{
	db Database
}

func (s Server) GetUser(ctx context.Context, id int) (User, error) {
    return s.db.GetUser(ctx, id)
}

func main() {
    mux := http.NewServeMux()
    TemplateRoutes(mux, Server{db: NewDB()})
    http.ListenAndServe(":8080", mux)
}
```

## Commands

```bash
muxt generate --find-receiver-type=Server  # Generate handlers
muxt check --find-receiver-type=Server     # Type check only
```

See [muxt-guide.md](muxt-guide.md) for complete documentation.