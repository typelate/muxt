# Testing `muxt generate`d Hypertext Handlers with `domtest`

When building server-side applications with **`muxt`**-generated routes,
you often want to verify both the **HTTP response** (e.g., status codes, headers)
and the **HTML/DOM output** (e.g., specific elements, text content, or errors).
The [`domtest`](https://github.com/typelate/dom) package offers a way to test hypertext in a way intuitive for web developers. 

Below is an example test suite from the `blog_test` package, which illustrates how to integrate `domtest` with a `muxt` route function named `Routes`.
A typical BDD test pattern emerges. Each test case specifies `Given` (setup, optional), `When` (the request, required), and `Then` (assertions, optional).
By leveraging `domtest`’s various assertion helpers, you can check DOM structure and content directly.

## Example Usage

<details>
<summary>Full Example</summary>

This is an excerpt from a test.
To see the complete code run.

```shell
# I haven't actually run this script. It should get the gist across though.

go install golang.org/x/exp/cmd/txtar@latest
go install github.com/maxbrunsfeld/counterfeiter/v6@latest
git clone git@github.com:typelate/muxt.git
cd muxt
export TEST_TAR="${PWD}/cmd/muxt/testdata/blog.txt"
mkdir -p /tmp/example.com
cd /tmp/example.com/

txtar --extract <"${TEST_TAR}"

go mod tidy
muxt generate 
go test -v
```

</details>

```go
package blog_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html/atom"

	"github.com/typelate/dom/domtest"
	"github.com/typelate/dom/spec"

	"example.com/blog"
	"example.com/blog/internal/fake"
)

func TestBlog(t *testing.T) {
	for _, tt := range []domtest.Case[*testing.T, fake.App]{
		{
			Name: "viewing the home page",
			Given: func(t *testing.T, app *fake.App) {
				app.ArticleReturns(blog.Article{
					Title:   "Greetings!",
					Content: "Hello, friends!",
					Error:   nil,
				})
			},
			When: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/article/1", nil)
			},
			Then: domtest.Document(func(t *testing.T, document spec.Document, app *fake.App) {
				require.Equal(t, 1, app.ArticleArgsForCall(0))
				if heading := document.QuerySelector("h1"); assert.NotNil(t, heading) {
					require.Equal(t, "Greetings!", heading.TextContent())
				}
				if content := document.QuerySelector("p"); assert.NotNil(t, content) {
					require.Equal(t, "Hello, friends!", content.TextContent())
				}
			}),
		},
		{
			Name: "the page has an error",
			Given: func(t *testing.T, app *fake.App) {
				app.ArticleReturns(blog.Article{
					Error: fmt.Errorf("lemon"),
				})
			},
			When: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/article/1", nil)
			},
			Then: domtest.QuerySelector("#error-message", func(t *testing.T, msg spec.Element, app *fake.App) {
				require.Equal(t, "lemon", msg.TextContent())
			}),
		},
		{
			Name: "the page has an error and is requested by HTMX",
			Given: func(t *testing.T, app *fake.App) {
				app.ArticleReturns(blog.Article{
					Error: fmt.Errorf("lemon"),
				})
			},
			When: func(t *testing.T) *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/article/1", nil)
				req.Header.Set("HX-Request", "true")
				return req
			},
			Then: domtest.Fragment(atom.Body, func(t *testing.T, fragment spec.DocumentFragment, app *fake.App) {
				el := fragment.FirstElementChild()
				require.Equal(t, "lemon", el.TextContent())
				require.Equal(t, "*errors.errorString", el.GetAttribute("data-type"))
			}),
		},
		{
			Name: "when the id is not an integer",
			When: func(t *testing.T) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/article/banana", nil)
			},
			Then: func(t *testing.T, res *http.Response, f *fake.App) {
				require.Equal(t, http.StatusBadRequest, res.StatusCode)
			},
		},
	} {
		t.Run(tt.Name, tt.Run(func(app *fake.App) http.Handler {
			mux := http.NewServeMux()
			blog.Routes(mux, app)
			return mux
		}))
	}
}
```

### Key Components in the Example

- **`domtest.Case[*testing.T, fake.App]`**  
  Each case is parameterized by the test context (`*testing.T`) and your fake receiver type (`fake.App`).

- **`Given func(t *testing.T, app *fake.App)`**  
  Set up initial conditions—e.g., “ArticleReturns” to specify what happens when the route calls `app.Article(id)`.
-
- **`When func(t *testing.T) *http.Request`**  
  Creates the incoming request for this scenario (method, path, optional headers/body).

- **`Then ...`**  
  A function to **assert** the result, either at the HTTP level or the DOM level. This can be:
    - `domtest.Document(...)` for a full HTML doc,
    - `domtest.QuerySelector(...)` for a specific element,
    - `domtest.Fragment(...)` for partial responses, or
    - a custom function that checks `response.StatusCode` directly.

Inside each `Then` block, you can use `require` or `assert` from [stretchr/testify](https://github.com/stretchr/testify)
to fail the test if the expected DOM elements or status codes aren’t present.

The final closure passed into Run is for you to call the muxt generated `routes` function.
It receives the fake receiver as a parameter.

### Example: Checking DOM Content

```go
Then: domtest.Document(func(t *testing.T, document spec.Document, app *fake.App) {
    require.Equal(t, 1, app.ArticleArgsForCall(0)) // Did we call Article(1)?
    heading := document.QuerySelector("h1")
    if assert.NotNil(t, heading) {
        require.Equal(t, "Greetings!", heading.TextContent())
    }
    content := document.QuerySelector("p")
    if assert.NotNil(t, content) {
        require.Equal(t, "Hello, friends!", content.TextContent())
    }
}),
```

- The test ensures `app.Article(1)` was called.
- Looks up `<h1>` and `<p>` tags and verifies text content matches the expectation.

#### Example: Checking Error Scenarios

```go
{
  Name: "the page has an error",
  Given: func(t *testing.T, app *fake.App) {
    app.ArticleReturns(blog.Article{
      Error: fmt.Errorf("lemon"),
    })
  },
  When: func(t *testing.T) *http.Request {
    return httptest.NewRequest(http.MethodGet, "/article/1", nil)
  },
  Then: domtest.QuerySelector("#error-message", func(t *testing.T, msg spec.Element, app *fake.App) {
    require.Equal(t, "lemon", msg.TextContent())
  }),
},
```

- Sets up the domain method to return an error.
- Uses `domtest.QuerySelector("#error-message", ...)` to confirm the `<div id="error-message">` or similar element
  contains the string `"lemon"`.

### Example: Partial Responses [HTMX](http://htmx.org/)

Although the Document parser will allow incomplete documents, you may want to test document fragments.

```go
{
  Name: "the page has an error and is requested by HTMX",
  Given: func(t *testing.T, app *fake.App) {
    app.ArticleReturns(blog.Article{Error: fmt.Errorf("lemon")})
  },
  When: func(t *testing.T) *http.Request {
    req := httptest.NewRequest(http.MethodGet, "/article/1", nil)
    req.Header.Set("HX-Request", "true")
    return req
  },
  Then: domtest.Fragment(atom.Body, func(t *testing.T, fragment spec.DocumentFragment, app *fake.App) {
    el := fragment.FirstElementChild()
    require.Equal(t, "lemon", el.TextContent())
    require.Equal(t, "*errors.errorString", el.GetAttribute("data-type"))
  }),
},
```

- Simulates an **HTMX** request by adding `HX-Request: true`.
- Uses `domtest.Fragment(atom.Body, ...)` to parse only a `<body>` snippet or partial, checking content.

## Why vibe with `muxt` + `domtest`

1. **One-Stop Testing**
    - Tests your real `Routes(mux, fakes)` in a black-box manner: if Muxt or your domain logic break, the test fails.

2. **Domain + Presentation**
    - Check domain calls (e.g., `ArticleArgsForCall(0) == 1`) and the rendered DOM (the `<h1>` or error messages).

3. **Minimal Overhead**
    - `domtest` sets up a structure that’s easy to read, maintain, and expand. Adding new test cases or scenarios is
      straightforward.

4. **Supports TDD/BDD**
    - The table-driven approach with `Given`, `When`, and `Then` aligns naturally with Behavior-Driven Development or
      Extreme Programming’s quick feedback loop.

## Tips

1. **Name Scenarios Clearly**
   I did not do a great job of this in my example
   (e.g., “viewing the home page,” “the page has an error,” “when the id is not an integer.”)

2. **Use Mocking Tools**
    - For more complex domain interactions, consider a mocking library
      like [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter), generating stubs for your Muxt receiver
      methods.

3. **Keep Tests Atomic**
    - Each scenario should test one major idea: e.g., “user not logged in -> unauthorized,” or “invalid input -> show
      error.” Avoid piling too many steps into a single test.

4. **Combine with Muxt’s Type Checking**
    - If you’re using Muxt’s static type check feature, you’ll get extra assurance that your templates, route
      parameters, and domain method signatures align correctly before even hitting these tests.

_(this article was mostly generated using an LLM model)_