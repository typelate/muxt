# muxt check

Type-checks each template against the data type at its `ExecuteTemplate` call site and reports templates nothing renders.

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

In an `html/template` set, check also fails on HTML comments and markup that `html/template` writes differently from what the template says, each at its `file:line:col`:

- a `<!--` inside `<title>` or `<textarea>`: those elements hold text, not markup, so the page shows the comment;
- a `<!` that starts no comment or doctype (`<![if !IE]>`, `<!x>`), or a `<?`: `html/template` escapes the `<`, so the page shows the text;
- a comment opened as `<!-->` or `<!--->`: a browser ends the comment right there, but `html/template` strips everything through the next `-->`, and the message gives that position.

Comments in text, actions inside them (`<!-- {{.X}} -->`), and comments in attribute values, `<script>` and `<style>` pass ([err_check_html_comments.txt](../../../cmd/muxt/testdata/err_check_html_comments.txt), [reference_check_html_comments.txt](../../../cmd/muxt/testdata/reference_check_html_comments.txt)). Use `{{/* */}}` for a comment that must not reach the page.

`--allow-unused` lists templates nothing renders under `Unused templates (allowed by --allow-unused):` and does not fail on them. Use it for templates rendered some other way check cannot see, such as through `templates.Lookup(name).Execute`, a clone of the set, or code in another package the set is passed to. Type errors, and route templates with no generated handler, still fail ([err_check_with_unused_template.txt](../../../cmd/muxt/testdata/err_check_with_unused_template.txt)).

```bash
muxt check --allow-unused
```

`-v` prints `checking endpoint NAME` for each call site, and the declaration of the type under each field or method error. Flags: [cli.md](../cli.md#flags).

## Edge cases

- **A non-literal template name.** A call whose template name is not a literal, as in a `render(w, name, data)` helper, may render any template, so while one exists no template is reported unused. It does not need `--allow-unused`: check names that call with an `ExecuteTemplate not checked` line and reports nothing unused.
- **Shared templates.** A template shared between two templates variables is reported when either leaves it unused ([err_template_in_shared_file_unused_by_one_variable.txt](../../../cmd/muxt/testdata/err_template_in_shared_file_unused_by_one_variable.txt)).
- **`text/template` sets.** A `text/template` set is text, so the HTML comment and markup checks do not apply to it.
