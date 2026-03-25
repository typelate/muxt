---
name: muxt-htmx
description: "Muxt: Use when exploring, developing, or testing HTMX interactions in a Muxt codebase. Covers finding hx-* attributes, using --output-htmx-helpers, testing fragment chains, and verifying inter-route coupling."
---

# HTMX with Muxt

Explore and develop HTMX interactions in a Muxt codebase. Covers discovery, the generated HTMX helpers, and testing inter-route coupling.

## Exploring HTMX in a Codebase

### Find HTMX Interactions

Scan templates for HTMX attributes that trigger HTTP requests:

```bash
grep -rn 'hx-get\|hx-post\|hx-put\|hx-patch\|hx-delete' --include='*.gohtml' .
```

Each match is a client-side interaction that targets a Muxt route. Note the associated attributes:

```bash
# Find swap targets — where does the response go?
grep -rn 'hx-target\|hx-swap\|hx-select' --include='*.gohtml' .

# Find triggers — what starts the request?
grep -rn 'hx-trigger\|hx-confirm\|hx-boost' --include='*.gohtml' .
```

### Trace an Interaction

For each `hx-get="/some/path"` found:

1. Find the route template that handles `/some/path` using `muxt list-template-calls --match "/some/path"`
2. Read that template to see what HTML fragment it returns
3. Check the `hx-target` and `hx-swap` on the triggering element to see where the fragment lands
4. Check if the route uses `.HXRequest` to return different HTML for HTMX vs direct navigation

See [HTMX attributes reference](https://htmx.org/reference/#attributes) for all available attributes.

## HTMX Helpers (`--output-htmx-helpers`)

Enable the flag to generate helper methods on `TemplateData`:

```go
//go:generate muxt generate --use-receiver-type=Server --output-htmx-helpers
```

This generates two sets of methods:

### Response Header Helpers (set in templates)

Use these in templates to control HTMX client behavior:

| Template Call | HTTP Header | HTMX Docs |
|---------------|-------------|-----------|
| `{{.HXLocation "/path"}}` | `HX-Location` | [hx-location](https://htmx.org/headers/hx-location/) |
| `{{.HXPushURL "/path"}}` | `HX-Push-Url` | [hx-push-url](https://htmx.org/attributes/hx-push-url/) |
| `{{.HXRedirect "/path"}}` | `HX-Redirect` | [hx-redirect](https://htmx.org/headers/hx-redirect/) |
| `{{.HXRefresh}}` | `HX-Refresh: true` | [response headers](https://htmx.org/reference/#response_headers) |
| `{{.HXReplaceURL "/path"}}` | `HX-Replace-Url` | [hx-replace-url](https://htmx.org/attributes/hx-replace-url/) |
| `{{.HXReswap "outerHTML"}}` | `HX-Reswap` | [hx-reswap](https://htmx.org/attributes/hx-swap/) |
| `{{.HXRetarget "#id"}}` | `HX-Retarget` | [response headers](https://htmx.org/reference/#response_headers) |
| `{{.HXReselect ".selector"}}` | `HX-Reselect` | [response headers](https://htmx.org/reference/#response_headers) |
| `{{.HXTrigger "event"}}` | `HX-Trigger` | [hx-trigger](https://htmx.org/headers/hx-trigger/) |
| `{{.HXTriggerAfterSettle "event"}}` | `HX-Trigger-After-Settle` | [hx-trigger](https://htmx.org/headers/hx-trigger/) |
| `{{.HXTriggerAfterSwap "event"}}` | `HX-Trigger-After-Swap` | [hx-trigger](https://htmx.org/headers/hx-trigger/) |

### Request Header Readers (check in templates)

Use these to detect HTMX requests and branch template output:

| Template Call | HTTP Header | HTMX Docs |
|---------------|-------------|-----------|
| `{{.HXRequest}}` | `HX-Request` | [hx-request](https://htmx.org/attributes/hx-request/) |
| `{{.HXBoosted}}` | `HX-Boosted` | [hx-boost](https://htmx.org/attributes/hx-boost/) |
| `{{.HXCurrentURL}}` | `HX-Current-URL` | [request headers](https://htmx.org/reference/#request_headers) |
| `{{.HXHistoryRestoreRequest}}` | `HX-History-Restore-Request` | [request headers](https://htmx.org/reference/#request_headers) |
| `{{.HXPrompt}}` | `HX-Prompt` | [hx-prompt](https://htmx.org/attributes/hx-prompt/) |
| `{{.HXTargetElementID}}` | `HX-Target` | [request headers](https://htmx.org/reference/#request_headers) |
| `{{.HXTriggerName}}` | `HX-Trigger-Name` | [request headers](https://htmx.org/reference/#request_headers) |
| `{{.HXTriggerElementID}}` | `HX-Trigger` | [request headers](https://htmx.org/reference/#request_headers) |

### Progressive Enhancement Pattern

Use `.HXRequest` to return fragments for HTMX and full pages for direct navigation:

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

See [HTMX server examples](https://htmx.org/examples/) for more interaction patterns.

## Template Fragments

HTMX endpoints frequently return partial HTML — a single row, a form field, a status badge — rather than full pages. Many templating languages need special fragment syntax to render part of a template. Go's `html/template` already has this built in: every `{{define "name"}}...{{end}}` block is a fragment that can be rendered independently with `{{template "name" .}}`.

This means muxt route templates are themselves fragments. A route template returns its `{{define}}` block as the response body. Sub-templates composed with `{{template}}` are also fragments. No special syntax or library support is needed — Go's template system provides [locality of behavior](https://htmx.org/essays/locality-of-behaviour/) for free.

The progressive enhancement pattern above shows this in action: the route template branches on `.HXRequest` to return either the fragment (`{{template "article-content" .}}`) or a full page wrapping it.

For background on why template fragments matter for hypermedia-driven applications, see [Template Fragments](https://htmx.org/essays/template-fragments/).

## Testing HTMX Fragment Chains

HTMX interactions form chains: a page contains an `hx-get` attribute pointing to another route, whose response swaps into a target element. These chains couple routes together. Tests should verify the coupling stays consistent.

**Do not use Given/When/Then table-driven tests for fragment chains.** Instead, write a single test function with helpers that exercise a sequence of requests and assert that the inter-route coupling is correct.

### Structure

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

### Exhaustive Coupling Assertions

In large codebases, assert that all HTMX attributes in a response are accounted for. This catches new `hx-get` or `hx-post` attributes that lack test coverage:

```go
func TestAllHTMXEndpointsAreTested(t *testing.T) {
    app := new(fake.App)
    mux := http.NewServeMux()
    p := TemplateRoutes(mux, app)

    // Render the full page
    homeReq := httptest.NewRequest("GET", p.Home(), nil)
    homeRec := httptest.NewRecorder()
    mux.ServeHTTP(homeRec, homeReq)
    homePage := domtest.ParseResponseDocument(t, homeRec.Result())

    // Map of CSS selector → allowed attribute values.
    // The key is the full selector so you can be specific (e.g. "form[hx-post]").
    for selector, expectedValues := range map[string][]string{
        "[hx-get]":      {p.EditArticle(1), p.EditArticle(2)},
        "[hx-post]":     {p.CreateArticle()},
        "form[hx-put]":  {p.UpdateArticle(1), p.UpdateArticle(2)},
        "[hx-delete]":   {p.DeleteArticle(1), p.DeleteArticle(2)},
    } {
        // Extract the hx-* attribute name from the selector
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

When a new `hx-get="/new/route"` appears in a template, this test fails — forcing the developer to add it to the allowed map and write a corresponding fragment chain test.

### Testing Response Headers

When templates set HTMX response headers via helpers like `.HXRetarget` or `.HXTrigger`, assert those headers in the chain test:

```go
    // Follow a POST that triggers a client-side event
    req := httptest.NewRequest("POST", "/task/1/complete", nil)
    req.Header.Set("HX-Request", "true")
    rec := httptest.NewRecorder()
    mux.ServeHTTP(rec, req)

    assert.Equal(t, "taskCompleted", rec.Header().Get("HX-Trigger"))
    assert.Equal(t, "#task-list", rec.Header().Get("HX-Retarget"))
```

## Testing HTMX Islands with chromedp

For dynamic [islands](https://htmx.org/essays/hypermedia-friendly-scripting/#islands) that load content via HTMX after the initial page render, `domtest` alone cannot verify the behavior because it doesn't execute JavaScript. Use [chromedp](https://github.com/chromedp/chromedp) to test these interactions in a real browser.

chromedp tests are slow and require a browser. Skip them in short mode or gate them behind a build tag so `go test` stays fast by default:

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
        // Navigate to the full page — this loads htmx.js
        chromedp.Navigate(ts.URL+"/dashboard"),
        // Wait for the island content to be swapped in by HTMX
        chromedp.WaitVisible("#stats-island", chromedp.ByQuery),
        chromedp.Text("#stats-island", &statsText, chromedp.ByQuery),
    )
    require.NoError(t, err)
    assert.Contains(t, statsText, "Total Articles")
}
```

If the island content comes from a fragment endpoint (one that returns different HTML for `HX-Request` vs direct navigation), the test must navigate to the parent page that triggers the `hx-get`, not to the fragment URL directly. The parent page loads htmx.js which then fires the request.

Use chromedp for targeted island tests only. Prefer `domtest` for everything else — it's faster and doesn't require a browser.

## Inline Field Validation

HTMX enables per-field validation by sending individual field values to the server as the user interacts with the form. Each field posts to a validation endpoint that returns the field container with error feedback.

### Pattern

Add `hx-post` to an input's wrapping `<div>`. Use `hx-target="this"` and `hx-swap="outerHTML"` so the server response replaces the entire field container:

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

The validation endpoint receives the request, checks the field, and returns the same container with error or success styling:

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

The handler validates and returns an error or nil:

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

By default, HTMX triggers on `change` for inputs. Add `hx-trigger` for different timing:

```html
<!-- Validate on blur (when the user tabs away) -->
<input name="email" hx-post="/contact/email" hx-trigger="blur">

<!-- Validate on keyup with debounce -->
<input name="username" hx-post="/signup/username" hx-trigger="keyup delay:500ms">
```

This is an HTMX-specific pattern. For standard HTML forms without HTMX, rely on HTML5 validation attributes (`required`, `pattern`, `min`/`max`) for client-side feedback, and handle validation errors on the full form submission. See [Forms](../muxt_forms/SKILL.md#re-rendering-after-validation-errors).

See [HTMX inline validation example](https://htmx.org/examples/inline-validation/).

## HTTP Status Codes and HTMX

By default, HTMX only swaps content for 2xx responses. Non-2xx responses are ignored unless you use the [response-targets extension](https://htmx.org/extensions/response-targets/), which lets you target different elements based on status code:

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

Without this extension, setting `StatusCode()` on error types (returning 404, 500, etc.) won't cause HTMX to display the error template — the response is silently dropped. If your handlers return non-2xx status codes for error states, either:

1. Use the `response-targets` extension with `hx-target-*` attributes to handle error responses
2. Return 200 with error content in the body (simpler, works without extensions)
3. Use `HX-Reswap` and `HX-Retarget` response headers to redirect error content to a different element

See [HTMX response-targets extension](https://htmx.org/extensions/response-targets/) for the full API.

## Reference

- [`muxt generate` flags](../../reference/commands/generate.md) — `--output-htmx-helpers` flag
- [HTMX Example](../../examples/counter-htmx/) — Counter app with HTMX helpers
- [Template Name Syntax](../../reference/template-names.md)
- [chromedp](https://github.com/chromedp/chromedp) — Headless Chrome for island testing

### Examples and Test Cases

- `docs/examples/counter-htmx/` — Complete counter app demonstrating `--output-htmx-helpers`
- `cmd/muxt/testdata/reference_htmx_helpers.txt` — `--output-htmx-helpers` flag, HX-Request detection, response header assertions
- `cmd/muxt/testdata/howto_form_basic.txt` — Form binding (relevant to HTMX POST patterns)
- `cmd/muxt/testdata/reference_status_codes.txt` — Status codes (relevant to HTMX response handling)

### HTMX Official Documentation

- [HTMX Documentation](https://htmx.org/docs/) — Core concepts
- [HTMX Attributes Reference](https://htmx.org/reference/#attributes) — All `hx-*` attributes
- [HTMX Request Headers](https://htmx.org/reference/#request_headers) — Headers sent by HTMX
- [HTMX Response Headers](https://htmx.org/reference/#response_headers) — Headers the server can set
- [HTMX Examples](https://htmx.org/examples/) — Common interaction patterns
- [HTMX Essays](https://htmx.org/essays/) — Design philosophy
