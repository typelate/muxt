# muxt fmt

Formats `.gohtml` templates without changing what the browser renders.

```bash
muxt fmt -w .
```

```text
muxt fmt [flags] [path ...]
```

Like `gofmt`, `muxt fmt` has no formatting options. Indentation is one tab per level. The flags only say where the result goes.

| Flag | Description |
|---|---|
| `-w, --write` | Write the result to the source file instead of standard output. |
| `-l, --list` | Print the name of each file whose formatting differs. |
| `-d, --diff` | Print a unified diff instead of the formatted source. |

The flags combine as they do in `gofmt`: `-l -w` lists and writes. With none of them, the formatted source goes to standard output ([reference_fmt_write.txt](../../../cmd/muxt/testdata/reference_fmt_write.txt), [reference_fmt_list.txt](../../../cmd/muxt/testdata/reference_fmt_list.txt), [reference_fmt_diff.txt](../../../cmd/muxt/testdata/reference_fmt_diff.txt)).

## Input

With no path, `muxt fmt` reads standard input and reports it as `<standard input>`. Standard input belongs to no Go package, even when `muxt fmt` runs in a package directory: an editor pipes a buffer from wherever it runs, so the package there may not be the buffer's ([reference_fmt_stdin_package.txt](../../../cmd/muxt/testdata/reference_fmt_stdin_package.txt)). `-w` needs a file or directory and is rejected without one.

A path that is a file is formatted whatever its extension. A path that is a directory is walked for regular files ending in `.gohtml`. The walk skips:

- directories named `testdata` or `vendor`, and directories starting with `.` or `_`;
- files starting with `.`;
- symbolic links. A link is formatted, and written through, only when it is named on the command line.

## Exit status

| Status | When |
|---|---|
| `0` | Every file was read, parsed and formatted. |
| `1` | A file could not be read or parsed, a file's Go package loaded with errors, a `muxt:fmt` comment is not valid, standard input could not be read, the command line is wrong (`-w` with no path), or `-l` or `-d` found a file that would change. |

A problem with one file is printed on stderr and the other files are still formatted. A template that does not parse is reported as `file:line:col: message` and left unchanged ([err_fmt_parse.txt](../../../cmd/muxt/testdata/err_fmt_parse.txt)). A path that does not exist is reported as the error opening it ([err_fmt_missing_file.txt](../../../cmd/muxt/testdata/err_fmt_missing_file.txt)).

## What it may change

`muxt fmt` never changes what the browser renders. It changes whitespace only where the browser ignores it, and leaves the rest as written.

- Whitespace between block-level siblings, and at the start and end of a block, is re-indented and may be removed or added.
- Inside running text, whitespace may become shorter but not another kind. Spaces stay a single space, a line break stays a single line break, and a gap that is empty stays empty. Prose is never re-wrapped, and two inline elements on separate lines are never joined.
- Whitespace removed at run time by a `{{-` or `-}}` trim marker may be moved. A marker that trims whitespace written in the source is never added.
- Whitespace is never inserted next to an action, because the action's output is unknown.

`muxt fmt` leaves these as written, byte for byte:

- the text of `script`, `style`, `textarea`, `title`, `pre`, and other raw text elements;
- the content of any element whose `white-space` is `pre`, `pre-wrap`, `pre-line`, or `break-spaces`, including descendants that set it back;
- elements whose CSS it cannot see: a dynamic `class` or `style` that an author rule could match, or any `class` or `id` when no Go package is loaded for the file (standard input, or a file outside a package), even next to a `<style>` element, since that may not be all the CSS ([reference_fmt_stdin_style.txt](../../../cmd/muxt/testdata/reference_fmt_stdin_style.txt));
- the surroundings of an element taken out of flow by `float` or `position: absolute` or `position: fixed`, and of an element that is `display: none` or `hidden`;
- regions marked with a [`muxt:fmt` directive](../../how-to/keep-template-formatting.md).

When `muxt fmt` cannot tell whether whitespace matters, it does not format that part ([reference_static_files_fmt_unresolved.txt](../../../cmd/muxt/testdata/reference_static_files_fmt_unresolved.txt)).

## Where CSS comes from

CSS decides whether an element is block or inline and whether it preserves whitespace. `muxt fmt` reads author CSS from three places:

1. `<style>` elements in the templates of the file's own package.
2. `style` attributes.
3. Stylesheets that a `<link rel="stylesheet">` or an `@import` reaches through the static assets route of `muxt generate`.

The route is recorded in the header of the generated file. A `href` under the `--output-static-assets-path`, written out or as `{{.Path.ReadStatic}}app.css`, is read from the `--input-static-assets-directory`, and so is each sheet it imports from there ([Static assets](generate.md#static-assets), [reference_static_files_fmt.txt](../../../cmd/muxt/testdata/reference_static_files_fmt.txt)).

A stylesheet anywhere else cannot be read: another site, a path no static assets route serves, or a file that is not there. Then author CSS is unknown for the whole template set, and every element that a rule could affect is left as written.

The templates of a package are all read, even when only one is named, because a layout may link the sheet that styles a partial. They are the files the package embeds with `//go:embed` whose names end in `.gohtml`, `.html`, `.htm`, `.tmpl` or `.gotmpl` ([reference_static_files_fmt_html_context.txt](../../../cmd/muxt/testdata/reference_static_files_fmt_html_context.txt)), and the other files of the run in the same directory. Other embedded files are not read. When one of them cannot be read or parsed, author CSS is unknown, the named file is left as written, and the unreadable one is reported with exit status `1` ([reference_static_files_fmt_unreadable_context.txt](../../../cmd/muxt/testdata/reference_static_files_fmt_unreadable_context.txt), [reference_static_files_fmt_broken_context.txt](../../../cmd/muxt/testdata/reference_static_files_fmt_broken_context.txt)).

When the package itself loads with errors, such as a `//go:embed` pattern that matches no file, the templates it embeds may be incomplete. Author CSS is unknown, and the load error is reported once, with exit status `1` ([err_fmt_package_load.txt](../../../cmd/muxt/testdata/err_fmt_package_load.txt)).

CSS is per package. A template in another directory belongs to another package, and its `<style>` elements and links do not apply, even when both directories are formatted in one run ([reference_fmt_package_per_directory.txt](../../../cmd/muxt/testdata/reference_fmt_package_per_directory.txt)).

## Limits

- The body of a `{{define}}` or `{{block}}` is formatted as if called from normal whitespace. A body called from inside `<pre>`, or from another element that preserves whitespace, is still formatted as a normal body, even when the caller is in the same file, and a changed edge of the body could become visible.
- CSS is read per package. When the page shell that holds the `<link>` lives in another Go package, the templates of this package see no author CSS: an element with a `class` or `id` is classified by the HTML defaults alone, as if no author rule matched it.
- Prose is never re-wrapped. There is no line width.
