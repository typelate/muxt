# muxt check

Type-checks each template against the data type at its `ExecuteTemplate` call site and reports templates nothing renders.

`use_templates_variable` comes from the directory's [`muxt.json`](../muxt-json.md) entry when there is one; `--use-templates-variable` is rejected there.

With no flags in the directory of a `muxt.json`, `check` checks every entry, dependencies first, from one package load. Each `ok:` line follows its entry's key, and the run stops at the first failure, named by key: `Error: example.com/m: fail: variable templates not found in package example.com/m`.

```bash
muxt check
```

```text
ok: 4 templates
```

```text
muxt check [package-dir] [flags]
```

The package directory defaults to the working directory and is resolved like `-C`; a missing directory or a file is an error ([reference_package_directory_argument.txt](../../../cmd/muxt/testdata/reference_package_directory_argument.txt)).

An `ExecuteTemplate` call whose template name is not a string literal cannot be checked; each one is named on stderr and counted in the summary, as in `ok: 4 templates, 1 call site skipped` ([Type Checking](../type-checking.md#preconditions)).

Run `generate` first: `check` reads receiver types from the generated file and reports a route template with no handler as waiting for one ([err_check_before_generate.txt](../../../cmd/muxt/testdata/err_check_before_generate.txt)).

What fails: [Type Checking](../type-checking.md#what-is-checked).

In an `html/template` set, `check` also fails, at `file:line:col`, where `html/template` writes the templates differently from how a browser reads them. It runs the standard library's `html/template` escaper over the same sources, so what it reports is what `html/template` does:

- a `<!--` inside `<title>` or `<textarea>`: those elements hold text, not markup, so the page shows the comment;
- a `<!` that starts no comment or doctype (`<![if !IE]>`, `<!x>`), or a `<?`: `html/template` escapes the `<`, so the page shows the text;
- a comment a browser ends somewhere other than `html/template` does: one opened as `<!-->` or `<!--->`, or closed with `--!>`, ends there in a browser, while `html/template` strips everything through the next `-->`, even one inside a template the comment calls; the message gives both positions;
- a comment, `<!` or `<?` inside `<xmp>` or `<plaintext>`: a browser shows their content as raw text, comments and `&lt;` included, while `html/template` reads it as markup, strips the comment and escapes the `<`;
- a template `html/template` refuses to execute, such as a `<` among the attributes of a tag an `{{if}}` opens, or a comment that never closes: check gives `html/template`'s reason. Only templates an `ExecuteTemplate` call names are checked this way, so a set with no such call is not; the error is reported in the template it comes from.

A partial is checked as it renders in text: in the element a `{{template}}` call in text is in, or at the top level when nothing calls it. One only called inside `<script>`, `<style>` or an attribute value is escaped there and not checked; one called inside `<title>` or `<textarea>` is checked for `<!--`.

Comments in text, actions inside them (`<!-- {{.X}} -->`), and comments in attribute values, `<script>` and `<style>` pass ([err_check_html_comments.txt](../../../cmd/muxt/testdata/err_check_html_comments.txt), [reference_check_html_comments.txt](../../../cmd/muxt/testdata/reference_check_html_comments.txt)). For a comment that must not reach the page, use `{{/* */}}`.

`--allow-unused` lists templates nothing renders under `Unused templates (allowed by --allow-unused):` and does not fail on them. Use it for templates rendered some other way check cannot see, such as through `templates.Lookup(name).Execute`, a clone of the set, or code in another package the set is passed to. Type errors, and route templates with no generated handler, still fail ([err_check_with_unused_template.txt](../../../cmd/muxt/testdata/err_check_with_unused_template.txt)).

```bash
muxt check --allow-unused
```

`-v` prints `checking endpoint NAME` for each call site, and the declaration of the type under each field or method error. Flags: [cli.md](../cli.md#flags).

## Edge cases

- **A non-literal template name.** A call whose template name is not a literal, as in a `render(w, name, data)` helper, may render any template, so while one exists no template is reported unused. It does not need `--allow-unused`: check names that call with an `ExecuteTemplate not checked` line and reports nothing unused.
- **Files of another package.** A template from another package's `embed.FS` is not reported unused: [rule and exception](../templates-variable.md#sharing-another-packages-files). A template in the package's own files is still reported.
- **Shared templates.** A template shared between two templates variables is reported when either leaves it unused ([err_template_in_shared_file_unused_by_one_variable.txt](../../../cmd/muxt/testdata/err_template_in_shared_file_unused_by_one_variable.txt)).
- **`text/template` sets.** A `text/template` set is text, so the HTML comment and markup checks do not apply to it.
