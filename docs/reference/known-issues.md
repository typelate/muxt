# Known Issues

Generate and check limits that fail with an error or behave unexpectedly, each with its message and the fix.

## Form validation attributes must be literals

Generated validation for a bound `<input>` ([form structs](call-parameters.md#form-structs)) reads the `min`, `max`, `pattern`, `minlength` and `maxlength` values at generate time, so they cannot come from a template action. `min="{{.MinAge}}"` in the `age-field` template fails generation, pointing at the `form` argument:

```text
template "age-field": validation attribute min on input "age" must be a literal; got the template action {{.MinAge}} — use a constant, or drop the template struct tag
```

Use a constant, or drop the `template` tag from that field.

[reference_validation_min_max.txt](../../cmd/muxt/testdata/reference_validation_min_max.txt) · [reference_validation_pattern.txt](../../cmd/muxt/testdata/reference_validation_pattern.txt)

Report a new one in an [issue](https://github.com/typelate/muxt/issues/new) with a minimal reproduction.
