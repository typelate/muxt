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

The request body is a **single-use stream**: at most one of `body`,
`unmarshalJSON(body)`, or `unmarshalForm(body)` may appear in a call —
using two fails generation.

### `body`

Binds `request.Body` as an `io.Reader`. The method parameter must be exactly
`io.Reader`; any other type fails generation. Use it for payloads the handler
must not reinterpret (webhooks, proxied uploads).

```gotmpl
{{define "POST /hooks Save(ctx, body)"}}{{.Result}}{{end}}
```
```go
func (s Server) Save(ctx context.Context, body io.Reader) (string, error)
```

[reference_body_reader.txt](../../cmd/muxt/testdata/reference_body_reader.txt) · [err_body_not_reader.txt](../../cmd/muxt/testdata/err_body_not_reader.txt) · [err_body_consumed_twice.txt](../../cmd/muxt/testdata/err_body_consumed_twice.txt)

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
- Uses `encoding/json` by default; under `--output-jsonv2` it uses
  `encoding/json/v2` `UnmarshalRead` (requires a go 1.25+ module built with
  `GOEXPERIMENT=jsonv2`).
- If the receiver method is not yet defined, the parameter synthesizes as
  `json.RawMessage` (`*jsontext.Decoder` under `--output-jsonv2`) so
  template-first iteration passes the raw payload through.

[reference_unmarshal_json.txt](../../cmd/muxt/testdata/reference_unmarshal_json.txt) · [reference_unmarshal_json_jsonv2.txt](../../cmd/muxt/testdata/reference_unmarshal_json_jsonv2.txt) · [reference_unmarshal_json_undefined.txt](../../cmd/muxt/testdata/reference_unmarshal_json_undefined.txt) · [reference_unmarshal_json_undefined_jsonv2.txt](../../cmd/muxt/testdata/reference_unmarshal_json_undefined_jsonv2.txt) · [err_unmarshal_json_bad_arg.txt](../../cmd/muxt/testdata/err_unmarshal_json_bad_arg.txt)

### `unmarshalForm(body)`

The explicit spelling of the existing `form` binding — **not** a separate body
decoder. Both spellings generate the identical `request.Form` binding
(`request.ParseForm` semantics, URL query merge included), so behavior is the
same by construction. On GET the bound values are the query string and the
`(body)` spelling is misleading; prefer `form` there.

[reference_unmarshal_form.txt](../../cmd/muxt/testdata/reference_unmarshal_form.txt) · [reference_form_equals_unmarshal_form.txt](../../cmd/muxt/testdata/reference_form_equals_unmarshal_form.txt)

## Server-Sent Events

Wrapping the method call in `sse(...)` makes the route stream [Server-Sent Events](https://developer.mozilla.org/docs/Web/API/Server-sent_events). The handler sets the event-stream headers, flushes, and drives the stream in one of two mutually exclusive modes.

**Callback mode** — the method takes `send` callbacks and emits one event per invocation:

```gotmpl
{{define "GET /clock sse(Clock(ctx, send))"}}{{.Result}}{{end}}
```
```go
func (s Server) Clock(ctx context.Context, send func(string) error) {
    t := time.NewTicker(time.Second)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case now := <-t.C:
            if err := send(now.Format(time.RFC3339)); err != nil {
                return // client disconnected or render failed
            }
        }
    }
}
```

**Return mode** — the method returns a stream as its only result; each value renders one event via the define body:

```gotmpl
{{define "GET /ticks sse(Ticks(ctx))"}}{{.Result}}{{end}}
```

| Return type | Behavior |
|---|---|
| `<-chan T` | one event per received value; the stream ends when the channel closes |
| `iter.Seq[T]` | one event per yielded value |
| `iter.Seq2[T, error]` | a non-nil yielded error lands on the event data (`.Err`); the event still renders |

Client disconnect (request-context cancellation) ends both modes.

| Rule | Detail |
|------|--------|
| Callback shape | `func(T) error` (`T` is `.Result`) or `func() error` |
| Named callbacks | `send`-prefixed arguments render the template named by the suffix: `sendClock` renders `{{define "Clock"}}` (which stays reusable via `{{template "Clock"}}`); the template must exist |
| JSON events | `marshalJSON(sendX)` marshals the callback argument as the event's `data:` payload instead of rendering a template (`encoding/json`, or `encoding/json/v2` under `--output-jsonv2`) — one stream can interleave rendered and JSON events |
| Method results | Nothing, only `error` (logged; the stream closes), or a stream type (return mode; no callbacks allowed) |
| Not allowed | a `response` argument, the `execute` callback, or mixing send callbacks with a stream result |
| Frame fields | `SSETemplateData` adds chainable `.Event`, `.ID`, `.Retry` setters alongside `.Result`, `.Request`, `.Err` |
| Undefined method | send callbacks synthesize as `func(any) error` |

**Per-event options.** Any send-family callback may declare a trailing
variadic of the generated `SSEEventOption` type (rename it with
`--output-sse-event-option-type`); declaring it is opt-in per method
signature, and `muxt check` verifies the element type:

```go
func (s Server) Feed(ctx context.Context, send func(msg string, opts ...SSEEventOption) error) {
    _ = send("hello", WithEvent("news"), WithEventID("42"), WithRetryDuration(3*time.Second))
}
```

| Constructor | Wire line |
|---|---|
| `WithEvent(string)` | `event: <v>` |
| `WithEventID(string)` | `id: <v>` |
| `WithRetryDuration(time.Duration)` | `retry: <ms>` |
| `WithJSONOptions(...json.Options)` | forwards `encoding/json/v2`/`jsontext` encoder options to that event's marshal — generated only under `--output-jsonv2`, meaningful only on `marshalJSON`-wrapped senders |

**Precedence (deliberate):** the template's chainable setters (`.Event`,
`.ID`, `.Retry`) run first, call-site options after — so Go-side options
override template defaults. The template states defaults; the method
overrides per event when it knows better. The `execute` callback (non-SSE
render control) takes no options variadic. Undefined-method synthesis
includes the variadic: `send func(result any, opts ...SSEEventOption) error`.

[reference_sse_send_options.txt](../../cmd/muxt/testdata/reference_sse_send_options.txt) · [reference_sse_options_override.txt](../../cmd/muxt/testdata/reference_sse_options_override.txt) · [reference_sse_options_mixed_signatures.txt](../../cmd/muxt/testdata/reference_sse_options_mixed_signatures.txt) · [reference_sse_options_type_flag.txt](../../cmd/muxt/testdata/reference_sse_options_type_flag.txt) · [reference_sse_marshal_json_options.txt](../../cmd/muxt/testdata/reference_sse_marshal_json_options.txt)

Pair the wrapper with `lastEventID` to resume after a reconnect. `lastEventID` reads the `Last-Event-Id` header and parses it like a path value (defaults to `string`); a typed parse failure returns 400 before the stream opens.

```gotmpl
{{define "GET /events sse(Stream(ctx, lastEventID, send))"}}{{.Result}}{{end}}
```

Migrating from earlier releases: the `execute` callback inside `sse(...)`, the reserved `sse` argument, and `sse`-prefixed callbacks (`sseClock`) were replaced by the `send` vocabulary; each old spelling fails generation with an error showing the new form.

[reference_sse.txt](../../cmd/muxt/testdata/reference_sse.txt) · [reference_sse_no_arg.txt](../../cmd/muxt/testdata/reference_sse_no_arg.txt) · [reference_sse_error_return.txt](../../cmd/muxt/testdata/reference_sse_error_return.txt) · [reference_sse_multiple_callbacks.txt](../../cmd/muxt/testdata/reference_sse_multiple_callbacks.txt) · [reference_sse_marshal_send.txt](../../cmd/muxt/testdata/reference_sse_marshal_send.txt) · [reference_sse_chan.txt](../../cmd/muxt/testdata/reference_sse_chan.txt) · [reference_sse_iter_seq.txt](../../cmd/muxt/testdata/reference_sse_iter_seq.txt) · [reference_sse_iter_seq2_error.txt](../../cmd/muxt/testdata/reference_sse_iter_seq2_error.txt) · [reference_sse_event_fields.txt](../../cmd/muxt/testdata/reference_sse_event_fields.txt) · [reference_last_event_id.txt](../../cmd/muxt/testdata/reference_last_event_id.txt) · [reference_sse_last_event_id_400.txt](../../cmd/muxt/testdata/reference_sse_last_event_id_400.txt)

## Datastar Framing

`datastar(Method(...))` at the outermost call position gives Datastar apps the
template-first workflow: templates render with Datastar-specific data types,
and streams speak the `datastar-patch-elements` / `datastar-patch-signals`
protocol over the shared SSE transport. Generated code is stdlib-only —
datastar-go is used only as a test-time wire-conformance reference.

| Route shape | Template data | Behavior |
|---|---|---|
| `datastar(Method(...))` | `DatastarTemplateData` | plain render with base helpers |
| `datastar(sse(Method(...)))` | `DatastarEventTemplateData` per event | render senders emit `datastar-patch-elements`; `marshalJSON(sendX)` senders emit `datastar-patch-signals` — one stream interleaves both in call order |
| `datastar(marshalJSON(Method(...)))` | `DatastarSignalsTemplateData` | standalone `application/json` signals response, `marshalJSON` execute-then-discard semantics |

**Event setters** (chainable, callable from inside the template) each become a
wire line: `.Selector(string)` → `data: selector <v>`, `.Mode(string)` →
`data: mode <v>` (values pass through unvalidated; Datastar's modes are
`outer`, `inner`, `remove`, `replace`, `prepend`, `append`, `before`,
`after`), `.Namespace(string)` → `data: namespace <v>`,
`.UseViewTransition(true)` → `data: useViewTransition true`. `.ID` and
`.Retry` work as on generic SSE events.

**Send options** reuse the SSE option machinery (opt-in trailing variadic,
template setters first, Go-side options win): render senders take
`DatastarPatchElementOption` (`WithSelector`, `WithSelectorID`, `WithMode`,
`WithNamespace`, `WithUseViewTransition`, plus the shared `WithEventID` and
`WithRetryDuration`); marshaled senders take `DatastarPatchSignalsOption`
(`WithOnlyIfMissing`, the shared constructors, and `WithJSONOptions` under
`--output-jsonv2`).

**`signals` input.** On datastar-framed routes the reserved `signals`
argument binds Datastar's client-sent signals to the method parameter's type:
GET and DELETE read the `datastar` query parameter, other methods read the
JSON body. Absent signals leave the parameter zero-valued; malformed JSON
responds 400. Elsewhere `signals` remains an ordinary identifier.

**Backend actions: `.Actions()`.** The datastar template-data types carry an
`.Actions()` accessor — a parallel to `.Path()` — with one method per route
(any representation), taking the route's typed path parameters and returning
an action that renders a Datastar backend-action expression with the verb
inferred from the route (`GET`→`@get`, `POST`→`@post`, …):

```gotmpl
<button data-on:click="{{ .Actions.UpdateUser .ID | .JS }}">save</button>
<div data-init="{{ .Actions.Refresh | .JS }}"></div>
<a data-on:click="{{ (.Actions.UpdateUser .ID).OpenWhenHidden true | .JS }}">defer</a>
```

`.JS` renders the action as `template.JS` — required because Datastar's
colon-form event attributes (`data-on:click`) put the value in a JavaScript
context where a plain string would render as a quoted literal Datastar cannot
execute. The value is **safe by construction**: the builder percent-encodes
each interpolated path segment (`url.PathEscape` — a value like `a/b?c=d`
stays one segment and round-trips through `request.PathValue`) and JS-string
escapes the whole URL and every string option value at render time. Inputs
are taken raw — do not pre-escape — and no method accepts raw JS.

Options are fluent copy-on-write setters (each returns a modified copy, so a
base action can be reused): `.ContentType`, `.OpenWhenHidden`, `.Selector`,
`.Retry`, `.RequestCancellation`, `.RetryInterval`, `.RetryScaler`,
`.RetryMaxWait`, `.RetryMaxCount`, rendering as the Datastar options object
(`@patch('/users/42', {openWhenHidden: true})`).

The safe path never traps you: an action is a JS *expression* that composes
with author-written JS (`data-on:click="$saving = true; {{ .Actions.Save .ID | .JS }}"`);
query strings are out of the builder's vocabulary — interpolate `.URL` (the
encoded path) or a `.Path` helper inside a hand-written expression, where
`html/template`'s own JS-string escaping protects the interpolation; and a
fully manual `@post('/literal')` remains ordinary template text under normal
contextual autoescaping.

[reference_datastar_actions.txt](../../cmd/muxt/testdata/reference_datastar_actions.txt) · [reference_datastar_actions_options.txt](../../cmd/muxt/testdata/reference_datastar_actions_options.txt) · [reference_datastar_actions_injection.txt](../../cmd/muxt/testdata/reference_datastar_actions_injection.txt) · [reference_datastar_actions_roundtrip.txt](../../cmd/muxt/testdata/reference_datastar_actions_roundtrip.txt) · [reference_datastar_actions_text_marshaler.txt](../../cmd/muxt/testdata/reference_datastar_actions_text_marshaler.txt) · [reference_datastar_actions_compose.txt](../../cmd/muxt/testdata/reference_datastar_actions_compose.txt)

**Flags.** `--use-datastar` wraps every route (mutually exclusive with
`--use-htmx`); `--output-datastar-template-data-type`,
`--output-datastar-event-template-data-type`, and
`--output-datastar-signals-template-data-type` rename the generated types.
The pre-release reserved arguments `elements`, `signal`, and `script` were
removed; each fails generation with an error showing the wrapper form.

[reference_datastar_framing_elements.txt](../../cmd/muxt/testdata/reference_datastar_framing_elements.txt) · [reference_datastar_mixed_stream.txt](../../cmd/muxt/testdata/reference_datastar_mixed_stream.txt) · [reference_datastar_send_options.txt](../../cmd/muxt/testdata/reference_datastar_send_options.txt) · [reference_datastar_framing_signals.txt](../../cmd/muxt/testdata/reference_datastar_framing_signals.txt) · [reference_datastar_signals_input.txt](../../cmd/muxt/testdata/reference_datastar_signals_input.txt) · [reference_datastar_conformance.txt](../../cmd/muxt/testdata/reference_datastar_conformance.txt) · [reference_datastar_type_name_flags.txt](../../cmd/muxt/testdata/reference_datastar_type_name_flags.txt)

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

**Multipart (`multipart/form-data`, file uploads):**
- [howto_multipart_file_upload.txt](../../cmd/muxt/testdata/howto_multipart_file_upload.txt) — End-to-end file upload walkthrough
- [reference_multipart_basic.txt](../../cmd/muxt/testdata/reference_multipart_basic.txt) — Single `*multipart.FileHeader` field
- [reference_multipart_multiple_files.txt](../../cmd/muxt/testdata/reference_multipart_multiple_files.txt) — `[]*multipart.FileHeader` field
- [reference_multipart_mixed.txt](../../cmd/muxt/testdata/reference_multipart_mixed.txt) — Mixed text + slice + file fields
- [reference_multipart_raw.txt](../../cmd/muxt/testdata/reference_multipart_raw.txt) — Raw `*multipart.Form` mode
- [reference_multipart_with_name_tag.txt](../../cmd/muxt/testdata/reference_multipart_with_name_tag.txt) — `name` tag rebind
- [reference_multipart_max_memory_flag.txt](../../cmd/muxt/testdata/reference_multipart_max_memory_flag.txt) — `--output-multipart-max-memory` flag
- [reference_multipart_parse_error.txt](../../cmd/muxt/testdata/reference_multipart_parse_error.txt) — Malformed body → 400

Using `form` and `multipart` in the same call is rejected (multipart parses url-encoded fields too).

**Server-Sent Events:**
- [reference_sse.txt](../../cmd/muxt/testdata/reference_sse.txt) — `sse(...)` wrapper with `lastEventID`
- [reference_sse_no_arg.txt](../../cmd/muxt/testdata/reference_sse_no_arg.txt) — `func() error` callback form
- [reference_sse_error_return.txt](../../cmd/muxt/testdata/reference_sse_error_return.txt) — error-returning method
- [reference_sse_synthesized_method.txt](../../cmd/muxt/testdata/reference_sse_synthesized_method.txt) — synthesized `func(any) error` signature
- [reference_last_event_id.txt](../../cmd/muxt/testdata/reference_last_event_id.txt) — `lastEventID` header parsing

**Multiple arguments:**
- [howto_call_with_multiple_args.txt](../../cmd/muxt/testdata/howto_call_with_multiple_args.txt) — Multiple params

**Receiver types:**
- [reference_receiver_with_pointer.txt](../../cmd/muxt/testdata/reference_receiver_with_pointer.txt) — Pointer receivers
- [reference_receiver_with_embedded_method.txt](../../cmd/muxt/testdata/reference_receiver_with_embedded_method.txt) — Embedded methods

**Browse all:** [cmd/muxt/testdata/](../../cmd/muxt/testdata/)
