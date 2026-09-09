# HTML is the API

A Muxt handler's response body is its contract. The form `action`, the input
`name`, the `hx-target`, the `href` — those are what the next request depends
on, so those are what a test should assert on.

Checking them does not need a browser.
[domtest](https://github.com/typelate/dom/tree/main/domtest) parses an
`*http.Response` into a queryable DOM, so a test asks for
`form input[name="todo"]` instead of running `strings.Contains` over the body.
A substring check keeps passing after a field escapes its form or loses its
`name`; the selector stops matching the moment either happens.

## What Muxt contributes

Generated code makes the whole route stack drivable in process:

| Generated symbol | What it gives the test |
|---|---|
| `TemplateRoutes(mux, receiver)` | Registers the real handlers on a fresh `*http.ServeMux` per test |
| `TemplateRoutePaths{}.MethodName(args)` | Builds request URLs from the generated route patterns — a renamed method or changed parameter list fails the build instead of returning a 404 |
| `RoutesReceiver` | An interface, so a test can substitute a stand-in and pin exactly what a template renders |

The receiver has to live in an importable package for any of this to work.
[Structure a project for testing](../how-to/receiver-package-and-testing.md)
covers that layout and which layer is worth faking.

## What domtest contributes

Parsers that take a response and return something with `QuerySelector`:

```go
res := rec.Result()

doc := domtest.ParseResponseDocument(t, res)
require.NotNil(t, doc.QuerySelector(".todoapp"))
assert.Equal(t, "todos", doc.QuerySelector("h1").TextContent())
```

A miss returns nil rather than a zero value, which is why `require.NotNil`
guards anything the test then dereferences.

## Fragments need the element they swap into

A route whose template name is a fragment returns a partial, and HTML parsing
is context-sensitive, so the fragment parser takes the parent element the
response will land in:

```go
fragment := domtest.ParseResponseDocumentFragment(t, res, atom.Tbody)
```

A `<tr>` outside a table is discarded, so `atom.Body` yields zero elements
where `atom.Tbody` yields the rows. Pass the element the swap targets and the
test checks the swap contract for free — a fragment that vanishes here would
not have landed in the page either.

## Where it stops

domtest does not execute JavaScript. It sees the markup the handler wrote,
including `hx-swap-oob` attributes and Datastar `data-*` signals, but not the
DOM htmx or Datastar produces afterward. Assert on the response here; reach for
a real browser (chromedp) only when the behavior under test belongs to the
browser.

## Read next

- [domtest](https://github.com/typelate/dom/tree/main/domtest) — the six
  parsers, failure behavior, testify conventions, and reusable assertions
  written against `spec.ElementQueries`
- Working tests in this repo: [simple](../examples/simple),
  [htmx-todo](../examples/htmx-todo),
  [datastar-search](../examples/datastar-search)
