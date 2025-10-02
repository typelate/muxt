# How to Write Receiver Methods

This guide shows you how to write clean, testable receiver methods for Muxt-generated handlers.

## Goal

Create receiver methods that:
- Return domain-oriented data instead of HTTP primitives
- Are easy to test without mocking HTTP
- Set appropriate status codes
- Work well with Muxt's type checking

## Prefer Domain Types Over HTTP Types

**Do this:**

```go
func (s Server) CreateUser(ctx context.Context, form CreateUserForm) (User, error) {
	// Implementation focuses on business logic
	user, err := s.db.InsertUser(ctx, form.Username, form.Email)
	return user, err
}
```

**Avoid this:**

```go
func (s Server) CreateUser(response http.ResponseWriter, request *http.Request) {
	// Tightly coupled to HTTP, harder to test
	form := parseForm(request)
	user, err := s.db.InsertUser(request.Context(), form.Username, form.Email)
	if err != nil {
		http.Error(response, err.Error(), 500)
		return
	}
	json.NewEncoder(response).Encode(user)
}
```

**Why?** Domain-oriented methods are easier to test, reuse, and reason about.

## How to Set HTTP Status Codes

Muxt provides multiple ways to control the response status code without using `http.ResponseWriter`.

### Method 1: Specify in Template Name

Set the expected success status code directly in the template name:

```gotemplate
{{define "POST /user 201 CreateUser(ctx, form)"}}
<!-- User created successfully -->
<div>User {{.Username}} created!</div>
{{end}}
```

The generated handler will use `201 Created` for successful responses.

### Method 2: StatusCode() Method on Result

Implement a `StatusCode() int` method on your result type:

```go
type UserResult struct {
	Username string
	Email    string
	code     int
}

func (r UserResult) StatusCode() int {
	return r.code
}

func (s Server) GetUser(ctx context.Context, id int) (UserResult, error) {
	user, err := s.db.GetUser(ctx, id)
	if err != nil {
		return UserResult{code: 404}, err
	}
	return UserResult{
		Username: user.Username,
		Email:    user.Email,
		code:     200,
	}, nil
}
```

### Method 3: StatusCode Field on Result

Add a `StatusCode int` field to your result struct:

```go
type UserResult struct {
	Username   string
	Email      string
	StatusCode int
}

func (s Server) GetUser(ctx context.Context, id int) (UserResult, error) {
	user, err := s.db.GetUser(ctx, id)
	if err != nil {
		return UserResult{StatusCode: 404}, err
	}
	return UserResult{
		Username:   user.Username,
		Email:      user.Email,
		StatusCode: 200,
	}, nil
}
```

### Method 4: Set Status in Template

For conditional status codes, use template helpers:

```gotemplate
{{define "GET /user/{id} GetUser(ctx, id)"}}
  {{- if .Err}}
    {{- with and (.StatusCode 400) (.Header "HX-Retarget" "#error")}}
      <div id='error'>{{.Err.Error}}</div>
    {{- end}}
  {{- else}}
    {{template "user-profile" .Result}}
  {{- end}}
{{end}}
```

## How to Handle Errors

Return errors from your methods—Muxt will handle them appropriately:

```go
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) {
	article, err := s.db.GetArticle(ctx, id)
	if err != nil {
		return Article{}, fmt.Errorf("article not found: %w", err)
	}
	return article, nil
}
```

In your template, check for errors:

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
<!DOCTYPE html>
<html>
<body>
  {{if .Err}}
    <div class="error">Error: {{.Err.Error}}</div>
  {{else}}
    <h1>{{.Result.Title}}</h1>
    <p>{{.Result.Content}}</p>
  {{end}}
</body>
</html>
{{end}}
```

**Note:** The template is fully rendered even when an error occurs. 

## How to Parse Form Parameters

Muxt automatically parses form and path parameters based on your method signature.

### Path Parameters

```gotemplate
{{define "GET /user/{id}/post/{postID} GetPost(ctx, id, postID)"}}
```

```go
func (s Server) GetPost(ctx context.Context, id int, postID int) (Post, error) {
	// `id` and `postID` are automatically parsed from the URL path
	return s.db.GetPost(ctx, id, postID)
}
```

### Form Parameters

```gotemplate
{{define "POST /login Login(ctx, username, password)"}}
```

```go
func (s Server) Login(ctx context.Context, username, password string) (Session, error) {
	// `username` and `password` are parsed from form fields
	session, err := s.auth.Login(ctx, username, password)
	return session, err
}
```

Muxt supports automatic parsing for:
- `int`, `int8`, `int16`, `int32`, `int64`
- `uint`, `uint8`, `uint16`, `uint32`, `uint64`
- `bool`
- `string`
- Types implementing `encoding.TextUnmarshaler`

## When to Use http.ResponseWriter

For special cases like file downloads or streaming:

```go
func (s Server) DownloadFile(response http.ResponseWriter, request *http.Request, id string) error {
	file, err := s.storage.Get(id)
	if err != nil {
		return err
	}

	response.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", file.Name))
	response.Header().Set("Content-Type", file.MimeType)
	response.WriteHeader(http.StatusOK)
	// Note: Don't write body here - return data for template to handle
	return nil
}
```

**When to skip Muxt entirely:** For file downloads, streaming responses, or WebSocket connections, you're better off writing a custom handler. Copy the generated code as a starting point if helpful, but register it separately.

If you need full control, register a custom handler outside the generated routes:

```go
func main() {
	mux := http.NewServeMux()
	hypertext.Routes(mux, srv)

	// Custom handler for special cases
	mux.HandleFunc("GET /download/{id}", handleDownload)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

## Tips for Clean Receiver Methods

**Return static types** - Avoid `any` or `interface{}`. The type checker can't help you with `any`. Give it real types.

**Keep HTTP concerns in templates** - Methods return data. Templates decide how to render it. Clean separation.

**Use context.Context** - Go proverb: "Accept interfaces, return structs." Context is the exception—it's the first parameter by convention.

**Handle errors explicitly** - Errors are values. Return them. Let the template decide how to display them.

**Test without HTTP** - Your receiver methods shouldn't need `httptest`. They're just methods. Test them like methods.

## Next Steps

- [Test your handlers](test-handlers.md) with `domtest`
- Review [call parameters reference](../reference/call-parameters.md) for parameter type details
- Review [call results reference](../reference/call-results.md) for return type handling
