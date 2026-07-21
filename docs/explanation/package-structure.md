# Package Structure for Templates

Understanding how to organize templates and the package-level requirements for Muxt code generation.

## The Package-Level Templates Variable

Muxt requires a **package-level variable** of type `*html/template.Template` to discover and analyze your templates.
This variable can be initialized in two ways:
1. Using `template.ParseFS` with an `embed.FS` for file-based templates
2. Using `template.New(name).Parse` for inline string templates

### Why Package-Level?

Muxt uses Go's static analysis to find your templates at generation time. It searches for:
1. A package-level variable declaration
2. Named `templates` (or the name passed via `--use-templates-variable`)
3. Of type `*html/template.Template`
4. Initialized with `template.ParseFS` or `template.New(name).Parse`, optionally wrapped with `template.Must(...)`

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

`ParseFS` uses `path.Match`, which has no recursive wildcard — `**` matches exactly one path segment, the same as `*`. List one glob per directory depth:
```go
//go:embed *.gohtml */*.gohtml
var templatesDir embed.FS

var templates = template.Must(template.ParseFS(templatesDir, "*.gohtml", "*/*.gohtml"))
```

## Common Patterns

### Pattern 1: Inline Templates

```go
//go:generate muxt generate --use-receiver-type=Client
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

*[(See Muxt CLI Test/import_v2_module)](../../cmd/muxt/testdata/reference_import_with_v2_module.txt)*

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

//go:generate muxt generate --use-receiver-type=App
var templates = template.Must(template.ParseFS(templatesDir,
    "pages/*.gohtml",
    "components/*.gohtml",
    "layouts/*.gohtml",
))
```

**Use when:** Templates are organized into logical subdirectories

### Pattern 4: Nested Directories

`go:embed` and `ParseFS` have no recursive glob, so list one pattern per depth. A bare directory name like `go:embed templates` embeds the tree recursively (excluding files that start with `.` or `_`), but `ParseFS` still needs an explicit glob for each level:

```go
//go:embed templates
var templatesDir embed.FS

//go:generate muxt generate --use-receiver-type=App
var templates = template.Must(template.ParseFS(templatesDir,
    "templates/*.gohtml",
    "templates/*/*.gohtml",
    "templates/*/*/*.gohtml",
))
```

**Use when:** Templates are nested a known, small number of levels deep.

**Note:** If a template file name starts with `.` or `_`, a bare directory pattern skips it; use `//go:embed all:templates` to include it. Prefer the bare pattern otherwise — `all:` embeds every hidden file in the tree.

### Pattern 5: Custom Parsing with Template Functions

```go
//go:embed pages/*.gohtml components/*.gohtml
var templatesDir embed.FS

//go:generate muxt generate --use-receiver-type=App
var templates = template.Must(
    template.New("").
        Funcs(template.FuncMap{
            "upper":      strings.ToUpper,
            "formatDate": formatDate,
        }).
        ParseFS(templatesDir, "pages/*.gohtml", "components/*.gohtml"),
)
```

**Use when:** Templates need custom functions or configuration

**Note:** The `template.FuncMap` composite literal must be written inline in the `Funcs` call. Muxt evaluates the `templates` expression statically and does not resolve a variable passed to `Funcs` — `Funcs(customFuncs)` fails with `expected a composite literal with type template.FuncMap`. The map values may be identifiers (`strings.ToUpper`, `formatDate`); only the map literal itself must be inline.

## Troubleshooting

### "No templates found"

**Cause:** Muxt can't find the package-level `templates` variable.

**Fix:**
- Ensure the variable is package-level (not in a function)
- Ensure the variable is named `templates`, or pass its name with `--use-templates-variable`
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
- List one glob per directory depth (`*.gohtml`, `*/*.gohtml`) — `path.Match` has no recursive wildcard, so `**` matches exactly one segment
- Ensure the pattern matches the actual file locations relative to the embed.FS root

## Related

- [Templates Variable Reference](../reference/templates-variable.md) - Detailed specification
- [Advanced Patterns](advanced-patterns.md) - Production patterns for template organization
