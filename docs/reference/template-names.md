# Template Name Syntax Reference

Complete specification for Muxt template naming. Use when pair programming to validate route definitions.

## Syntax

```
[METHOD ][HOST]/PATH[ HTTP_STATUS][ CALL]
```

**All components:**
```gotemplate
{{define "GET example.com/greet/{language} 200 Greeting(ctx, language)"}}{{end}}
```

**Minimal (path only):**
```gotemplate
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

```gotemplate
{{define "GET /"}}{{end}}              <!-- Root -->
{{define "GET /about"}}{{end}}         <!-- Static path -->
{{define "GET /user/{id}"}}{{end}}     <!-- Path parameter -->
{{define "GET /user/{id}/post/{postID}"}}{{end}}  <!-- Multiple parameters -->
```

[simple_get.txt](../../cmd/muxt/testdata/simple_get.txt) · [path_param.txt](../../cmd/muxt/testdata/path_param.txt)

### Path Matching Modes

```gotemplate
{{define "GET /{$}"}}{{end}}           <!-- Exact: "/" only, not "/foo" -->
{{define "GET /static/"}}{{end}}       <!-- Prefix: "/static/", "/static/foo", "/static/foo/bar" -->
{{define "GET /files/{path...}"}}{{end}}  <!-- Wildcard: captures "/files/a/b/c" as path="a/b/c" -->
```

**Note:** `/{$}` vs `/` behavior differs. Former is exact match, latter matches prefix.

[path_end.txt](../../cmd/muxt/testdata/path_end.txt) · [path_prefix.txt](../../cmd/muxt/testdata/path_prefix.txt)

### Path Parameters Are Type-Safe

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}{{end}}
```

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) {
    // id auto-parsed from string → int
    // Parse failure → 400 Bad Request (automatic)
}
```

**Note:** Path param names must match method param names exactly. Type conversion is automatic based on method signature.

[argument_path_param.txt](../../cmd/muxt/testdata/argument_path_param.txt)

## HTTP Methods

```gotemplate
{{define "GET /posts"}}{{end}}          <!-- Read -->
{{define "POST /posts"}}{{end}}         <!-- Create -->
{{define "PUT /posts/{id}"}}{{end}}     <!-- Replace -->
{{define "PATCH /posts/{id}"}}{{end}}   <!-- Update -->
{{define "DELETE /posts/{id}"}}{{end}}  <!-- Delete -->
```

**Without method prefix:** Matches all methods (GET, POST, PUT, PATCH, DELETE, etc.)

[simple_patch.txt](../../cmd/muxt/testdata/simple_patch.txt)

## Status Codes

**Three formats:**

```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}{{end}}      <!-- Integer with method -->
{{define "GET /admin http.StatusUnauthorized"}}{{end}}        <!-- Constant -->
{{define "GET /error 400"}}{{end}}                             <!-- Integer -->
```

**Status code precedence:**
1. Template name (shown above)
2. Result type with `StatusCode()` method
3. Result type with `StatusCode` field
4. Error with `StatusCode()` method
5. Template `.StatusCode(int)` call
6. Default (200 for success, 500 for errors)

Use template name for static codes (201 for POST), methods for dynamic codes (404 from errors).

[status_codes.txt](../../cmd/muxt/testdata/status_codes.txt)

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
| `form` | struct or `url.Values` | `request.Form` | Yes |
| Path param | Any parseable | `request.PathValue(name)` | Yes |
| Form field | Any parseable | `request.Form.Get(name)` | Yes |

**Parseable types:** `string`, `int*`, `uint*`, `bool`, `encoding.TextUnmarshaler`

### Examples

```gotemplate
{{define "GET /profile Profile(ctx)"}}{{end}}
{{define "POST /login Login(ctx, form)"}}{{end}}  <!-- Form fields -->
{{define "GET /user/{id} GetUser(ctx, id)"}}{{end}}  <!-- Path param -->
{{define "GET /user/{userID}/post/{postID} GetPost(ctx, userID, postID)"}}{{end}}  <!-- Multiple path params -->
{{define "POST /upload Upload(ctx, response, request)"}}{{end}}  <!-- HTTP primitives -->
```

Parameter names in template must match method signature exactly. Case-sensitive.

[call_F.txt](../../cmd/muxt/testdata/call_F.txt) · [call_F_with_multiple_arguments.txt](../../cmd/muxt/testdata/call_F_with_multiple_arguments.txt) · [argument_context.txt](../../cmd/muxt/testdata/argument_context.txt)

## Host Matching

```gotemplate
{{define "api.example.com/v1/users ListUsers(ctx)"}}{{end}} <!-- Specific host -->
{{define "admin.example.com/ AdminHome(ctx)"}}{{end}}       <!-- Admin subdomain -->
{{define "example.com/{$} Home(ctx)"}}{{end}}               <!-- Exact host + path -->
```

Host patterns enable multi-tenant apps or API versioning by subdomain. Omit host to match all.

## Common Patterns

**REST resources:**
```gotemplate
{{define "GET /posts ListPosts(ctx)"}}{{end}}
{{define "POST /posts 201 CreatePost(ctx, form)"}}{{end}}
{{define "GET /posts/{id} GetPost(ctx, id)"}}{{end}}
{{define "PUT /posts/{id} UpdatePost(ctx, id, form)"}}{{end}}
{{define "DELETE /posts/{id} 204 DeletePost(ctx, id)"}}{{end}}
```

**Nested resources:**
```gotemplate
{{define "GET /users/{userID}/posts/{postID} GetUserPost(ctx, userID, postID)"}}{{end}}
```

**File serving:**
```gotemplate
{{define "GET /static/ ServeStatic(response, request)"}}{{end}}
```

**Wildcards for paths:**
```gotemplate
{{define "GET /files/{path...} ServeFile(ctx, path)"}}{{end}}  <!-- path captures "a/b/c.txt" -->
```

[path_wildcard.txt](../../cmd/muxt/testdata/path_wildcard.txt)

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
<status>       ::= <integer> | <identifier> "." <identifier>
<call>         ::= <identifier> "(" [<identifier> {"," <identifier>}] ")"
<identifier>   ::= <letter> {<letter> | <digit> | "_"}
```

**Notes:** Path segments may include `{param}` or `{param...}`. Unreserved chars: `[a-zA-Z0-9-_.~]`.

## Test Files by Category

**Basics:**
- [simple_get.txt](../../cmd/muxt/testdata/simple_get.txt) — GET with no params
- [path_param.txt](../../cmd/muxt/testdata/path_param.txt) — Path parameters
- [simple_patch.txt](../../cmd/muxt/testdata/simple_patch.txt) — PATCH method

**Path patterns:**
- [path_end.txt](../../cmd/muxt/testdata/path_end.txt) — `/{$}` exact match
- [path_prefix.txt](../../cmd/muxt/testdata/path_prefix.txt) — Prefix matching
- [path_wildcard.txt](../../cmd/muxt/testdata/path_wildcard.txt) — Wildcard `{...}`

**Call expressions:**
- [call_F.txt](../../cmd/muxt/testdata/call_F.txt) — Basic call
- [call_F_with_multiple_arguments.txt](../../cmd/muxt/testdata/call_F_with_multiple_arguments.txt) — Multiple args
- [argument_context.txt](../../cmd/muxt/testdata/argument_context.txt) — `ctx` parameter
- [argument_path_param.txt](../../cmd/muxt/testdata/argument_path_param.txt) — Path param parsing

**Status codes:**
- [status_codes.txt](../../cmd/muxt/testdata/status_codes.txt) — Various status patterns

**Forms:**
- [F_is_defined_and_form_type_is_a_struct.txt](../../cmd/muxt/testdata/F_is_defined_and_form_type_is_a_struct.txt) — Struct form binding

**Complete apps:**
- [blog.txt](../../cmd/muxt/testdata/blog.txt) — Full blog application

**Error cases:**
- [error_wrong_argument_type.txt](../../cmd/muxt/testdata/error_wrong_argument_type.txt) — Type mismatch

**Browse all:** [cmd/muxt/testdata/](../../cmd/muxt/testdata/)
