# Known Issues

## Form validation attributes must be literals

Generated validation for a bound `<input>` ([form structs](call-parameters.md#form-structs)) reads attribute values at generate time. `min="{{.MinAge}}"` fails with `strconv.ParseInt: parsing "{{.MinAge}}": invalid syntax`, pointing at the `form` argument. Use a constant, or drop the `template` tag from that field.

[reference_validation_min_max.txt](../../cmd/muxt/testdata/reference_validation_min_max.txt) · [reference_validation_pattern.txt](../../cmd/muxt/testdata/reference_validation_pattern.txt)

## TemplateRoutePaths method names must be exportable

Two handlers whose names differ only in the case of the first letter (`list` and `List`) produce the same exported `TemplateRoutePaths` method, and generation fails:

```text
TemplateRoutePaths method name collision: handlers "list" and "List" both produce method "List"
```

A handler name whose first character has no uppercase form (`一覧`) fails with `cannot export identifier "一覧" for TemplateRoutePaths method: first character '一' has no uppercase form`. Leading underscores are stripped, so `_list` also produces `List`. Rename the handler to start with a cased letter.

[err_route_paths_method_collision.txt](../../cmd/muxt/testdata/err_route_paths_method_collision.txt)

## `sse` is no longer a call argument

`GET /events Stream(ctx, lastEventID, sse)` fails with `unknown argument sse`. Wrap the call and take `execute` as the callback ([Server-Sent Events](call-parameters.md#server-sent-events)):

```gotmpl
{{define "GET /events sse(Stream(ctx, lastEventID, execute))"}}{{end}}
```

[Open an issue](https://github.com/typelate/muxt/issues/new) with a minimal reproduction, or send a PR that adds a `cmd/muxt/testdata/err_*.txt` script.
