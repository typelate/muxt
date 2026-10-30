# Known Issues

## Form validation attributes must be literals

Generated validation for a bound `<input>` ([form structs](call-parameters.md#form-structs)) reads the `min`, `max`, `pattern`, `minlength` and `maxlength` values at generate time, so they cannot come from a template action. `min="{{.MinAge}}"` in the `age-field` template fails generation, pointing at the `form` argument:

```text
template "age-field": validation attribute min on input "age" must be a literal; got the template action {{.MinAge}} — use a constant, or drop the template struct tag
```

Use a constant, or drop the `template` tag from that field.

[reference_validation_min_max.txt](../../cmd/muxt/testdata/reference_validation_min_max.txt) · [reference_validation_pattern.txt](../../cmd/muxt/testdata/reference_validation_pattern.txt)

## TemplateRoutePaths method names must be exportable

Two handlers whose names differ only in the case of the first letter (`list` and `List`) produce the same exported `TemplateRoutePaths` method, and generation fails:

```text
TemplateRoutePaths method name collision: handlers "list" and "List" both produce method "List"
```

A handler name whose first character has no uppercase form (`一覧`) fails with `cannot export identifier "一覧" for TemplateRoutePaths method: first character '一' has no uppercase form`. Leading underscores are stripped, so `_list` also produces `List`. Rename the handler to start with a cased letter.

[err_route_paths_method_collision.txt](../../cmd/muxt/testdata/err_route_paths_method_collision.txt)

## HTML comments cannot straddle control flow

An HTML comment that starts in one branch of a control-flow action and ends outside it fails to parse with `unexpected … in HTML input`, although the standard library accepted it:

```gotmpl
{{if .A}}<!-- a {{end}} -->
<!-- {{if .A}} -->x{{end}}
```

Keep the whole comment inside one branch.

[Open an issue](https://github.com/typelate/muxt/issues/new) with a minimal reproduction, or send a PR that adds a `cmd/muxt/testdata/err_*.txt` script.
