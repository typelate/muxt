# Call Parameters

Call arguments bind request data to method parameters by position; parameter names do not matter. Only the names below, the route's path wildcards, and calls to other receiver methods are allowed; anything else fails generation with `unknown argument NAME; did you mean X?` or `unknown argument NAME; expected one of: ...` ([err_unknown_argument_with_suggestion.txt](../../cmd/muxt/testdata/err_unknown_argument_with_suggestion.txt)).

```gotmpl
{{define "POST /user/{id} UpdateUser(ctx, id, form)"}}{{end}}
```

```go
func (s Server) UpdateUser(ctx context.Context, id int, form UpdateUserForm) (User, error)
```

Value and pointer receivers both work ([reference_receiver_with_pointer.txt](../../cmd/muxt/testdata/reference_receiver_with_pointer.txt)). Methods promoted from embedded fields are found ([reference_receiver_with_embedded_method.txt](../../cmd/muxt/testdata/reference_receiver_with_embedded_method.txt)).

## Arguments

| Argument | Parameter type | Bound from |
|---|---|---|
| `ctx` | `context.Context` | `request.Context()` ([howto_arg_context.txt](../../cmd/muxt/testdata/howto_arg_context.txt)) |
| `request` | `*http.Request` | the request ([howto_arg_request.txt](../../cmd/muxt/testdata/howto_arg_request.txt)) |
| `response` | `http.ResponseWriter` | the response; muxt sets no status code or headers, and still renders the template after the method returns. Generate [warns](commands/generate.md#warnings) |
| path wildcard | a [parseable type](#parseable-types) | `request.PathValue(name)` ([howto_arg_path_param.txt](../../cmd/muxt/testdata/howto_arg_path_param.txt)) |
| `form` | struct or `url.Values` | `request.Form`: the query string plus a url-encoded body on POST, PUT, and PATCH |
| `unmarshalForm(body)` | same as `form` | the same binding, spelled out ([reference_form_equals_unmarshal_form.txt](../../cmd/muxt/testdata/reference_form_equals_unmarshal_form.txt)) |
| `multipart` | struct or `*multipart.Form` | `request.MultipartForm` |
| `body` | `io.Reader`, exactly | `request.Body` ([err_body_not_reader.txt](../../cmd/muxt/testdata/err_body_not_reader.txt) · [reference_body_reader.txt](../../cmd/muxt/testdata/reference_body_reader.txt)) |
| `unmarshalJSON(body)` | any type `encoding/json` decodes | `request.Body` |
| `signals` | same as `unmarshalJSON(body)` | shorthand for it, and diagnostics print that form; requires `--output-datastar` ([err_signals_without_datastar.txt](../../cmd/muxt/testdata/err_signals_without_datastar.txt) · [datastar-live-view.md](../how-to/datastar-live-view.md)) |
| `lastEventID` | a [parseable type](#parseable-types) | the `Last-Event-Id` header ([reference_last_event_id.txt](../../cmd/muxt/testdata/reference_last_event_id.txt)) |
| `execute` | `func(T) error` or `func() error` | a callback that renders the template: [the `execute` callback](call-results.md#the-execute-callback), or [below](#server-sent-events) inside `sse` |
| a receiver method call, such as `Author(id)` | its first result | called before the outer method under the same argument rules, except that `execute` and sse callbacks cannot be nested; a second `error` or `bool` result ends the handler as in [Result Shapes](call-results.md#result-shapes) ([reference_call_with_expression_arg.txt](../../cmd/muxt/testdata/reference_call_with_expression_arg.txt) · [reference_call_with_bool_return.txt](../../cmd/muxt/testdata/reference_call_with_bool_return.txt)) |

Form fields are not arguments; bind them through `form` or `multipart`.

## Type Resolution

Without `--use-receiver-type` ([cli.md](cli.md#flags)), muxt infers the signature: path values and `lastEventID` are `string`, `form` is `url.Values`, `multipart` is `*multipart.Form`, `body` is `io.Reader`, `unmarshalJSON(body)` is `json.RawMessage`, and the method returns `any`. With it, muxt parses each argument to the method's declared parameter type ([howto_arg_no_receiver.txt](../../cmd/muxt/testdata/howto_arg_no_receiver.txt) · [howto_call_with_path_param.txt](../../cmd/muxt/testdata/howto_call_with_path_param.txt)).

An argument may appear more than once in a call, including inside nested calls, when every use is passed to a parameter of an identical type, such as `Sum(id, id)` with two `int` parameters or `(request, request)` with two `*http.Request` parameters. The value is parsed once and reused. A later use whose parameter type differs from the first fails generation and names both types, so `AnyString(id, id)` with `(any, string)` is rejected. A method muxt infers gets one parameter of the default type per use, numbering the later ones: `F(request, request)` infers `F(request *http.Request, request2 *http.Request)` ([reference_repeated_argument_on_undefined_method.txt](../../cmd/muxt/testdata/reference_repeated_argument_on_undefined_method.txt)).

## Parseable Types

| Parameter type | Parser |
|---|---|
| `string` | none |
| `int`, `int8`, `int16`, `int32`, `int64` | `strconv.ParseInt`, base 10 |
| `uint`, `uint8`, `uint16`, `uint32`, `uint64` | `strconv.ParseUint`, base 10 |
| `bool` | `strconv.ParseBool` |
| `float32`, `float64` | `strconv.ParseFloat`; form and multipart fields only, never path values or `lastEventID` |
| a type whose pointer implements `encoding.TextUnmarshaler` | `UnmarshalText`; `time.Time` qualifies |

[reference_path_with_typed_param.txt](../../cmd/muxt/testdata/reference_path_with_typed_param.txt) · [reference_form_float_fields.txt](../../cmd/muxt/testdata/reference_form_float_fields.txt) · [howto_arg_with_text_unmarshaler.txt](../../cmd/muxt/testdata/howto_arg_with_text_unmarshaler.txt)

## Parse Failures

A path value, typed form field, or `lastEventID` that fails to parse responds 400. The method is not called. The template still renders with a zero [`.Result`](call-results.md#templatedata) and the error in `.Err`. On an `execute` route nothing renders; the response is an empty 400. On an sse route the 400 is sent before the stream opens ([reference_sse_with_typed_form_field.txt](../../cmd/muxt/testdata/reference_sse_with_typed_form_field.txt)).

A url-encoded body that `request.ParseForm` rejects responds 400 through `http.Error` without rendering ([reference_form_parse_error.txt](../../cmd/muxt/testdata/reference_form_parse_error.txt)). A malformed multipart body sets `.Err` and responds 400 ([reference_multipart_parse_error.txt](../../cmd/muxt/testdata/reference_multipart_parse_error.txt)).

## Form Structs

```go
type LoginForm struct {
    Username string `name:"user-name"` // bound from the "user-name" field
    Password string                    // bound from "Password", case-sensitive
    Remember bool
    Tags     []string                  // every value for "Tags"
}
```

[howto_form_with_struct.txt](../../cmd/muxt/testdata/howto_form_with_struct.txt) · [howto_form_with_field_tag.txt](../../cmd/muxt/testdata/howto_form_with_field_tag.txt) · [howto_form_with_slice.txt](../../cmd/muxt/testdata/howto_form_with_slice.txt)

A `template:"name"` tag names the template that holds the field's `<input>`, matched by the field's bound name. Muxt then generates validation from that input's `min`, `max`, `minlength`, `maxlength` and `pattern` attributes and responds 400 before the method runs. `min` and `max` apply to numeric and temporal input types; `pattern` applies to textual ones. Attribute values must be literals ([Known Issues](known-issues.md)).

```gotmpl
{{define "age-field"}}<input type="number" name="age" min="0" max="120">{{end}}
```

```go
type SignupForm struct {
    Age int `name:"age" template:"age-field"` // age=200 responds 400
}
```

[reference_validation_min_max.txt](../../cmd/muxt/testdata/reference_validation_min_max.txt) · [reference_validation_pattern.txt](../../cmd/muxt/testdata/reference_validation_pattern.txt)

## Multipart

Use `multipart` for `multipart/form-data` bodies, which file inputs require. The [struct rules](#form-structs) apply; file fields are `*multipart.FileHeader` or `[]*multipart.FileHeader`.

```go
type UploadForm struct {
    Title  string                  `name:"title"`
    Avatar *multipart.FileHeader   `name:"avatar"`
    Photos []*multipart.FileHeader `name:"photos"`
}
```

- `--output-multipart-max-memory` ([cli.md](cli.md#generate-output-flags)) sets the `ParseMultipartForm` limit.
- A url-encoded body still binds the text fields; file fields stay nil ([reference_multipart_url_encoded_fallback.txt](../../cmd/muxt/testdata/reference_multipart_url_encoded_fallback.txt)).
- `form` and `multipart` in the same call fail generation ([err_multipart_with_form_in_nested_call.txt](../../cmd/muxt/testdata/err_multipart_with_form_in_nested_call.txt)).

[howto_multipart_file_upload.txt](../../cmd/muxt/testdata/howto_multipart_file_upload.txt) · [reference_multipart_raw.txt](../../cmd/muxt/testdata/reference_multipart_raw.txt)

## Request Body

A call that reads the body twice, such as `(form, body)` or `(body, unmarshalJSON(body))`, fails generation ([err_body_consumed_twice.txt](../../cmd/muxt/testdata/err_body_consumed_twice.txt)).

`unmarshalJSON(body)` does not check `Content-Type`. A malformed or empty body responds 400 without calling the method.

```gotmpl
{{define "POST /users CreateUser(ctx, unmarshalJSON(body))"}}{{.Result.Name}}{{end}}
```

[reference_unmarshal_json.txt](../../cmd/muxt/testdata/reference_unmarshal_json.txt) · [reference_unmarshal_json_undefined.txt](../../cmd/muxt/testdata/reference_unmarshal_json_undefined.txt)

## Server-Sent Events

Wrap the call in `sse` to stream events. The handler sets `Content-Type: text/event-stream`, `Cache-Control: no-store`, and `Connection: keep-alive`, flushes, then calls the method. Each `execute` call renders the template as one event and flushes it.

```gotmpl
{{define "GET /clock sse(Clock(ctx, lastEventID, execute))"}}{{.Result}}{{end}}
```

```go
func (s Server) Clock(ctx context.Context, lastEventID string, execute func(time.Time) error) {
    t := time.NewTicker(time.Second)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case now := <-t.C:
            if err := execute(now); err != nil {
                return
            }
        }
    }
}
```

Return when `ctx` is done or `execute` returns an error: the client is gone, the template failed, or the write failed.

| Rule | Detail |
|---|---|
| Wrapper | exactly one method call; any other shape, such as `sse()`, is an ordinary call to a function named `sse` ([reference_sse.txt](../../cmd/muxt/testdata/reference_sse.txt)) |
| Callback | `func(T) error` or `func() error`, inline or a named or aliased func type ([reference_callback_named_func_type.txt](../../cmd/muxt/testdata/reference_callback_named_func_type.txt)) |
| Method results | none, or `error`; a returned error is logged and the stream closes ([reference_sse_error_return.txt](../../cmd/muxt/testdata/reference_sse_error_return.txt)) |
| `response` argument | not allowed ([err_sse_with_response.txt](../../cmd/muxt/testdata/err_sse_with_response.txt)) |
| Undefined method | fails generation: `method NAME using the execute callback must be defined on the receiver type` |
| Extra callbacks | arguments prefixed `sse`, such as `sseClock`, render the template of the same name, which must exist at generate time; each has its own `T` and may sit anywhere in the call ([reference_sse_multiple_callbacks.txt](../../cmd/muxt/testdata/reference_sse_multiple_callbacks.txt)) |
| Template data | `*SSETemplateData[R, T]`: `.Result`, `.Err`, `.Request`, `.Receiver`, `.Path`, `.String`, and chainable `.Event`, `.ID`, and `.Retry` setters. No `.Ok`, `.StatusCode`, `.Header`, or `.Redirect` |

### Datastar

Under `--output-datastar`:

| Rule | Detail |
|---|---|
| Events | `datastar-patch-elements`, with each rendered line as a `data: elements` line; `.Selector`, `.Mode`, and `.UseViewTransition` replace `.Event` ([reference_output_datastar_elements.txt](../../cmd/muxt/testdata/reference_output_datastar_elements.txt)) |
| `Signals` suffix | a callback named like `countsSignals`, typed `func(T) error`, marshals its argument as a `datastar-patch-signals` event instead of rendering; the prefix is only a label ([reference_output_datastar_signals_events.txt](../../cmd/muxt/testdata/reference_output_datastar_signals_events.txt) · [err_signals_callback_without_datastar.txt](../../cmd/muxt/testdata/err_signals_callback_without_datastar.txt)) |
| `--output-htmx` | mutually exclusive ([err_output_htmx_and_datastar.txt](../../cmd/muxt/testdata/err_output_htmx_and_datastar.txt)) |
