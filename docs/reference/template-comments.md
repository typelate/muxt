# Template Comments

Disable template code with `{{/* */}}`, never with `<!-- -->`.

A Go template comment is removed when the template is parsed, so nothing inside it runs or reaches the response. It may span lines, and `{{- /* c */ -}}` trims the whitespace around it as well:

```gotmpl
{{/*
<button hx-delete="{{.Path.DeleteTodo .Result.ID}}">Delete</button>
*/}}
```

An HTML comment is markup. In text, `html/template` strips `<!-- c -->` from the response, but the actions inside it still run: `<!-- {{.X}} -->` writes nothing and still evaluates `.X`, and an error from it fails the render. A route that renders a fragment for htmx puts its output straight into the page, so a comment that leaks shows up there.

Generated handlers render with `html/template`. muxt's own template engine treats comments the same way ([internal/template/state.go](../../internal/template/state.go), [internal/template/escape.go](../../internal/template/escape.go)). [`muxt check`](commands/check.md) reports the shapes listed under edge cases that the page would show, and [`muxt fmt`](commands/fmt.md) leaves them as written.

## Edge cases

The comment forms `html/template` writes differently from what the template says, each checked by running it:

| Template source | Output | Notes |
|---|---|---|
| `{{ /* c */ }}` | parse error | no space is allowed between `{{` and `/*` |
| `{{/* c */ }}` | parse error | nor between `*/` and `}}` |
| `<title>T<!-- c --></title>`, same in `<textarea>` | `&lt;!-- c -->` | those elements hold text, so the reader sees the comment |
| `<![if !IE]>`, `<!x>`, `<?x>` in text | `&lt;![if !IE]>` and so on | escaped and visible |
| `a<!-->b<p>c</p>-->d` | `ad` | the browser ends a comment at `<!-->`; `html/template` strips through the next `-->`, hiding `b` and `c` |
| `<p title="<!-- c -->">` | kept | attribute values are not stripped |
| `/* c */` or `<!-- c` inside `<script>` | stripped | JavaScript comments are removed too |
| a `template.HTML` value containing `<!-- c -->` | kept | trusted HTML is written as is |

An HTML comment in text never reaches the browser. If one must, write it from a `template.HTML` value.
