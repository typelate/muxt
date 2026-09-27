# Templates Variable

Muxt finds route templates through a package-level variable of type `*template.Template`, named `templates` unless `--use-templates-variable` says otherwise. A missing or non-package-level variable fails with `variable NAME not found in package PATH` ([err_missing_templates_variable.txt](../../cmd/muxt/testdata/err_missing_templates_variable.txt)).

```go
//go:embed *.gohtml
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))
```

[reference_template_embed_gen_decl.txt](../../cmd/muxt/testdata/reference_template_embed_gen_decl.txt)

## Supported initializers

The initializer may use only these calls, and every string argument must be a literal:

| Call | Constraint |
|------|------------|
| `template.Must`, `template.New`, `template.ParseFS` | package functions |
| `.ParseFS`, `.Parse`, `.New`, `.Delims`, `.Option` | chained on the result |
| `.Funcs` | argument is an inline `template.FuncMap{...}` literal; each value is an expression that type-checks to a func |

The `ParseFS` filesystem must be an `embed.FS` variable. A non-literal string fails with `expected string literal got NAME`, a non-literal `Funcs` argument with `expected a template.FuncMap composite literal got NAME`, and any other call with `unsupported function NAME` or `unsupported method NAME`.

```go
//go:embed *.gohtml pages/*.gohtml
var templateFS embed.FS

var templates = template.Must(template.New("").Funcs(template.FuncMap{"add": add}).ParseFS(templateFS, "*.gohtml", "pages/*.gohtml"))
```

[reference_template_with_multiple_parsefs.txt](../../cmd/muxt/testdata/reference_template_with_multiple_parsefs.txt)

## Multiple Template Variables

Pass `--use-templates-variable` once per variable. Each variable is its own template namespace with its own `Funcs` and `Option`s, so `{{define "header"}}` in one does not overwrite it in another. Routes from all variables go into one generated routes function; the same pattern in two variables fails with `duplicate route pattern`.

```bash
muxt generate --use-templates-variable=adminTemplates --use-templates-variable=publicTemplates
```

One run per variable, each with its own `--output-*` names, keeps the sets apart ([serve public and admin routes from one package](../how-to/multiple-route-sets.md)).

[reference_multiple_templates_variables.txt](../../cmd/muxt/testdata/reference_multiple_templates_variables.txt) · [err_duplicate_route_different_variables.txt](../../cmd/muxt/testdata/err_duplicate_route_different_variables.txt)
