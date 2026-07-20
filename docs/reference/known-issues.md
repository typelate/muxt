# Known Issues and Limitations

Current limitations and workarounds for Muxt.

## Form Validation Reads Static Attributes Only

**Issue:** Muxt generates parse-time validation (`min`, `max`, `minlength`,
`maxlength`, `pattern`) by reading those attributes off the `<input>` element
bound to a form field. The binding is opt-in: the form struct field must carry
a `template:"name"` tag naming the template that contains the matching
`<input name=...>`. Fields without the tag (or whose template has no matching
input) get no generated validation. `min`/`max` apply only to numeric and
temporal input types (`number`, `range`, `date`, `time`, ...); `pattern` only
to textual ones (`text`, `email`, `password`, ...).

**Static value — validation generated:**
```gotmpl
{{define "age-field"}}<input type="number" name="age" min="0" max="120">{{end}}
```
```go
type SignupForm struct {
    Age int `template:"age-field"`
}
```
Muxt parses `0` and `120` against the field type and emits bounds checks in the handler (a request with `age=200` fails before your method runs).

**Templated value — generation fails:**
```gotmpl
<input type="number" name="age" min="{{.MinAge}}">
```
Muxt has only the literal `{{.MinAge}}` at generate time. It cannot parse that as the field's type, so `muxt generate` fails with an error for the attribute — it is not silently skipped.

**Workaround:** Use constant `min`/`max`/`pattern` values for fields you want muxt to validate, and remove the `template` tag from fields whose attributes must stay dynamic.

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

**Issue:** Custom template functions must appear in the templates variable's initialization chain — the `Funcs` call has to come before `Parse`/`ParseFS`.

**Example:**
```go
var templates = template.Must(
    template.New("").
        Funcs(template.FuncMap{"add": func(a, b int) int { return a + b }}).
        ParseFS(fs, "*.gohtml"),
)
```

If functions are added after `ParseFS`, or outside the variable's initialization expression, type checking won't recognize them.

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
