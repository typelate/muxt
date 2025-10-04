# Naming Templates

`muxt generate` will read your HTML templates and generate/register an [`http.HandlerFunc`](https://pkg.go.dev/net/http#HandlerFunc) for each template with a name that matches an expected pattern.

If a template name does not match an expected pattern, the template is ignored by `muxt`.

Since Go 1.22, the standard library route **mu**ltiple**x**er can parse path parameters.

It expects strings like this:

`[METHOD ][HOST]/[PATH]`

Muxt extends this by adding optional fields for the status code and a method call:

`[METHOD ][HOST]/[PATH ][HTTP_STATUS ][CALL]`

A template name pattern that `muxt` understands looks like this:

```gotemplate
{{define "GET /greet/{language} 200 Greeting(ctx, language)" }}
<h1>{{.Hello}}</h1>
{{end}}
```

## Basic Examples

### Simple GET Request

```gotemplate
{{define "GET /"}}
<h1>Hello, world!</h1>
{{end}}
```

*[(See Muxt CLI Test/simple_get)](../../cmd/muxt/testdata/simple_get.txt)*

### Path Parameters

```gotemplate
{{define "GET /user/{id}"}}
<h1>User ID: {{.Result.ID}}</h1>
{{end}}
```

*[(See Muxt CLI Test/path_param)](../../cmd/muxt/testdata/path_param.txt)*

### Typed Path Parameters

When using `--receiver-type`, path parameters are automatically parsed to match method signatures:

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
<h1>{{.Result.Title}}</h1>
{{end}}
```

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) {
	// id is automatically parsed from string to int
}
```

*[(See Muxt CLI Test/path_param_typed)](../../cmd/muxt/testdata/path_param_typed.txt)*

### HTTP Methods

```gotemplate
{{define "GET /posts"}}...{{end}}
{{define "POST /posts"}}...{{end}}
{{define "PATCH /posts/{id}"}}...{{end}}
{{define "DELETE /posts/{id}"}}...{{end}}
```

*[(See Muxt CLI Test/simple_patch)](../../cmd/muxt/testdata/simple_patch.txt)*

### Path Endings

Use `{$}` to match exact paths:

```gotemplate
{{define "GET /{$}"}}
<!-- Matches only "/" exactly -->
{{end}}
```

*[(See Muxt CLI Test/path_end)](../../cmd/muxt/testdata/path_end.txt)*

Use trailing slash for prefix matching:

```gotemplate
{{define "GET /static/"}}
<!-- Matches "/static/" and all sub-paths -->
{{end}}
```

*[(See Muxt CLI Test/path_prefix)](../../cmd/muxt/testdata/path_prefix.txt)*

## Status Codes

### Integer Status Codes

```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}
<!-- Returns 201 Created on success -->
{{end}}

{{define "GET /admin 401"}}
<!-- Returns 401 Unauthorized -->
{{end}}
```

### Constant Status Codes

```gotemplate
{{define "GET /error http.StatusBadRequest"}}
<!-- Returns 400 Bad Request -->
{{end}}
```

*[(See Muxt CLI Test/status_codes)](../../cmd/muxt/testdata/status_codes.txt)*

## Call Expressions

### Basic Call

```gotemplate
{{define "GET /profile Profile(ctx)"}}
<h1>{{.Result.Name}}</h1>
{{end}}
```

```go
func (s Server) Profile(ctx context.Context) (UserProfile, error) {
	// ...
}
```

*[(See Muxt CLI Test/call_F)](../../cmd/muxt/testdata/call_F.txt)*

### Multiple Arguments

```gotemplate
{{define "POST /login Login(ctx, username, password)"}}
<!-- ... -->
{{end}}
```

```go
func (s Server) Login(ctx context.Context, username, password string) (Session, error) {
	// username and password parsed from form fields
}
```

*[(See Muxt CLI Test/call_F_with_multiple_arguments)](../../cmd/muxt/testdata/call_F_with_multiple_arguments.txt)*

### Path Parameters in Calls

```gotemplate
{{define "GET /user/{userID}/post/{postID} GetPost(ctx, userID, postID)"}}
```

```go
func (s Server) GetPost(ctx context.Context, userID, postID int) (Post, error) {
	// userID and postID parsed from URL path
}
```

*[(See Muxt CLI Test/call_F_with_argument_path_param)](../../cmd/muxt/testdata/call_F_with_argument_path_param.txt)*

### Special Arguments

Muxt recognizes special argument names:

- `ctx` → `context.Context` from `request.Context()`
- `request` → `*http.Request`
- `response` → `http.ResponseWriter`
- `form` → `url.Values` from `request.Form`

```gotemplate
{{define "POST /upload Upload(ctx, response, request)"}}
```

```go
func (s Server) Upload(ctx context.Context, response http.ResponseWriter, request *http.Request) error {
	// Full access to HTTP primitives when needed
}
```

*[(See Muxt CLI Test/argument_context)](../../cmd/muxt/testdata/argument_context.txt)*
*[(See Muxt CLI Test/argument_request)](../../cmd/muxt/testdata/argument_request.txt)*
*[(See Muxt CLI Test/argument_response)](../../cmd/muxt/testdata/argument_response.txt)*

## [`*http.ServeMux`](https://pkg.go.dev/net/http#ServeMux) Patterns

Here is an excerpt from [the standard library documentation](https://pkg.go.dev/net/http#hdr-Patterns-ServeMux):

> Patterns can match the method, host and path of a request. Some examples:
> - "/index.html" matches the path "/index.html" for any host and method.
> - "GET /static/" matches a GET request whose path begins with "/static/".
> - "example.com/" matches any request to the host "example.com".
> - "example.com/{$}" matches requests with host "example.com" and path "/".
> - "/b/{bucket}/o/{objectname...}" matches paths whose first segment is "b" and whose third segment is "o". The name "bucket" denotes the second segment and "objectname" denotes the remainder of the path.

## Template Name Specification

```bnf
<route> ::= [ <method> <space> ] [ <host> ] <path> [ <space> <http_status> ] [ <space> <call_expr> ]

<method> ::= "GET" | "POST" | "PUT" | "PATCH" | "DELETE"

<host> ::= <hostname> | <ip_address>

<hostname> ::= <label> { "." <label> }
<label> ::= <letter> { <letter> | <digit> | "-" }
<ip_address> ::= <digit>+ "." <digit>+ "." <digit>+ "." <digit>+

<path> ::= "/" [ <path_segment> { "/" <path_segment> } [ "/" ] ]
<path_segment> ::= <unreserved_characters>+

<http_status> ::= <integer> | <qualified_identifier>
<integer> ::= <digit> { <digit> }
<qualified_identifier> ::= <identifier> "." <identifier>

<call_expr> ::= <identifier> "(" [ <identifier> { "," <identifier> } ] ")"

<identifier> ::= <letter> { <letter> | <digit> | "_" }

<space> ::= " "

<letter> ::= "a" | ... | "z" | "A" | ... | "Z"
<digit> ::= "0" | ... | "9"
<unreserved_characters> ::= <letter> | <digit> | "-" | "_" | "." | "~"
```

## More Examples

Browse the complete test suite for more examples:

- [All CLI tests](../../cmd/muxt/testdata/)
- [Blog example](../../cmd/muxt/testdata/blog.txt) - Complete application
- [Form handling](../../cmd/muxt/testdata/F_is_defined_and_form_type_is_a_struct.txt)
- [Error handling](../../cmd/muxt/testdata/error_wrong_argument_type.txt)
