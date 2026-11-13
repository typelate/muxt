# muxt list-template-callers

Lists, for each template, the Go `ExecuteTemplate` call sites and the `{{template}}` actions that render it.

`use_templates_variable` comes from the directory's [`muxt.json`](../muxt-json.md) entry when there is one; `--use-templates-variable` is rejected there.

With no flags in the directory of a `muxt.json`, it lists every entry, dependencies first, from one package load, each under its key line. `--format json` prints one array whose objects have a `Key`. It stops at the first failure, named by key.

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
