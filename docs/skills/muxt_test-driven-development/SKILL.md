---
name: muxt-test-driven-development
description: "Muxt: Use when creating new Muxt templates, adding routes, writing receiver methods, or implementing features in a Muxt codebase using TDD. Covers template-first design for GET routes, method-first design for POST/PATCH/DELETE, and red-green-refactor with domtest."
---

# Template-Driven Development

Create new templates and receiver methods using TDD. The approach differs by HTTP method:

- **GET routes**: template-first (design the view, then implement the method)
- **POST/PATCH/DELETE routes**: receiver-method-first (design the behavior, then wire the template)

## Test Dependencies

```bash
go get github.com/typelate/dom/domtest
go get github.com/stretchr/testify/{assert,require}
go install github.com/maxbrunsfeld/counterfeiter/v6@latest
```

Generate test doubles from the `RoutesReceiver` interface:

```go
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . RoutesReceiver
```

```bash
go generate ./...
```

## GET Routes: Template First

### 1. Write the Template

Start with the template name (route pattern + call) and body:

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
{{with $err := .Err}}
  <div class="error">{{$err.Error}}</div>
{{else}}
  <article>
    <h1>{{.Result.Title}}</h1>
    <p>{{.Result.Body}}</p>
  </article>
{{end}}
{{end}}
```

The template name is the contract. It declares:
- The route pattern (`GET /article/{id}`)
- The method to call (`GetArticle`)
- The parameters (`ctx` = context, `id` = path parameter)

### Route Pattern Syntax

The route pattern part follows `net/http.ServeMux` syntax: `[METHOD ][HOST]/[PATH]`. Muxt extends this with an optional status code and call expression.

Path wildcards:
- `{name}` matches a single path segment (`/article/{id}` matches `/article/42`)
- `{name...}` matches the remainder of the path (`/files/{path...}` matches `/files/a/b/c`)
- `{$}` matches only the end of the URL (`/{$}` matches `/` but not `/anything`)
- Wildcards must be full path segments — `/b_{bucket}` is not valid

The wildcard name must be a valid Go identifier. Muxt uses it to match call parameters: `{id}` in the pattern maps to `id` in the call expression.

Precedence: more specific patterns take priority. `GET /article/{id}` and `GET /article/new` can coexist — the literal `/article/new` is more specific. Two patterns that match the same requests conflict and cause a panic at registration.

A trailing slash acts as an anonymous `{...}` wildcard: `/images/` matches any path under `/images/`. ServeMux redirects `/images` to `/images/` unless `/images` is registered separately.

See [`net/http.ServeMux`](https://pkg.go.dev/net/http#ServeMux) for the full specification.

### 2. Write the Test

Use the Given-When-Then table-driven pattern:

```go
func TestArticle(t *testing.T) {
    type (
        Given struct{ app *fake.App }
        When  struct{}
        Then  struct{ app *fake.App }
        Case  struct {
            Name  string
            Given func(*testing.T, Given)
            When  func(*testing.T, When) *http.Request
            Then  func(*testing.T, Then, *http.Response)
        }
    )

    runCase := func(t *testing.T, tc Case) {
        app := new(fake.App)
        mux := http.NewServeMux()
        TemplateRoutes(mux, app)

        if tc.Given != nil { tc.Given(t, Given{app: app}) }
        req := tc.When(t, When{})
        rec := httptest.NewRecorder()
        mux.ServeHTTP(rec, req)
        if tc.Then != nil { tc.Then(t, Then{app: app}, rec.Result()) }
    }

    for _, tc := range []Case{
        {
            Name: "viewing an article",
            Given: func(t *testing.T, g Given) {
                g.app.GetArticleReturns(Article{Title: "Hello", Body: "World"}, nil)
            },
            When: func(t *testing.T, _ When) *http.Request {
                return httptest.NewRequest(http.MethodGet, "/article/1", nil)
            },
            Then: func(t *testing.T, then Then, res *http.Response) {
                assert.Equal(t, http.StatusOK, res.StatusCode)
                doc := domtest.ParseResponseDocument(t, res)
                h1 := doc.QuerySelector("h1")
                require.NotNil(t, h1)
                assert.Equal(t, "Hello", h1.TextContent())
            },
        },
    } {
        t.Run(tc.Name, func(t *testing.T) { runCase(t, tc) })
    }
}
```

### 3. Implement the Receiver Method

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) {
    return s.db.FindArticle(ctx, id)
}
```

### 4. Run `go generate` and `go test`

```bash
go generate ./...
go test ./...
```

## POST/PATCH/DELETE Routes: Method First

For mutation routes, design the behavior first:

### 1. Write the Receiver Method

```go
func (s Server) DeleteArticle(ctx context.Context, id int) error {
    return s.db.DeleteArticle(ctx, id)
}
```

### 2. Write the Template

```gotemplate
{{define "DELETE /article/{id} DeleteArticle(ctx, id)"}}
{{if .Err}}
  <div role="alert">{{.Err.Error}}</div>
{{else}}
  <p>Article deleted.</p>
{{end}}
{{end}}
```

### 3. Write the Test and Iterate

Same Given-When-Then pattern, but focus on verifying the method was called correctly:

```go
{
    Name: "deleting an article",
    When: func(t *testing.T, _ When) *http.Request {
        return httptest.NewRequest(http.MethodDelete, "/article/1", nil)
    },
    Then: func(t *testing.T, then Then, res *http.Response) {
        assert.Equal(t, http.StatusOK, res.StatusCode)
        require.Equal(t, 1, then.app.DeleteArticleCallCount())
    },
},
```

For form-based mutations (POST with form data), see [Forms](forms.md).

## Choosing Call Parameters

See [Call Parameters Reference](../reference/call-parameters.md) for the full table.

| Parameter | Type | Use When |
|-----------|------|----------|
| `ctx` | `context.Context` | Always (recommended first param) |
| `form` | struct | POST/PUT/PATCH with form data (see [Forms](forms.md)) |
| `{param}` | string/int/custom | Path parameter extraction |
| `request` | `*http.Request` | Need headers, cookies, full request |
| `response` | `http.ResponseWriter` | Streaming, file downloads, custom headers |

## Choosing Return Types

See [Call Results Reference](../reference/call-results.md) for the full table.

| Pattern | Use When |
|---------|----------|
| `(T, error)` | Most endpoints (can fail) |
| `T` | Infallible operations (static pages) |
| `error` | No data needed (health checks, deletes) |
| `(T, bool)` | Early exit/redirect (bool=false skips template) |

## Setting Status Codes

Four ways to set HTTP status codes, in order of precedence:

**1. Template name** (static, most common):
```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}
```

**2. StatusCode() method on return type** (dynamic):
```go
type UserResult struct {
	User User
	code int
}
func (r UserResult) StatusCode() int { return r.code }
```

**3. StatusCode field on return type:**
```go
type UserResult struct {
	StatusCode int
    User User
}
```

**4. In template** (for error cases):
```gotemplate
{{with and (.StatusCode 404) .Err}}<div>Not found</div>{{end}}
```

Use template name for static codes (201 for POST). Use methods/fields for dynamic codes (404 from errors).

## When to Skip Muxt

For file downloads, streaming, or WebSockets, write custom handlers alongside Muxt routes:

```go
mux := http.NewServeMux()
TemplateRoutes(mux, srv)
mux.HandleFunc("GET /download/{id}", handleDownload) // Custom handler
```

Methods that need `response http.ResponseWriter` or `request *http.Request` can still use Muxt — add them as call parameters. But if the method is fundamentally about streaming bytes, a custom handler is simpler.

## Control-Flow Templates

Use a high-level template for error checking, delegating the happy path to a sub-template:

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  {{template "article-page" .Result}}
{{end}}
{{end}}

{{define "article-page"}}
<article>
  <h1>{{.Title}}</h1>
  <p>{{.Body}}</p>
</article>
{{end}}
```

This keeps route templates focused on control flow and sub-templates focused on rendering.

## Template Decomposition

Break large templates into focused sub-templates:

```gotemplate
{{define "GET /dashboard Dashboard(ctx)"}}
{{if .Err}}
  {{template "error" .Err}}
{{else}}
  <main>
    {{template "dashboard-stats" .Result.Stats}}
    {{template "dashboard-recent" .Result.Recent}}
  </main>
{{end}}
{{end}}
```

Each sub-template can be tested independently.

## Type-Checked URLs

Use `$.Path` in templates for `href` and `action` attributes instead of hardcoded strings. `TemplateRoutes` returns a `TemplateRoutePaths` value, and each route template gets access to it via `.Path`:

```gotemplate
{{define "GET /{$} Home(ctx)"}}
<nav>
  <a href="{{$.Path.ListArticles}}">Articles</a>
  <a href="{{$.Path.GetArticle 42}}">Article 42</a>
</nav>
{{end}}

{{define "POST /article 201 CreateArticle(ctx, form)"}}
<form method="post" action="{{$.Path.CreateArticle}}">
  <input name="title">
  <button type="submit">Create</button>
</form>
{{end}}
```

This gives you compile-time checking: if a route is renamed or its path parameters change, `go generate` produces updated methods and the compiler catches any mismatches.

In tests, use the same `TemplateRoutePaths` methods to build request URLs:

```go
paths := TemplateRoutes(mux, receiver)
req := httptest.NewRequest(http.MethodGet, paths.GetArticle(1), nil)
```

## Integration Testing

Prefer real implementations over mocks when practical:

```go
func TestArticleIntegration(t *testing.T) {
    db := setupTestDB(t) // testcontainers, docker-compose, or sqlite
    server := Server{db: db}

    mux := http.NewServeMux()
    paths := TemplateRoutes(mux, server)

    // Test the full stack
    req := httptest.NewRequest(http.MethodGet, paths.GetArticle(1), nil)
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    doc := domtest.ParseResponseDocument(t, rec.Result())
    assert.Equal(t, "Hello", doc.QuerySelector("h1").TextContent())
}
```

Use counterfeiter fakes for fast unit tests. Use real implementations with testcontainers or docker-compose for integration tests that catch real bugs.

## Red-Green-Refactor Checklist

1. **Red**: Write a failing test (template + test case, no method body yet)
2. **Green**: Implement the receiver method, run `go generate && go test`
3. **Refactor**: Extract sub-templates, simplify method logic

A failing test may be an expected `go test` or `muxt check` compilation failure.

## Reference

- [Call Parameters](../reference/call-parameters.md)
- [Call Results](../reference/call-results.md)
- [Template Name Syntax](../reference/template-names.md)
- [domtest](https://github.com/typelate/dom) — HTML assertion library
- [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter) — Test double generation

### Test Cases (`cmd/muxt/testdata/`)

These txtar test files demonstrate the behaviors documented in this skill:

| Feature | Test File |
|---------|-----------|
| Basic route with handler | `tutorial_basic_handler.txt` |
| Blog example with domtest | `tutorial_blog_example.txt` |
| Status codes in template names | `reference_status_codes.txt` |
| Path parameter types (int, bool, etc.) | `reference_path_with_typed_param.txt` |
| Custom TextUnmarshaler path param | `howto_arg_with_text_unmarshaler.txt` |
| Context parameter | `howto_arg_context.txt` |
| Request parameter | `howto_arg_request.txt` |
| Response parameter | `howto_arg_response.txt` |
| Error return handling | `reference_call_with_error_return.txt` |
| Bool return (early exit) | `reference_call_with_bool_return.txt` |
| Redirect helpers | `reference_redirect_helpers.txt` |
| Type-checked URLs ($.Path) | `howto_arg_with_text_unmarshaler.txt` |
| Multiple template files | `reference_multiple_template_files.txt` |
| Sub-template decomposition | `reference_multiple_hypermedia_children.txt` |

For form-related test cases, see [Forms](forms.md#test-cases-cmdmuxttestdata).
