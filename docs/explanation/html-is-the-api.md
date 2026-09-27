# HTML is the API

A handler's response body is its contract. The form `action`, input `name`, `hx-target` and `href` decide the next request, so tests assert on them.

[domtest](https://github.com/typelate/dom/tree/main/domtest) parses an `*http.Response` into a queryable DOM, so a test asks for `form input[name="todo"]` instead of running `strings.Contains` over the body. A substring check still passes when the input leaves the form or loses its `name`; the selector does not.

## What muxt contributes

The generated code makes the whole route stack drivable in process:

| Generated symbol | Use in a test |
|---|---|
| `TemplateRoutes(mux, receiver)` | Registers the real handlers on a fresh `*http.ServeMux` |
| `TemplateRoutePaths{}.MethodName(args)` | Builds request URLs from the route patterns, so a renamed method or changed path parameters fail the build instead of returning 404 |
| `RoutesReceiver` | An interface, so a test substitutes a stand-in and pins what a template renders |

[Structure a project for testing](../how-to/receiver-package-and-testing.md) covers the layout and which layer to fake.

## What domtest contributes

`domtest.ParseResponseDocument(t, res)` returns a document with `QuerySelector`; [Find untested template behavior](../tutorials/find-untested-template-behavior.md) walks through one.

## Fragments need the element they swap into

HTML parsing is context-sensitive, so the fragment parser takes the parent element the response lands in: `domtest.ParseResponseDocumentFragment(t, res, atom.Tbody)`. A `<tr>` parsed under `atom.Body` is discarded; under `atom.Tbody` it is a row. A fragment that vanishes here would not have landed in the page either ([simple example test](../examples/simple/template_routes_test.go)).

## htmx and Datastar widen the API

htmx and Datastar write the interaction graph into attributes: which element issues a request, to what URL, and where the response lands. Attribute selectors (`[hx-post]`, `[data-on\:click]`) make the graph queryable from a single response.

`http.ServeMux.Handler` returns an empty pattern when nothing would serve the request, so a sweep over every `[hx-post]` catches a template and a route that drift apart.

### Resolve targets from the trigger

`hx-target` is inherited: `<tbody hx-target="closest tr">` sets the target for the `<td hx-get=...>` inside it. Start at the trigger and walk up with `trigger.Closest("[hx-target]")`.

`closest`, `find`, `this`, `next` and `previous` are htmx verbs, not CSS: `QuerySelector("closest tr")` looks for a `<closest>` element containing a `<tr>` and matches nothing.

| `hx-target` value | Resolve with |
|---|---|
| `#id`, `.class`, any CSS | `doc.QuerySelector(target)` |
| `closest tr` | `trigger.Closest("tr")` |
| `find td` | `trigger.QuerySelector("td")` |
| `this` | the trigger itself |

An `hx-swap-oob="true"` element's `id` has to exist in the page being patched.

### Datastar hides the URL in an expression

`data-on:click="@post('{{.Path.Increment}}')"` holds the URL inside a JavaScript expression, so extract it from the attribute. `html/template` escapes `/` as `\/` in that context, so unescape before routing ([datastar-counter test](../examples/datastar-counter/template_test.go), `jsPath`).

## Where it stops

A sweep proves the graph is wired, not that htmx or Datastar behaves: domtest does not run JavaScript. Use a browser ([chromedp](https://github.com/chromedp/chromedp)) only for behavior that belongs to the browser.
