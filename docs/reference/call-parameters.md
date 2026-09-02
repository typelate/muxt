# Call Parameters Reference

Parameters in call expressions determine how Muxt generates handlers and parses request data. Use this reference when reviewing parameter bindings with team members.

## Parameter Binding Quick Reference

| Parameter Name | Type | Source | Parsed | Use When |
|----------------|------|--------|--------|----------|
| `ctx` | `context.Context` | `request.Context()` | N/A | Need request context (always recommended first param) |
| `request` | `*http.Request` | Direct | N/A | Need headers, cookies, or full request |
| `response` | `http.ResponseWriter` | Direct | N/A | Streaming, file downloads, custom headers |
| `form` | struct or `url.Values` | `request.Form` | Yes | Bind query parameters and, on POST/PUT/PATCH, the `application/x-www-form-urlencoded` body |
| `multipart` | struct or `*multipart.Form` | `request.MultipartForm` | Yes | Bind form fields with file uploads (`multipart/form-data`) |
| `execute` | `func(T) error` or `func() error` | render callback | N/A | Render under a lock or control when the template runs |
| `lastEventID` | Any parseable | `request.Header.Get("Last-Event-Id")` | Yes | Resume an SSE stream from the client's last event |
| `body` | `io.Reader` (exactly) | `request.Body` | No | Read the raw request body stream |
| `unmarshalJSON(body)` | Any JSON-unmarshalable | `request.Body` | Yes | Decode a JSON request body into a struct parameter |
| `unmarshalForm(body)` | struct or `url.Values` | `request.Form` | Yes | Explicit spelling of `form`; same binding |
| `signals` | Any JSON-unmarshalable | `request.Body` | Yes | Shorthand for `unmarshalJSON(body)`; requires `--output-datastar` |
| Path param | Any parseable | `request.PathValue(name)` | Yes | Extract from URL path |

These names (plus path parameters) are the only identifiers allowed as call
arguments — anything else fails generation with `unknown argument`. Individual
form fields cannot be passed as arguments; bind them through `form` or
`multipart`. Arguments bind to method parameters by position; the method's own
parameter names don't need to match.

[howto_arg_context.txt](../../cmd/muxt/testdata/howto_arg_context.txt) · [howto_arg_request.txt](../../cmd/muxt/testdata/howto_arg_request.txt) · [howto_arg_response.txt](../../cmd/muxt/testdata/howto_arg_response.txt)

## Type Resolution

**Without `--use-receiver-type`:** Path params are `string`, return types are `any`

```gotmpl
{{define "GET /user/{id} GetUser(ctx, id)"}}{{end}}
```
```go
type RoutesReceiver interface {
    GetUser(ctx context.Context, id string) any  // id: string, return: any
}
```

This allows you to stub out Go code while iterating in template source.

[howto_arg_no_receiver.txt](../../cmd/muxt/testdata/howto_arg_no_receiver.txt)

**With `--use-receiver-type=Server`:** Muxt looks up method signature, uses actual types

```go
func (s Server) GetUser(ctx context.Context, id int) (_ User, _ error) { return  }
```
```go
type RoutesReceiver interface {
    GetUser(ctx context.Context, id int) (User, error)  // id: int, return: (User, error)
}
```

Generated handler parses `id` from string to `int` automatically. Parse failures return 400 Bad Request.

Always use `--use-receiver-type` for production. Type safety prevents runtime errors.

[howto_call_with_path_param.txt](../../cmd/muxt/testdata/howto_call_with_path_param.txt)

## Parseable Types

Muxt auto-parses path and form parameters to these types:

| Type Category | Types | Parser | Notes |
|---------------|-------|--------|-------|
| **Integers** | `int`, `int8`, `int16`, `int32`, `int64` | `strconv.ParseInt` | Base 10 |
| **Unsigned** | `uint`, `uint8`, `uint16`, `uint32`, `uint64` | `strconv.ParseUint` | Base 10 |
| **Boolean** | `bool` | `strconv.ParseBool` | Accepts: `1`, `t`, `T`, `true`, `True`, `TRUE` and the `0`/`f`/`false` equivalents |
| **String** | `string` | None | Passed through |
| **Custom** | Implements `encoding.TextUnmarshaler` | `UnmarshalText()` | Define custom parsing |

**Parse failures:** Return 400 Bad Request automatically.

[reference_path_with_typed_param.txt](../../cmd/muxt/testdata/reference_path_with_typed_param.txt)

**Custom parsing example:**
```go
type UserID string

func (id *UserID) UnmarshalText(text []byte) error {
    *id = UserID(strings.ToLower(string(text)))
    return nil
}
```

[howto_arg_with_text_unmarshaler.txt](../../cmd/muxt/testdata/howto_arg_with_text_unmarshaler.txt)

## Form Parameters

**Generic url.Values for fields:**
```gotmpl
{{define "POST /login Login(ctx, form)"}}{{end}}
```
```go
func (s Server) Login(ctx context.Context, form url.Values) (Session, error) {
    // username, password from request.Form.Get("username"), request.Form.Get("password")
}
```

[howto_form_basic.txt](../../cmd/muxt/testdata/howto_form_basic.txt)

**Struct binding:**
```gotmpl
{{define "POST /login Login(ctx, form)"}}{{end}}
```
```go
type LoginForm struct {
    Username string
    Password string
    Remember bool
}

func (s Server) Login(ctx context.Context, form LoginForm) (Session, error) {
    // All fields populated from request.Form
}
```

**Struct tags for field mapping:**
```go
type LoginForm struct {
    Username string `name:"user-name"`  // Maps to form field "user-name"
    Password string `name:"user-pass"`  // Maps to form field "user-pass"
}
```

Struct field names must match form field names exactly (case-sensitive) unless using the `name` tag.

[howto_form_with_struct.txt](../../cmd/muxt/testdata/howto_form_with_struct.txt) · [howto_form_with_field_tag.txt](../../cmd/muxt/testdata/howto_form_with_field_tag.txt)

## Multipart Parameters

Use `multipart` instead of `form` when the request body is `multipart/form-data` — required for `<input type="file">` uploads. Muxt calls `request.ParseMultipartForm` and binds both text fields and file fields.

`form` and `multipart` are **mutually exclusive** in the same call — `ParseMultipartForm` populates `request.PostForm`, so `multipart` is a strict superset of `form` for routes that accept multipart bodies.

**Struct binding with file fields:**
```gotmpl
{{define "POST /upload 201 Upload(ctx, multipart)"}}{{end}}
```
```go
import "mime/multipart"

type UploadForm struct {
    Title  string                  `name:"title"`
    Tags   []string                `name:"tag"`
    Avatar *multipart.FileHeader   `name:"avatar"`  // single file
    Photos []*multipart.FileHeader `name:"photos"`  // multiple files for the same name
}

func (s Server) Upload(ctx context.Context, form UploadForm) (Result, error) {
    f, err := form.Avatar.Open()
    if err != nil { return Result{}, err }
    defer f.Close()
    // ... read and store the file ...
}
```

[howto_multipart_file_upload.txt](../../cmd/muxt/testdata/howto_multipart_file_upload.txt) · [reference_multipart_basic.txt](../../cmd/muxt/testdata/reference_multipart_basic.txt) · [reference_multipart_multiple_files.txt](../../cmd/muxt/testdata/reference_multipart_multiple_files.txt) · [reference_multipart_mixed.txt](../../cmd/muxt/testdata/reference_multipart_mixed.txt)

**Raw `*multipart.Form` access:**
```gotmpl
{{define "POST /upload Upload(ctx, multipart)"}}{{end}}
```
```go
func (s Server) Upload(ctx context.Context, form *multipart.Form) error {
    for name, files := range form.File { ... }
    return nil
}
```

[reference_multipart_raw.txt](../../cmd/muxt/testdata/reference_multipart_raw.txt)

**Max upload size:** Defaults to 32 MiB. Override with `--output-multipart-max-memory=<size>` (e.g. `64MB`, `128MiB`). Data exceeding this limit spills to the OS temp directory per the standard `mime/multipart` semantics.

**Parse errors:** A malformed multipart body sets `.Err` and responds `400 Bad Request` (unlike `form`, which silently ignores body parse errors).

[reference_multipart_max_memory_flag.txt](../../cmd/muxt/testdata/reference_multipart_max_memory_flag.txt) · [reference_multipart_parse_error.txt](../../cmd/muxt/testdata/reference_multipart_parse_error.txt)

## Request Body

The request body is a **single-use stream**. `body` and `unmarshalJSON(body)`
each read it directly; `form`, `multipart`, and `unmarshalForm(body)` parse it
once through `request.ParseForm` / `request.ParseMultipartForm` (repeats reuse
the cached result). A call that reads the body more than once — for example
`(form, body)` or `(multipart, unmarshalJSON(body))` — fails generation.

### `body`

Binds `request.Body` as an `io.Reader`. The method parameter must be exactly
`io.Reader`; any other type fails generation. Use it for payloads the handler
must not reinterpret (webhooks, proxied uploads). Like the other reserved
names, `body` cannot be used as a path wildcard name.

```gotmpl
{{define "POST /hooks Save(ctx, body)"}}{{.Result}}{{end}}
```
```go
func (s Server) Save(ctx context.Context, body io.Reader) (string, error)
```

[reference_body_reader.txt](../../cmd/muxt/testdata/reference_body_reader.txt) · [err_body_not_reader.txt](../../cmd/muxt/testdata/err_body_not_reader.txt) · [err_body_consumed_twice.txt](../../cmd/muxt/testdata/err_body_consumed_twice.txt) · [err_form_and_body.txt](../../cmd/muxt/testdata/err_form_and_body.txt)

### `unmarshalJSON(body)`

Decodes the JSON request body into the Go type of the method parameter at that
position. The wrapper's only valid argument is `body`. The decode target comes
from the receiver method signature, so `muxt check` verifies it.

```gotmpl
{{define "POST /users CreateUser(ctx, unmarshalJSON(body))"}}{{.Result.Name}}{{end}}
```
```go
func (s Server) CreateUser(ctx context.Context, u User) (User, error)
```

- A malformed (or empty) body responds 400 Bad Request and the method is not
  called. The wrapper does not check the request `Content-Type`.
- Decodes with `encoding/json`.
- If the receiver method is not yet defined, the parameter synthesizes as
  `json.RawMessage` so template-first iteration passes the raw payload through.

Under `--output-datastar` the reserved `signals` argument is shorthand for
`unmarshalJSON(body)` — Datastar sends the page's signal state as the JSON
request body, so both spellings bind identically. Without the flag, `signals`
fails generation with an error naming the shorthand.

[reference_unmarshal_json.txt](../../cmd/muxt/testdata/reference_unmarshal_json.txt) · [reference_unmarshal_json_undefined.txt](../../cmd/muxt/testdata/reference_unmarshal_json_undefined.txt) · [err_unmarshal_json_bad_arg.txt](../../cmd/muxt/testdata/err_unmarshal_json_bad_arg.txt) · [reference_output_datastar_signals.txt](../../cmd/muxt/testdata/reference_output_datastar_signals.txt) · [err_signals_without_datastar.txt](../../cmd/muxt/testdata/err_signals_without_datastar.txt)

### `unmarshalForm(body)`

The explicit spelling of the existing `form` binding — **not** a separate body
decoder. Both spellings generate the identical `request.Form` binding
(`request.ParseForm` semantics, URL query merge included), so behavior is the
same by construction. On GET the bound values are the query string and the
`(body)` spelling is misleading; prefer `form` there.

[reference_unmarshal_form.txt](../../cmd/muxt/testdata/reference_unmarshal_form.txt) · [reference_form_equals_unmarshal_form.txt](../../cmd/muxt/testdata/reference_form_equals_unmarshal_form.txt)

## Server-Sent Events

Wrapping the method call in `sse(...)` makes the route stream [Server-Sent Events](https://developer.mozilla.org/docs/Web/API/Server-sent_events). The handler sets the event-stream headers, flushes, then calls your method with a render callback at the `execute` argument's position. The method calls the callback once per event; each call renders the template into a fresh frame and flushes it.

```gotmpl
{{define "GET /clock sse(Clock(ctx, execute))"}}{{.Result}}{{end}}
```
```go
func (s Server) Clock(ctx context.Context, execute func(string) error) {
    t := time.NewTicker(time.Second)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case now := <-t.C:
            if err := execute(now.Format(time.RFC3339)); err != nil {
                return // client disconnected
            }
        }
    }
}
```

| Rule | Detail |
|------|--------|
| Callback shape | `func(T) error` (`T` is `.Result`) or `func() error` |
| Method results | Nothing, or only `error` (a returned error is logged; the stream closes) |
| Not allowed | a `response` argument |
| Frame fields | `SSETemplateData` adds chainable `.Event`, `.ID`, `.Retry` setters alongside `.Result`, `.Request`, `.Err` |
| Under `--output-datastar` | `.Event` is replaced by the fixed `datastar-patch-elements` event name; `.Selector`, `.Mode`, and `.UseViewTransition` setters become the patch option lines. A `Signals`-suffixed callback (`countsSignals func(T) error`) marshals its argument as a `datastar-patch-signals` event |
| Response headers | `Content-Type: text/event-stream`, `Cache-Control: no-store`, `Connection: keep-alive` |
| Undefined method | Synthesized as `func(any) error` |
| Extra callbacks | `sse`-prefixed arguments (`sse(Events(sseClock, execute, sseMetrics))`) each render the same-named template |

Pair the wrapper with `lastEventID` to resume after a reconnect. `lastEventID` reads the `Last-Event-Id` header and parses it like a path value (defaults to `string`); a typed parse failure returns 400 before the stream opens.

```gotmpl
{{define "GET /events sse(Stream(ctx, lastEventID, execute))"}}{{.Result}}{{end}}
```

[reference_sse.txt](../../cmd/muxt/testdata/reference_sse.txt) · [reference_sse_no_arg.txt](../../cmd/muxt/testdata/reference_sse_no_arg.txt) · [reference_sse_error_return.txt](../../cmd/muxt/testdata/reference_sse_error_return.txt) · [reference_sse_multiple_callbacks.txt](../../cmd/muxt/testdata/reference_sse_multiple_callbacks.txt) · [reference_last_event_id.txt](../../cmd/muxt/testdata/reference_last_event_id.txt)

## Advanced Patterns

**Mixing path, form, and special parameters:**
```gotmpl
{{define "POST /user/{id}/update UpdateUser(ctx, id, form)"}}{{end}}
```
```go
func (s Server) UpdateUser(ctx context.Context, id int, form UpdateUserForm) error {
    // id from path, form fields from request body, ctx from request context
}
```

**Pointer receivers (both work):**
```go
func (s Server) GetUser(ctx context.Context, id int) (User, error)   // Value
func (s *Server) GetUser(ctx context.Context, id int) (User, error)  // Pointer
```

[reference_receiver_with_pointer.txt](../../cmd/muxt/testdata/reference_receiver_with_pointer.txt)

**Embedded fields (method promotion):**
```go
type Auth struct{}
func (Auth) Login(ctx context.Context, username, password string) (Session, error)

type Server struct {
    Auth  // Login promoted to Server
}
```

[reference_receiver_with_embedded_method.txt](../../cmd/muxt/testdata/reference_receiver_with_embedded_method.txt)

## Validation and Error Handling

**Muxt handles type parsing. Your methods handle validation:**

```go
type CreateUserForm struct {
    Email    string
    Password string
}

func (s Server) CreateUser(ctx context.Context, form CreateUserForm) (User, error) {
    if !isValidEmail(form.Email) {
        return User{}, errors.New("invalid email")
    }
    if len(form.Password) < 8 {
        return User{}, errors.New("password too short")
    }
    // ...
}
```

**Parse errors return 400 automatically:**
- Request to `/user/abc` with `GetUser(ctx, id int)` → 400 Bad Request
- Form field "age=xyz" bound to an `Age int` form struct field → 400 Bad Request

Validation errors should return from your method. Display them in templates with `{{if .Err}}`.

[reference_path_with_typed_param.txt](../../cmd/muxt/testdata/reference_path_with_typed_param.txt)

## Test Files

Every binding above links its test archives inline; browse
[cmd/muxt/testdata](../../cmd/muxt/testdata) for the full catalog —
`howto_*` for task-oriented examples, `reference_*` for feature
documentation, and `err_*` for pinned error messages.
