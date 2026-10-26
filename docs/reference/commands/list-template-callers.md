# muxt list-template-callers

Lists, for each template, the Go `ExecuteTemplate` call sites and the `{{template}}` actions that render it.

```bash
muxt list-template-callers --match=error --match='^GET'
```

```text
template "GET /{$} ListTodos(form, execute)" called by:

  - template_routes.go:223:12 execute_template "GET /{$} ListTodos(form, execute)" *TemplateData[RoutesReceiver, TodoPage]


template "error" called by:

  - template.gohtml:84:44 template "PATCH /todos/{id} ToggleTodo(id)" error
```

Takes an optional `[package-dir]`, resolved like `-C`; a missing directory, a file, or a pattern is an error ([reference_package_directory_argument.txt](../../../cmd/muxt/testdata/reference_package_directory_argument.txt)).

Paths are absolute; shortened here. Each line is the position, the kind of call, a template name (the enclosing template for `template`; the executed template for `execute_template`), and the type passed as dot. `--match` filters by a regular expression on the template name; repeatable. Flags: [cli.md](../cli.md#flags). Inverse: [`list-template-calls`](list-template-calls.md). Script: [howto_list_template_callers.txt](../../../cmd/muxt/testdata/howto_list_template_callers.txt).
