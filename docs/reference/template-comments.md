# Template Comments

Disable template code with `{{/* */}}`, never with `<!-- -->`.

A Go template comment is removed when the template is parsed, so nothing inside it runs or reaches the response. An HTML comment is markup: `html/template` decides per context whether to strip it, keep it, or escape it into visible text, and the template actions inside it still run. A route that renders a fragment for htmx puts that output straight into the page, so a comment that leaks shows up there.

Generated handlers render with `html/template`. muxt's own template engine treats comments the same way ([internal/template/state.go](../../internal/template/state.go), [internal/template/escape.go](../../internal/template/escape.go)).

| Template source | Output | Notes |
|---|---|---|
| `{{/* any text, any number of lines */}}` | nothing | the comment form to use |
| `{{- /* c */ -}}` | nothing | trims the whitespace around it as well |
| `{{ /* c */ }}` | parse error | no space is allowed between `{{` and `/*` |
| `{{/* c */ }}` | parse error | nor between `*/` and `}}` |
| `<!-- c -->` in text | nothing | stripped from the response |
| `<!-- {{.X}} -->` in text | nothing | stripped, but `.X` still runs, and an error from it fails the render |
| `<title>T<!-- c --></title>`, same in `<textarea>` | `&lt;!-- c -->` | escaped: the reader sees the comment as text |
| `<![if !IE]>`, `<!x>`, `<?x>` in text | `&lt;![if !IE]>` and so on | escaped and visible |
| `a<!-->b<p>c</p>-->d` | `ad` | the browser ends a comment at `<!-->`; `html/template` strips through the next `-->`, hiding `b` and `c` |
| `<p title="<!-- c -->">` | kept | attribute values are not stripped |
| `/* c */` or `<!-- c` inside `<script>` | stripped | JavaScript comments are removed too |
| a `template.HTML` value containing `<!-- c -->` | kept | trusted HTML is written as is |

To switch off a block of markup while you work on it, wrap it in `{{/*` and `*/}}`. The actions inside do not run, and the block cannot reach the browser in any context.

```gotmpl
{{/*
<button hx-delete="{{.Path.DeleteTodo .Result.ID}}">Delete</button>
*/}}
```

An HTML comment in text never reaches the browser. If one must, write it from a `template.HTML` value.

[`muxt check`](commands/check.md) reports the shapes the page would show: a `<!--` inside `<title>` or `<textarea>`, a `<!` or `<?` that starts no comment or doctype, and a comment opened as `<!-->`. [`muxt fmt`](commands/fmt.md) leaves these shapes as written.
