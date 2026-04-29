---
name: muxt-forms
description: "Muxt: Use when creating HTML forms in a Muxt codebase — form struct design, field mapping, accessible form HTML with ARIA, validation attributes, field type parsing, error display, multipart/file uploads, and testing form submissions."
---

# Form Creation and Binding

Create forms that bind to Go structs with type-safe field parsing and validation. Muxt generates server-side code from your template and struct definitions.

## When to use this skill

- Adding a POST/PATCH/PUT route that accepts form data.
- Need server-side validation generated from HTML attributes.
- Need accessible forms (labels, ARIA, error display).
- Uploading files (multipart).

## Form struct + name mapping

Field names map to HTML `name` attributes using the exact Go field name. Use `name` tag to override (`name:"full-name"`). Use the `template` tag to point at the block whose validation attributes muxt should scan. Slice fields accept multiple values.

See `references/examples.md` for full struct, HTML, and re-render-on-error examples.

## Template name syntax

Use a `form` parameter in the call expression. Status code optional (defaults to 200):

```gotmpl
{{define "POST /article 201 CreateArticle(ctx, form)"}}...{{end}}
{{define "PATCH /article/{id} UpdateArticle(ctx, id, form)"}}...{{end}}
```

The `form` parameter tells muxt to parse the request body as form data and bind it to the struct type in the method signature.

## HTML5 validation attributes

Muxt generates server-side validation for **a subset** of HTML5 validation attributes. Not all are supported — write tests for the validation behavior you depend on.

### Supported

| Attribute | Input types | Generated check |
|-----------|------------|-----------------|
| `min` | `number`, `range`, `date`, `month`, `week`, `time`, `datetime-local` | `if value < min` → 400 |
| `max` | same as `min` | `if value > max` → 400 |
| `pattern` | `text`, `search`, `url`, `tel`, `email`, `password` | `regexp.MustCompile(pattern).MatchString(value)` → 400 |
| `minlength` | all | `if len(value) < minlength` → 400 |
| `maxlength` | all | `if len(value) > maxlength` → 400 |

### Not supported (validate in the receiver method if needed)

`required`, `step`, `accept`, `multiple` — silent on the server side.

When validation fails, the generated handler returns 400 with an error message. Display with `role="alert"`.

## Field type parsing

| Go type | Notes |
|---------|-------|
| `string` | No parsing. |
| `int*`, `uint*` | `strconv` parsing. |
| `bool` | `strconv.ParseBool` (`"true"`, `"1"`, `"t"`, etc. → true). |
| `encoding.TextUnmarshaler` | Custom types implementing `UnmarshalText` (e.g., `time.Time`). |
| `[]string` / `[]int` | Multiple values collected. |

Unsupported field types (`url.URL`, maps, nested structs) → generation error. Use scalars, slices of scalars, or `TextUnmarshaler`.

## Receiver method

```go
func (s Server) CreateArticle(ctx context.Context, form CreateArticleForm) (Article, error) { ... }
```

Return `(T, error)` to render the result or display an error.

## Re-rendering after validation errors

For standard form submissions, return a result type with named error fields and use `.Request.FormValue` to repopulate inputs from the request. The result type carries per-field errors; `.Request.FormValue` preserves submitted values — no need to echo form data through the result struct. Full pattern in `references/examples.md`.

For per-field inline validation (validate as the user types), see [HTMX Inline Validation](../muxt_htmx/SKILL.md#inline-field-validation) — requires HTMX and a dedicated validation endpoint per field.

## Testing

Write tests for every validation constraint you rely on, especially attributes muxt does not enforce server-side. Test both valid and boundary-violation submissions. See `references/examples.md` for complete patterns: valid submissions, min/max boundary, pattern, slice fields, per-field error display.

## File uploads (multipart)

Use the `multipart` parameter (mutually exclusive with `form`) when the request is `multipart/form-data` — required for `<input type="file">`. Set `enctype="multipart/form-data"` on the form. Use `*multipart.FileHeader` for single files, `[]*multipart.FileHeader` for multiple, `*multipart.Form` for raw access. Default max size 32 MiB; override with `--output-multipart-max-memory=<size>`. Full struct/template/method/error-handling patterns in `references/examples.md`.

## Reference files

- `references/examples.md` — full HTML/Go examples for struct design, accessible HTML, slice fields, re-render on error, every test pattern (valid, boundaries, pattern, slice, per-field errors), and multipart uploads.

## External reference

- [Call Parameters](../../reference/call-parameters.md), [Call Results](../../reference/call-results.md), [Template Name Syntax](../../reference/template-names.md)
- [Template-Driven Development](../muxt_test-driven-development/SKILL.md) — TDD workflow for all route types.

### Test cases (`cmd/muxt/testdata/`)

Form basics: `howto_form_basic.txt`, `howto_form_with_struct.txt`, `howto_form_with_field_tag.txt`, `howto_form_with_slice.txt`, `reference_form_field_types.txt`, `reference_form_with_html_min_attr.txt`, `reference_form_with_empty_struct.txt`.

Validation: `reference_validation_min_max.txt`, `reference_validation_pattern.txt`.

Errors: `err_form_unsupported_field_type.txt`, `err_form_unsupported_composite.txt`, `err_form_bool_return.txt`, `err_form_unsupported_return.txt`, `err_form_with_undefined_method.txt`.

Multipart: `reference_multipart_basic.txt`, `reference_multipart_multiple_files.txt`, `reference_multipart_mixed.txt`, `reference_multipart_raw.txt`, `reference_multipart_with_name_tag.txt`, `reference_multipart_max_memory_flag.txt`, `reference_multipart_parse_error.txt`, `howto_multipart_file_upload.txt`, `err_multipart_with_form.txt`.
