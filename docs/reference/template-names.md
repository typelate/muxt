# Template Name Syntax Reference

Complete specification for Muxt template naming. Use when pair programming to validate route definitions.

## Syntax

```
[METHOD ][HOST]/PATH[ HTTP_STATUS][ CALL]
```

**All components:**
```gotmpl
{{define "GET example.com/greet/{language} 200 Greeting(ctx, language)"}}{{end}}
```

**Minimal (path only):**
```gotmpl
{{define "/"}}{{end}}
```

**Key rules:**
- Templates without matching names are ignored (not an error)
- Path is required, all other components optional
- Space-separated components, order matters
- Uses Go 1.22+ `http.ServeMux` pattern matching

## Quick Reference Table

| Component | Format | Example | Required |
|-----------|--------|---------|----------|
| METHOD | `GET`, `POST`, `PUT`, `PATCH`, `DELETE` | `GET` | No |
| HOST | `example.com` | `api.example.com` | No |
| PATH | `/path/{param}` | `/user/{id}` | **Yes** |
| STATUS | `200` or `http.StatusOK` | `201` | No |
| CALL | `Method(args...)` | `GetUser(ctx, id)` | No |

## Path Patterns

### Basic Paths

```gotmpl
{{define "GET /"}}{{end}}              <!-- Root -->
{{define "GET /about"}}{{end}}         <!-- Static path -->
{{define "GET /user/{id}"}}{{end}}     <!-- Path parameter -->
{{define "GET /user/{id}/post/{postID}"}}{{end}}  <!-- Multiple parameters -->
```

[tutorial_basic_route.txt](../../cmd/muxt/testdata/tutorial_basic_route.txt) · [howto_path_param.txt](../../cmd/muxt/testdata/howto_path_param.txt)

### Path Matching Modes

```gotmpl
{{define "GET /{$}"}}{{end}}           <!-- Exact: "/" only, not "/foo" -->
{{define "GET /static/"}}{{end}}       <!-- Prefix: "/static/", "/static/foo", "/static/foo/bar" -->
{{define "GET /files/{path...}"}}{{end}}  <!-- Wildcard: captures "/files/a/b/c" as path="a/b/c" -->
```

**Note:** `/{$}` vs `/` behavior differs. Former is exact match, latter matches prefix.

[reference_path_exact_match.txt](../../cmd/muxt/testdata/reference_path_exact_match.txt) · [reference_path_prefix.txt](../../cmd/muxt/testdata/reference_path_prefix.txt)

### Path Parameters Are Type-Safe

```gotmpl
{{define "GET /article/{id} GetArticle(ctx, id)"}}{{end}}
```

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) {
    // id auto-parsed from string → int
    // Parse failure → 400 Bad Request (automatic)
}
```

**Note:** Path param names must match method param names exactly. Type conversion is automatic based on method signature.

[howto_arg_path_param.txt](../../cmd/muxt/testdata/howto_arg_path_param.txt)

## HTTP Methods

```gotmpl
{{define "GET /posts"}}{{end}}          <!-- Read -->
{{define "POST /posts"}}{{end}}         <!-- Create -->
{{define "PUT /posts/{id}"}}{{end}}     <!-- Replace -->
{{define "PATCH /posts/{id}"}}{{end}}   <!-- Update -->
{{define "DELETE /posts/{id}"}}{{end}}  <!-- Delete -->
```

**Without method prefix:** Matches all methods (GET, POST, PUT, PATCH, DELETE, etc.)

[howto_patch_method.txt](../../cmd/muxt/testdata/howto_patch_method.txt)

## Status Codes

**Three formats:**

```gotmpl
{{define "POST /user 201 CreateUser(ctx, form)"}}{{end}}      <!-- Integer with method -->
{{define "GET /admin http.StatusUnauthorized"}}{{end}}        <!-- Constant -->
{{define "GET /error 400"}}{{end}}                             <!-- Integer -->
```

**Status code precedence** (first non-zero wins, highest to lowest):
1. Template `.StatusCode(int)` call
2. Error status: `400` on a parse/path/form error, `500` when the method returns a non-nil error
3. Result type `StatusCode()` method, else result type `StatusCode` field
4. Template-name code (shown above), else `200` — or `204` when the rendered body is empty

A returned error is always `500`; the error's own methods are not consulted. To return another code for a failure, set it in the template with `.StatusCode`. Use the template name for static codes (`201` for POST). Full precedence and examples: [Call Results](call-results.md#status-code-control).

[reference_status_codes.txt](../../cmd/muxt/testdata/reference_status_codes.txt)

## Call Expressions

### Syntax

```
MethodName(arg1, arg2, ...)
```

**No spaces allowed** between method name and parentheses. Arguments are comma-separated identifiers.

### Parameter Sources

| Parameter Name | Type | Source | Auto-parsed |
|----------------|------|--------|-------------|
| `ctx` | `context.Context` | `request.Context()` | N/A |
| `request` | `*http.Request` | Direct | N/A |
| `response` | `http.ResponseWriter` | Direct | N/A |
| `form` | struct or `url.Values` | `request.Form` (after `ParseForm`) | Yes |
| `multipart` | struct or `*multipart.Form` | `request.MultipartForm` (after `ParseMultipartForm`) | Yes |
| `execute` | `func(T) error` or `func() error` | render callback (see below) | N/A |
| `sse` | `func(T) error` or `func() error` | Server-Sent Events render callback (see below) | N/A |
| `lastEventID` | Any parseable | `request.Header.Get("Last-Event-Id")` | Yes |
| Path param | Any parseable | `request.PathValue(name)` | Yes |
| Form field | Any parseable | `request.Form.Get(name)` | Yes |

`form` and `multipart` are mutually exclusive in the same call site. Use
`form` for routes that handle `application/x-www-form-urlencoded` bodies and
`multipart` for routes that handle `multipart/form-data` (file uploads, etc.).
In struct-binding mode, `multipart` additionally supports
`*multipart.FileHeader` and `[]*multipart.FileHeader` fields, sourced from
`request.MultipartForm.File`. The default `maxMemory` for `ParseMultipartForm`
is 32 MiB; override with the `--output-multipart-max-memory=<size>` generator
flag (e.g. `64MB`, `128MiB`). Per `mime/multipart`'s standard semantics,
upload data exceeding `maxMemory` spills to the OS temp directory.

`execute` is the render callback. Instead of muxt rendering the template after
the method returns, muxt passes a closure into the method at `execute`'s
position and the method decides when (and whether) to render. The method param
must be `func(T) error` — `T` becomes `.Result` in the template — or
`func() error` (then `T` is `struct{}`). A method using `execute` must return
only `error`. Because the receiver controls when the closure runs, you can
render while holding a lock so the template observes a consistent snapshot of
state. If the callback is never invoked the response body is empty and muxt
returns `204 No Content`.

`sse` is the render callback for **Server-Sent Events**. Like `execute`, muxt
passes a closure into the method at `sse`'s position, but the handler first
establishes an event stream (`Content-Type: text/event-stream`, initial flush)
and the method may call the closure many times — once per event. The param must
be `func(T) error` (`T` becomes `.Result`) or `func() error`. Each call renders
the template into a pooled buffer and writes one SSE frame, then flushes. The
method returns nothing or only `error` (a returned error is logged and closes
the stream). `sse` is mutually exclusive with `execute` and `response`. The
template data is an `SSETemplateData` value: alongside `.Result`, `.Request`,
and `.Err` it exposes chainable `.Event`, `.ID`, and `.Retry` setters for the
SSE frame fields. When the method is not defined on the receiver, muxt
synthesizes the callback as `func(any) error`.

A route with a base `sse` argument may take **additional `sse`-prefixed
callbacks** — `Events(sse, sseClock, sseMetrics)`. Each gets its own closure in
the method call and renders a different template: the base `sse` renders the
route's own template, while a prefixed callback renders the template named
exactly after the argument (`sseClock` renders `{{define "sseClock"}}`). Those
templates must exist at generate time. Each callback has its own result type and
builds its own `SSETemplateData`, so they need not share a `T`.

[reference_sse_multiple_callbacks.txt](../../cmd/muxt/testdata/reference_sse_multiple_callbacks.txt)

The `execute` and `sse` callback parameter may be a **named or aliased func
type** — `type RenderFunc func(T) error` or `type RenderFunc = func(T) error` —
not only an inline `func(T) error`. Muxt resolves the underlying signature, so
`T` is read from it the same way.

[reference_callback_named_func_type.txt](../../cmd/muxt/testdata/reference_callback_named_func_type.txt)

`lastEventID` reads the `Last-Event-Id` request header — the value a browser
replays when reconnecting an SSE stream — and parses it to the method param type
like a path value (`string` by default, or any parseable type). A
`{lastEventID}` path wildcard takes precedence over the header.

**Parseable types:** `string`, `int*`, `uint*`, `bool`, `encoding.TextUnmarshaler`

### Examples

```gotmpl
{{define "GET /profile Profile(ctx)"}}{{end}}
{{define "POST /login Login(ctx, form)"}}{{end}}  <!-- Form fields -->
{{define "GET /user/{id} GetUser(ctx, id)"}}{{end}}  <!-- Path param -->
{{define "GET /user/{userID}/post/{postID} GetPost(ctx, userID, postID)"}}{{end}}  <!-- Multiple path params -->
{{define "POST /upload Upload(ctx, response, request)"}}{{end}}  <!-- HTTP primitives -->
{{define "GET /events Stream(ctx, lastEventID, sse)"}}{{end}}  <!-- Server-Sent Events -->
```

Parameter names in template must match method signature exactly. Case-sensitive.

[howto_call_method.txt](../../cmd/muxt/testdata/howto_call_method.txt) · [howto_call_with_multiple_args.txt](../../cmd/muxt/testdata/howto_call_with_multiple_args.txt) · [howto_arg_context.txt](../../cmd/muxt/testdata/howto_arg_context.txt) · [reference_sse.txt](../../cmd/muxt/testdata/reference_sse.txt) · [reference_last_event_id.txt](../../cmd/muxt/testdata/reference_last_event_id.txt)

## Host Matching

```gotmpl
{{define "api.example.com/v1/users ListUsers(ctx)"}}{{end}} <!-- Specific host -->
{{define "admin.example.com/ AdminHome(ctx)"}}{{end}}       <!-- Admin subdomain -->
{{define "example.com/{$} Home(ctx)"}}{{end}}               <!-- Exact host + path -->
```

Host patterns enable multi-tenant apps or API versioning by subdomain. Omit host to match all.

## Common Patterns

**REST resources:**
```gotmpl
{{define "GET /posts ListPosts(ctx)"}}{{end}}
{{define "POST /posts 201 CreatePost(ctx, form)"}}{{end}}
{{define "GET /posts/{id} GetPost(ctx, id)"}}{{end}}
{{define "PUT /posts/{id} UpdatePost(ctx, id, form)"}}{{end}}
{{define "DELETE /posts/{id} 204 DeletePost(ctx, id)"}}{{end}}
```

**Nested resources:**
```gotmpl
{{define "GET /users/{userID}/posts/{postID} GetUserPost(ctx, userID, postID)"}}{{end}}
```

**File serving:**
```gotmpl
{{define "GET /static/ ServeStatic(response, request)"}}{{end}}
```

**Wildcards for paths:**
```gotmpl
{{define "GET /files/{path...} ServeFile(ctx, path)"}}{{end}}  <!-- path captures "a/b/c.txt" -->
```

[reference_path_exact_match.txt](../../cmd/muxt/testdata/reference_path_exact_match.txt)

## Go 1.22+ ServeMux Behavior

Muxt uses `http.ServeMux` pattern matching ([docs](https://pkg.go.dev/net/http#hdr-Patterns-ServeMux)):

- `"/index.html"` — path only, any host/method
- `"GET /static/"` — method + path prefix
- `"example.com/"` — host + any path
- `"example.com/{$}"` — host + exact path "/"
- `"/b/{bucket}/o/{name...}"` — segments + wildcard

**Precedence:** Most specific pattern wins. `GET /posts/{id}` beats `/posts/{id}` beats `/{path...}`.

## Formal Grammar (BNF)

```bnf
<route>        ::= [<method> " "] [<host>] <path> [" " <status>] [" " <call>]
<method>       ::= "GET" | "POST" | "PUT" | "PATCH" | "DELETE"
<host>         ::= <hostname> | <ipv4>
<path>         ::= "/" [<segment> [<path>] ["/"]]
<status>       ::= <integer> | "http.Status" <identifier>
<call>         ::= <identifier> "(" [<identifier> {"," <identifier>}] ")"
<identifier>   ::= <letter> {<letter> | <digit> | "_"}
```

**Notes:** Path segments may include `{param}` or `{param...}`. Unreserved chars: `[a-zA-Z0-9-_.~]`.

## Test Files by Category

**Basics:**
- [tutorial_basic_route.txt](../../cmd/muxt/testdata/tutorial_basic_route.txt) — GET with no params
- [howto_path_param.txt](../../cmd/muxt/testdata/howto_path_param.txt) — Path parameters
- [howto_patch_method.txt](../../cmd/muxt/testdata/howto_patch_method.txt) — PATCH method

**Path patterns:**
- [reference_path_exact_match.txt](../../cmd/muxt/testdata/reference_path_exact_match.txt) — `/{$}` exact match
- [reference_path_prefix.txt](../../cmd/muxt/testdata/reference_path_prefix.txt) — Prefix matching

**Call expressions:**
- [howto_call_method.txt](../../cmd/muxt/testdata/howto_call_method.txt) — Basic call
- [howto_call_with_multiple_args.txt](../../cmd/muxt/testdata/howto_call_with_multiple_args.txt) — Multiple args
- [howto_arg_context.txt](../../cmd/muxt/testdata/howto_arg_context.txt) — `ctx` parameter
- [howto_arg_path_param.txt](../../cmd/muxt/testdata/howto_arg_path_param.txt) — Path param parsing

**Status codes:**
- [reference_status_codes.txt](../../cmd/muxt/testdata/reference_status_codes.txt) — Various status patterns

**Forms:**
- [howto_form_with_struct.txt](../../cmd/muxt/testdata/howto_form_with_struct.txt) — Struct form binding

**Complete apps:**
- [tutorial_blog_example.txt](../../cmd/muxt/testdata/tutorial_blog_example.txt) — Full blog application

**Error cases:**
- [err_duplicate_pattern.txt](../../cmd/muxt/testdata/err_duplicate_pattern.txt) — Duplicate route pattern

**Browse all:** [cmd/muxt/testdata/](../../cmd/muxt/testdata/)
