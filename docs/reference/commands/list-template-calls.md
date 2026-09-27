# muxt list-template-calls

Lists, for each template, the `{{template}}` actions it makes.

```bash
muxt list-template-calls --match='^PATCH'
```

```text
template "PATCH /todos/{id} ToggleTodo(id)" calls:

  - template.gohtml:84:44 template "error" error
  - template.gohtml:87:12 template "footer" TodoChange
  - template.gohtml:86:12 template "todo-item" *Todo
```

Each line is the action's position, the kind (`template`), the called template, and the type passed as dot. `--match` filters by a regular expression on the template name; repeatable. Flags: [cli.md](../cli.md#flags). Inverse: [`list-template-callers`](list-template-callers.md). Script: [howto_list_template_calls.txt](../../../cmd/muxt/testdata/howto_list_template_calls.txt).
