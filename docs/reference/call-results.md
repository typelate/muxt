# Call Results

What the method returns sets `.Result`, `.Err`, and the status code.

## Result Shapes

| Method results | `.Result` | `.Err` | `.Ok` |
|---|---|---|---|
| `T` | the value | nil | true |
| `(T, error)` | the value, even on error | the error; status 500 | always false |
| `(T, bool)` | the value | nil | true; on `false` the handler responds 200 with an empty body |
| `error`, with an `execute` argument | the callback's argument, `struct{}` for `func() error` | nil; a returned error responds `500 failed to render page` and discards any rendered output | true after a successful call |

The second of two results must be `error` or `bool`. Zero results, or three or more, fail generation outside `sse`. A lone `error` result without `execute` is the `T` shape: the error is `.Result`.

```go
func (s Server) About() AboutPage
func (s Server) GetUser(ctx context.Context, id int) (User, error)
func (s Server) Download(response http.ResponseWriter, id int) (File, bool)
```

```gotmpl
{{define "GET /user/{id} GetUser(ctx, id)"}}{{with .Err}}<p>{{.}}</p>{{else}}<h1>{{.Result.Name}}</h1>{{end}}{{end}}
```

The template renders on a method error, so branch on `.Err`.

[howto_call_method.txt](../../cmd/muxt/testdata/howto_call_method.txt) · [reference_call_with_error_return.txt](../../cmd/muxt/testdata/reference_call_with_error_return.txt) · [reference_call_with_bool_return.txt](../../cmd/muxt/testdata/reference_call_with_bool_return.txt)

## The `execute` Callback

With `execute` in the call, muxt passes a render closure to the method instead of rendering after it returns.

```gotmpl
{{define "GET /count Count(execute)"}}<p>{{.Result}}</p>{{end}}
```

```go
func (s *Server) Count(execute func(int) error) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    return execute(s.n)
}
```

- The callback renders at most once; a second call returns an error ([reference_execute_callback_multiple_calls.txt](../../cmd/muxt/testdata/reference_execute_callback_multiple_calls.txt)).
- Returning nil without calling it responds with an empty body and the name's status, or 204 when the name has none.

[reference_execute_callback.txt](../../cmd/muxt/testdata/reference_execute_callback.txt) · [reference_execute_callback_no_arg.txt](../../cmd/muxt/testdata/reference_execute_callback_no_arg.txt)

## JSON Responses

`marshalJSON(GetUser(ctx))` responds `application/json` with the marshaled result instead of the rendered template.

```gotmpl
{{define "GET /api/user 201 marshalJSON(GetUser(ctx))"}}{{end}}
```

| Method results | Outcome |
|---|---|
| `T` or `(T, error)` | `T` is marshaled with `encoding/json` |
| `(T, bool)`, `error` alone, or a `T` implementing `error` | generation error |

- The template body still executes, so `.StatusCode` and `.Header` calls reach the JSON response. Its output is discarded on success.
- On a method error the rendered output is sent as `text/html` with status 500; render JSON in the `{{if .Err}}` branch if you need JSON errors.
- A marshal failure responds 500 with the status text.
- A `response` argument inside the wrapper fails generation.

[reference_marshal_json.txt](../../cmd/muxt/testdata/reference_marshal_json.txt) · [reference_marshal_json_side_effects.txt](../../cmd/muxt/testdata/reference_marshal_json_side_effects.txt) · [reference_marshal_json_method_error.txt](../../cmd/muxt/testdata/reference_marshal_json_method_error.txt) · [reference_marshal_json_marshal_error.txt](../../cmd/muxt/testdata/reference_marshal_json_marshal_error.txt) · [err_marshal_json_with_response.txt](../../cmd/muxt/testdata/err_marshal_json_with_response.txt)

## TemplateData

Templates receive `*TemplateData[R, T]`, where `R` is the receiver interface and `T` the method's first result.

| Method | Returns |
|---|---|
| `.Result` | `T` |
| `.Err` | the method error joined with any [parse errors](call-parameters.md#parse-failures), or nil |
| `.Ok` | `bool`, per the shape table above |
| `.Request` | `*http.Request` |
| `.Receiver` | `R` |
| `.Path` | `TemplateRoutePaths`, one method per route named after the method with its first letter uppercased, taking one argument per path wildcard in order: `{{.Path.GetUser .Result.ID}}`. Each returns a `TemplateRoute` that prints as the path and has `Method() string`, the route's HTTP method, empty when the pattern names none ([reference_template_route.txt](../../cmd/muxt/testdata/reference_template_route.txt)). String and `TextMarshaler` values are path-escaped; a trailing `{name...}` value is escaped per segment ([reference_path_param_escaping.txt](../../cmd/muxt/testdata/reference_path_param_escaping.txt)) |
| `.MuxtVersion` | the generating muxt version |
| `.StatusCode code` | the data, for chaining |
| `.Header key value` | the data, for chaining |
| `.Redirect url code` | the data and an error when `code` is outside 300 to 399 |
| `.RedirectMultipleChoices url`, `.RedirectMovedPermanently url`, `.RedirectFound url`, `.RedirectSeeOther url` | `.Redirect` with 300, 301, 302, or 303 |
| `.String` | `""`, so `{{.Header "HX-Trigger" "saved"}}` prints nothing |

```gotmpl
{{define "GET /profile Profile(ctx)"}}{{if .Request.Header.Get "HX-Request"}}{{.Result.Name}}{{else}}<html>...</html>{{end}}{{end}}
{{define "GET /user/{id} GetUser(ctx, id)"}}<a href="{{.Path.GetUser .Result.ID}}">{{.Request.PathValue "id"}}</a>{{end}}
```

[reference_template_data_stringer.txt](../../cmd/muxt/testdata/reference_template_data_stringer.txt) · [reference_redirect_helpers.txt](../../cmd/muxt/testdata/reference_redirect_helpers.txt)

## Status Code Control

The first non-zero value wins:

| Priority | Source |
|---|---|
| 1 | `{{.StatusCode 404}}` in the template |
| 2 | 400 on a [parse error](call-parameters.md#parse-failures), 500 on a method error |
| 3 | the result's `StatusCode() int` method, else its `StatusCode` field |
| 4 | the template name's code, else 200, or 204 when the rendered body is empty |

An error's own `StatusCode` method is not consulted.

```gotmpl
{{if .Err}}{{.StatusCode 404}}<p>not found</p>{{end}}
```

[reference_status_codes.txt](../../cmd/muxt/testdata/reference_status_codes.txt) · [reference_empty_body_status.txt](../../cmd/muxt/testdata/reference_empty_body_status.txt)
