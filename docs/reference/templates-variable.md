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

[reference_template_multiple_parsefs.txt](../../cmd/muxt/testdata/reference_template_multiple_parsefs.txt)

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
