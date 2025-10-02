# How to Test Muxt Handlers

Test your HTTP handlers by checking both the HTTP response and the actual HTML output.

## Goal

Write tests that verify:
- HTTP behavior (status codes, headers)
- HTML structure (the DOM)
- Your business logic actually works

## Prerequisites

Install testing dependencies:

```bash
go get github.com/crhntr/dom/domtest
go get github.com/stretchr/testify/assert
go get github.com/stretchr/testify/require
```

Install counterfeiter for generating test doubles:

```bash
go install github.com/maxbrunsfeld/counterfeiter/v6@latest
```

## The Testing Pattern

The recommended pattern uses custom types within your test function to organize setup, execution, and assertions.

### Basic Structure

```go
package blog_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crhntr/dom/domtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"example.com/blog"
	"example.com/blog/internal/fake"
)

func TestBlog(t *testing.T) {
	type (
		// Given holds test setup data
		Given struct {
			app *fake.App
		}

		// When holds data needed to construct requests
		When struct{}

		// Then holds data for assertions
		Then struct {
			app *fake.App
		}

		// Case defines a test case
		Case struct {
			Name     string
			Template string
			Given    func(*testing.T, Given)
			When     func(*testing.T, When) *http.Request
			Then     func(*testing.T, Then, *http.Response)
		}
	)

	runCase := func(t *testing.T, tc Case) {
		if tc.When == nil {
			t.Fatal("When must not be nil")
		}

		app := new(fake.App)
		mux := http.NewServeMux()
		blog.Routes(mux, app)

		if tc.Given != nil {
			tc.Given(t, Given{app: app})
		}

		req := tc.When(t, When{})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if tc.Then != nil {
			tc.Then(t, Then{app: app}, rec.Result())
		}
	}

	for _, tc := range []Case{
		{
			Name:     "viewing an article",
			Template: "GET /article/{id}",
			Given: func(t *testing.T, g Given) {
				g.app.GetArticleReturns(blog.Article{
					Title:   "Hello World",
					Content: "Welcome to my blog!",
				}, nil)
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
		t.Run(tc.Name, func(t *testing.T) {
			runCase(t, tc)
		})
	}
}
```

## Testing DOM Content

Use `domtest.ParseResponseDocument` to parse and query the HTML:

```go
{
	Name:     "article displays title and content",
	Template: "GET /article/{id}",
	Given: func(t *testing.T, g Given) {
		g.app.GetArticleReturns(blog.Article{
			Title:   "My Article",
			Content: "Article content here",
		}, nil)
	},
	When: func(t *testing.T, _ When) *http.Request {
		return httptest.NewRequest(http.MethodGet, "/article/1", nil)
	},
	Then: func(t *testing.T, then Then, res *http.Response) {
		// Verify method was called
		require.Equal(t, 1, then.app.GetArticleCallCount())

		// Parse and query the DOM
		doc := domtest.ParseResponseDocument(t, res)

		heading := doc.QuerySelector("h1")
		require.NotNil(t, heading)
		assert.Equal(t, "My Article", heading.TextContent())

		content := doc.QuerySelector("p")
		require.NotNil(t, content)
		assert.Equal(t, "Article content here", content.TextContent())
	},
},
```

## Testing Specific Elements

Query for specific elements by selector:

```go
{
	Name:     "error message displays",
	Template: "GET /article/{id}",
	Given: func(t *testing.T, g Given) {
		g.app.GetArticleReturns(blog.Article{}, errors.New("not found"))
	},
	When: func(t *testing.T, _ When) *http.Request {
		return httptest.NewRequest(http.MethodGet, "/article/999", nil)
	},
	Then: func(t *testing.T, _ Then, res *http.Response) {
		doc := domtest.ParseResponseDocument(t, res)
		errorDiv := doc.QuerySelector(".error")
		require.NotNil(t, errorDiv)
		assert.Contains(t, errorDiv.TextContent(), "not found")
	},
},
```

## Testing HTMX Partial Responses

For partial HTML responses, parse as a fragment:

```go
{
	Name:     "HTMX request returns fragment",
	Template: "GET /article/{id}",
	Given: func(t *testing.T, g Given) {
		g.app.GetArticleReturns(blog.Article{Title: "Partial"}, nil)
	},
	When: func(t *testing.T, _ When) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/article/1", nil)
		req.Header.Set("HX-Request", "true")
		return req
	},
	Then: func(t *testing.T, _ Then, res *http.Response) {
		assert.Equal(t, http.StatusOK, res.StatusCode)

		// Parse as fragment for partial responses
		frag := domtest.ParseResponseDocumentFragment(t, res, atom.Div)
		el := frag.FirstElementChild()
		require.NotNil(t, el)
	},
},
```

## Testing HTTP Responses

Check status codes and headers directly:

```go
{
	Name:     "invalid ID returns 400",
	Template: "GET /article/{id}",
	When: func(t *testing.T, _ When) *http.Request {
		return httptest.NewRequest(http.MethodGet, "/article/invalid", nil)
	},
	Then: func(t *testing.T, _ Then, res *http.Response) {
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
	},
},
```

## Generate Test Doubles with Counterfeiter

Add counterfeiter directives to generate fakes:

```go
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

package blog

//counterfeiter:generate . ArticleGetter
type ArticleGetter interface {
	GetArticle(ctx context.Context, id int) (Article, error)
}
```

Generate the fakes:

```bash
go generate ./...
```

This creates `fake/fake_article_getter.go` for use in tests.

## Organizing Test Data

Add fields to the `Given`, `When`, and `Then` structs as needed:

```go
type (
	Given struct {
		app       *fake.App
		articleID int
		userID    int
	}

	When struct {
		articleID int
	}

	Then struct {
		app    *fake.App
		userID int
	}
)
```

Use them in your `runCase` function:

```go
runCase := func(t *testing.T, tc Case) {
	app := new(fake.App)
	articleID := 42
	userID := 123

	if tc.Given != nil {
		tc.Given(t, Given{
			app:       app,
			articleID: articleID,
			userID:    userID,
		})
	}

	req := tc.When(t, When{articleID: articleID})
	// ... rest of runCase
}
```

## Complete Example

```go
package blog_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crhntr/dom/domtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"example.com/blog"
	"example.com/blog/internal/fake"
)

func TestBlog(t *testing.T) {
	type (
		Given struct {
			app *fake.ArticleGetter
		}
		When  struct{}
		Then  struct {
			app *fake.ArticleGetter
		}
		Case struct {
			Name     string
			Template string
			Given    func(*testing.T, Given)
			When     func(*testing.T, When) *http.Request
			Then     func(*testing.T, Then, *http.Response)
		}
	)

	runCase := func(t *testing.T, tc Case) {
		if tc.When == nil {
			t.Fatal("When must not be nil")
		}

		app := new(fake.ArticleGetter)
		mux := http.NewServeMux()
		blog.Routes(mux, app)

		if tc.Given != nil {
			tc.Given(t, Given{app: app})
		}

		req := tc.When(t, When{})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if tc.Then != nil {
			tc.Then(t, Then{app: app}, rec.Result())
			if testing.Verbose() && t.Failed() {
				t.Log(rec.Body.String())
			}
		}
	}

	for _, tc := range []Case{
		{
			Name:     "successful article view",
			Template: "GET /article/{id}",
			Given: func(t *testing.T, g Given) {
				g.app.GetArticleReturns(blog.Article{
					Title:   "Test Article",
					Content: "Test content",
				}, nil)
			},
			When: func(t *testing.T, _ When) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/article/1", nil)
			},
			Then: func(t *testing.T, then Then, res *http.Response) {
				assert.Equal(t, http.StatusOK, res.StatusCode)
				require.Equal(t, 1, then.app.GetArticleCallCount())

				doc := domtest.ParseResponseDocument(t, res)
				h1 := doc.QuerySelector("h1")
				require.NotNil(t, h1)
				assert.Equal(t, "Test Article", h1.TextContent())
			},
		},
		{
			Name:     "article not found",
			Template: "GET /article/{id}",
			Given: func(t *testing.T, g Given) {
				g.app.GetArticleReturns(blog.Article{}, errors.New("not found"))
			},
			When: func(t *testing.T, _ When) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/article/999", nil)
			},
			Then: func(t *testing.T, _ Then, res *http.Response) {
				doc := domtest.ParseResponseDocument(t, res)
				errorEl := doc.QuerySelector(".error")
				require.NotNil(t, errorEl)
				assert.Contains(t, errorEl.TextContent(), "not found")
			},
		},
		{
			Name:     "HTMX partial response",
			Template: "GET /article/{id}",
			Given: func(t *testing.T, g Given) {
				g.app.GetArticleReturns(blog.Article{Title: "Partial"}, nil)
			},
			When: func(t *testing.T, _ When) *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/article/1", nil)
				req.Header.Set("HX-Request", "true")
				return req
			},
			Then: func(t *testing.T, _ Then, res *http.Response) {
				frag := domtest.ParseResponseFragment(t, res)
				el := frag.FirstElementChild()
				require.NotNil(t, el)
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			runCase(t, tc)
		})
	}
}
```

## Testing Tips

**Name tests clearly** - Future you should understand what broke from the test name alone.

**Use the Template field** - It helps you track which routes are tested. Some generators use this field.

**Keep runCase simple** - Universal setup goes in `runCase`. Test-specific setup goes in `Given`.

**Verify fake calls** - Don't just check output. Verify your methods were called with the right arguments.

**Test error cases** - The happy path is easy. Error handling is where bugs hide.

**Log the body on failure** - The pattern shows this: `if testing.Verbose() && t.Failed() { t.Log(rec.Body.String()) }`. Helpful for debugging.

## Why This Pattern?

**Type safety** - The Given/When/Then structs make it clear what data is available where.

**Flexibility** - Add fields to these structs as your tests get more complex.

**Consistency** - All tests follow the same structure. Easy to read. Easy to write.

**Composability** - Share setup code in `runCase`. Keep test-specific logic in the case definitions. Move it before test cases to give readers a high level view of what is going on in the test (before the Case slices muddles things up).

## Next Steps

- Review [writing receiver methods](write-receiver-methods.md) for testable designs
- Explore the [domtest documentation](https://github.com/crhntr/dom)
- Learn about [Muxt's type checking](../reference/type-checking.md) for catching errors earlier
