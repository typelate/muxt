# Template Name Syntax

A template named in this shape becomes a route:

```text
[METHOD ][HOST]/PATH[ STATUS][ CALL]
```

```gotmpl
{{define "GET example.com/greet/{language} 200 Greeting(ctx, language)"}}{{end}}
{{define "/"}}{{end}}
```

| Component | Values | Default |
|-----------|--------|---------|
| METHOD | `GET`, `POST`, `PUT`, `PATCH`, `DELETE` | all methods |
| HOST | any text before the first `/` | all hosts |
| PATH | a `net/http` pattern starting with `/` | required |
| STATUS | `201` or `http.StatusCreated` | `200` |
| CALL | `Method(arg, ...)` | render with no call; dot is [TemplateData](call-results.md#templatedata) with an empty `.Result` |

Templates whose names do not match are ordinary templates. Any other all-caps word in the METHOD position, `HEAD` included, fails with `HEAD method not allowed; allowed methods: GET, POST, PUT, PATCH, and DELETE`.

[tutorial_basic_route.txt](../../cmd/muxt/testdata/tutorial_basic_route.txt)

## Path

Muxt registers the METHOD, HOST and PATH text on an `http.ServeMux`, so [its pattern rules](https://pkg.go.dev/net/http#hdr-Patterns-ServeMux) apply.

```gotmpl
{{define "GET /{$}"}}{{end}}
{{define "GET /user/{id}/post/{postID}"}}{{end}}
{{define "GET /files/{path...} ServeFile(ctx, path)"}}{{end}}
```

Muxt adds three rules. A path other than `/` may not contain an empty segment, a trailing `/` or `//` (`path has an empty segment`), so ServeMux's trailing-slash prefix matching is reachable only through `/`, which matches every path, or a `{name...}` wildcard. A wildcard name must be a Go identifier, unique within the path, and not one of `ctx`, `request`, `response`, `form`, `multipart`, `body`, `execute`, `lastEventID` (`path parameter name NAME conflicts with a reserved identifier`). The same pattern in two templates fails with `duplicate route pattern`.

[reference_path_exact_match.txt](../../cmd/muxt/testdata/reference_path_exact_match.txt) · [howto_path_param.txt](../../cmd/muxt/testdata/howto_path_param.txt) · [err_duplicate_pattern.txt](../../cmd/muxt/testdata/err_duplicate_pattern.txt)

## Status

```gotmpl
{{define "POST /user 201 CreateUser(ctx, form)"}}{{end}}
{{define "GET /admin http.StatusUnauthorized"}}{{end}}
```

The name sets the default; [Call Results](call-results.md#status-code-control) lists what overrides it. A status in the name plus a `response` argument fails generation.

[reference_status_codes.txt](../../cmd/muxt/testdata/reference_status_codes.txt)

## Call

```gotmpl
{{define "GET /user/{id} GetUser(ctx, id)"}}{{end}}
{{define "POST /login Login(ctx, form)"}}{{end}}
{{define "GET /api/user marshalJSON(GetUser(ctx))"}}{{end}}
{{define "GET /events sse(Stream(ctx, lastEventID, execute))"}}{{end}}
```

The call is parsed as a Go expression. [Call Parameters](call-parameters.md) lists every argument and the `sse`, `unmarshalJSON` and `unmarshalForm` wrappers. [Call Results](call-results.md) covers what the method may return and the [`marshalJSON`](call-results.md#json-responses) wrapper.

[howto_call_with_multiple_args.txt](../../cmd/muxt/testdata/howto_call_with_multiple_args.txt)
