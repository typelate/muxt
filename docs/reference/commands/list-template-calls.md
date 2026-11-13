# muxt list-template-calls

Lists, for each template, the `{{template}}` actions it makes.

`use_templates_variable` comes from the directory's [`muxt.json`](../muxt-json.md) entry when there is one; `--use-templates-variable` is rejected there.

With no flags in the directory of a `muxt.json`, it lists every entry, dependencies first, from one package load, [keyed](../cli.md#keyed-output). It stops at the first failure, named by key.

```bash
muxt list-template-calls --match='^PATCH'
```

```text
template "PATCH /todos/{id} ToggleTodo(id)" calls:

  - template.gohtml:84:44 template "error" error
  - template.gohtml:87:12 template "footer" TodoChange
  - template.gohtml:86:12 template "todo-item" *Todo
```

Takes an optional `[package-dir]`, resolved like `-C`; a missing directory, a file, or a pattern is an error ([reference_package_directory_argument.txt](../../../cmd/muxt/testdata/reference_package_directory_argument.txt)).

Each line is the action's position, the kind (`template`), the called template, and the type passed as dot. `--match` filters by a regular expression on the template name; repeatable. Flags: [cli.md](../cli.md#flags). Inverse: [`list-template-callers`](list-template-callers.md). Script: [howto_list_template_calls.txt](../../../cmd/muxt/testdata/howto_list_template_calls.txt).
