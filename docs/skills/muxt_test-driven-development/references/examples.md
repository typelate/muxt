# muxt TDD: examples

## GET route — template-first

The template name is the contract. It declares: route pattern (`GET /article/{id}`), method to call (`GetArticle`), parameters (`ctx`, `id`).

```gotmpl
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

### Route pattern syntax

Follows `net/http.ServeMux` syntax: `[METHOD ][HOST]/[PATH]`. Muxt extends with optional status code and call expression.

Path wildcards:
- `{name}` matches a single path segment.
- `{name...}` matches the remainder (`/files/{path...}`).
- `{$}` matches end of URL only.
- Wildcards must be full segments — `/b_{bucket}` is not valid.

The wildcard name must be a valid Go identifier; muxt matches it to call parameters: `{id}` → `id`.

Precedence: more specific patterns win. `GET /article/{id}` and `GET /article/new` coexist (literal beats wildcard). Two patterns matching the same requests panic at registration.

A trailing slash acts as `{...}`: `/images/` matches anything under `/images/`. ServeMux redirects `/images` to `/images/` unless `/images` is registered separately.

See [`net/http.ServeMux`](https://pkg.go.dev/net/http#ServeMux).

### Test (Given-When-Then table)

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

### Receiver method

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) {
    return s.db.FindArticle(ctx, id)
}
```

## DELETE / PATCH / POST — method-first

```go
func (s Server) DeleteArticle(ctx context.Context, id int) error {
    return s.db.DeleteArticle(ctx, id)
}
```

```gotmpl
{{define "DELETE /article/{id} DeleteArticle(ctx, id)"}}
{{if .Err}}
  <div role="alert">{{.Err.Error}}</div>
{{else}}
  <p>Article deleted.</p>
{{end}}
{{end}}
```

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

For form-based mutations (POST with form data), see [`muxt_forms`](../../muxt_forms/SKILL.md).

## Type-checked URLs

Use `$.Path` in templates for `href` and `action` instead of hardcoded strings. `TemplateRoutes` returns a `TemplateRoutePaths` value; each route gets a corresponding method:

```gotmpl
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

Compile-time checking: rename a route and `go generate` produces updated methods, and the compiler catches any mismatch.

In tests:

```go
paths := TemplateRoutes(mux, receiver)
req := httptest.NewRequest(http.MethodGet, paths.GetArticle(1), nil)
```

## Control-flow templates

```gotmpl
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

Route templates focus on control flow; sub-templates focus on rendering.

## Template decomposition

```gotmpl
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

## Integration testing

Prefer real implementations over mocks when practical:

```go
func TestArticleIntegration(t *testing.T) {
    db := setupTestDB(t) // testcontainers, docker-compose, or sqlite
    server := Server{db: db}

    mux := http.NewServeMux()
    paths := TemplateRoutes(mux, server)

    req := httptest.NewRequest(http.MethodGet, paths.GetArticle(1), nil)
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    doc := domtest.ParseResponseDocument(t, rec.Result())
    assert.Equal(t, "Hello", doc.QuerySelector("h1").TextContent())
}
```

Counterfeiter fakes for fast unit tests; testcontainers/docker-compose for integration tests that catch real bugs.
