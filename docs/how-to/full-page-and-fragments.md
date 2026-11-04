# How to serve full pages and fragments from one route

One URL often needs three responses: the whole page for a direct visit, the main content for an htmx link that swaps `#main`, and only the results list when a search box swaps `#results`. Write one route template that picks among layout partials, rather than three routes. The examples use `--output-htmx` ([flags](../reference/cli.md#flags)) for the `.HX*` readers. `ListArticles` and its result stand for your own method and type.

## Split the page into partials

```gotmpl
{{define "layout-start"}}<!DOCTYPE html>
<html lang="en">
<head><title>Articles</title><script src="/static/htmx.min.js"></script></head>
<body>
<nav><a href="{{.Path.ListArticles}}" hx-get="{{.Path.ListArticles}}" hx-target="#main" hx-push-url="true">Articles</a></nav>
<main id="main">{{end}}

{{define "layout-end"}}</main>
</body>
</html>{{end}}

{{define "articles-main"}}<h1>Articles</h1>
<input type="search" name="q" value="{{.Result.Query}}"
       hx-get="{{.Path.ListArticles}}" hx-trigger="input changed delay:300ms"
       hx-target="#results" hx-swap="outerHTML" hx-push-url="true">
{{template "article-results" .}}{{end}}

{{define "article-results"}}<ul id="results">
{{range .Result.Articles}}<li>{{.Title}}</li>{{end}}
</ul>{{end}}
```

Each partial takes the route's `TemplateData`, so every one of them can call `.Path` and read `.Result`.

## Branch in the route template

```gotmpl
{{define "GET /articles ListArticles(ctx, form)"}}
{{- .Header "Vary" "HX-Request, HX-Target" -}}
{{- if and .HXRequest (not .HXHistoryRestoreRequest) -}}
	{{- if eq .HXTargetElementID "results"}}{{template "article-results" .}}
	{{- else}}{{template "articles-main" .}}{{end -}}
{{- else -}}
	{{template "layout-start" .}}{{template "articles-main" .}}{{template "layout-end" .}}
{{- end -}}
{{end}}
```

- `.HXRequest` is true when the request carries `HX-Request: true`, and `.HXTargetElementID` is the id of the element the response will replace, from `HX-Target`.
- `not .HXHistoryRestoreRequest` sends the whole page when htmx restores history (see edge cases).
- `Vary` tells the browser and any proxy that the response depends on those two request headers, so a cached `#results` list is not served for a direct visit to the same URL.
- `hx-push-url="true"` puts the requested URL in the address bar, so the back button and a copied link both reach a route that can render the full page.

## Let muxt check the partials

`muxt check` checks each `{{template}}` call against the type of dot it passes, so a partial shared by several routes is checked once per calling route, against each route's result type ([type checking](../reference/type-checking.md#what-is-checked)). A partial that reads `.Result.Query` fails the check for any route whose result has no `Query`.

## Skip the branch with hx-select

When rendering the whole page every time is cheap enough, leave the route template unbranched and let the client pick the part it wants:

```html
<a href="/articles" hx-get="/articles" hx-target="#main" hx-select="#main" hx-swap="outerHTML" hx-push-url="true">Articles</a>
```

`hx-select` takes the element matching the selector out of the response and swaps only that. The server renders and sends the full page on each request, and no `Vary` header is needed because every response is the same.

## Edge cases

- **History restore.** When the user goes back to a URL that `hx-push-url` added and htmx has no snapshot of it, htmx requests that URL with `HX-Request: true` and `HX-History-Restore-Request: true`, without `HX-Target`, and puts the response in place of the whole body. A fragment there would replace the page, which is why the route template guards on `not .HXHistoryRestoreRequest`.
- **Headers are client input.** The `.HX*` readers cannot tell a user from a script that sets the same headers, so never decide access on them.
- **`Vary` is set, not added.** `.Header` calls `Header().Set`, so a later `.Header "Vary"` in a partial replaces this one rather than adding to it ([call results](../reference/call-results.md#templatedata)). Set it once, in the route template.

## Datastar

A Datastar action sends `Datastar-Request: true`. With `--output-datastar`, branch on `{{if .DatastarRequest}}` and set `Vary` to `Datastar-Request` the same way; [Datastar live view](datastar-live-view.md) covers routes that answer Datastar actions.
