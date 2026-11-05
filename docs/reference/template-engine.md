# Template Engine

muxt's template engine is public so that a module can require it: [`muxt test-template-mutations`](commands/test-template-mutations.md#how-the-pre-pass-works) requires it from the module under test in its coverage pre-pass, and tools can use it too.

- [`github.com/typelate/muxt/template`](../../template/doc.go) parses templates, recording source positions and HTML structure, and executes the trees it parses.
- [`github.com/typelate/muxt/template/html`](../../template/html/template.go) builds the `html/template` API on it, with contextual escaping. Its content types (`HTML`, `URL`, `JS` and the rest) are its own, and it also trusts values of `html/template`'s.
- `github.com/typelate/muxt/template/lex` is the lexer both use.

The packages are versioned with the CLI, so the engine of muxt v0.23.0 is the one at that version:

```sh
go get github.com/typelate/muxt/template@v0.23.0
```

There is no stability promise beyond the CLI's own, and any release may change their API.

`template/html` writes the template blocks each execution reaches to a cover profile when `MUXT_TEMPLATE_COVER` names a directory. The pre-pass sets it; unset, nothing is written.

The `html/template` shim the pre-pass overlays has three known gaps: `Template.Tree` and `AddParseTree` use `template.Tree`, not `*parse.Tree`, and `Error.Node` is a value with only a `String` method, not a `parse.Node`. The engine also cannot execute a `{{template}}` call in a tag, as in `<input name='q'{{template "attrs" .}}>`, which `html/template` executes in attribute context.
