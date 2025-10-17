# Call Parameters Reference

Parameters in call expressions determine how Muxt generates handlers and parses request data. Use this reference when reviewing parameter bindings with team members.

## Parameter Binding Quick Reference

| Parameter Name | Type | Source | Parsed | Use When |
|----------------|------|--------|--------|----------|
| `ctx` | `context.Context` | `request.Context()` | N/A | Need request context (always recommended first param) |
| `request` | `*http.Request` | Direct | N/A | Need headers, cookies, or full request |
| `response` | `http.ResponseWriter` | Direct | N/A | Streaming, file downloads, custom headers |
| `form` | struct or `url.Values` | `request.Form` | Yes | Bind all form fields at once |
| Path param | Any parseable | `request.PathValue(name)` | Yes | Extract from URL path |
| Form field | Any parseable | `request.Form.Get(name)` | Yes | Individual form field |

Parameter names in template call must match method signature exactly.

[howto_arg_context.txt](../../cmd/muxt/testdata/howto_arg_context.txt) · [howto_arg_request.txt](../../cmd/muxt/testdata/howto_arg_request.txt) · [howto_arg_response.txt](../../cmd/muxt/testdata/howto_arg_response.txt)

## Type Resolution

**Without `--use-receiver-type`:** Path params are `string`, return types are `any`

```gotemplate
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

[reference_arg_text_unmarshaler.txt](../../cmd/muxt/testdata/reference_arg_text_unmarshaler.txt)

## Form Parameters

**Generic url.Values for fields:**
```gotemplate
{{define "POST /login Login(ctx, form)"}}{{end}}
```
```go
func (s Server) Login(ctx context.Context, form url.Values) (Session, error) {
    // username, password from request.Form.Get("username"), request.Form.Get("password")
}
```

**Struct binding:**
```gotemplate
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

[reference_form_with_struct_binding.txt](../../cmd/muxt/testdata/reference_form_with_struct_binding.txt) · [reference_form_field_tag.txt](../../cmd/muxt/testdata/reference_form_field_tag.txt)

## Advanced Patterns

**Mixing path, form, and special parameters:**
```gotemplate
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
- [reference_arg_text_unmarshaler.txt](../../cmd/muxt/testdata/reference_arg_text_unmarshaler.txt) — Custom `TextUnmarshaler`

**Forms:**
- [reference_form_with_struct_binding.txt](../../cmd/muxt/testdata/reference_form_with_struct_binding.txt) — Struct form binding
- [reference_form_field_tag.txt](../../cmd/muxt/testdata/reference_form_field_tag.txt) — `name` tag mapping
- [reference_form_with_slice_field.txt](../../cmd/muxt/testdata/reference_form_with_slice_field.txt) — Form slices
- [err_form_unsupported_field_type.txt](../../cmd/muxt/testdata/err_form_unsupported_field_type.txt) — Unsupported types

**Multiple arguments:**
- [howto_call_with_multiple_args.txt](../../cmd/muxt/testdata/howto_call_with_multiple_args.txt) — Multiple params

**Receiver types:**
- [reference_receiver_with_pointer.txt](../../cmd/muxt/testdata/reference_receiver_with_pointer.txt) — Pointer receivers
- [reference_receiver_with_embedded_method.txt](../../cmd/muxt/testdata/reference_receiver_with_embedded_method.txt) — Embedded methods

**Errors:**
- [err_arg_type_mismatch.txt](../../cmd/muxt/testdata/err_arg_type_mismatch.txt) — Parse errors

**Browse all:** [cmd/muxt/testdata/](../../cmd/muxt/testdata/)
