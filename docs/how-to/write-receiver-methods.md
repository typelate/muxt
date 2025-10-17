# How to Write Receiver Methods

Write clean, testable receiver methods: return domain types, not HTTP primitives.

## Prefer Domain Types

**Good:**
```go
func (s Server) CreateUser(ctx context.Context, form CreateUserForm) (User, error) {
	return s.db.InsertUser(ctx, form.Username, form.Email)
}
```

**Bad:**
```go
func (s Server) CreateUser(response http.ResponseWriter, request *http.Request) {
	// Tightly coupled to HTTP, harder to test
}
```

## Set Status Codes

**1. Template name:**
```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}
```

**2. StatusCode() method:**
```go
func (r UserResult) StatusCode() int { return r.code }
```

**3. StatusCode field:**
```go
type UserResult struct {
	StatusCode int
	Username   string
}
```

**4. In template:**
```gotemplate
{{with .StatusCode 404}}<div>Not found</div>{{end}}
```

## Handle Errors

Return errors from methods, check in templates:

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) {
	article, err := s.db.GetArticle(ctx, id)
	if err != nil {
		return Article{}, fmt.Errorf("article not found: %w", err)
	}
	return article, nil
}
```

```gotemplate
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  <h1>{{.Result.Title}}</h1>
{{end}}
``` 

## Parse Parameters

Muxt auto-parses path and form parameters from method signatures.

**Path parameters:**
```gotemplate
{{define "GET /user/{id}/post/{postID} GetPost(ctx, id, postID)"}}
```
```go
func (s Server) GetPost(ctx context.Context, id int, postID int) (Post, error) {
	return s.db.GetPost(ctx, id, postID)  // id, postID auto-parsed
}
```

**Form parameters:**
```gotemplate
{{define "POST /login Login(ctx, username, password)"}}
```
```go
func (s Server) Login(ctx context.Context, username, password string) (Session, error) {
	return s.auth.Login(ctx, username, password)  // from form fields
}
```

Supported: `string`, `int*`, `uint*`, `bool`, `encoding.TextUnmarshaler`

## When to Use http.ResponseWriter

For file downloads, streaming, WebSockets: skip Muxt, write custom handlers.

```go
func main() {
	mux := http.NewServeMux()
	TemplateRoutes(mux, srv)
	mux.HandleFunc("GET /download/{id}", handleDownload)  // Custom
	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

## Best Practices

- **Return concrete types** - Avoid `any`/`interface{}`, type checker needs real types
- **Keep HTTP in templates** - Methods return data, templates render it
- **Use context.Context** - First parameter by convention
- **Return errors** - Let templates decide how to display
- **Test without HTTP** - Methods are just methods, no `httptest` needed

## Next

- [Test handlers](test-handlers.md)
- [Call parameters reference](../reference/call-parameters.md)
- [Call results reference](../reference/call-results.md)
