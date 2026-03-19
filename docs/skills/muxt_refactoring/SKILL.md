---
name: muxt-refactoring
description: "Muxt: Use when renaming receiver methods, changing route patterns, moving templates between files, splitting packages, or adding/removing method parameters in a Muxt codebase. Guides safe refactoring through Muxt's template-method coupling chain."
---

# Refactoring Routes

Muxt couples templates, route patterns, and receiver methods by name. Refactoring any one of these requires updating the others. This skill guides safe refactoring through the coupling chain.

## Safe Refactoring Loop

After every change, run this loop:

```bash
go generate ./...    # regenerate handler code
muxt check           # type-check templates against generated code
go test ./...        # run tests
```

`muxt check` validates against the already-generated code, so it must run after `go generate`. Stop and fix any errors before proceeding to the next change.

## Renaming a Receiver Method

### 1. Find All References

Find templates that call the method:

```bash
muxt list-template-callers --match "GetArticle"
```

Find Go references to the method using gopls. Point at the method definition:

```bash
# References to a concrete method (file:line:column)
gopls references main.go:19:20

# References to the generated interface method
gopls references template_routes.go:21:2

# Find all concrete implementations of the interface method
gopls implementation template_routes.go:21:2
```

### 2. Update the Template Name

Change the method name in every template that calls it:

```gotmpl
{{/* Before */}}
{{define "GET /article/{id} GetArticle(ctx, id)"}}...{{end}}

{{/* After */}}
{{define "GET /article/{id} FetchArticle(ctx, id)"}}...{{end}}
```

### 3. Regenerate First

Regenerate so the `RoutesReceiver` interface updates to match the new template name:

```bash
go generate ./...
```

### 4. Rename the Method with gopls

Now rename the concrete method. Because the generated interface already uses the new name, gopls can safely rename the implementation:

```bash
# Dry-run: preview the rename diff
gopls rename -d main.go:19:20 FetchArticle

# Apply the rename
gopls rename -w main.go:19:20 FetchArticle
```

If you rename the method *before* regenerating, gopls will refuse because the old generated interface still uses the old name and would break the interface constraint. Always update the template name and regenerate first.

Alternatively, rename via the generated interface method to cascade to all implementations:

```bash
gopls rename -w template_routes.go:21:2 FetchArticle
```

### 5. Update Tests

Update test assertions that reference the method (e.g., counterfeiter call counts):

```go
// Before
require.Equal(t, 1, app.GetArticleCallCount())

// After
require.Equal(t, 1, app.FetchArticleCallCount())
```

### 6. Regenerate and Test

```bash
go generate ./...
go test ./...
```

## Changing a Route Pattern

### 1. Update the Template Name

```gotmpl
{{/* Before */}}
{{define "GET /article/{id} GetArticle(ctx, id)"}}...{{end}}

{{/* After */}}
{{define "GET /posts/{slug} GetArticle(ctx, slug)"}}...{{end}}
```

If the parameter name or type changes, update the method signature too.

### 2. Update `$.Path` Callers

Search templates for `$.Path.GetArticle` and update the arguments:

```gotmpl
{{/* Before */}}
<a href="{{$.Path.GetArticle 42}}">View</a>

{{/* After — slug is now a string */}}
<a href="{{$.Path.GetArticle "my-post"}}">View</a>
```

### 3. Update Tests

Use `TemplateRoutePaths` methods in tests instead of hardcoded paths:

```go
paths := TemplateRoutes(mux, receiver)

// Before
req := httptest.NewRequest(http.MethodGet, "/article/1", nil)

// After — uses the generated path method
req := httptest.NewRequest(http.MethodGet, paths.GetArticle("my-post"), nil)
```

### 4. Update the Method Signature (if types changed)

```go
// Before
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) { ... }

// After
func (s Server) GetArticle(ctx context.Context, slug string) (Article, error) { ... }
```

## Moving Templates Between Files

Muxt is file-agnostic within a package. Moving a `{{define}}` block between `.gohtml` files doesn't require any code changes — just move the block.
File order may impact overriding a template.
Ensure the `//go:embed` directive covers the new file:

```go
//go:embed *.gohtml
var templateFS embed.FS
```

If you're adding a subdirectory, add it to the embed pattern:

```go
//go:embed *.gohtml partials/*.gohtml
var templateFS embed.FS
```

After moving, run `muxt check` to verify nothing broke.

## Splitting Into Multiple Packages

Use `--use-receiver-type-package` for cross-package receivers:

```bash
muxt generate --use-receiver-type=Handler --use-receiver-type-package=example.com/internal/app
```

This tells Muxt to look up the receiver type in a different package than the templates.

### Steps

1. Move the receiver type and methods to the target package
2. Update the `//go:generate` directive with the package flag
3. Run `go generate ./...`
4. Fix any import issues in the generated code

## Adding or Removing Parameters

### Adding a Parameter

1. Add the parameter to the template call expression:

```gotmpl
{{/* Before */}}
{{define "GET /article/{id} GetArticle(ctx, id)"}}...{{end}}

{{/* After — added request parameter */}}
{{define "GET /article/{id} GetArticle(ctx, id, request)"}}...{{end}}
```

2. Add the parameter to the method signature:

```go
func (s Server) GetArticle(ctx context.Context, id int, r *http.Request) (Article, error) { ... }
```

3. Regenerate and test.

### Removing a Parameter

1. Remove from the template call expression
2. Remove from the method signature
3. Update any test code that depends on the parameter
4. Regenerate and test

## Cleanup After Refactoring

When you change the generated output file name or switch between single-file and multi-file output, Muxt cleans up orphaned generated files. The `//go:generate` directive controls this:

```go
// Single file output (default)
//go:generate muxt generate --use-receiver-type=Server

// Custom output file
//go:generate muxt generate --use-receiver-type=Server -o handlers_gen.go
```

If you switch output strategies, delete the old generated files manually or let Muxt's cleanup handle it on the next `go generate`.

## Finding Duplicates and Coupling

Use `muxt list-template-calls` and `muxt list-template-callers` with `--format json` and jq to analyze template relationships before refactoring.

### List All Route Templates

```bash
muxt list-template-callers --format json | jq '[.Templates[].Name]'
```

### Find Duplicate Route Patterns

Two templates with the same HTTP method + path cause a panic at registration. Find them before `go generate`:

```bash
muxt list-template-callers --format json | jq '
  [.Templates[].Name
   | select(test("^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS) "))
  ] | group_by(split(" ") | .[0:2] | join(" "))
    | map(select(length > 1))
    | .[]
'
```

### Find Sub-templates Shared by Multiple Routes

Shared sub-templates create coupling between routes. Before renaming or removing a sub-template, check which routes depend on it:

```bash
muxt list-template-calls --format json | jq '
  [.Templates[]
   | select(.Name | test("^(GET|POST|PUT|PATCH|DELETE) "))
   | {route: .Name, refs: [.References[].Name]}
  ] | [.[].refs[]]
    | group_by(.) | map(select(length > 1)) | map(.[0])
'
```

### Find All Callers of a Sub-template

Once you know a sub-template is shared, find exactly which routes call it:

```bash
muxt list-template-calls --format json | jq '
  [.Templates[]
   | select(.References[]?.Name == "edit-row")
   | .Name
  ]
'
```

### Find What a Route Template Calls

```bash
muxt list-template-calls --match "SubmitFormEditRow" --format json | jq '
  [.Templates[].References[] | {name: .Name, data: .Data}]
'
```

## Reference

- [Call Parameters](../../reference/call-parameters.md)
- [Call Results](../../reference/call-results.md)
- [Template Name Syntax](../../reference/template-names.md)
- [Debug Generation Errors](../muxt_debug-generation-errors/SKILL.md) — When refactoring triggers errors

### Test Cases (`cmd/muxt/testdata/`)

| Feature | Test File |
|---------|-----------|
| Receiver in different package | `reference_receiver_with_different_package.txt` |
| Multiple generated route files | `reference_multiple_generated_routes.txt` |
| Multiple template files | `reference_multiple_template_files.txt` |
| Cleanup: orphaned files | `reference_cleanup_orphaned_files.txt` |
| Cleanup: routes func change | `reference_cleanup_routes_func_change.txt` |
| Cleanup: different routes func | `reference_cleanup_different_routes_func.txt` |
| Cleanup: switch to multiple files | `reference_cleanup_switch_to_multiple_files.txt` |
| Cleanup: switch to single file | `reference_cleanup_switch_to_single_file.txt` |
| Receiver with pointer | `reference_receiver_with_pointer.txt` |
| Receiver with embedded method | `reference_receiver_with_embedded_method.txt` |
