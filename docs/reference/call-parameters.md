# Call Parameters Reference

Parameters in call expressions determine how Muxt generates handlers and parses request data. Use this reference when reviewing parameter bindings with team members.

## Parameter Binding Quick Reference

| Parameter Name | Type | Source | Parsed | Use When |
|----------------|------|--------|--------|----------|
| `ctx` | `context.Context` | `request.Context()` | N/A | Need request context (always recommended first param) |
| `request` | `*http.Request` | Direct | N/A | Need headers, cookies, or full request |
| `response` | `http.ResponseWriter` | Direct | N/A | Streaming, file downloads, custom headers |
| `form` | struct or `url.Values` | `request.Form` | Yes | Bind all form fields at once (`application/x-www-form-urlencoded`) |
| `multipart` | struct or `*multipart.Form` | `request.MultipartForm` | Yes | Bind form fields with file uploads (`multipart/form-data`) |
| `execute` | `func(T) error` or `func() error` | render callback | N/A | Render under a lock or control when the template runs |
| `send` | `func(T) error` or `func() error` | render callback (streaming) | N/A | Stream one SSE event per call, rendering the route's define body (inside `sse(...)`) |
| `sendX` | `func(T) error` | render callback (streaming) | N/A | Stream one SSE event per call, rendering the `X`-named template (inside `sse(...)`) |
| `elements` | `func(T) error` or `func() error` | render callback (streaming) | N/A | Stream Datastar `datastar-patch-elements` events (requires `--use-datastar`) |
| `signal` | `func(T, bool) error` | marshal callback | N/A | Emit Datastar `datastar-patch-signals` JSON (requires `--use-datastar`) |
| `script` | `func(T) error` or `func() error` | render callback | N/A | Respond `text/javascript` from the same-named template (requires `--use-datastar`) |
| `lastEventID` | Any parseable | `request.Header.Get("Last-Event-Id")` | Yes | Resume an SSE stream from the client's last event |
| `body` | `io.Reader` | `request.Body` | N/A | Read/decode the raw request body |
| Path param | Any parseable | `request.PathValue(name)` | Yes | Extract from URL path |
| Form field | Any parseable | `request.Form.Get(name)` | Yes | Individual form field |

Parameter names in template call must match method signature exactly.

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
| **Boolean** | `bool` | `strconv.ParseBool` | Accepts: `1`/`t`/`true`, `0`/`f`/`false` (case-insensitive) |
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

**Parse errors:** Malformed multipart bodies are captured into `td.errList` with `td.errStatusCode = http.StatusBadRequest` (unlike `form`, which silently ignores parse errors).

[reference_multipart_max_memory_flag.txt](../../cmd/muxt/testdata/reference_multipart_max_memory_flag.txt) · [reference_multipart_parse_error.txt](../../cmd/muxt/testdata/reference_multipart_parse_error.txt)

## Request Body

`body` binds the raw request body as an `io.Reader` so your method reads or decodes it directly.

```gotmpl
{{define "POST /raw Save(ctx, body)"}}{{.Result}}{{end}}
```
```go
func (s Server) Save(ctx context.Context, body io.Reader) (Result, error) {
    data, err := io.ReadAll(body)
    // ...
}
```

`body` is a single-use stream — read it once.

[reference_body_reader.txt](../../cmd/muxt/testdata/reference_body_reader.txt) · [reference_empty_body_status.txt](../../cmd/muxt/testdata/reference_empty_body_status.txt)

### Decode wrappers

Two wrappers decode `body` into the method parameter's type before the call. Their single argument must be the `body` identifier — anything else is an error.

```gotmpl
{{define "POST /users CreateUser(ctx, unmarshalJSON(body))"}}{{.Result}}{{end}}
```
```go
func (s Server) CreateUser(ctx context.Context, u User) (User, error) {
    // u decoded from the JSON request body
}
```

**`unmarshalJSON(body)`** — decodes the JSON request body into the parameter type.

- Uses `encoding/json` by default; under `--output-jsonv2` it uses `encoding/json/v2`'s `UnmarshalRead`.
- When the method is undefined, the parameter is a raw pass-through: `json.RawMessage` (or `*jsontext.Decoder` under `--output-jsonv2`).
- Decode errors respond 400 Bad Request.

**`unmarshalForm(body)`** — decodes the form-encoded body into the parameter (a struct via `name:"..."` tags, or `net/url.Values`). The `form` argument on non-GET methods is sugar for `unmarshalForm(body)`.

`body` is a single-use stream, so a wrapper consumes it — don't also take `body` (or another wrapper) in the same call.

[reference_unmarshal_json.txt](../../cmd/muxt/testdata/reference_unmarshal_json.txt) · [reference_unmarshal_json_jsonv2.txt](../../cmd/muxt/testdata/reference_unmarshal_json_jsonv2.txt) · [reference_unmarshal_json_undefined.txt](../../cmd/muxt/testdata/reference_unmarshal_json_undefined.txt) · [reference_unmarshal_json_undefined_jsonv2.txt](../../cmd/muxt/testdata/reference_unmarshal_json_undefined_jsonv2.txt) · [reference_unmarshal_form.txt](../../cmd/muxt/testdata/reference_unmarshal_form.txt) · [reference_form_equals_unmarshal_form.txt](../../cmd/muxt/testdata/reference_form_equals_unmarshal_form.txt) · [err_unmarshal_json_bad_arg.txt](../../cmd/muxt/testdata/err_unmarshal_json_bad_arg.txt)

## Response Representation Wrappers

A call at the end of a template name may be wrapped in a **representation wrapper** at the outermost position. The wrapper changes how the response is encoded without affecting parameter binding inside the call.

The grammar is:
```
frame( representation( Method(args…) ) )
```

where both `frame` (e.g. `htmx(...)`, `datastar(...)`) and `representation` are optional.

### sse(...)

`sse(...)` makes the route stream [Server-Sent Events](https://developer.mozilla.org/docs/Web/API/Server-sent_events). The handler sets `Content-Type: text/event-stream` headers, flushes, and then drives the stream. Two mutually exclusive modes are supported: **callback mode** and **return mode**.

#### Callback mode — send / sendX

The wrapped method takes render callbacks and returns `error` or nothing. One event is emitted per callback invocation.

**`send`** — renders the route's own define body:

```gotmpl
{{define "GET /events sse(Stream(ctx, lastEventID, send))"}}{{.Result}}{{end}}
```
```go
func (s Server) Stream(ctx context.Context, lastEventID string, send func(data string) error) {
    _ = send("hello-" + lastEventID)
}
```

**`sendX`** (camelCase name after `send`) — renders the template named by the remainder (e.g. `sendClock` → template `Clock`; `sendMetrics` → `Metrics`). The referenced template must exist or generation fails. This lets one stream emit multiple named templates:

```gotmpl
{{define "GET /events sse(Stream(ctx, send, sendClock))"}}body:{{.Result}}{{end}}
{{define "Clock"}}clock:{{.Result}}{{end}}
```
```go
func (s Server) Stream(ctx context.Context, send func(data string) error, sendClock func(data string) error) {
    _ = send("hi")
    _ = sendClock("tick")
}
```

A `send` callback with no argument renders the define body without a `.Result` value:

```gotmpl
{{define "GET /ping sse(Ping(send))"}}pong{{end}}
```
```go
func (s Server) Ping(send func() error) {
    _ = send()
}
```

**`marshalJSON(sendX)`** — wraps a send callback so it marshals its argument as JSON event data instead of rendering a template (uses `encoding/json`; `encoding/json/v2` `MarshalWrite` under `--output-jsonv2`):

```gotmpl
{{define "GET /events sse(Stream(ctx, marshalJSON(sendStatus)))"}}{{end}}
```
```go
type Status struct {
    OK bool `json:"ok"`
}

func (s Server) Stream(ctx context.Context, sendStatus func(Status) error) {
    _ = sendStatus(Status{OK: true})
    // event body: data: {"ok":true}
}
```

| Rule | Detail |
|------|--------|
| `send` callback shape | `func(T) error` (`T` is `.Result`) or `func() error` |
| `sendX` callback shape | `func(T) error` |
| Method results | Nothing, or only `error` (a returned error is logged; the stream closes) |
| Mutually exclusive with | return mode (see below); cannot mix callbacks and iterator/channel returns |
| Frame fields | `SSETemplateData` adds chainable `.Event`, `.ID`, `.Retry` setters alongside `.Result`, `.Request`, `.Err` |
| Undefined method | Synthesized with `send` as `func(any) error` |

Pair `send` with `lastEventID` to resume after a reconnect. `lastEventID` reads the `Last-Event-Id` header and parses it like a path value (defaults to `string`); a typed parse failure returns 400 before the stream opens:

```gotmpl
{{define "GET /events sse(Stream(ctx, lastEventID, send))"}}{{.Result}}{{end}}
```

[reference_sse.txt](../../cmd/muxt/testdata/reference_sse.txt) · [reference_sse_wrapper.txt](../../cmd/muxt/testdata/reference_sse_wrapper.txt) · [reference_sse_no_arg.txt](../../cmd/muxt/testdata/reference_sse_no_arg.txt) · [reference_sse_error_return.txt](../../cmd/muxt/testdata/reference_sse_error_return.txt) · [reference_sse_sendx.txt](../../cmd/muxt/testdata/reference_sse_sendx.txt) · [reference_sse_marshal_send.txt](../../cmd/muxt/testdata/reference_sse_marshal_send.txt) · [reference_last_event_id.txt](../../cmd/muxt/testdata/reference_last_event_id.txt)

#### Return mode — channel and iterator

When the wrapped method returns a stream type, each yielded value renders one SSE event via the define body. No callback arguments are used.

**`<-chan T`:**

```gotmpl
{{define "GET /events sse(Ticks(ctx))"}}{{.Result}}{{end}}
```
```go
func (s Server) Ticks(ctx context.Context) <-chan string {
    ch := make(chan string, 2)
    ch <- "x"
    ch <- "y"
    close(ch)
    return ch
}
```

**`iter.Seq[T]`:**

```gotmpl
{{define "GET /events sse(Ticks(ctx))"}}{{.Result}}{{end}}
```
```go
func (s Server) Ticks(ctx context.Context) iter.Seq[string] {
    return func(yield func(string) bool) {
        for _, v := range []string{"a", "b", "c"} {
            if !yield(v) {
                return
            }
        }
    }
}
```

**`iter.Seq2[T, error]`** — a non-nil yielded error is placed on the event template-data error list (`.Error`, a `[]error`); otherwise the value renders via `.Result`:

```gotmpl
{{define "GET /events sse(Ticks(ctx))"}}{{if .Error}}err:{{range .Error}}{{.}}{{end}}{{else}}ok:{{.Result}}{{end}}{{end}}
```
```go
func (s Server) Ticks(ctx context.Context) iter.Seq2[string, error] {
    return func(yield func(string, error) bool) {
        if !yield("good", nil) {
            return
        }
        yield("", errors.New("boom"))
    }
}
```

| Return type | Error surface |
|-------------|---------------|
| `<-chan T` | None (values only) |
| `iter.Seq[T]` | None (values only) |
| `iter.Seq2[T, error]` | Non-nil error → `.Error []error` on the event template data |

A method returning a channel or iterator that is **not** wrapped in `sse(...)` is a generation error; the error message directs you to add the wrapper.

[reference_sse_chan.txt](../../cmd/muxt/testdata/reference_sse_chan.txt) · [reference_sse_iter_seq.txt](../../cmd/muxt/testdata/reference_sse_iter_seq.txt) · [reference_sse_iter_seq2_error.txt](../../cmd/muxt/testdata/reference_sse_iter_seq2_error.txt)

### marshalJSON(...)

`marshalJSON(...)` makes the route respond `application/json`. The wrapped method returns `(T)` or `(T, error)`; the return value is marshaled as the response body.

```gotmpl
{{define "GET /api/user marshalJSON(GetUser(ctx))"}}{{end}}
```
```go
type User struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

func (s Server) GetUser(ctx context.Context) (User, error) {
    return User{Name: "Ada", Age: 36}, nil
}
```

A method error or marshal error responds 500. The define body is not rendered.

| Rule | Detail |
|------|--------|
| Marshal | `encoding/json` by default; `encoding/json/v2` `MarshalWrite` under `--output-jsonv2` |
| Method results | `(T)` or `(T, error)`; exactly one non-error result required |
| Error on method error | 500 Internal Server Error |
| Error on marshal error | 500 Internal Server Error |
| Invalid shapes | No result, only `error`, non-error second result, or more than two results → generation error |
| Composition | Composes with `unmarshalJSON(body)`: `marshalJSON(Increment(ctx, unmarshalJSON(body)))` |

[reference_marshal_json.txt](../../cmd/muxt/testdata/reference_marshal_json.txt)

## Frontend Framing

A **framing wrapper** is the outermost layer of a call: `frame( representation( Method(args…) ) )`. It selects which **render template-data type** the handler uses — and with it, which frontend helper methods the template may call.

### htmx(...)

`htmx(Method(...))` renders with a dedicated `HTMXTemplateData` type. The HX* helpers (`.HXRedirect`, `.HXTrigger`, etc.) live on `HTMXTemplateData`, **not** on the plain `TemplateData`.

```gotmpl
{{define "GET /set htmx(Set(ctx))"}}{{.HXRedirect "/next"}}done{{end}}
```

The minimal (unframed) `TemplateData` carries the base helpers (`.Result`, `.Request`, `.StatusCode`, `.Header`, `.Redirect`, `.RedirectSeeOther`, …) but no HX* helpers. An unframed route that calls `.HXRedirect` fails `muxt check`:

```gotmpl
{{define "GET / Index(ctx)"}}{{.HXRedirect "/x"}}{{end}}
```
```
field or method HXRedirect not found on TemplateData[...]
```

[reference_htmx_framing.txt](../../cmd/muxt/testdata/reference_htmx_framing.txt) · [reference_htmx_template_data_minimal.txt](../../cmd/muxt/testdata/reference_htmx_template_data_minimal.txt)

### --use-htmx auto-wraps every route

`--use-htmx` wraps **every** route's call in `htmx(...)` — there is no per-route opt-out under the flag. Every route renders with `HTMXTemplateData`, so a plain (unwrapped) template name may still call `.HXRedirect`. Because no route is unframed, the minimal `TemplateData` type is not emitted.

```gotmpl
{{define "GET /go Go(ctx)"}}{{.HXRedirect "/done"}}ok{{end}}
```

To mix htmx and non-htmx routes in one file, **omit the flag** and write `htmx(...)` explicitly only on the routes that need it:

```gotmpl
{{define "GET /plain Plain(ctx)"}}<p>{{.Result}}</p>{{end}}
{{define "GET /framed htmx(Framed(ctx))"}}<p>{{.Result}}</p>{{end}}
```

[reference_htmx_auto_wrap.txt](../../cmd/muxt/testdata/reference_htmx_auto_wrap.txt) · [reference_htmx_mixed.txt](../../cmd/muxt/testdata/reference_htmx_mixed.txt)

### Options and composition

| Topic | Detail |
|-------|--------|
| Render type name | Defaults to `HTMXTemplateData`; override with `--output-htmx-template-data-type=Name`. |
| Composition with `sse(...)` | `htmx(sse(Stream(ctx, send)))` keeps the **generic** SSE event marshaler (plain `data:` lines). Framing only changes the non-SSE render type, not the SSE wire format. |
| `datastar(...)` | Reserved for a later phase; not yet available as a framing wrapper. |

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
func (s Server) CreateUser(ctx context.Context, email, password string) (User, error) {
    if !isValidEmail(email) {
        return User{}, errors.New("invalid email")
    }
    if len(password) < 8 {
        return User{}, errors.New("password too short")
    }
    // ...
}
```

**Parse errors return 400 automatically:**
- Request to `/user/abc` with `GetUser(ctx, id int)` → 400 Bad Request
- Form field "age=xyz" with `age int` param → 400 Bad Request

Validation errors should return from your method. Display them in templates with `{{if .Err}}`.

[err_arg_type_mismatch.txt](../../cmd/muxt/testdata/err_arg_type_mismatch.txt)

## Test Files by Category

**Parameter sources:**
- [howto_arg_context.txt](../../cmd/muxt/testdata/howto_arg_context.txt) — `ctx` parameter
- [howto_arg_request.txt](../../cmd/muxt/testdata/howto_arg_request.txt) — `request` parameter
- [howto_arg_response.txt](../../cmd/muxt/testdata/howto_arg_response.txt) — `response` parameter
- [howto_arg_path_param.txt](../../cmd/muxt/testdata/howto_arg_path_param.txt) — Path param extraction

**Type parsing:**
- [reference_path_with_typed_param.txt](../../cmd/muxt/testdata/reference_path_with_typed_param.txt) — Typed path params
- [howto_arg_with_text_unmarshaler.txt](../../cmd/muxt/testdata/howto_arg_with_text_unmarshaler.txt) — Custom `TextUnmarshaler`

**Forms:**
- [howto_form_basic.txt](../../cmd/muxt/testdata/howto_form_basic.txt) — Basic form with url.Values
- [howto_form_with_struct.txt](../../cmd/muxt/testdata/howto_form_with_struct.txt) — Struct form binding
- [howto_form_with_field_tag.txt](../../cmd/muxt/testdata/howto_form_with_field_tag.txt) — `name` tag mapping
- [howto_form_with_slice.txt](../../cmd/muxt/testdata/howto_form_with_slice.txt) — Form slices
- [reference_form_field_types.txt](../../cmd/muxt/testdata/reference_form_field_types.txt) — All supported field types
- [reference_form_with_empty_struct.txt](../../cmd/muxt/testdata/reference_form_with_empty_struct.txt) — Empty struct edge case
- [err_form_unsupported_field_type.txt](../../cmd/muxt/testdata/err_form_unsupported_field_type.txt) — Unsupported types

**Multipart (`multipart/form-data`, file uploads):**
- [howto_multipart_file_upload.txt](../../cmd/muxt/testdata/howto_multipart_file_upload.txt) — End-to-end file upload walkthrough
- [reference_multipart_basic.txt](../../cmd/muxt/testdata/reference_multipart_basic.txt) — Single `*multipart.FileHeader` field
- [reference_multipart_multiple_files.txt](../../cmd/muxt/testdata/reference_multipart_multiple_files.txt) — `[]*multipart.FileHeader` field
- [reference_multipart_mixed.txt](../../cmd/muxt/testdata/reference_multipart_mixed.txt) — Mixed text + slice + file fields
- [reference_multipart_raw.txt](../../cmd/muxt/testdata/reference_multipart_raw.txt) — Raw `*multipart.Form` mode
- [reference_multipart_with_name_tag.txt](../../cmd/muxt/testdata/reference_multipart_with_name_tag.txt) — `name` tag rebind
- [reference_multipart_max_memory_flag.txt](../../cmd/muxt/testdata/reference_multipart_max_memory_flag.txt) — `--output-multipart-max-memory` flag
- [reference_multipart_parse_error.txt](../../cmd/muxt/testdata/reference_multipart_parse_error.txt) — Malformed body → 400
- [err_multipart_with_form.txt](../../cmd/muxt/testdata/err_multipart_with_form.txt) — `form` + `multipart` rejected

**Server-Sent Events (`sse(...)` wrapper):**
- [reference_sse.txt](../../cmd/muxt/testdata/reference_sse.txt) — `sse(...)` with `send` and `lastEventID`
- [reference_sse_wrapper.txt](../../cmd/muxt/testdata/reference_sse_wrapper.txt) — `sse(Method(ctx, lastEventID, send))` basic wrapper form
- [reference_sse_no_arg.txt](../../cmd/muxt/testdata/reference_sse_no_arg.txt) — `func() error` callback form
- [reference_sse_error_return.txt](../../cmd/muxt/testdata/reference_sse_error_return.txt) — error-returning method
- [reference_sse_synthesized_method.txt](../../cmd/muxt/testdata/reference_sse_synthesized_method.txt) — synthesized `func(any) error` signature
- [reference_sse_sendx.txt](../../cmd/muxt/testdata/reference_sse_sendx.txt) — `sendClock` renders the `Clock` template
- [reference_sse_marshal_send.txt](../../cmd/muxt/testdata/reference_sse_marshal_send.txt) — `marshalJSON(sendStatus)` emits JSON event data
- [reference_sse_chan.txt](../../cmd/muxt/testdata/reference_sse_chan.txt) — `<-chan T` return mode
- [reference_sse_iter_seq.txt](../../cmd/muxt/testdata/reference_sse_iter_seq.txt) — `iter.Seq[T]` return mode
- [reference_sse_iter_seq2_error.txt](../../cmd/muxt/testdata/reference_sse_iter_seq2_error.txt) — `iter.Seq2[T, error]` with `.Error` template data

**JSON response (`marshalJSON(...)` wrapper):**
- [reference_marshal_json.txt](../../cmd/muxt/testdata/reference_marshal_json.txt) — `marshalJSON(Method(ctx))` → `application/json`
- [reference_last_event_id.txt](../../cmd/muxt/testdata/reference_last_event_id.txt) — `lastEventID` header parsing

**Frontend framing (`htmx(...)` wrapper):**
- [reference_htmx_framing.txt](../../cmd/muxt/testdata/reference_htmx_framing.txt) — `htmx(Method(ctx))` renders with `HTMXTemplateData`
- [reference_htmx_template_data_minimal.txt](../../cmd/muxt/testdata/reference_htmx_template_data_minimal.txt) — unframed `TemplateData` has no HX* helpers
- [reference_htmx_auto_wrap.txt](../../cmd/muxt/testdata/reference_htmx_auto_wrap.txt) — `--use-htmx` auto-wraps every route
- [reference_htmx_mixed.txt](../../cmd/muxt/testdata/reference_htmx_mixed.txt) — mix framed and unframed routes by omitting the flag

**Multiple arguments:**
- [howto_call_with_multiple_args.txt](../../cmd/muxt/testdata/howto_call_with_multiple_args.txt) — Multiple params

**Receiver types:**
- [reference_receiver_with_pointer.txt](../../cmd/muxt/testdata/reference_receiver_with_pointer.txt) — Pointer receivers
- [reference_receiver_with_embedded_method.txt](../../cmd/muxt/testdata/reference_receiver_with_embedded_method.txt) — Embedded methods

**Errors:**
- [err_arg_type_mismatch.txt](../../cmd/muxt/testdata/err_arg_type_mismatch.txt) — Parse errors

**Browse all:** [cmd/muxt/testdata/](../../cmd/muxt/testdata/)
