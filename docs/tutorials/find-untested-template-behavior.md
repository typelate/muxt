# Find Untested Template Behavior

A passing test suite tells you the templates render. It does not tell you whether any test would notice if they rendered something *else*. `muxt test-template-mutations` answers that: it changes one action at a time, re-runs your tests against each change, and reports the changes nothing caught.

In this tutorial you run it against an example that already passes its tests, watch it report that nothing is covered, and write one test that turns two of those reports green.

## Prerequisites

- The muxt repository checked out, and Go installed.
- 5 minutes. Each variation is a full `go test` run.

## Step 1: Confirm the example passes

```bash
cd docs/examples/htmx-counter
go test ./...
```

```
ok  	github.com/typelate/muxt/docs/examples/htmx-counter
```

Green. The suite runs `muxt check`, so the templates are type-correct, and the HTMX helper tests pass.

## Step 2: Ask what the tests actually pin down

```bash
muxt test-template-mutations --seed 1 --state ""
```

```
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

**Zero killed.** Twenty ways to change what these templates render, and not one of them makes a test fail. The suite proves the templates compile; it asserts nothing about the HTML they produce.

`--seed 1` makes the run reproducible, so your output matches this page. Before mutating anything the command runs the tests once unmutated — that is the `baseline ok` line. A red baseline stops the run, because every mutant would otherwise look caught by the failure that was already there.

`--state ""` turns off the verdict cache, so every run here starts from nothing and the counts on this page are the ones you see. A real run records what it learned and re-tries only what changed. [Step 6](#step-6-work-through-the-rest) comes back to that.

## Step 3: Read one miss

Take this one:

```
MISS 17:2 template-drop
```

Line 17 of `template.gohtml` is the counter on the home page:

```gotemplate
{{template "count" .Result}}
```

`template-drop` renders that partial as nothing at all. The page comes back with no counter in it, every test still passes. So no test looks at the counter.

The operator names the assertion you are missing. `template-drop` says *assert on something that partial produces*.

## Step 4: Write the test it is asking for

Create `docs/examples/htmx-counter/counter_page_test.go`:

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
go test ./...
muxt test-template-mutations --seed 1 --state ""
```

```
20 mutants, 1 killed, 19 missed, 0 skipped
```

`MISS 17:2 template-drop` is gone. Dropping the partial now removes `#count` from the document, `QuerySelector` returns nil, and `require.NotNil` fails the test. The mutant is killed.

## Step 5: Notice the one it still misses

This one survived:

```
MISS 62:19 action-zero
```

Line 62 is inside the partial you just asserted on:

```gotemplate
{{define "count" -}}
		<div id='count'>{{.}}</div>
{{- end}}
```

`action-zero` replaces the value with the zero value of its own type — and the type is `int64`, so it renders `0`. Your fixture starts the counter at zero and asserts `"0"`. The mutation changes zero to zero. The test cannot tell the difference.

**A fixture at the zero value cannot detect a mutation to the zero value.** Give the counter a value:

```go
	mux := http.NewServeMux()
	srv := new(Server)
	srv.Increment()
	TemplateRoutes(mux, srv)
```

and assert on it:

```go
	assert.Equal(t, "1", count.TextContent())
```

```bash
go test ./...
muxt test-template-mutations --seed 1 --state ""
```

```
20 mutants, 2 killed, 18 missed, 0 skipped
```

Two killed. The test asks the same question of the same element; it just asks it about a value the mutation can destroy. This is the kind of gap that reading a test cannot show you and coverage percentages cannot either: the line was covered the whole time.

## Step 6: Work through the rest

Eighteen are left. Most are the other routes, whose HTMX and non-HTMX branches nothing exercises, but one is still on this page: line 14 drops the shared `imports` partial, and no test looks at the script tag it writes. Narrow the run to one template while you work on it:

```bash
muxt test-template-mutations --template-pattern '^POST /count$' --seed 1 --state "" -v
```

Each operator asks for a particular assertion:

| Operator | The change it makes | What to assert |
|----------|--------------------|----------------|
| `action-zero` | the value renders as the zero value of its own type | that value's text or attribute — from a fixture that is *not* the zero value |
| `action-empty` | the value renders as nothing, when its type could not be resolved | that value's text or attribute |
| `if-true`, `if-false` | the branch is taken unconditionally | one case per side of the condition |
| `range-never` | the loop body never runs, so the `{{else}}` runs instead | the number of rows, not just the first |
| `with-empty` | the `with` body is skipped, so the `{{else}}` runs instead | something only the body produces |
| `template-drop` | the partial renders nothing at all | something only that partial produces |
| `operands` | one named input is replaced | that input specifically |

Drop `--state ""` outside this tutorial. Verdicts are recorded in `testdata/template-mutations.json` and a re-run only re-tries what changed, which is what makes a large project's second run fast. Commit that file alongside the tests that produced it.

Writing an assertion retries what it could have changed. A run records which test caught each mutant, so adding a test retries the misses — the ones a new assertion could close — while every kill some untouched test is still witness to stands. That is why the run right after you write an assertion is the one that proves it.

The cache is for this loop, not for CI. It watches the templates, the types, each test's source and each package's `testdata`, but a test can read any file, and one that compares against a guide in `docs/` changes its assertion when that guide does. In CI, keep the state file as a build artifact and let each run start from nothing.

Not every miss is worth a test. A mutation to a decorative wrapper may be one you accept. The report tells you what is unasserted; you decide what deserves an assertion.

## What's next

- [HTML is the API](../explanation/html-is-the-api.md) — why the rendered markup is the contract worth asserting on, and what domtest gives you to assert with
- [Structure a project for testing](../how-to/receiver-package-and-testing.md) — receiver in a library package, faking the service layer, testing through the generated routes
- [`muxt test-template-mutations`](../reference/commands/test-template-mutations.md) — every flag, the operators in full, and how mutants reach the build
