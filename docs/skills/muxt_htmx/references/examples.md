# muxt + HTMX: examples

## HTMX helpers (`--output-htmx-helpers`)

Enable to generate helper methods on `TemplateData`:

```go
//go:generate muxt generate --use-receiver-type=Server --output-htmx-helpers
```

### Response header helpers (set in templates)

| Template call | HTTP header | HTMX docs |
|---------------|-------------|-----------|
| `{{.HXLocation "/path"}}` | `HX-Location` | [hx-location](https://htmx.org/headers/hx-location/) |
| `{{.HXPushURL "/path"}}` | `HX-Push-Url` | [hx-push-url](https://htmx.org/attributes/hx-push-url/) |
| `{{.HXRedirect "/path"}}` | `HX-Redirect` | [hx-redirect](https://htmx.org/headers/hx-redirect/) |
| `{{.HXRefresh}}` | `HX-Refresh: true` | [response headers](https://htmx.org/reference/#response_headers) |
| `{{.HXReplaceURL "/path"}}` | `HX-Replace-Url` | [hx-replace-url](https://htmx.org/attributes/hx-replace-url/) |
| `{{.HXReswap "outerHTML"}}` | `HX-Reswap` | [hx-reswap](https://htmx.org/attributes/hx-swap/) |
| `{{.HXRetarget "#id"}}` | `HX-Retarget` | [response headers](https://htmx.org/reference/#response_headers) |
| `{{.HXReselect ".sel"}}` | `HX-Reselect` | [response headers](https://htmx.org/reference/#response_headers) |
| `{{.HXTrigger "event"}}` | `HX-Trigger` | [hx-trigger](https://htmx.org/headers/hx-trigger/) |
| `{{.HXTriggerAfterSettle "event"}}` | `HX-Trigger-After-Settle` | [hx-trigger](https://htmx.org/headers/hx-trigger/) |
| `{{.HXTriggerAfterSwap "event"}}` | `HX-Trigger-After-Swap` | [hx-trigger](https://htmx.org/headers/hx-trigger/) |

### Request header readers (check in templates)

| Template call | HTTP header | HTMX docs |
|---------------|-------------|-----------|
| `{{.HXRequest}}` | `HX-Request` | [hx-request](https://htmx.org/attributes/hx-request/) |
| `{{.HXBoosted}}` | `HX-Boosted` | [hx-boost](https://htmx.org/attributes/hx-boost/) |
| `{{.HXCurrentURL}}` | `HX-Current-URL` | [request headers](https://htmx.org/reference/#request_headers) |
| `{{.HXHistoryRestoreRequest}}` | `HX-History-Restore-Request` | [request headers](https://htmx.org/reference/#request_headers) |
| `{{.HXPrompt}}` | `HX-Prompt` | [hx-prompt](https://htmx.org/attributes/hx-prompt/) |
| `{{.HXTargetElementID}}` | `HX-Target` | [request headers](https://htmx.org/reference/#request_headers) |
| `{{.HXTriggerName}}` | `HX-Trigger-Name` | [request headers](https://htmx.org/reference/#request_headers) |
| `{{.HXTriggerElementID}}` | `HX-Trigger` | [request headers](https://htmx.org/reference/#request_headers) |

## Progressive enhancement

Use `.HXRequest` to return fragments for HTMX and full pages for direct nav:

```gotmpl
{{define "GET /article/{id} GetArticle(ctx, id)"}}
{{if .HXRequest}}
  {{template "article-content" .}}
{{else}}
<!DOCTYPE html>
<html>
<head><title>{{.Result.Title}}</title></head>
<body>{{template "article-content" .}}</body>
</html>
{{end}}
{{end}}
```

## Template fragments — locality of behaviour

HTMX endpoints frequently return partial HTML. Go's `html/template` already provides this — every `{{define "name"}}...{{end}}` is a fragment renderable with `{{template "name" .}}`. No special syntax needed.

Muxt route templates are themselves fragments. Sub-templates composed with `{{template}}` are also fragments. See [Template Fragments](https://htmx.org/essays/template-fragments/) for the broader argument.

## Testing fragment chains

HTMX interactions form chains: a page contains an `hx-get` attribute pointing to another route, whose response swaps into a target element. Tests verify the inter-route coupling.

**Do not use Given/When/Then table-driven tests for fragment chains.** Use a single test function that exercises a sequence of requests:

```go
func TestArticleWorkflow(t *testing.T) {
    app := new(fake.App)
    mux := http.NewServeMux()
    TemplateRoutes(mux, app)
    var p TemplateRoutePaths

    // Load the article list page
    listReq := httptest.NewRequest("GET", p.ListArticles(), nil)
    listRec := httptest.NewRecorder()
    mux.ServeHTTP(listRec, listReq)
    assert.Equal(t, http.StatusOK, listRec.Code)
    listPage := domtest.ParseResponseDocument(t, listRec.Result())

    // Verify HTMX attributes couple correctly to routes
    editBtn := listPage.QuerySelector("[hx-get]")
    require.NotNil(t, editBtn)
    assert.Equal(t, p.EditArticle(1), editBtn.GetAttribute("hx-get"))
    assert.Equal(t, "#article-1", editBtn.GetAttribute("hx-target"))

    // Follow the hx-get to the edit form fragment
    editReq := httptest.NewRequest("GET", p.EditArticle(1), nil)
    editReq.Header.Set("HX-Request", "true")
    editRec := httptest.NewRecorder()
    mux.ServeHTTP(editRec, editReq)
    assert.Equal(t, http.StatusOK, editRec.Code)
    editFragment := domtest.ParseResponseDocumentFragment(t, editRec.Result(), atom.Div)

    // Verify the fragment contains the target element from hx-target="#article-1"
    targetEl := editFragment.QuerySelector("#article-1")
    require.NotNil(t, targetEl, "fragment must contain element matching hx-target")
    titleInput := editFragment.QuerySelector("input[name=title]")
    require.NotNil(t, titleInput)

    // Submit the edit form
    updateForm := url.Values{"title": []string{"Updated Title"}}
    updateReq := httptest.NewRequest("PUT", p.UpdateArticle(1), strings.NewReader(updateForm.Encode()))
    updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    updateReq.Header.Set("HX-Request", "true")
    updateRec := httptest.NewRecorder()
    mux.ServeHTTP(updateRec, updateReq)
    assert.Equal(t, http.StatusOK, updateRec.Code)
}
```

## Exhaustive coupling assertions

Catch new `hx-get` / `hx-post` that lack test coverage:

```go
func TestAllHTMXEndpointsAreTested(t *testing.T) {
    app := new(fake.App)
    mux := http.NewServeMux()
    p := TemplateRoutes(mux, app)

    homeReq := httptest.NewRequest("GET", p.Home(), nil)
    homeRec := httptest.NewRecorder()
    mux.ServeHTTP(homeRec, homeReq)
    homePage := domtest.ParseResponseDocument(t, homeRec.Result())

    for selector, expectedValues := range map[string][]string{
        "[hx-get]":      {p.EditArticle(1), p.EditArticle(2)},
        "[hx-post]":     {p.CreateArticle()},
        "form[hx-put]":  {p.UpdateArticle(1), p.UpdateArticle(2)},
        "[hx-delete]":   {p.DeleteArticle(1), p.DeleteArticle(2)},
    } {
        attr := selector[strings.LastIndex(selector, "hx-"):]
        attr = attr[:strings.IndexByte(attr, ']')]

        var found []string
        for _, el := range homePage.QuerySelectorAll(selector) {
            found = append(found, el.GetAttribute(attr))
        }
        assert.ElementsMatch(t, expectedValues, found,
            "selector %q: unexpected attribute values", selector)
    }
}
```

A new `hx-get="/new/route"` in a template fails this test, forcing a corresponding fragment chain test.

## Testing response headers

```go
req := httptest.NewRequest("POST", "/task/1/complete", nil)
req.Header.Set("HX-Request", "true")
rec := httptest.NewRecorder()
mux.ServeHTTP(rec, req)

assert.Equal(t, "taskCompleted", rec.Header().Get("HX-Trigger"))
assert.Equal(t, "#task-list", rec.Header().Get("HX-Retarget"))
```

## Islands with chromedp

For islands that load via HTMX after initial render, `domtest` can't verify (no JS). Use chromedp:

```go
//go:build chromedp

func TestDashboardIsland(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping chromedp test in short mode")
    }

    mux := http.NewServeMux()
    TemplateRoutes(mux, srv)
    ts := httptest.NewServer(mux)
    defer ts.Close()

    ctx, cancel := chromedp.NewContext(context.Background())
    defer cancel()

    var statsText string
    err := chromedp.Run(ctx,
        chromedp.Navigate(ts.URL+"/dashboard"),
        chromedp.WaitVisible("#stats-island", chromedp.ByQuery),
        chromedp.Text("#stats-island", &statsText, chromedp.ByQuery),
    )
    require.NoError(t, err)
    assert.Contains(t, statsText, "Total Articles")
}
```

If the island content comes from a fragment endpoint (returns different HTML for `HX-Request` vs direct), navigate to the *parent* page (which loads htmx.js and fires the request), not the fragment URL directly. Use chromedp for targeted island tests only — prefer domtest elsewhere.

## Inline field validation

Add `hx-post` to an input's wrapping `<div>`. Use `hx-target="this"` and `hx-swap="outerHTML"`:

```gotmpl
{{define "GET /contact Contact(ctx)"}}
<form hx-post="{{$.Path.SubmitContact}}">
  <div hx-target="this" hx-swap="outerHTML">
    <label for="email">Email</label>
    <input id="email" name="email" type="email" required
           hx-post="{{$.Path.ValidateEmail}}"
           aria-describedby="email-err">
    <p id="email-err"></p>
  </div>
  <button type="submit">Send</button>
</form>
{{end}}
```

Validation endpoint returns the same container with error or success styling:

```gotmpl
{{define "POST /contact/email ValidateEmail(request)"}}
<div hx-target="this" hx-swap="outerHTML">
  <label for="email">Email</label>
  <input id="email" name="email" type="email" required
         hx-post="{{$.Path.ValidateEmail}}"
         value="{{.Request.FormValue "email"}}"
         {{if .Err}}aria-invalid="true"{{end}}
         aria-describedby="email-err">
  {{with .Err}}
    <p id="email-err" role="alert">{{.Error}}</p>
  {{else}}
    <p id="email-err"></p>
  {{end}}
</div>
{{end}}
```

```go
func (s Server) ValidateEmail(r *http.Request) error {
    email := r.FormValue("email")
    if !isValidEmail(email) {
        return fmt.Errorf("invalid email address")
    }
    if s.db.EmailExists(email) {
        return fmt.Errorf("that email is already taken")
    }
    return nil
}
```

Default trigger is `change`. For different timing:

```html
<input name="email" hx-post="/contact/email" hx-trigger="blur">
<input name="username" hx-post="/signup/username" hx-trigger="keyup delay:500ms">
```

HTMX-specific pattern. For standard HTML forms without HTMX, rely on HTML5 validation attributes and full-form-submit error handling — see [Forms](../../muxt_forms/SKILL.md#re-rendering-after-validation-errors).

See [HTMX inline validation example](https://htmx.org/examples/inline-validation/).

## Status codes and HTMX

HTMX swaps only on 2xx by default. Non-2xx is ignored unless you use the [response-targets extension](https://htmx.org/extensions/response-targets/):

```html
<div hx-ext="response-targets">
  <button hx-post="/task"
          hx-target="#result"
          hx-target-404="#not-found"
          hx-target-5*="#server-error">
    Submit
  </button>
</div>
```

Without it, `StatusCode()` returning 404/500 won't render the error template — the response is silently dropped. Options:

1. Use `response-targets` with `hx-target-*` attributes.
2. Return 200 with error content in the body (simpler).
3. Use `HX-Reswap`/`HX-Retarget` response headers to redirect error content.
