# Type Checking Reference

How `muxt check` validates template actions at compile time.

## Overview

`muxt check` performs static analysis of Go templates. Since `html/template` uses reflection at runtime, static analysis cannot catch all errors. Use concrete types to maximize type checking coverage.

## How It Works

1. **Find templates:** Scan for `ExecuteTemplate` calls with string literal names
2. **Extract data types:** Infer data type from call site (second parameter)
3. **Parse template:** Parse template source to AST
4. **Type check actions:** Validate field accesses, method calls, function calls against Go types

## Type Resolution

**Concrete types (checked):**
```go
func (s Server) GetUser(ctx context.Context) (User, error) {
    return User{Name: "Alice", Email: "alice@example.com"}, nil
}
```
```gotemplate
{{define "GET /user GetUser(ctx)"}}
<h1>{{.Result.Name}}</h1>       <!-- OK: User has Name field -->
<p>{{.Result.Email}}</p>        <!-- OK: User has Email field -->
<p>{{.Result.Phone}}</p>        <!-- ERROR: User has no Phone field -->
{{end}}
```

**Interface types (unchecked):**
```go
func (s Server) GetData(ctx context.Context) (any, error) {
    return SomeData{}, nil
}
```
```gotemplate
{{define "GET /data GetData(ctx)"}}
{{.Result.Anything}}  <!-- No error: any disables type checking -->
{{end}}
```

## Limitations

**Not supported:**
- `any` / `interface{}` fields — Type checking disabled
- Dynamic template names — `ExecuteTemplate(w, getTemplateName(), data)`
- Complex pipeline expressions — May produce false negatives
- JetBrains GoLand `gotype` comments — Not consulted

**Partially supported:**
- Range over maps — Key/value types inferred when map type is concrete
- Method calls on interfaces — Checked if interface type is known
- Template functions — Checked if registered in `Funcs()` call

## Best Practices

**Use concrete types:**
```go
// Good
func (s Server) GetUser(ctx context.Context) (User, error)
func (s Server) GetPosts(ctx context.Context) ([]Post, error)
func (s Server) GetStats(ctx context.Context) (map[string]int, error)

// Avoid
func (s Server) GetUser(ctx context.Context) (any, error)
func (s Server) GetData(ctx context.Context) (interface{}, error)
```

**Static template names:**
```go
// Good
templates.ExecuteTemplate(w, "user-profile", data)

// Cannot check
templateName := getTemplateName(r)
templates.ExecuteTemplate(w, templateName, data)
```

**Type all struct fields:**
```go
// Good
type User struct {
    Name  string
    Email string
}

// Avoid
type User struct {
    Name string
    Data any  // Disables checking for .Data accesses
}
```

## Example

**Method:**
```go
type Post struct {
    Title   string
    Author  string
    Content string
}

func (s Server) GetPost(ctx context.Context, id int) (Post, error) {
    return s.db.GetPost(ctx, id)
}
```

**Template:**
```gotemplate
{{define "GET /posts/{id} GetPost(ctx, id)"}}
<h1>{{.Result.Title}}</h1>         <!-- OK -->
<p>By {{.Result.Author}}</p>       <!-- OK -->
<div>{{.Result.Content}}</div>     <!-- OK -->
<span>{{.Result.PublishedAt}}</span>  <!-- ERROR: Post has no PublishedAt field -->
{{end}}
```

**Check output:**
```
Error: template action references undefined field: PublishedAt
  Template: GET /posts/{id} GetPost(ctx, id)
  Type: Post
```

## Related

- [check_types.txt](../../cmd/muxt/testdata/check_types.txt) — Type checking test cases
- [github.com/typelate/check](https://pkg.go.dev/github.com/typelate/check) — Type checker implementation
- [known-issues.md](known-issues.md) — Known type checking limitations