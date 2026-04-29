---
name: muxt-test-driven-development
description: "Muxt: Use when creating new Muxt templates, adding routes, writing receiver methods, or implementing features in a Muxt codebase using TDD. Covers template-first design for GET routes, method-first for POST/PATCH/DELETE, and red-green-refactor with domtest."
---

# Template-Driven Development

Create new templates and receiver methods using TDD. Approach by HTTP method:

- **GET routes**: template-first (design the view, then implement the method).
- **POST/PATCH/DELETE routes**: receiver-method-first (design the behavior, then wire the template).

## Test dependencies

```bash
go get github.com/typelate/dom/domtest
go get github.com/stretchr/testify/{assert,require}
go install github.com/maxbrunsfeld/counterfeiter/v6@latest
```

Generate test doubles from the `RoutesReceiver` interface:

```go
//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate . RoutesReceiver
```

```bash
go generate ./...
```

## GET routes — template-first

1. **Write the template.** Template name = contract: route pattern + call expression. See `references/examples.md` for the full pattern syntax (wildcards, precedence, trailing slash) and a worked template + GWT-table test.
2. **Write the test** using the Given-When-Then table-driven pattern. Stub the fake receiver, build the request, parse the response with domtest, assert.
3. **Implement the receiver method** with the signature implied by the call expression.
4. `go generate ./...` and `go test ./...`.

## POST/PATCH/DELETE routes — method-first

1. **Write the receiver method** first.
2. **Write the template** (route pattern, call expression, error rendering).
3. **Write the test** focusing on verifying the method was called correctly (`fake.MethodCallCount()`).

For form-based mutations (POST with form data), see [`muxt_forms`](../muxt_forms/SKILL.md).

## Choosing call parameters

See [Call Parameters Reference](../../reference/call-parameters.md) for the full table.

| Parameter | Type | Use when |
|-----------|------|----------|
| `ctx` | `context.Context` | Always (recommended first param) |
| `form` | struct | POST/PUT/PATCH with form data ([Forms](../muxt_forms/SKILL.md)) |
| `{param}` | string/int/custom | Path parameter extraction |
| `request` | `*http.Request` | Need headers, cookies, full request |
| `response` | `http.ResponseWriter` | Streaming, file downloads, custom headers |

## Choosing return types

See [Call Results Reference](../../reference/call-results.md) for the full table.

| Pattern | Use when |
|---------|----------|
| `(T, error)` | Most endpoints |
| `T` | Infallible operations (static pages) |
| `error` | No data needed (health checks, deletes) |
| `(T, bool)` | Early exit/redirect (`bool=false` skips template) |

## Setting status codes

Four ways, in precedence order:

1. **Template name** (static, most common):
   ```gotmpl
   {{define "POST /user 201 CreateUser(ctx, form)"}}...{{end}}
   ```
2. **`StatusCode() int` method** on the return type (dynamic).
3. **`StatusCode int` field** on the return type.
4. **In template** (for error cases): `{{with and (.StatusCode 404) .Err}}<div>Not found</div>{{end}}`.

Template name for static codes; methods/fields for dynamic codes.

## When to skip Muxt

For file downloads, streaming, or WebSockets, write custom handlers alongside Muxt routes:

```go
mux := http.NewServeMux()
TemplateRoutes(mux, srv)
mux.HandleFunc("GET /download/{id}", handleDownload)
```

Methods that need `response http.ResponseWriter` or `request *http.Request` can still use Muxt — add them as call parameters. But if the method is fundamentally about streaming bytes, a custom handler is simpler.

## Type-checked URLs

Use `$.Path` in templates for `href` and `action` instead of hardcoded strings — see `references/examples.md`. Compile-time checking: rename a route, `go generate` updates the methods, the compiler catches mismatches. In tests, `paths := TemplateRoutes(mux, receiver)` then `paths.GetArticle(1)`.

## Red-green-refactor checklist

1. **Red** — write a failing test (template + test case, no method body yet).
2. **Green** — implement the receiver method, run `go generate && go test`.
3. **Refactor** — extract sub-templates, simplify method logic.

A failing test may be an expected `go test` or `muxt check` compilation failure.

## Reference files

- `references/examples.md` — full GET template + GWT test, DELETE/PATCH method-first walkthroughs, route pattern syntax details, control-flow templates, template decomposition, type-checked URLs in tests, integration testing.

## External reference

- [Call Parameters](../../reference/call-parameters.md), [Call Results](../../reference/call-results.md), [Template Name Syntax](../../reference/template-names.md)
- [domtest](https://github.com/typelate/dom), [counterfeiter](https://github.com/maxbrunsfeld/counterfeiter)

### Test cases (`cmd/muxt/testdata/`)

| Feature | Test file |
|---------|-----------|
| Basic route with handler | `tutorial_basic_handler.txt` |
| Blog example with domtest | `tutorial_blog_example.txt` |
| Status codes in template names | `reference_status_codes.txt` |
| Path parameter types | `reference_path_with_typed_param.txt` |
| Custom TextUnmarshaler path param | `howto_arg_with_text_unmarshaler.txt` |
| Context, Request, Response parameters | `howto_arg_context.txt`, `howto_arg_request.txt`, `howto_arg_response.txt` |
| Error / bool returns | `reference_call_with_error_return.txt`, `reference_call_with_bool_return.txt` |
| Redirect helpers | `reference_redirect_helpers.txt` |
| Multiple template files / sub-template decomposition | `reference_multiple_template_files.txt`, `reference_multiple_hypermedia_children.txt` |

Form-related test cases: [Forms](../muxt_forms/SKILL.md#test-cases-cmdmuxttestdata).
