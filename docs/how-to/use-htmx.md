# How to Use HTMX with Muxt

Build interactive UIs with server-rendered HTML. No JavaScript compilation, no client-side state management, no complexity.

## Why HTMX + Muxt

**HTMX** sends HTML over the wire instead of JSON. Your server returns fragments. Browser swaps them in. Simple.

**Muxt** generates type-safe handlers. Your methods return structs. Templates render HTML. No manual marshaling.

**Together:** Type-safe end-to-end. Domain logic in Go, presentation in templates, interactivity from HTML attributes.

## Basic Pattern

**Full page with HTMX:**
```gotemplate
{{define "GET / Home(ctx)"}}
<!DOCTYPE html>
<html>
<head>
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
</head>
<body>
    <button hx-get="/greet" hx-target="#content">Click</button>
    <div id="content"></div>
</body>
</html>
{{end}}
```

**Partial response:**
```gotemplate
{{define "GET /greet Greet(ctx)"}}
<div id="content">{{.Result.Message}}</div>
{{end}}
```

```go
func (s Server) Greet(ctx context.Context) Greeting {
    return Greeting{Message: "Hello from server"}
}
```

**Result:** Button click fetches `/greet`, swaps HTML into `#content`. Zero JavaScript written.

## Progressive Enhancement: Full Pages + Partials

One route, two responses. HTMX requests get fragments, direct navigation gets full pages.

```gotemplate
{{define "article-content"}}
  <h2>{{.Result.Title}}</h2>
  <p>{{.Result.Content}}</p>
{{end}}

{{define "GET /article/{id} GetArticle(ctx, id)"}}
{{if .Request.Header.Get "HX-Request"}}
  {{template "article-content" .}}
{{else}}
<!DOCTYPE html>
<html>
<head>
    <title>{{.Result.Title}}</title>
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
</head>
<body>
  {{template "article-content" .}}
</body>
</html>
{{end}}
{{end}}
```

**Why this matters:** URLs work with curl, search engines, browser history. HTMX is enhancement, not requirement.

## HTMX Response Headers

Control client behavior from templates using `.Header` and `.StatusCode`:

```gotemplate
{{define "POST /task/{id}/complete CompleteTask(ctx, id)"}}
{{if .Err}}
  {{with and (.StatusCode 400) (.Header "HX-Retarget" "#error")}}
    <div class="error">{{.Err.Error}}</div>
  {{end}}
{{else}}
  {{with .Header "HX-Trigger" "taskCompleted"}}
    <div class="task completed">✓ Done</div>
  {{end}}
{{end}}
{{end}}
```

**Common headers:**
- `HX-Redirect` - Client-side redirect
- `HX-Retarget` - Change target element
- `HX-Reswap` - Change swap strategy
- `HX-Trigger` - Trigger client-side events

See [HTMX response headers docs](https://htmx.org/reference/#response_headers) for complete list.

## Forms with Validation

**Templates:**
```gotemplate
{{define "GET /users ListUsers(ctx)"}}
<form hx-post="/users" hx-target="#user-list" hx-swap="beforeend">
    <input name="username" required>
    <input name="email" type="email" required>
    <button type="submit">Add</button>
</form>
<div id="user-list">
    {{range .Result.Users}}<div>{{.Username}}</div>{{end}}
</div>
{{end}}

{{define "POST /users 201 CreateUser(ctx, username, email)"}}
<div>{{.Result.Username}}</div>
{{end}}
```

**Domain methods:**
```go
func (s *Server) CreateUser(ctx context.Context, username, email string) (User, error) {
    if !isValidEmail(email) {
        return User{}, errors.New("invalid email")
    }
    user := User{Username: username, Email: email}
    s.users = append(s.users, user)
    return user, nil
}
```

**Validation errors:**
```gotemplate
{{define "POST /signup Signup(ctx, email, password)"}}
{{if .Err}}
  {{with and (.StatusCode 422) (.Header "HX-Retarget" "#errors")}}
    <div id="errors">{{.Err.Error}}</div>
  {{end}}
{{else}}
  {{with .Header "HX-Redirect" "/"}}Redirecting...{{end}}
{{end}}
{{end}}
```

## Production Patterns

**Inline editing:**
```gotemplate
{{define "GET /task/{id}/edit EditTask(ctx, id)"}}
<form hx-put="/task/{{.Result.ID}}" hx-target="closest div">
    <input name="title" value="{{.Result.Title}}">
    <button type="submit">Save</button>
</form>
{{end}}

{{define "PUT /task/{id} UpdateTask(ctx, id, title)"}}
<div>
    <span>{{.Result.Title}}</span>
    <button hx-get="/task/{{.Result.ID}}/edit" hx-target="closest div">Edit</button>
</div>
{{end}}
```

**Delete with confirmation:**
```gotemplate
{{define "DELETE /task/{id} DeleteTask(ctx, id)"}}{{end}}
```
```html
<button hx-delete="/task/{{.ID}}" hx-confirm="Delete?" hx-target="closest div" hx-swap="outerHTML">
    Delete
</button>
```

**Infinite scroll:**
```gotemplate
{{define "GET /articles ListArticles(ctx, page)"}}
{{range .Result.Articles}}<article>{{.Title}}</article>{{end}}
{{if .Result.HasMore}}
<div hx-get="/articles?page={{.Result.NextPage}}" hx-trigger="revealed" hx-swap="outerHTML">
    Loading...
</div>
{{end}}
{{end}}
```

## Architecture Guidelines

**URLs work everywhere** - HTMX requests and direct navigation both work. Check `HX-Request` header, return fragments or full pages.

**HTTP semantics matter** - Use correct status codes (200, 201, 400, 422). HTMX respects HTTP.

**Server-side validation** - Return errors as HTML. Client gets styled error messages, not JSON.

**Semantic HTML** - HTMX enhances HTML. Bad HTML = bad UX regardless of HTMX.

**Domain logic stays pure** - Methods return structs. Templates render HTML. Clean separation.

## Performance Characteristics

**Bandwidth:** HTML fragments typically smaller than JSON + client-side rendering framework. No framework.js to download.

**Latency:** One round trip per interaction (same as SPA). No hydration delay.

**CPU:** Server renders HTML (cheap), client swaps DOM (cheaper than React). Lower battery drain on mobile.

**Caching:** Standard HTTP caching works. No cache invalidation complexity.

## When Not to Use HTMX

**Rich client interactions** - Canvas drawing, complex drag-drop, real-time games → Use JavaScript.

**Offline-first apps** - Service workers + IndexedDB → Use SPA.

**Mobile apps** - Native UI → Use native frameworks.

HTMX excels at CRUD apps, admin panels, dashboards, content sites. Know your use case.

## Testing HTMX Routes

```go
func TestInlineEdit(t *testing.T) {
    req := httptest.NewRequest("GET", "/task/1/edit", nil)
    req.Header.Set("HX-Request", "true")
    rec := httptest.NewRecorder()

    mux.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code)
    frag := domtest.ParseResponseDocumentFragment(t, rec.Result(), atom.Form)
    input := frag.QuerySelector("input[name=title]")
    assert.NotNil(t, input)
}
```

See [test-handlers.md](test-handlers.md) for complete testing patterns.

## Next

- [HTMX docs](https://htmx.org/docs/) - Complete reference
- [HTMX examples](https://htmx.org/examples/) - Common patterns
- [Test handlers](test-handlers.md) - Testing with domtest
