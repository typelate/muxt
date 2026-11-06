# Find Untested Template Behavior

`muxt test-template-mutations` ([reference](../reference/commands/test-template-mutations.md)) reports the template actions no test depends on. You run it on an example app, then write the test that closes one miss.

Prerequisite: a checkout of this repository. The commands run muxt from the checkout with `go run`, so nothing is installed.

## Step 1: Confirm the example passes

```bash
cd docs/examples/htmx-counter
go test ./...
```

## Step 2: Run the mutations

```bash
go run github.com/typelate/muxt test-template-mutations --seed 1
```

```text
20 mutants across 6 templates (complexity 10, seed 1)
baseline ok

template_routes.go:38:13 ExecuteTemplate "/ Count()" (dot: *main.TemplateData[main.RoutesReceiver, int64])
  "/ Count()" template.gohtml (complexity 1, dot: *main.TemplateData[main.RoutesReceiver, int64])
    MISS 14:2 template-drop
    MISS 17:2 template-drop
  "count" template.gohtml via {{template}} (complexity 1, dot: int64)
    MISS 62:19 action-zero
...
20 mutants, 0 killed, 20 missed, 0 skipped
```

Twenty changes to the rendered HTML pass every test. `--seed 1` makes the run reproducible. The command exits non-zero while any mutant is missed.

## Step 3: Read one miss

```text
MISS 17:2 template-drop
```

Line 17 of `template.gohtml` is `{{template "count" .Result}}`. `template-drop` renders that partial as nothing, and every test still passes, so no test asserts on anything only that partial produces.

## Step 4: Write the test

Create `counter_page_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/typelate/dom/domtest"
)

func TestCounterPage(t *testing.T) {
	mux := http.NewServeMux()
	TemplateRoutes(mux, new(Server))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	doc := domtest.ParseResponseDocument(t, rec.Result())
	count := doc.QuerySelector("#count")
	require.NotNil(t, count)
	assert.Equal(t, "0", count.TextContent())
}
```

```bash
go test ./... && go run github.com/typelate/muxt test-template-mutations --seed 1
```

```text
20 mutants, 1 killed, 19 missed, 0 skipped
```

Dropping the partial now removes `#count`, so `require.NotNil` fails.

## Step 5: Fix the fixture

This one missed:

```text
MISS 62:19 action-zero
```

Line 62, in the `count` partial, is `<div id='count'>{{.}}</div>`. `action-zero` renders `{{.}}` as `0`, the `int64` zero. The counter starts at zero and the test asserts `"0"`, so it cannot see the change. In `counter_page_test.go`, replace the `TemplateRoutes` line with:

```go
	srv := new(Server)
	srv.Increment()
	TemplateRoutes(mux, srv)
```

and change the assertion to:

```go
	assert.Equal(t, "1", count.TextContent())
```

```bash
go test ./... && go run github.com/typelate/muxt test-template-mutations --seed 1
```

```text
20 mutants, 2 killed, 18 missed, 0 skipped
```

## Step 6: Work through the rest

Narrow the run to one template and read the remaining misses:

```bash
go run github.com/typelate/muxt test-template-mutations --template-pattern '^POST /count$' --seed 1 -v
```

Each operator names the assertion that kills its mutant:

| Operator | What to assert |
|----------|----------------|
| `action-zero` | that value's text or attribute, from a fixture that is not the zero value |
| `action-empty` | that value's text or attribute |
| `if-true`, `if-false` | one case per side of the condition |
| `range-never` | the number of rows, not only the first |
| `with-empty` | something only the body produces |
| `template-drop` | something only that partial produces |
| `operands` | that input specifically |

Write an assertion for each miss whose behavior matters to you; not every miss needs a test. For CI, the [reference](../reference/commands/test-template-mutations.md) covers `--diff` and `--workers`, and [Timing](../reference/commands/test-template-mutations.md#timing) covers the progress lines of a long run.

## Next

- [HTML is the API](../explanation/html-is-the-api.md): why the rendered markup is the contract to assert on.
- [Structure a project for testing](../how-to/receiver-package-and-testing.md): put the receiver where a test can fake it.
