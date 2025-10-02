# How to Use HTMX with Muxt

Build dynamic web pages without writing JavaScript.

## Goal

Use HTMX to make your pages interactive:
- Update parts of the page without full reloads
- Handle forms without leaving the page
- Build modern UIs with just HTML

## Prerequisites

- Basic understanding of [HTMX](https://htmx.org/)
- A working Muxt setup
- Familiarity with Go templates

## Include HTMX in Your Template

Add the HTMX script to your base template:

```gotemplate
{{define "GET / Home(ctx)"}}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>My App</title>
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
</head>
<body>
    <div id="content">
        <h1>Welcome</h1>
        <button hx-get="/greet" hx-target="#content">Click me</button>
    </div>
</body>
</html>
{{end}}
```

## Handle HTMX Requests

Create a route that returns partial HTML for HTMX requests:

```gotemplate
{{define "GET /greet Greet(ctx)"}}
<div id="content">
    <h2>Hello from HTMX!</h2>
    <p>This content was loaded dynamically.</p>
    <button hx-get="/" hx-target="#content">Go back</button>
</div>
{{end}}
```

```go
func (s Server) Greet(ctx context.Context) Greeting {
    return Greeting{Message: "Hello from HTMX!"}
}
```

## Detect HTMX Requests in Templates

Use the `Request` field to check if the request came from HTMX:

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
{{- if .Request.Header.Get "HX-Request"}}
  <!-- Return partial HTML for HTMX -->
  <div class="article">
    <h2>{{.Result.Title}}</h2>
    <p>{{.Result.Content}}</p>
  </div>
{{- else}}
  <!-- Return full page for direct navigation -->
  <!DOCTYPE html>
  <html>
  <head>
      <title>{{.Result.Title}}</title>
      <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  </head>
  <body>
    <div class="article">
      <h2>{{.Result.Title}}</h2>
      <p>{{.Result.Content}}</p>
    </div>
  </body>
  </html>
{{- end}}
{{end}}
```

## Set HTMX Response Headers

Use template helpers to set HTMX-specific response headers.

### Copy HTMX Helper Code

The `docs/htmx` directory contains helper code for working with HTMX response headers. Copy `htmx.go` into your package:

```bash
cp docs/htmx/htmx.go internal/hypertext/
```

This provides template methods like:
- `.Header(key, value)` - Set response headers
- `.StatusCode(code)` - Set status code
- `.HXRetarget(selector)` - Set `HX-Retarget` header (add it yourself, copy from ../htmx/)
- `.HXReswap(strategy)` - Set `HX-Reswap` header (add it yourself, copy from ../htmx/)

### Use HTMX Headers in Templates

```gotemplate
{{define "POST /task/{id}/complete CompleteTask(ctx, id)"}}
{{- if .Err}}
  {{- with and (.StatusCode 400) (.Header "HX-Retarget" "#error") (.Header "HX-Reswap" "innerHTML")}}
    <div class="error">{{.Err.Error}}</div>
  {{- end}}
{{- else}}
  {{- with .Header "HX-Trigger" "taskCompleted"}}
    <div class="task completed">
      <span>✓ Task completed</span>
    </div>
  {{- end}}
{{- end}}
{{end}}
```

This template:
- Returns error HTML with retargeting on failure
- Returns success HTML with a trigger event on success
- Uses template helpers to set HTMX headers

## Build a Form with HTMX

Create an interactive form that updates without page reload:

### Template with Form

```gotemplate
{{define "GET /users ListUsers(ctx)"}}
<!DOCTYPE html>
<html>
<head>
    <title>Users</title>
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
</head>
<body>
    <h1>Users</h1>

    <form hx-post="/users" hx-target="#user-list" hx-swap="beforeend">
        <input type="text" name="username" placeholder="Username" required>
        <input type="email" name="email" placeholder="Email" required>
        <button type="submit">Add User</button>
    </form>

    <div id="user-list">
        {{range .Result.Users}}
        <div class="user">{{.Username}} ({{.Email}})</div>
        {{end}}
    </div>
</body>
</html>
{{end}}

{{define "POST /users 201 CreateUser(ctx, username, email)"}}
<div class="user">{{.Result.Username}} ({{.Result.Email}})</div>
{{end}}
```

### Receiver Methods

```go
type Server struct {
    users []User
}

func (s Server) ListUsers(ctx context.Context) UserList {
    return UserList{Users: s.users}
}

func (s *Server) CreateUser(ctx context.Context, username, email string) (User, error) {
    user := User{
        ID:       len(s.users) + 1,
        Username: username,
        Email:    email,
    }
    s.users = append(s.users, user)
    return user, nil
}
```

## Handle Form Validation Errors

Return validation errors as HTML fragments:

```gotemplate
{{define "POST /signup Signup(ctx, email, password)"}}
{{- if .Err}}
  {{- with and (.StatusCode 422) (.Header "HX-Retarget" "#errors")}}
    <div id="errors" class="error">
      {{.Err.Error}}
    </div>
  {{- end}}
{{- else}}
  {{- with .Header "HX-Redirect" "/"}}
    <div>Account created! Redirecting...</div>
  {{- end}}
{{- end}}
{{end}}
```

```go
func (s Server) Signup(ctx context.Context, email, password string) (User, error) {
    if !isValidEmail(email) {
        return User{}, errors.New("Invalid email address")
    }
    if len(password) < 8 {
        return User{}, errors.New("Password must be at least 8 characters")
    }
    return s.createUser(email, password)
}
```

## Common HTMX Patterns

### Inline Editing

```gotemplate
{{define "GET /task/{id}/edit EditTask(ctx, id)"}}
<form hx-put="/task/{{.Result.ID}}" hx-target="closest div">
    <input name="title" value="{{.Result.Title}}">
    <button type="submit">Save</button>
    <button hx-get="/task/{{.Result.ID}}" hx-target="closest div">Cancel</button>
</form>
{{end}}

{{define "PUT /task/{id} UpdateTask(ctx, id, title)"}}
<div>
    <span>{{.Result.Title}}</span>
    <button hx-get="/task/{{.Result.ID}}/edit" hx-target="closest div">Edit</button>
</div>
{{end}}
```

### Delete with Confirmation

```gotemplate
{{define "DELETE /task/{id} DeleteTask(ctx, id)"}}
<!-- Returns empty, element is removed from DOM -->
{{end}}
```

```html
<div id="task-{{.ID}}">
    <span>{{.Title}}</span>
    <button
        hx-delete="/task/{{.ID}}"
        hx-confirm="Are you sure?"
        hx-target="#task-{{.ID}}"
        hx-swap="outerHTML">
        Delete
    </button>
</div>
```

### Infinite Scroll

```gotemplate
{{define "GET /articles ListArticles(ctx, page)"}}
{{- range .Result.Articles}}
<article>
    <h2>{{.Title}}</h2>
    <p>{{.Content}}</p>
</article>
{{- end}}

{{- if .Result.HasMore}}
<div hx-get="/articles?page={{.Result.NextPage}}"
     hx-trigger="revealed"
     hx-swap="outerHTML">
    Loading more...
</div>
{{- end}}
{{end}}
```

## Tips for HTMX with Muxt

**Design URLs for both HTMX and direct access** - Your routes should work whether someone clicks a link or visits directly. Check `HX-Request` header and return full pages or fragments accordingly.

**Use semantic HTML** - HTMX enhances HTML. If your HTML is bad, HTMX can't fix it.

**Set appropriate status codes** - Errors should return error codes. HTMX respects HTTP.

**Leverage HTMX headers** - `HX-Retarget`, `HX-Trigger`, `HX-Redirect` let you control the browser from the server. Use them.

**Keep templates simple** - Complex logic goes in Go. Templates render data. That's it.

## Next Steps

- Explore the [HTMX documentation](https://htmx.org/) for more patterns
- Review [HTMX examples](https://htmx.org/examples/) for inspiration
- Learn about [testing HTMX responses](test-handlers.md#testing-htmx-partial-responses) with domtest
- Check the [`docs/htmx` directory](../htmx/) for reference implementation
