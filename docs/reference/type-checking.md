# Type Checking

[`muxt check`](commands/check.md) uses [github.com/typelate/check](https://pkg.go.dev/github.com/typelate/check).

## Preconditions

An `ExecuteTemplate` call is checked when its template name is a literal and its data argument has a statically known type.

```go
templates.ExecuteTemplate(w, "user-profile", data) // checked
templates.ExecuteTemplate(w, name, data)           // not checked: name is not a literal
```

Generated handlers meet both conditions, so every route template is checked.

## What is checked

Each action is resolved against the type of dot at that point: field accesses, method calls (including on interface-typed values), registered template functions, and `{{template}}` calls with the type they pass along.

```gotmpl
{{define "GET /user GetUser(ctx)"}}
<p>{{.Result.Email}}</p>  <!-- ok -->
<p>{{.Result.Phone}}</p>  <!-- error: User has no Phone field -->
{{end}}
```

Each error names the call site, the template position, then the type:

```text
/home/me/app/template_routes.go:38:13 ExecuteTemplate "GET /user GetUser(ctx)" *TemplateData[RoutesReceiver, User]
/home/me/app/template.gohtml:3:13: executing "GET /user GetUser(ctx)" at <.Result.Phone>: field or method Phone not found on User

  type User struct {
    Email string
  }

Error: fail: 1 error
```

[reference_check_types.txt](../../cmd/muxt/testdata/reference_check_types.txt) · [err_check_with_wrong_field.txt](../../cmd/muxt/testdata/err_check_with_wrong_field.txt)

Check also fails on:

- a defined template nothing renders, by route or by `ExecuteTemplate` ([err_check_with_unused_template.txt](../../cmd/muxt/testdata/err_check_with_unused_template.txt))
- content in a template file outside every `{{define}}` block; files holding only comments and whitespace are fine ([err_check_with_dead_code_outside_define.txt](../../cmd/muxt/testdata/err_check_with_dead_code_outside_define.txt))
- a field or method access on an `any` value: `field or method Name not found on any` ([howto_path_param.txt](../../cmd/muxt/testdata/howto_path_param.txt))

## What is not checked

| Case | Effect |
|------|--------|
| Anything after the initializer (`init`, a later `templates = ...`) | Not evaluated. Only the variable's initializer expression is read; a `Funcs`/`ParseFS` chain in `init` leaves `muxt check` reporting `ok: 0 templates` and `muxt generate` writing 0 routes. |
| GoLand `gotype` comments | Not read. |
