# Known Issues and Limitations

Current limitations and workarounds for Muxt.

## HTML Validity Requirement

**Issue:** Template HTML must be valid for form validation generation.

**Not allowed (invalid HTML):**
```gotmpl
<details {{if .Open}}open{{end}}>
    <p>Content</p>
</details>
```

**Workaround:**
```gotmpl
{{define "content"}}<p>Content</p>{{end}}

{{if .Open}}
<details open>{{template "content"}}</details>
{{else}}
<details>{{template "content"}}</details>
{{end}}
```

**Allowed (actions in attribute values):**
```gotmpl
<div title="{{.HelpText}}" class="{{.ClassName}}">
    <p>Content</p>
</div>
```

**Warning:** Muxt warns if template source contains invalid HTML.

## Type Checking Limitations

**Issue:** Not all Go template features are fully type-checked.

**Known limitations:**
- `any` / `interface{}` fields disable type checking for that field
- GoLand `gotype` comments not consulted
- Complex pipeline expressions may produce false negatives
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

## Nested Template Contexts

**Issue:** Nested `{{template}}` or `{{block}}` calls may not preserve full type context.

**Example:**
```gotmpl
{{define "user"}}
<div>{{.Name}}</div>  <!-- Type checking depends on caller context -->
{{end}}

{{define "GET /profile Profile(ctx)"}}
{{template "user" .Result}}  <!-- Context passed explicitly -->
{{end}}
```

**Workaround:** Pass data explicitly and ensure subtemplates don't assume specific types.

## TemplateRoutePaths Method Name Collision

**Issue:** `TemplateRoutePaths` methods are always exported so they can be called from templates. This has two consequences:

1. **Collision:** If a receiver has both an exported and unexported handler method that differ only in the first letter's case (e.g., `List` and `list`), generation fails because both would produce the same `TemplateRoutePaths.List()` method.
2. **Unexportable identifiers:** Handler methods starting with `_` (e.g., `_list`) cannot be exported, so generation fails.

**Errors:**
- `TemplateRoutePaths method name collision: handlers list and List both produce method List`
- `cannot export identifier "_list" for TemplateRoutePaths method: first character '_' has no uppercase form`

**Fix:** Rename the handler method to start with a letter.

**Test files:** `err_path_method_collision.txt`, `err_path_method_unexportable.txt`

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
