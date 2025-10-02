# Call Parameters

In template names, you add a call expression to specify which method handles the request and what parameters it receives.

The parameter names and types determine how Muxt generates the handler.

## Special Parameter Names

Muxt recognizes these special parameter names:

- `ctx` → `context.Context` from `request.Context()`
- `request` → `*http.Request`
- `response` → `http.ResponseWriter`
- `form` → `url.Values` from `request.Form`

Path parameters (like `{userID}` in the route) default to `string` but can be typed when using `--receiver-type`.

*[(See Muxt CLI Test/argument_context)](../../cmd/muxt/testdata/argument_context.txt)*
*[(See Muxt CLI Test/argument_request)](../../cmd/muxt/testdata/argument_request.txt)*
*[(See Muxt CLI Test/argument_response)](../../cmd/muxt/testdata/argument_response.txt)*

## Without Receiver Type

Without `--receiver-type`, Muxt generates an interface with `any` as the return type:

Template:
```gotemplate
{{define "GET /project/{projectID}/task/{taskID} F(ctx, response, request, projectID, taskID)"}}
Hello, world!
{{end}}
```

Generated interface:
```go
type RoutesReceiver interface {
  F(ctx context.Context, response http.ResponseWriter, request *http.Request, projectID string, taskID string) any
}
```

Path parameters are `string` by default.

*[(See Muxt CLI Test/argument_no_receiver)](../../cmd/muxt/testdata/argument_no_receiver.txt)*

## With Receiver Type

When you provide `--receiver-type=Server`, Muxt:
1. Looks up the method signature on `Server`
2. Generates parsers for typed path parameters
3. Uses the actual return type in the interface

Your receiver:
```go
package server

import (
  "context"
  "net/http"
)

type Server struct{}

type Data struct{}

func (Server) F(ctx context.Context, response http.ResponseWriter, request *http.Request, projectID uint32, taskID int8) Data {
	return Data{}
}
```

Template (same as before):
```gotemplate
{{define "GET /project/{projectID}/task/{taskID} F(ctx, response, request, projectID, taskID)"}}
Hello, world!
{{end}}
```

Generated interface:
```go
type RoutesReceiver interface {
    F(ctx context.Context, response http.ResponseWriter, request *http.Request, projectID uint32, taskID int8) Data
}
```

Notice `projectID` is now `uint32` and `taskID` is `int8`, matching your receiver method.

*[(See Muxt CLI Test/call_F_with_argument_path_param)](../../cmd/muxt/testdata/call_F_with_argument_path_param.txt)*

## Automatic Type Parsing

Muxt generates parsers for these types:

### Numeric Types

- `int`, `int8`, `int16`, `int32`, `int64`
- `uint`, `uint8`, `uint16`, `uint32`, `uint64`
- `float32`, `float64`

*[(See Muxt CLI Test/path_param_typed)](../../cmd/muxt/testdata/path_param_typed.txt)*

### Boolean

- `bool` - Parses "true"/"false", "1"/"0", "yes"/"no"

### String

- `string` - Passed through with no parsing

### Custom Types

If a type implements [`encoding.TextUnmarshaler`](https://pkg.go.dev/encoding#TextUnmarshaler), Muxt will use `UnmarshalText`:

```go
type UserID string

func (id *UserID) UnmarshalText(text []byte) error {
	// Custom parsing logic
	*id = UserID(strings.ToLower(string(text)))
	return nil
}
```

*[(See Muxt CLI Test/argument_text_encoder)](../../cmd/muxt/testdata/argument_text_encoder.txt)*

## Form Parameters

Parameters that aren't path variables or special names are parsed from form data:

Template:
```gotemplate
{{define "POST /login Login(ctx, username, password)"}}
```

Receiver:
```go
func (s Server) Login(ctx context.Context, username, password string) (Session, error) {
	// username and password come from form fields
}
```

Muxt calls `request.ParseForm()` and extracts values by parameter name.

*[(See Muxt CLI Test/F_is_defined_and_form_type_is_a_struct)](../../cmd/muxt/testdata/F_is_defined_and_form_type_is_a_struct.txt)*

## Form Structs

You can use a struct type for form parameters. Muxt will generate code to populate the struct fields from form values:

```go
type LoginForm struct {
	Username string
	Password string
	Remember bool
}

func (s Server) Login(ctx context.Context, form LoginForm) (Session, error) {
	// form fields automatically populated
}
```

Template:
```gotemplate
{{define "POST /login Login(ctx, form)"}}
```

Field names in the struct must match form field names (case-sensitive by default).

### Struct Tags for Form Fields

Use the `name` struct tag to map form fields with different names:

```go
type LoginForm struct {
	Username string `name:"user-name"`
	Password string `name:"user-pass"`
}
```

This allows form fields like `user-name` and `user-pass` to map to struct fields with Go-idiomatic names.

*[(See Muxt CLI Test/form_field_tag)](../../cmd/muxt/testdata/form_field_tag.txt)*
*[(See Muxt CLI Test/F_is_defined_and_form_type_is_a_struct)](../../cmd/muxt/testdata/F_is_defined_and_form_type_is_a_struct.txt)*

## Pointer Receivers

Muxt works with both value and pointer receivers:

```go
func (s Server) GetUser(ctx context.Context, id int) (User, error)   // Value receiver
func (s *Server) GetUser(ctx context.Context, id int) (User, error)  // Pointer receiver
```

*[(See Muxt CLI Test/method_receiver_is_a_pointer)](../../cmd/muxt/testdata/method_receiver_is_a_pointer.txt)*

## Embedded Methods

Methods from embedded fields are automatically discovered:

```go
type Auth struct{}
func (Auth) Login(ctx context.Context, username, password string) (Session, error)

type Server struct {
	Auth  // Embedded field
}
```

Templates can call `Login` on `Server` because it's promoted from the embedded `Auth` field.

*[(See Muxt CLI Test/receiver_embedded_field_method)](../../cmd/muxt/testdata/receiver_embedded_field_method.txt)*

## Mixing Parameter Types

You can mix path parameters, form parameters, and special parameters:

```gotemplate
{{define "POST /user/{id}/update UpdateUser(ctx, id, form)"}}
```

```go
type UpdateUserForm struct {
	Name  string
	Email string
}

func (s Server) UpdateUser(ctx context.Context, id int, form UpdateUserForm) error {
	// id from path, form fields from request body
}
```

## Parameter Validation

Muxt doesn't do validation. That's your receiver method's job:

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

Return errors. Let templates decide how to display them.

## Type Errors

If a parameter can't be parsed, Muxt returns `400 Bad Request`:

```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
```

```go
func (s Server) GetUser(ctx context.Context, id int) (User, error)
```

Request to `/user/abc` returns 400 because "abc" can't parse to `int`.

*[(See Muxt CLI Test/error_wrong_argument_type)](../../cmd/muxt/testdata/error_wrong_argument_type.txt)*

## More Examples

- [Multiple arguments](../../cmd/muxt/testdata/call_F_with_multiple_arguments.txt)
- [Form slices](../../cmd/muxt/testdata/F_is_defined_and_form_slice_field.txt)
- [Unsupported types](../../cmd/muxt/testdata/F_is_defined_and_form_has_unsupported_field_type.txt)
- [All CLI tests](../../cmd/muxt/testdata/)
