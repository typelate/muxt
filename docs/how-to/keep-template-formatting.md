# Keep part of a template as written

`muxt fmt` changes only whitespace the browser ignores, so most templates need nothing. Use a `muxt:fmt` comment when you want a region left exactly as you wrote it, such as a hand-aligned block.

There are three directives. Each is a template comment containing only the directive. Trim markers are allowed: `{{- /* muxt:fmt off */ -}}`.

## Keep a region

`off` leaves everything after the comment as written until the matching `on`. They must be siblings: in the same element, the same branch of an `if`, or the same `define`.

```gohtml
<div>
{{/* muxt:fmt off */}}
<p>  a  </p>
   <p>b</p>
{{/* muxt:fmt on */}}
<p>c</p>
</div>
```

An `off` with no `on` runs to the end of its parent.

## Keep the next node

`preserve` leaves the next sibling as written: an element with its children, or an `if` through its `{{end}}`. Whitespace and comments between the directive and that node are skipped.

```gohtml
<div>
{{/* muxt:fmt preserve */}}
  <ul>
     <li>a</li>   <li>b</li>
</ul>
<p>c</p>
</div>
```

Only the `ul` is kept. The `p` is formatted.

## Keep a file

`off` as the first thing in a file, with no `on` after it, keeps the whole file. So does `preserve` in that position. An `off` closed by a later `on` keeps only the region between them.

```gohtml
{{/* muxt:fmt off */}}
<table>
...
```

## Errors

A comment that starts with `muxt:fmt` and is not exactly one of the three is an error. The file is left unchanged and the error names where the comment is ([err_fmt_directive.txt](../../cmd/muxt/testdata/err_fmt_directive.txt)):

```text
page.gohtml:2:1: unknown directive "muxt:fmt of": want muxt:fmt off, muxt:fmt on, or muxt:fmt preserve
```

An `on` with no `off` before it in the same element, branch, or `define` is also an error, as is a `preserve` with no node after it.

Reference: [muxt fmt](../reference/commands/fmt.md). Directive tests: [directive_test.go](../../internal/template/format/directive_test.go).
