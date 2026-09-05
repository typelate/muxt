# How to extend TemplateData with request-aware helpers

Templates sometimes need logic that reads both HTTP primitives and domain data — an authorization check, protocol detection, a feature flag from context. Putting it in the domain type couples the domain to HTTP; putting it in the template is untestable. Add a method to the generated `TemplateData[R, T]` instead: it already holds the request and the result, and templates can call any exported method on their dot.

Define the methods in a file `go generate` does not rewrite:

```go
// Authorization using request context and domain data
func (data *TemplateData[R, T]) CanEditPortfolio(p Portfolio) bool {
    ctx := data.Request().Context()
    session, ok := user.SessionFromContext(ctx)
    if !ok || session.UserID == "" || p.AuthorID == "" {
        return false
    }
    return p.AuthorID == session.UserID
}

// Protocol detection for progressive enhancement
func (data *TemplateData[R, T]) IsHXRequest() bool {
    return data.Request().Header.Get("HX-Request") == "true"
}

// Map domain-error status codes onto the response
// (see ../how-to/domain-error-status-codes.md)
func (data *TemplateData[R, T]) ErrorStatusCode() *TemplateData[R, T] {
    var sc interface{ StatusCode() int }
    if errors.As(data.Err(), &sc) {
        return data.StatusCode(sc.StatusCode())
    }
    return data
}
```

The `--output-htmx` flag generates `HXRequest` and the other `HX*` header helpers; only hand-write `IsHXRequest` without that flag.

Use them like any other template data:

```gotmpl
{{define "GET /portfolio/{id} GetPortfolio(ctx, id)"}}
{{if .CanEditPortfolio .Result}}
  <button hx-get="{{.Path.EditPortfolio .Result.ID}}">Edit</button>
{{end}}

{{if .IsHXRequest}}
  <div class="portfolio-content">...</div>  <!-- HTMX partial -->
{{else}}
  <!DOCTYPE html><html>...</html>  <!-- Full page for direct navigation -->
{{end}}
{{end}}
```

Domain types stay pure, templates get domain-aware helpers, and `muxt check` type-checks the calls.

Reach for this whenever logic needs both the request and the result: authorization, HTMX-vs-browser detection, feature flags from context, locale-aware rendering. Pure formatting belongs in [template functions](template-functions.md); business logic belongs in receiver methods.
