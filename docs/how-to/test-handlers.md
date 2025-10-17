# How to Test Muxt Handlers

Test HTTP responses and HTML structure with table-driven tests.

## Install Dependencies

```bash
go get github.com/crhntr/dom/domtest
go get github.com/stretchr/testify/{assert,require}
go install github.com/maxbrunsfeld/counterfeiter/v6@latest
```

## Pattern: Given-When-Then

Use typed structs for table-driven tests:

```go
func TestBlog(t *testing.T) {
	type (
		Given struct { app *fake.App }
		When  struct {}
		Then  struct { app *fake.App }
		Case  struct {
			Name     string
			Given    func(*testing.T, Given)
			When     func(*testing.T, When) *http.Request
			Then     func(*testing.T, Then, *http.Response)
		}
	)

	runCase := func(t *testing.T, tc Case) {
		app := new(fake.App)
		mux := http.NewServeMux()
		blog.Routes(mux, app)

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
				g.app.GetArticleReturns(blog.Article{Title: "Hello World"}, nil)
			},
			When: func(t *testing.T, _ When) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/article/1", nil)
			},
			Then: func(t *testing.T, then Then, res *http.Response) {
				assert.Equal(t, http.StatusOK, res.StatusCode)
				doc := domtest.ParseResponseDocument(t, res)
				h1 := doc.QuerySelector("h1")
				require.NotNil(t, h1)
				assert.Equal(t, "Hello World", h1.TextContent())
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) { runCase(t, tc) })
	}
}
```

## Test DOM Content

Query HTML with CSS selectors:

```go
doc := domtest.ParseResponseDocument(t, res)
h1 := doc.QuerySelector("h1")
assert.Equal(t, "My Article", h1.TextContent())

errorDiv := doc.QuerySelector(".error")
assert.Contains(t, errorDiv.TextContent(), "not found")
```

## Test HTMX Partials

Parse fragments for partial responses:

```go
req.Header.Set("HX-Request", "true")
// ...
frag := domtest.ParseResponseDocumentFragment(t, res, atom.Div)
el := frag.FirstElementChild()
require.NotNil(t, el)
```

## Test HTTP Responses

```go
assert.Equal(t, http.StatusBadRequest, res.StatusCode)
assert.Equal(t, "application/json", res.Header.Get("Content-Type"))
```

## Generate Test Doubles

```go
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . ArticleGetter
```

```bash
go generate ./...  # Creates fake/fake_article_getter.go
```

## Tips

- **Name tests clearly** - Test name should explain what broke
- **Verify fake calls** - Check methods were called with right arguments
- **Test error cases** - Error handling is where bugs hide
- **Log on failure** - `if testing.Verbose() && t.Failed() { t.Log(rec.Body.String()) }`
- **Extend structs** - Add fields to Given/When/Then as tests grow

## Why This Pattern?

- **Type safe** - Structs clarify available data
- **Consistent** - All tests follow same structure
- **Composable** - Share setup in `runCase`, specifics in cases

## Next

- [Write receiver methods](write-receiver-methods.md)
- [domtest docs](https://github.com/crhntr/dom)
- [Type checking](../reference/type-checking.md)
