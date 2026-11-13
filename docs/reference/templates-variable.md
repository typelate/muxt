# Templates Variable

Muxt finds route templates through a package-level variable of type `*template.Template`, named `templates` unless `--use-templates-variable` says otherwise. A missing or non-package-level variable fails with `variable NAME not found in package PATH` ([err_missing_templates_variable.txt](../../cmd/muxt/testdata/err_missing_templates_variable.txt)).

```go
//go:embed *.gohtml
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))
```

[reference_template_embed_gen_decl.txt](../../cmd/muxt/testdata/reference_template_embed_gen_decl.txt)

## Supported initializers

The initializer may use only these calls. Every string argument is a literal, or the name of a package-level `const` or `var` whose initializer is a literal ([reference_template_pattern_constant.txt](../../cmd/muxt/testdata/reference_template_pattern_constant.txt)):

| Call | Constraint |
|------|------------|
| `template.Must`, `template.New`, `template.ParseFS` | package functions |
| `.ParseFS`, `.Parse`, `.New`, `.Delims`, `.Option` | chained on the result |
| `.Funcs` | argument is a `template.FuncMap{...}` literal, or the name of a package-level `var` or `const` holding one ([reference_funcs_variable.txt](../../cmd/muxt/testdata/reference_funcs_variable.txt)); each value is an expression that type-checks to a func |

The `ParseFS` filesystem must be an `embed.FS` variable of this package, or an exported one of another package of the same module (see [Sharing another package's files](#sharing-another-packages-files)). A string argument muxt cannot read, such as a function result, fails with `expected string literal got NAME`. A `Funcs` argument it cannot read, such as a function result or a map built in `init`, fails with `Funcs: cannot read funcs: muxt reads a template.FuncMap literal, or a package-level var or const holding one` ([err_funcs_unreadable.txt](../../cmd/muxt/testdata/err_funcs_unreadable.txt)). `ParseGlob` fails with `unsupported function ParseGlob: use ParseFS with an embed.FS` ([err_template_parse_glob.txt](../../cmd/muxt/testdata/err_template_parse_glob.txt)), and any other call with `unsupported function NAME` or `unsupported method NAME`.

```go
//go:embed *.gohtml pages/*.gohtml
var templateFS embed.FS

var templates = template.Must(template.New("").Funcs(template.FuncMap{"add": add}).ParseFS(templateFS, "*.gohtml", "pages/*.gohtml"))
```

[reference_template_with_multiple_parsefs.txt](../../cmd/muxt/testdata/reference_template_with_multiple_parsefs.txt)

## Sharing another package's files

```go
var templates = template.Must(template.Must(template.New("").
	Funcs(template.FuncMap{"upper": fragments.Upper}).
	ParseFS(fragments.TemplateSource, "*.gohtml")).
	ParseFS(templateSource, "*.gohtml"))
```

[howto_shared_template_fragments.txt](../../cmd/muxt/testdata/howto_shared_template_fragments.txt)

- The other package's own `//go:embed` patterns decide which files exist.
- A route template in a shared file is a route of every package that parses it.
- A later definition of a name replaces an earlier one; bare `muxt` lists these under `Overrides:` ([cli](cli.md)). A shared file replacing the package's own definition is an error: parse the shared FS first.
- Share functions by exporting them and naming each in a `FuncMap` literal. `Funcs(fragments.Funcs)` is an error that says so.
- A variable that is unexported, not an `embed.FS`, or from another module is an error naming the rule.
- Shared templates are outside both the [unused check](commands/check.md) and mutation testing in every package, so nothing reports a shared template that no package uses. A shared route template with no generated handler is still reported.

## Multiple Template Variables

Pass `--use-templates-variable` once per variable. Each variable is its own template namespace with its own `Funcs` and `Option`s, so `{{define "header"}}` in one does not overwrite it in another. Routes from all variables go into one generated routes function; the same pattern in two variables fails with `duplicate route pattern`.

```bash
muxt generate --use-templates-variable=adminTemplates --use-templates-variable=publicTemplates
```

One run per variable, each with its own `--output-*` names, keeps the sets apart ([serve public and admin routes from one package](../how-to/multiple-route-sets.md)).

[reference_multiple_templates_variables.txt](../../cmd/muxt/testdata/reference_multiple_templates_variables.txt) · [err_duplicate_route_different_variables.txt](../../cmd/muxt/testdata/err_duplicate_route_different_variables.txt)
