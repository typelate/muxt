# Package Structure

[Templates Variable](../reference/templates-variable.md) is the specification; [Package Layout](../reference/package-layout.md) shows a full package.

## Why package-level

Muxt reads the package for the variable named by `--use-templates-variable` (default `templates`) and evaluates its initializer statically, a chain of the calls listed in [Templates Variable](../reference/templates-variable.md). A variable built inside a function has no initializer, so muxt cannot see it. For the same reason `Funcs` needs a literal `template.FuncMap`. Templates need not be files: `template.New("GET / List()").Parse(...)` works ([reference_import_with_v2_module.txt](../../cmd/muxt/testdata/reference_import_with_v2_module.txt)).

## What embed allows

`//go:embed` takes files in the `.go` file's directory and below, so templates live with, or under, the file that embeds them:

```text
internal/hypertext/
├── templates.go          //go:embed lives here
├── index.gohtml          sibling: allowed
└── pages/
    └── dashboard.gohtml  child: allowed
```

An `internal/templates/` directory beside `internal/hypertext/` is out of reach.

## Globs are per depth

Neither `//go:embed` patterns nor `ParseFS` have a recursive wildcard. `**` matches exactly one segment, the same as `*`, so list one pattern per depth:

```go
//go:embed *.gohtml */*.gohtml
var templatesDir embed.FS

var templates = template.Must(template.ParseFS(templatesDir, "*.gohtml", "*/*.gohtml"))
```

A bare directory name, `//go:embed templates`, embeds the tree recursively, but `ParseFS` still needs a glob per level.
