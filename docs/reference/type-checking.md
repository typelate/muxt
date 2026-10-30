# Type Checking

[`muxt check`](commands/check.md) uses muxt's own checker (`internal/check`, derived from typelate/check and htt).

## Preconditions

An `ExecuteTemplate` call is checked when its template name is a literal and its data argument has a statically known type.

```go
templates.ExecuteTemplate(w, "user-profile", data) // checked
templates.ExecuteTemplate(w, name, data)           // not checked: name is not a literal
```

Generated handlers meet both conditions, so every route template is checked.

A call whose name is not a literal, such as one in a `render(w, name, data)` helper, is skipped, and check says so on stderr, once per call site, so the templates it renders are not silently unchecked. The count is in the summary ([reference_check_non_literal_template_name.txt](../../cmd/muxt/testdata/reference_check_non_literal_template_name.txt)):

```text
/home/me/app/render.go:42:9: ExecuteTemplate not checked: the template name is not a string literal, so which template it renders is not known until run time
ok: 4 templates, 1 call site skipped
```

Such a call may render any template, so while one exists in the package no template is reported unused. To check the templates it renders, call `ExecuteTemplate` with a literal name and a typed value at each call site:

```go
// not checked
func renderTemplate(w http.ResponseWriter, name string, data any) error {
	return templates.ExecuteTemplate(w, name, data)
}

// checked against PageData
err := templates.ExecuteTemplate(w, "page", PageData{Title: "Home"})
```

Alternatively, make the page a route template with a call, so the generated handler renders it.

A call whose name is a literal but whose data is `any` (or another interface) is not skipped: it is checked against that interface type, so any field access on dot fails with `field or method Name not found on any` ([check_any_data.txtar](../../internal/analysis/testdata/check/check_any_data.txtar)). Pass the concrete type.

## What is checked

Each action is resolved against the type of dot at that point: field accesses, method calls (including on interface-typed values), registered template functions, and `{{template}}` calls with the type they pass along.

```gotmpl
{{define "GET /user GetUser(ctx)"}}
<p>{{.Result.Email}}</p>  <!-- ok -->
<p>{{.Result.Phone}}</p>  <!-- error: User has no Phone field -->
{{end}}
```

Each error names the call site, then the template position and what failed:

```text
/home/me/app/template_routes.go:38:13 ExecuteTemplate "GET /user GetUser(ctx)" *TemplateData[RoutesReceiver, User]
/home/me/app/template.gohtml:3:13: executing "GET /user GetUser(ctx)" at <.Result.Phone>: field or method Phone not found on User
```

`-v` follows a field or method error with the declaration of the type it was looked up on:

```text
/home/me/app/template.gohtml:3:13: executing "GET /user GetUser(ctx)" at <.Result.Phone>: field or method Phone not found on User

  type User struct {
    Email string
  }
```

A partial that several calls render with the same type of dot fails the same way from each. The error is printed once, under the first call that reaches it; after every call is checked, it is repeated once with the other calls that reach it ([check_shared_partial_error.txtar](../../internal/analysis/testdata/check/check_shared_partial_error.txtar)):

```text
/home/me/app/server.go:17:9 ExecuteTemplate "intakes" AdminPage
/home/me/app/footer.gohtml:1:30: executing "footer" at <.Viewer>: field or method Viewer not found on AdminPage

/home/me/app/footer.gohtml:1:30: executing "footer" at <.Viewer>: field or method Viewer not found on AdminPage
  also reached from:
    /home/me/app/server.go:21:9 ExecuteTemplate "users" AdminPage
    /home/me/app/server.go:25:9 ExecuteTemplate "settings" AdminPage
```

[reference_check_types.txt](../../cmd/muxt/testdata/reference_check_types.txt) · [err_check_with_wrong_field.txt](../../cmd/muxt/testdata/err_check_with_wrong_field.txt)

Check also fails on:

- a defined template nothing renders, by route or by `ExecuteTemplate` ([err_check_with_unused_template.txt](../../cmd/muxt/testdata/err_check_with_unused_template.txt)); not reported while a call with a non-literal template name exists, since that call may render any template; `--allow-unused` lists these without failing, for templates rendered some other way than a `templates.ExecuteTemplate` call in the package
- content in a template file outside every `{{define}}` block; files holding only comments and whitespace are fine ([err_check_with_dead_code_outside_define.txt](../../cmd/muxt/testdata/err_check_with_dead_code_outside_define.txt))
- a field or method access on an `any` value: `field or method Name not found on any` ([howto_path_param.txt](../../cmd/muxt/testdata/howto_path_param.txt))

`printf`-family format verbs are verified against the argument types. With a string `Name`, `{{printf "%d" .Name}}` is an error.

## What is not checked

| Case | Effect |
|------|--------|
| Anything after the initializer (`init`, a later `templates = ...`) | Not evaluated. Only the variable's initializer expression is read; a `Funcs`/`ParseFS` chain in `init` leaves `muxt check` reporting `ok: 0 templates` and `muxt generate` writing 0 routes. |
| GoLand `gotype` comments | Not read. |
| Go compile errors | Not reported: a package that does not build can pass `muxt check`. Run `go vet`. |
