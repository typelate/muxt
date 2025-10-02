# Package Structure for Templates

Understanding how to organize templates and the package-level requirements for Muxt code generation.

## The Package-Level Templates Variable

Muxt requires a **package-level variable** of type `*html/template.Template` to discover and analyze your templates.
This variable can be initialized in two ways:
1. Using `template.ParseFS` with an `embed.FS` for file-based templates
2. Using `template.Parse` for inline string templates

### Why Package-Level?

Muxt uses Go's static analysis to find your templates at generation time. It searches for:
1. A package-level variable declaration
2. Of type `*html/template.Template`
3. Initialized with `template.ParseFS`, `template.Parse`, or wrapped with `template.Must(...)`

**This works:**
```go
//go:embed *.gohtml
var templatesDir embed.FS

var templates = template.Must(template.ParseFS(templatesDir, "*.gohtml"))
```

**This doesn't work (function-local):**
```go
func loadTemplates() *template.Template {
    // Muxt can't find this
    return template.Must(template.ParseFS(templatesDir, "*.gohtml"))
}
```

## Embed Package Limitations

The `embed` package has strict rules about what files it can include. These limitations affect how you organize your templates.

### Sibling and Child Files Only

The `//go:embed` directive can only include:
- Files in the **same directory** as the `.go` file
- Files in **subdirectories** (children)

It **cannot** include:
- Parent directories
- Sibling directories of parent directories

**Valid structure:**
```
internal/hypertext/
├── templates.go          (has //go:embed)
├── index.gohtml          (✓ sibling)
├── pages/
│   └── dashboard.gohtml  (✓ child)
└── components/
    └── nav.gohtml        (✓ child)
```

**Invalid structure:**
```
internal/
├── hypertext/
│   └── templates.go      (has //go:embed)
└── templates/            (✗ cannot embed ../templates)
    └── index.gohtml
```

### Embedding Multiple Directories

To include templates from multiple subdirectories, use multiple patterns or a wildcard:

```go
//go:embed pages/*.gohtml components/*.gohtml partials/*.gohtml
var templatesDir embed.FS

var templates = template.Must(template.ParseFS(templatesDir, "*/*.gohtml"))
```

Or simpler:
```go
//go:embed */*.gohtml *.gohtml
var templatesDir embed.FS

var templates = template.Must(template.ParseFS(templatesDir, "**/*.gohtml", "*.gohtml"))
```

## Separating Parsing Logic from Configuration

You can parse templates in a separate function while keeping the package-level variable for Muxt discovery.

### Pattern: Parser Function with Package-Level Variable

```go
package hypertext

import (
    "embed"
    "html/template"
)

//go:embed */*.gohtml *.gohtml
var templatesDir embed.FS

//go:generate muxt generate --receiver-type=App --routes-func=Routes
var templates = parseTemplates()

// parseTemplates configures template parsing with custom settings
func parseTemplates() *template.Template {
    tmpl := template.New("").Funcs(template.FuncMap{
        "formatDate": formatDate,
        "markdown":   renderMarkdown,
    })

    // Custom delimiters if needed
    tmpl.Delims("[[", "]]")

    return template.Must(tmpl.ParseFS(templatesDir, "**/*.gohtml", "*.gohtml"))
}

func formatDate(t time.Time) string {
    return t.Format("2006-01-02")
}

func renderMarkdown(s string) template.HTML {
    // ... markdown rendering logic consider https://github.com/microcosm-cc/bluemonday and https://github.com/russross/blackfriday
	return ""
}
```

**Why this works:**
- The package-level `templates` variable is visible to Muxt
- Muxt can trace the initialization to `parseTemplates()`
- Muxt discovers the `embed.FS` variable and `ParseFS` patterns
- Your parsing logic stays clean and testable

### What Muxt Needs to Discover

Muxt performs static analysis to find:

1. **The embed directive and variable** - to know which files are templates
2. **The ParseFS call** - to understand which patterns load which templates
3. **Template configuration** - custom delimiters, function maps (for advanced type checking)

The package-level variable is the entry point for this discovery. As long as Muxt can trace from the variable to the `embed.FS` and `ParseFS` call, your setup will work.

## Common Patterns

### Pattern 1: Inline Templates

```go
//go:generate muxt generate --receiver-type=Client
var templates = template.Must(template.New("GET / List()").Parse(`
<ul>
{{range .Result}}
	<li>{{.Name}}</li>
{{end}}
</ul>

{{- define "POST /items CreateItem(name)" -}}
<div>Item {{.Result.Name}} created</div>
{{- end -}}
`))
```

**Use when:**
- Small projects or prototypes
- All templates fit comfortably in one string
- You want templates and code in the same file for quick iteration

**Note:** The first template name must be provided to `template.New()`. Additional templates are defined using `{{define "name"}}...{{end}}` within the string.

*[(See Muxt CLI Test/import_v2_module)](../../cmd/muxt/testdata/import_v2_module.txt)*

### Pattern 2: Simple Single Directory

```go
//go:embed *.gohtml
var templatesDir embed.FS

//go:generate muxt generate
var templates = template.Must(template.ParseFS(templatesDir, "*.gohtml"))
```

**Use when:** All templates are in one directory and you want them in separate files

### Pattern 3: Subdirectories with Common Patterns

```go
//go:embed pages/*.gohtml components/*.gohtml layouts/*.gohtml
var templatesDir embed.FS

//go:generate muxt generate --receiver-type=App
var templates = template.Must(template.ParseFS(templatesDir,
    "pages/*.gohtml",
    "components/*.gohtml",
    "layouts/*.gohtml",
))
```

**Use when:** Templates are organized into logical subdirectories

### Pattern 4: All Templates Recursively

```go
//go:embed **/*.gohtml
var templatesDir embed.FS

//go:generate muxt generate --receiver-type=App
var templates = template.Must(template.ParseFS(templatesDir, "**/*.gohtml"))
```

**Use when:** Deep directory structure with many nested template directories

### Pattern 5: Custom Parsing with Template Functions

```go
//go:embed **/*.gohtml
var templatesDir embed.FS

//go:generate muxt generate --receiver-type=App
var templates = template.Must(
    template.New("").
        Funcs(customFuncs).
        ParseFS(templatesDir, "**/*.gohtml"),
)

var customFuncs = template.FuncMap{
    "upper":      strings.ToUpper,
    "formatDate": formatDate,
}
```

**Use when:** Templates need custom functions or configuration

## Troubleshooting

### "No templates found"

**Cause:** Muxt can't find the package-level `templates` variable.

**Fix:**
- Ensure the variable is package-level (not in a function)
- Verify it's type `*template.Template`
- Check that it uses `template.ParseFS` or `template.Must(template.ParseFS(...))`

### "Cannot embed parent directories"

**Cause:** Trying to embed files outside the package directory or in parent directories.

**Fix:**
- Move `templates.go` to a directory that's a parent or sibling of your template files
- Restructure so all templates are in the same directory or subdirectories

### "Pattern doesn't match templates"

**Cause:** The glob pattern in `ParseFS` doesn't match your file structure.

**Fix:**
- Use `**/*.gohtml` for recursive matching
- List patterns explicitly if you want specific subdirectories
- Ensure the pattern matches the actual file locations relative to the embed.FS root

## Related

- [How to Integrate Muxt](../how-to/integrate-existing-project.md) - Practical integration examples
- [Templates Variable Reference](../reference/templates-variable.md) - Detailed specification
- [Advanced Patterns](advanced-patterns.md) - Production patterns for template organization