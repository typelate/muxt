# Call Results

How Muxt handles what your receiver methods return.

## Return Signatures

Muxt supports these return patterns:

```go
func (s Server) Method() T                  // Single value
func (s Server) Method() (T, error)         // Value and error
func (s Server) Method() (T, bool)          // Value and boolean
func (s Server) Method() error              // Error only
```

The first return value becomes `.Result` in your template. The error (if any) becomes `.Err`.

*[(See Muxt CLI Test/call_method_with_two_returns)](../../cmd/muxt/testdata/call_method_with_two_returns.txt)*

## Single Return Value

```go
func (s Server) GetUser(ctx context.Context, id int) User {
	return s.db.FindUser(id)
}
```

Template:
```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
<h1>{{.Result.Name}}</h1>
<p>{{.Result.Email}}</p>
{{end}}
```

`.Result` contains the User. `.Err` is always nil.

## Value and Error

```go
func (s Server) GetUser(ctx context.Context, id int) (User, error) {
	user, err := s.db.FindUser(id)
	if err != nil {
		return User{}, fmt.Errorf("user not found: %w", err)
	}
	return user, nil
}
```

Template:
```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  <h1>{{.Result.Name}}</h1>
  <p>{{.Result.Email}}</p>
{{end}}
{{end}}
```

`.Result` contains the User (zero value if error). `.Err` contains the error (nil if success).

*[(See Muxt CLI Test/call_expression_argument_with_error_last_result)](../../cmd/muxt/testdata/call_expression_argument_with_error_last_result.txt)*

## Value and Boolean

```go
func (s Server) GetUser(ctx context.Context, id int) (User, bool) {
	user, ok := s.cache.Get(id)
	return user, ok
}
```

If the boolean is `true`, the handler returns early without executing the template.

Use this for cache hits, redirects, or cases where you've already written the response.

*[(See Muxt CLI Test/call_expression_argument_with_bool_last_result)](../../cmd/muxt/testdata/call_expression_argument_with_bool_last_result.txt)*

## Error Only

```go
func (s Server) Healthcheck() error {
	if err := s.db.Ping(); err != nil {
		return err
	}
	return nil
}
```

Template:
```gotemplate
{{define "GET /health Healthcheck()"}}
{{if .Err}}
  <div>Unhealthy: {{.Err.Error}}</div>
{{else}}
  <div>OK</div>
{{end}}
{{end}}
```

`.Result` is `struct{}` (empty). `.Err` contains the error (nil if healthy).

## TemplateData Structure

Your template receives a `TemplateData[T]` where `T` is the first return value:

```go
type TemplateData[T any] struct {
	// Accessible fields
	Result  T
	Err     error
	request *http.Request
	// ... other internal fields
}

func (d TemplateData[T]) Request() *http.Request {
	return d.request
}
```

In templates:
- `.Result` - The returned value
- `.Err` - The returned error
- `.Request` - The HTTP request

## Custom Error Types with Status Codes

Implement `StatusCode() int` on your error types to control HTTP status:

```go
type NotFoundError struct {
	Message string
}

func (e NotFoundError) Error() string {
	return e.Message
}

func (e NotFoundError) StatusCode() int {
	return http.StatusNotFound
}

func (s Server) GetUser(ctx context.Context, id int) (User, error) {
	user, err := s.db.FindUser(id)
	if err != nil {
		return User{}, NotFoundError{Message: "user not found"}
	}
	return user, nil
}
```

The handler automatically uses `404 Not Found` when the error is returned.

## Custom Result Types with Status Codes

Implement `StatusCode() int` on result types:

```go
type UserResult struct {
	User User
	code int
}

func (r UserResult) StatusCode() int {
	return r.code
}

func (s Server) GetUser(ctx context.Context, id int) (UserResult, error) {
	user, err := s.db.FindUser(id)
	if err != nil {
		return UserResult{code: 404}, err
	}
	return UserResult{User: user, code: 200}, nil
}
```

Or use a `StatusCode` field:

```go
type UserResult struct {
	User       User
	StatusCode int
}

func (s Server) GetUser(ctx context.Context, id int) (UserResult, error) {
	user, err := s.db.FindUser(id)
	if err != nil {
		return UserResult{StatusCode: 404}, err
	}
	return UserResult{User: user, StatusCode: 200}, nil
}
```

## Accessing the Request

Use `.Request` to access HTTP request data in templates:

```gotemplate
{{define "GET /profile Profile(ctx)"}}
<p>Request path: {{.Request.URL.Path}}</p>
<p>User agent: {{.Request.Header.Get "User-Agent"}}</p>

{{if .Request.Header.Get "HX-Request"}}
  <!-- Partial response for HTMX -->
  <div>{{.Result.Name}}</div>
{{else}}
  <!-- Full page -->
  <!DOCTYPE html>
  <html>...</html>
{{end}}
{{end}}
```

## Result Type Requirements

For `muxt check` to work, your result types should be concrete:

**Good:**
```go
func (s Server) GetUser(ctx context.Context) (User, error)
func (s Server) GetUsers(ctx context.Context) ([]User, error)
func (s Server) GetStats(ctx context.Context) (map[string]int, error)
```

**Avoid:**
```go
func (s Server) GetUser(ctx context.Context) (any, error)          // type checker can't help
func (s Server) GetUser(ctx context.Context) (interface{}, error)  // type checker can't help
```

Return static types. Let the type checker verify your templates.

## Unsupported Return Types

Muxt rejects certain composite return types in the second position:

**Not allowed:**
```go
func (s Server) Method() (T, chan error)   // channels not supported
func (s Server) Method() (T, []error)      // slices not supported
func (s Server) Method() (T, map[string]error) // maps not supported
```

**Allowed:**
```go
func (s Server) Method() (T, error)  // error
func (s Server) Method() (T, bool)   // bool
```

*[(See Muxt CLI Test/F_returns_a_value_and_an_unsupported_type)](../../cmd/muxt/testdata/F_returns_a_value_and_an_unsupported_type.txt)*
*[(See Muxt CLI Test/F_returns_a_value_and_an_unsupported_composite_type)](../../cmd/muxt/testdata/F_returns_a_value_and_an_unsupported_composite_type.txt)*

## More Examples

- [Boolean returns](../../cmd/muxt/testdata/F_returns_a_value_and_a_boolean.txt)
- [Import result types](../../cmd/muxt/testdata/result_import_result_type.txt)
- [Named result types](../../cmd/muxt/testdata/result_named_result_type.txt)
- [All CLI tests](../../cmd/muxt/testdata/)
