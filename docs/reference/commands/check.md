# muxt check

Type-checks each template against the data type at its `ExecuteTemplate` call site and reports templates nothing renders.

```bash
muxt check
```

```text
ok: 4 templates
```

Run `generate` first: `check` reads receiver types from the generated file and reports a route template with no handler as waiting for one ([err_check_before_generate.txt](../../../cmd/muxt/testdata/err_check_before_generate.txt)).

What fails: [Type Checking](../type-checking.md#what-is-checked). A template shared between two templates variables is reported when either leaves it unused ([err_template_in_shared_file_unused_by_one_variable.txt](../../../cmd/muxt/testdata/err_template_in_shared_file_unused_by_one_variable.txt)).

`-v` prints `checking endpoint NAME` for each call site. Flags: [cli.md](../cli.md#flags).
