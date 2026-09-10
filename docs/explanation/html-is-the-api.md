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

## htmx and Datastar widen the API

htmx and Datastar write the interaction graph into attributes: which element
issues a request, to what URL, and where the response lands. Attributes travel
in the response body, and attribute selectors reach hyphenated and
colon-separated names, so the graph is queryable — `[hx-post]`, `[hx-target]`,
`[data-on\:click]`.

That turns questions that sound like browser questions into graph questions.
Does this button's click have somewhere to go? Does what it targets exist? Both
are answerable from a single response.

The generated mux answers the routing half. `http.ServeMux.Handler` reports the
pattern that would serve a request, and an empty pattern means nothing would:

```go
for el := range doc.QuerySelectorSequence("[hx-post]") {
	url := el.GetAttribute("hx-post")
	_, pattern := mux.Handler(httptest.NewRequest(http.MethodPost, url, nil))
	assert.NotEmptyf(t, pattern, "hx-post=%q is not routable", url)
}
```

A sweep like that fails the moment a template and a route pattern drift apart —
the drift that otherwise surfaces as a 404 after a click.

### Resolve targets from the trigger, not the declaration

`hx-target` is inherited, so the element declaring it is often not the element
issuing the request: a `<tbody hx-target="closest tr">` sets the target for the
`<td hx-get=...>` inside it. Start at the trigger and walk up for the nearest
declaration with `trigger.Closest("[hx-target]")`.

The value is not plain CSS either. `closest`, `find`, `this`, `next`, and
`previous` are htmx syntax, and `dom` compiles selectors with
`cascadia.MustCompile` — so handing `closest tr` to `QuerySelector` panics.
Split the verb off first; `spec.Element` already has what each one needs.

| `hx-target` value | resolve with |
|---|---|
| `#id`, `.class`, any CSS | `doc.QuerySelector(target)` |
| `closest tr` | `trigger.Closest("tr")` |
| `find td` | `trigger.QuerySelector("td")` |
| `this` | the trigger itself |

The same shape covers `hx-swap-oob`: a fragment's out-of-band element carries an
`id`, and that `id` has to exist in the page being patched.

### Datastar hides the URL in an expression

`data-on:click="@post('/increment')"` holds the action inside a JavaScript
expression, so match the attribute and extract the URL. `data-on:*` is a
JavaScript context to `html/template`, which renders `/` as `\/` — unescape
before routing. [datastar-counter](../examples/datastar-counter) keeps a
`jsPath` helper for exactly this.

## Where it stops

A sweep proves the graph is wired: the trigger exists, its URL is served, its
target is present. It does not prove htmx or Datastar behaves — nothing binds a
listener and no swap happens, because domtest does not execute JavaScript. It
sees the markup the handler wrote, not the DOM the library produces from it.
Assert the contract here; reach for a real browser (chromedp) only when the
behavior under test genuinely belongs to the browser.

## Read next

- [Find Untested Template Behavior](../tutorials/find-untested-template-behavior.md) —
  which parts of that contract your tests actually pin down, found by mutating the
  templates and re-running the suite
- [domtest](https://github.com/typelate/dom/tree/main/domtest) — the six
  parsers, failure behavior, testify conventions, and reusable assertions
  written against `spec.ElementQueries`
- Working tests in this repo: [simple](../examples/simple),
  [htmx-todo](../examples/htmx-todo),
  [datastar-search](../examples/datastar-search)
