# muxt refactoring: examples

## Renaming a receiver method

### 1. Find references

```bash
# Templates that call the method
muxt list-template-callers --match "GetArticle"

# Go references via gopls (file:line:column points at the method definition)
gopls references main.go:19:20

# References to the generated interface method
gopls references template_routes.go:21:2

# All concrete implementations of the interface method
gopls implementation template_routes.go:21:2
```

### 2. Update the template name(s)

```gotmpl
{{/* Before */}}
{{define "GET /article/{id} GetArticle(ctx, id)"}}...{{end}}

{{/* After */}}
{{define "GET /article/{id} FetchArticle(ctx, id)"}}...{{end}}
```

### 3. Regenerate first (so the interface updates to the new name)

```bash
go generate ./...
```

### 4. Rename the method with gopls

```bash
gopls rename -d main.go:19:20 FetchArticle   # dry-run
gopls rename -w main.go:19:20 FetchArticle   # apply
```

If you rename *before* regenerating, gopls refuses — the old generated interface still uses the old name. Always update template + regenerate first.

Alternatively, rename via the generated interface method to cascade to all implementations:

```bash
gopls rename -w template_routes.go:21:2 FetchArticle
```

### 5. Update tests

```go
// Before
require.Equal(t, 1, app.GetArticleCallCount())

// After
require.Equal(t, 1, app.FetchArticleCallCount())
```

### 6. Regenerate + test

```bash
go generate ./...
go test ./...
```

## Changing a route pattern

### 1. Update the template name

```gotmpl
{{/* Before */}}
{{define "GET /article/{id} GetArticle(ctx, id)"}}...{{end}}

{{/* After */}}
{{define "GET /posts/{slug} GetArticle(ctx, slug)"}}...{{end}}
```

If the param name or type changes, update the method signature too.

### 2. Update `$.Path` callers in templates

```gotmpl
{{/* Before */}}
<a href="{{$.Path.GetArticle 42}}">View</a>

{{/* After — slug is now a string */}}
<a href="{{$.Path.GetArticle "my-post"}}">View</a>
```

### 3. Update tests — use `TemplateRoutePaths` methods, not hardcoded paths

```go
paths := TemplateRoutes(mux, receiver)

// Before
req := httptest.NewRequest(http.MethodGet, "/article/1", nil)

// After
req := httptest.NewRequest(http.MethodGet, paths.GetArticle("my-post"), nil)
```

### 4. Update the method signature (if types changed)

```go
// Before
func (s Server) GetArticle(ctx context.Context, id int) (Article, error) { ... }

// After
func (s Server) GetArticle(ctx context.Context, slug string) (Article, error) { ... }
```

## Moving templates between files

Muxt is file-agnostic within a package. Moving a `{{define}}` block between `.gohtml` files needs no code change — just move the block. **File order may impact overriding a template.** Ensure `//go:embed` covers the new file:

```go
//go:embed *.gohtml
var templateFS embed.FS
```

Subdirectory? Add it:

```go
//go:embed *.gohtml partials/*.gohtml
var templateFS embed.FS
```

Run `muxt check` after moving.

## Splitting into multiple packages

`--use-receiver-type-package` for cross-package receivers:

```bash
muxt generate --use-receiver-type=Handler --use-receiver-type-package=example.com/internal/app
```

Steps:
1. Move the receiver type and methods to the target package.
2. Update the `//go:generate` directive with the package flag.
3. `go generate ./...`.
4. Fix any import issues in the generated code.

## Adding or removing parameters

### Add

```gotmpl
{{/* Before */}}
{{define "GET /article/{id} GetArticle(ctx, id)"}}...{{end}}

{{/* After — added request parameter */}}
{{define "GET /article/{id} GetArticle(ctx, id, request)"}}...{{end}}
```

```go
func (s Server) GetArticle(ctx context.Context, id int, r *http.Request) (Article, error) { ... }
```

Regenerate + test.

### Remove

1. Remove from the template call expression.
2. Remove from the method signature.
3. Update any test code that depended on the parameter.
4. Regenerate + test.

## Cleanup after refactoring

Switching output strategies (single ↔ multi-file, custom output file): muxt cleans up orphaned generated files on the next `go generate`. The `//go:generate` directive controls it:

```go
// Single file output (default)
//go:generate muxt generate --use-receiver-type=Server

// Custom output file
//go:generate muxt generate --use-receiver-type=Server -o handlers_gen.go
```

If you change output strategies, delete the old generated files manually or let muxt's cleanup handle it.

## Analyzing coupling with `--format json` + jq

### List all route templates

```bash
muxt list-template-callers --format json | jq '[.Templates[].Name]'
```

### Find duplicate route patterns (same method + path → panic at registration)

```bash
muxt list-template-callers --format json | jq '
  [.Templates[].Name
   | select(test("^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS) "))
  ] | group_by(split(" ") | .[0:2] | join(" "))
    | map(select(length > 1))
    | .[]
'
```

### Find sub-templates shared by multiple routes

```bash
muxt list-template-calls --format json | jq '
  [.Templates[]
   | select(.Name | test("^(GET|POST|PUT|PATCH|DELETE) "))
   | {route: .Name, refs: [.References[].Name]}
  ] | [.[].refs[]]
    | group_by(.) | map(select(length > 1)) | map(.[0])
'
```

### Find all callers of a specific sub-template

```bash
muxt list-template-calls --format json | jq '
  [.Templates[]
   | select(.References[]?.Name == "edit-row")
   | .Name
  ]
'
```

### Find what a route template calls

```bash
muxt list-template-calls --match "SubmitFormEditRow" --format json | jq '
  [.Templates[].References[] | {name: .Name, data: .Data}]
'
```
