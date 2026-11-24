# Template Engine

muxt's template engine is the module [`github.com/typelate/htt`](https://github.com/typelate/htt), so that a module can require it: [`muxt test-template-mutations`](commands/test-template-mutations.md#how-the-pre-pass-works) requires it in its coverage pre-pass, and tools can use it too.

- `github.com/typelate/htt` parses templates, recording source positions and HTML structure, and executes the trees it parses.
- `github.com/typelate/htt/template` builds the `html/template` API on it, with contextual escaping. Its content types (`HTML`, `URL`, `JS` and the rest) are its own, and it also trusts values of `html/template`'s.
- `github.com/typelate/htt/lex` is the lexer both use.

The module has its own versions; the one muxt uses is in muxt's `go.mod`. There is no stability promise, and any release may change the API.

`github.com/typelate/htt/template` writes the template blocks each execution reaches to a cover profile when `MUXT_TEMPLATE_COVER` names a directory. The pre-pass sets it; unset, nothing is written.

The `html/template` shim the pre-pass overlays has three known gaps: `Template.Tree` and `AddParseTree` use `*htt.Tree`, not `*parse.Tree`, and `Error.Node` is an `htt.Node`, not a `parse.Node`. `github.com/typelate/htt/template` escapes with a copy of `html/template`'s escaper from one Go release, named in the header of its [`esc_escape.go`](https://github.com/typelate/htt/blob/main/template/esc_escape.go): its output is that release's, whatever Go builds the program. The parser muxt checks and formats with does not end a `script` or `style` element at an end tag that holds an attribute or a `/`, as in `</script a="b">`, where a browser does.
