# Template Variable Requirements

Requirements for the global `*template.Template` variable that Muxt uses to find route templates.

## Required Pattern

Muxt requires a **package-level variable** with type `*template.Template`:

```go
package server

import (
	"embed"
	"html/template"
)

//go:embed *.gohtml
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))
```

[reference_template_embed_gen_decl.txt](../../cmd/muxt/testdata/reference_template_embed_gen_decl.txt)

## Requirements

| Requirement | Details |
|-------------|---------|
| **Scope** | Package-level (not function-level) |
| **Type** | `*template.Template` |
| **Initialization** | Assignment expression (not const) |
| **First argument** | `embed.FS` variable when using `ParseFS` |

## Supported Functions

**Package functions:**
- [`template.Must`](https://pkg.go.dev/html/template#Must)
- [`template.New`](https://pkg.go.dev/html/template#New)
- [`template.ParseFS`](https://pkg.go.dev/html/template#ParseFS)
- [`template.Parse`](https://pkg.go.dev/html/template#Parse)

**Template methods:**
- [`Template.ParseFS`](https://pkg.go.dev/html/template#Template.ParseFS)
- [`Template.Parse`](https://pkg.go.dev/html/template#Template.Parse)
- [`Template.New`](https://pkg.go.dev/html/template#Template.New)
- [`Template.Funcs`](https://pkg.go.dev/html/template#Template.Funcs)
- [`Template.Delims`](https://pkg.go.dev/html/template#Template.Delims)
- [`Template.Option`](https://pkg.go.dev/html/template#Template.Option)

## Common Patterns

**Single directory:**
```go
//go:embed *.gohtml
var fs embed.FS
var templates = template.Must(template.ParseFS(fs, "*.gohtml"))
```

**Nested directories:**
```go
//go:embed *.gohtml pages/*.gohtml components/*.gohtml
var fs embed.FS
var templates = template.Must(template.ParseFS(fs, "*.gohtml", "pages/*.gohtml", "components/*.gohtml"))
```

[reference_template_with_multiple_parsefs.txt](../../cmd/muxt/testdata/reference_template_with_multiple_parsefs.txt)

**With custom functions:**
```go
var templates = template.Must(
	template.New("").
		Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }}).
		ParseFS(fs, "*.gohtml"),
)
```

**Different variable name:**
```bash
muxt generate --use-templates-variable=myTemplates
```
```go
var myTemplates = template.Must(template.ParseFS(fs, "*.gohtml"))
```

## Multiple Template Variables

`--use-templates-variable` can be passed multiple times. Each variable becomes the root of an independent namespace.

```bash
muxt generate --use-templates-variable=adminTemplates --use-templates-variable=publicTemplates
```

```go
//go:embed admin/*.gohtml
var adminFS embed.FS
var adminTemplates = template.Must(template.ParseFS(adminFS, "admin/*.gohtml"))

//go:embed public/*.gohtml
var publicFS embed.FS
var publicTemplates = template.Must(template.ParseFS(publicFS, "public/*.gohtml"))
```

Each generated handler calls `ExecuteTemplate` on its own variable, so each variable carries its own:

- **Template name namespace** — `{{define "header"}}` in `adminTemplates` does not collide with `{{define "header"}}` in `publicTemplates`. `*template.Template` has a global namespace; later definitions silently overwrite earlier ones, so without separate variables every name in the app must be globally unique.
- **`Funcs` map** — register admin-only template functions on `adminTemplates` without exposing them to public pages.
- **Template `Option`s** — e.g. `Option("missingkey=error")` can apply to one set without forcing the same on the other.
- **`embed.FS`** — each variable can pull from a different filesystem, including filesystems exported by separate Go packages.

This is what makes templates portable across projects: a reusable admin component can ship its own template variable with its own names and helpers, and a consumer can mount it without worrying about name collisions in their own template set.

Routes from all variables are combined into a single generated `TemplateRoutes` function. Duplicate route patterns across variables are detected at generation time — if both `adminTemplates` and `publicTemplates` define `GET /`, generation fails with a `duplicate route pattern` error.

[reference_multiple_templates_variables.txt](../../cmd/muxt/testdata/reference_multiple_templates_variables.txt)
[err_duplicate_route_different_variables.txt](../../cmd/muxt/testdata/err_duplicate_route_different_variables.txt)

## How Muxt Uses This

**`muxt check`:**
- Scans for `ExecuteTemplate` calls with string literals
- Maps template names to data types
- Works without finding template variable

**`muxt generate`:**
- Finds template variable by name (`--use-templates-variable` flag)
- Parses embedded files to find route templates
- Generates handlers for templates matching route pattern

## Troubleshooting

**Variable not found:**
- Ensure variable is package-level (not in function)
- Verify variable name matches `--use-templates-variable` flag
- Check variable type is `*template.Template`

**Templates not discovered:**
- Verify `embed.FS` variable has correct `//go:embed` directive
- Ensure glob patterns in `ParseFS` match your files
- Check template names follow route syntax (see [template-names.md](template-names.md))

If your configuration differs from these patterns, [open an issue](https://github.com/typelate/muxt/issues/new) with your setup.
