# Call Results

What the method returns sets `.Result`, `.Err`, and the status code.

## Result Shapes

| Method results | `.Result` | `.Err` | `.Ok` |
|---|---|---|---|
| `T` | the value | nil | true |
| `(T, error)` | the value, even on error | the error; status 500 | always false |
| `(T, bool)` | the value | nil | true; on `false` the handler responds 200 with an empty body |
| `error`, or an interface embedding it | `struct{}` | the error; status 500, or its `StatusCode()` when the declared type has one ([howto_error_only_return.txt](../../cmd/muxt/testdata/howto_error_only_return.txt) · [reference_error_only_with_status_code.txt](../../cmd/muxt/testdata/reference_error_only_with_status_code.txt)) | true when the error is nil |
| `error`, with an `execute` argument | the callback's argument, `struct{}` for `func() error` | nil; a returned error responds `500 failed to render page` and discards any rendered output | true after a successful call |

The second of two results must be `error` or `bool`. Zero results, or three or more, fail generation outside `sse`. A lone result of a concrete type that implements `error` is the `T` shape: the value is `.Result`.

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
| `.Path` | `TemplateRoutePaths`: one method per route ([names](#route-methods)), taking one argument per path wildcard in order and returning the path as a `string`: `{{.Path.GetUser .Result.ID}}` |
| `.Route` | `TemplateRouteBuilder`: the same methods, each returning a `TemplateRoute` with `Path()`, `String()` (the path, so `{{.Route.GetUser .Result.ID}}` prints it), `Method()` (empty when the pattern names none) and `HasMethod()`. A method that takes arguments is called in parentheses before its result is selected from: `{{(.Route.GetUser .Result.ID).Method}}`, or `{{or .Route.Index.Method "GET"}}` to fall back for a pattern with no method ([reference_template_route.txt](../../cmd/muxt/testdata/reference_template_route.txt)) |
| `.MuxtVersion` | the generating muxt version |
| `.StatusCode code` | the data, for chaining |
| `.Header key value` | the data, for chaining |
| `.Redirect url code` | the data and an error when `code` is outside 300 to 399 |
| `.RedirectMultipleChoices url`, `.RedirectMovedPermanently url`, `.RedirectFound url`, `.RedirectSeeOther url` | `.Redirect` with 300, 301, 302, or 303 |
| `.String` | `""`: a stray `{{.}}` and a chained setter such as `{{.Header "HX-Trigger" "saved"}}` print nothing, and `muxt check` does not flag `{{.}}`. Print `{{.Result}}` for the value |

```gotmpl
{{define "GET /profile Profile(ctx)"}}{{if .Request.Header.Get "HX-Request"}}{{.Result.Name}}{{else}}<html>...</html>{{end}}{{end}}
{{define "GET /user/{id} GetUser(ctx, id)"}}<a href="{{.Path.GetUser .Result.ID}}">{{.Request.PathValue "id"}}</a>{{end}}
```

[reference_template_data_stringer.txt](../../cmd/muxt/testdata/reference_template_data_stringer.txt) · [reference_redirect_helpers.txt](../../cmd/muxt/testdata/reference_redirect_helpers.txt)

With `--output-htmx`, `TemplateData` also has the response setters `.HXLocation`, `.HXPushURL`, `.HXRedirect`, `.HXRefresh`, `.HXReplaceURL`, `.HXReswap`, `.HXRetarget`, `.HXReselect`, `.HXTrigger`, `.HXTriggerAfterSettle` and `.HXTriggerAfterSwap`, which chain and print nothing, and the request readers `.HXBoosted`, `.HXCurrentURL`, `.HXHistoryRestoreRequest`, `.HXPrompt`, `.HXRequest`, `.HXTargetElementID`, `.HXTriggerName` and `.HXTriggerElementID`.

### Route methods

A route with a call takes the call's name in PascalCase: `GET /user/{id} GetUser(ctx, id)` is `GetUser`. When two routes call the same method, each is named `<route name>Calling<Method>`, for example `ReadDupOneCallingDup`.

A route without a call is named from its pattern: a verb for the HTTP method (`GET` is `Read`, `POST` `Create`, `PUT` `Replace`, `PATCH` `Update`, `DELETE` `Delete`, and none for a pattern without a method), the host and literal segments in PascalCase (`/` alone is `Index`, `{$}` is `Exact`), then `By` and the wildcards joined with `And`: `GET /hello/{name}` is `ReadHelloByName`. Two routes may not produce the same name ([known issues](known-issues.md)).

`TemplateRoutes` returns the paths type, so `paths.Index()` is a `string`; the `TemplateRoute` comes from `.Route` in a template. Under a path prefix the paths of `/`, `{$}` and any route whose pattern ends in `/` keep their trailing slash: `/app/`, `/app/static/`. An empty string value drops its segment, because `http.ServeMux` never matches an empty `{name}` segment. String and `TextMarshaler` values are path-escaped; a trailing `{name...}` value is escaped per segment ([reference_path_param_escaping.txt](../../cmd/muxt/testdata/reference_path_param_escaping.txt)).

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
