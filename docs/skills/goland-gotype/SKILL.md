---
name: muxt-goland-gotype
description: "Muxt: Use when adding gotype comments to Go HTML templates for JetBrains GoLand IDE support. This is a GoLand-only feature — muxt check does not use gotype comments."
---

# GoLand gotype Comments

Add `gotype` comments to sub-templates so JetBrains GoLand can provide IDE completion and type checking for dot context fields.

**This is a GoLand-only feature.** `muxt check` has its own static analysis and does not use `gotype` comments. See [Type Checking](../../reference/type-checking.md) and [`muxt check`](../../reference/commands/check.md).

## Syntax

Place a `gotype` comment as the first line inside a template definition:

```gotemplate
{{define "article-card"}}
{{- /* gotype: example.com/hypertext.Article */ -}}
<div class="card">
  <h2>{{.Title}}</h2>
  <p>{{.Summary}}</p>
</div>
{{end}}
```

The value is a fully qualified Go type path. GoLand uses it to resolve field access on `.` within that template.

## When to Use

Add `gotype` when all of the following apply:
- You are using JetBrains GoLand
- A sub-template is called via `{{template "name" .SomeField}}`
- The dot context type differs from the parent's `TemplateData[R, T]`

## Example

```gotemplate
{{define "GET /article/{id} GetArticle(ctx, id)"}}
{{if .Err}}
  <div class="error">{{.Err.Error}}</div>
{{else}}
  {{template "article-page" .Result}}
{{end}}
{{end}}

{{define "article-page"}}
{{- /* gotype: example.com/hypertext.Article */ -}}
<article>
  <h1>{{.Title}}</h1>
  <p>{{.Body}}</p>
</article>
{{end}}
```

## Reference

- [Type Checking](../../reference/type-checking.md)
- [`muxt check`](../../reference/commands/check.md)
- [Call Results](../../reference/call-results.md)

### Test Cases (`cmd/muxt/testdata/`)

- `reference_check_types.txt` — `muxt check` type checking behavior
- `reference_check_with_all_templates_used.txt` — All templates used check
- `err_check_with_wrong_field.txt` — Error when template accesses invalid field
