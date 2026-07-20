# Known Issues and Limitations

Current limitations and workarounds for Muxt.

## Form Validation Reads Static Attributes Only

**Issue:** Muxt generates parse-time validation (`min`, `max`, `pattern`) by reading those attributes off `<input>` elements bound to a form field. It parses the literal attribute string against the field's Go type, so the value must be a constant.

**Static value — validation generated:**
```gotmpl
<input type="number" name="age" min="0" max="120">
```
Muxt parses `0` and `120` against the field type and emits bounds checks in the handler (a request with `age=200` fails before your method runs).

**Templated value — not a constant:**
```gotmpl
<input type="number" name="age" min="{{.MinAge}}">
```
Muxt has only the literal `{{.MinAge}}` at generate time, not a number, so it cannot emit a bounds check for that attribute.

**Workaround:** Use constant `min`/`max`/`pattern` values for fields you want muxt to validate.

[reference_validation_min_max.txt](../../cmd/muxt/testdata/reference_validation_min_max.txt) · [reference_validation_pattern.txt](../../cmd/muxt/testdata/reference_validation_pattern.txt)

## Type Checking Limitations

**Issue:** Not all Go template features are fully type-checked.

**Known limitations:**
- `any` / `interface{}` fields disable type checking for that field
- GoLand `gotype` comments not consulted
- Dynamic template names cannot be checked

**Example of unchecked case:**
```go
type Data struct {
    User any  // Disables checking
}
```
```gotmpl
{{.Result.User.AnythingHere}}  <!-- No error, even if invalid -->
```

**Workaround:** Use concrete types:
```go
type User struct {
    Name string
}

type Data struct {
    User User  // Type checked
}
```

See [type-checking.md](type-checking.md) for details.

## Template Function Registration

**Issue:** Custom template functions must be registered before `muxt check` runs.

**Example:**
```go
var templates = template.Must(
    template.New("").
        Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }}).
        ParseFS(fs, "*.gohtml"),
)
```

If functions are added after `ParseFS`, type checking won't recognize them.

## TemplateRoutePaths Method Name Collision

**Issue:** `TemplateRoutePaths` methods are always exported so they can be called from templates. This has two consequences:

1. **Collision:** If a receiver has both an exported and unexported handler method that differ only in the first letter's case (e.g., `List` and `list`), generation fails because both would produce the same `TemplateRoutePaths.List()` method.
2. **Unexportable identifiers:** Handler methods whose first character has no uppercase form (e.g., a name starting with a CJK character) cannot produce an exported method, so generation fails. Leading underscores are stripped during identifier normalization (`_list` produces `List`, which can then collide with a `List` handler).

**Errors:**
- `TemplateRoutePaths method name collision: handlers "list" and "List" both produce method "List"`
- `cannot export identifier "一覧" for TemplateRoutePaths method: first character '一' has no uppercase form`

**Fix:** Rename the handler method to start with a cased letter.

## Removed: `sse` as a Call Argument

**Issue:** Older muxt versions accepted `sse` as a reserved call argument
(`GET /events Stream(ctx, lastEventID, sse)`). That syntax was removed; current
versions fail with `unknown argument sse`.

**Fix:** Wrap the call instead and use `execute` for the callback:
`GET /events sse(Stream(ctx, lastEventID, execute))`. See
[Template Names](template-names.md).

## Reporting Issues

Found a limitation not listed here? [Open an issue](https://github.com/typelate/muxt/issues/new) with:
- Minimal reproduction
- Expected behavior
- Actual behavior

Better yet: Submit a PR updating this list.

## Alternatives

If Muxt's limitations are blocking your use case, consider:
- [templ](https://templ.guide) — Type-safe templating with Go-like syntax
- Standard `http.HandlerFunc` — Manual handlers with full control
