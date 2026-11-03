# muxt fmt

Re-indents `.gohtml` templates without changing what the browser renders.

```bash
muxt fmt -w .
```

```text
muxt fmt [flags] [path ...]
```

`muxt fmt` changes only the leading whitespace of existing lines, strips trailing whitespace, and collapses runs of blank lines to one. It never joins or splits a line, never edits a token, and leaves what it is unsure about exactly as written, whole element at a time.

Like `gofmt`, it has no formatting options. Indentation is one tab per level. The flags only say where the result goes.

| Flag | Description |
|---|---|
| `-w, --write` | Write the result to the source file instead of standard output. |
| `-l, --list` | Print the name of each file whose formatting differs. |
| `-d, --diff` | Print a unified diff instead of the formatted source. |

The flags combine as they do in `gofmt`: `-l -w` lists and writes, and exits `1` when a file changed. With none of them, the formatted source goes to standard output ([reference_fmt_write.txt](../../../cmd/muxt/testdata/reference_fmt_write.txt), [reference_fmt_list.txt](../../../cmd/muxt/testdata/reference_fmt_list.txt), [reference_fmt_diff.txt](../../../cmd/muxt/testdata/reference_fmt_diff.txt)).

## Input

With no path, `muxt fmt` reads standard input and reports it as `<standard input>`. Standard input belongs to no Go package, even when `muxt fmt` runs in a package directory: an editor pipes a buffer from wherever it runs, so the package there may not be the buffer's ([reference_fmt_stdin_package.txt](../../../cmd/muxt/testdata/reference_fmt_stdin_package.txt)). `-w` needs a file or directory and is rejected without one.

A path that is a file is formatted whatever its extension. A path that is a directory is walked for regular files ending in `.gohtml`. The walk skips:

- generated files, see [Generated files](#generated-files);
- directories named `testdata` or `vendor`, and directories starting with `.` or `_`;
- files starting with `.`;
- symbolic links. A link is formatted, and written through, only when it is named on the command line.

## Exit status

| Status | When |
|---|---|
| `0` | Every file was read, parsed and formatted. |
| `1` | A file could not be read or parsed, a file's Go package loaded with errors, a `muxt:fmt` comment is not valid, standard input could not be read, the command line is wrong (`-w` with no path), or `-l` or `-d` found a file that would change. |

A problem with one file is printed on stderr and the other files are still formatted. A template that does not parse is reported as `file:line:col: message` and left unchanged ([err_fmt_parse.txt](../../../cmd/muxt/testdata/err_fmt_parse.txt)). A path that does not exist is reported as the error opening it ([err_fmt_missing_file.txt](../../../cmd/muxt/testdata/err_fmt_missing_file.txt)).

## What it changes

`muxt fmt` is a reindenter. Every line that exists in the source still exists in the output, with the same text after its indentation. The one thing that changes is how much whitespace comes before that text (and the whitespace after it, which is removed).

The safety rule, in terms of what a reader of the page could notice: whitespace between two pieces of content can change in amount, never in whether it exists and never in kind. A gap with no whitespace stays empty. A gap with spaces and no line break stays a gap with no line break. A gap with a line break keeps at least one line break. So a space the browser shows between two words is still there, and two inline elements that touch still touch. `fmt` checks this against the HTML parser's own tree, so a gap is judged by the element the browser puts it in, not by where it sits in the source ([reference_fmt_write.txt](../../../cmd/muxt/testdata/reference_fmt_write.txt)).

Beyond that:

- Spaces inside a line are left as written, and prose is never re-wrapped. There is no line width.
- Trailing whitespace is stripped only where the run is one `fmt` may re-indent; inside text it leaves as written it stays.
- Blank lines: a run of several becomes one. Blank lines are never added and never removed entirely. The final newline of the file is kept.
- Line endings are normalized to LF (CRLF and lone CR), everywhere in the file, like `gofmt`: raw text, attribute values, comments, and regions left as written or under `muxt:fmt off` included. The browser does the same before it reads the page, so the page does not change ([format_test.go](../../../internal/template/format/format_test.go), case "CRLF becomes LF"; [emit_test.go](../../../internal/template/format/emit_test.go)).
- Trim markers (`{{-`, `-}}`) and the spacing inside an action are never touched. Whitespace that a trim marker removes at run time may be re-indented, since it is never rendered, but a marker is never added or removed.
- Attribute continuation lines in a tag are indented one level deeper than the tag. Spaces after the tabs, used to line attributes up, are kept only when the source line already starts with tabs followed by spaces; otherwise the whole indent becomes tabs.

## Depth

Depth follows the tag structure of the source, one tab per level ([full_page.txtar](../../../internal/template/format/testdata/full_page.txtar)):

- A `>` or `/>` alone on a line sits at the depth of its tag.
- Element children go one level deeper than their element. `<html>`, `<head>`, `<body>` and `<!DOCTYPE>` stand at the same column, and their children go one level deeper.
- The bodies of `{{if}}`, `{{with}}`, `{{range}}` and their `{{else}}` branches go one level deeper than the keyword lines, which stay at the enclosing depth. A branch does not change the depth after its `{{end}}`.
- `{{template}}` and `{{block}}` are transparent: the content of a `{{block}}` sits at the depth of the block itself.
- `{{define}}` is a top-level declaration wherever it sits. Its opener, body and `{{end}}` start at column 0, and the body is indented from there.
- Optional end tags (`li`, `dt`, `dd`, `p`, `option`, `tr`, `td`, `th`, `thead`, `tbody`, `tfoot`, `colgroup`, `caption`) are implied the way the HTML Standard says. Any other element left unclosed ends at its parent's end tag.
- A multi-line `{{/* comment */}}` shifts as a unit, its continuation lines moving by the same amount as its first line, when each continuation line starts with the old indent. A multi-line action keeps its opening line as written, because string literals inside it may contain whitespace.
- `<template>` contents are re-indented like any other markup.

## What is left as written

When `muxt fmt` cannot be sure a change is invisible, it leaves the whole element as written: the opening line, the body and the closing line. The closing tag of its parent still lands at the parent's depth, and its siblings are still re-indented. These are the cases:

- Text of `script`, `style`, `textarea`, `title`, `pre`, `iframe`, `noembed`, `noframes`, `noscript`, `listing`, `xmp`, `plaintext` and other raw text elements. Every byte after `>` up to the closing tag counts, including a line break the parser drops. An element left as written whose closing tag is on its opening line may still have that line re-indented.
- Any element whose computed `white-space` preserves whitespace (`pre`, `pre-wrap`, `break-spaces`), and everything inside it. Under `pre-line` the whitespace is re-indented but the number of line breaks is kept exactly.
- Elements whose CSS it cannot see. A rule that sets `white-space`, `white-space-collapse` or `all` and might match the element makes it unknown, as does a stylesheet that could not be loaded or a `style` attribute that contains an action. So does such a rule inside `@media`, `@supports`, `@container` or `@layer` that could give the element a different `white-space`, or an `all: revert`. `all: unset` and `all: initial` keep the element collapsing, so it is still re-indented ([whitespace_test.go](../../../internal/analysis/html/whitespace_test.go)). A class that no such rule names is harmless when a package is loaded: display (block or inline) does not matter, only `white-space` does.
- Every element with a `class` or `id`, when no Go package is loaded for the file (standard input, or a file outside a module or outside any embedding package). Author CSS is unknown there, even next to a `<style>` element, since that may not be all the CSS ([reference_fmt_stdin_style.txt](../../../cmd/muxt/testdata/reference_fmt_stdin_style.txt)).
- Custom elements (a tag name with a hyphen, or an `is=` attribute) and declarative shadow hosts (an element with a `<template shadowrootmode>` child), because script or the shadow tree may restyle them.
- Text inside SVG (`text`, `tspan`, `textPath`, and any SVG element with text or an action of its own) and MathML token elements (`mi`, `mo`, `mn`, `ms`, `mtext`, `annotation-xml`), because those do not follow the HTML collapsing rules.
- Misnested markup that the browser moves or repairs (a stray `</p>`, table content foster-parented out of its table, an unclosed `pre` held open past an end tag). The gap goes to the most preserving of the elements it could belong to ([misnested.txtar](../../../internal/template/format/testdata/misnested.txtar), [unclosed_p.txtar](../../../internal/template/format/testdata/unclosed_p.txtar)).
- Tags it cannot read: a start tag cut off by an action in a way the parser cannot follow, and a head with text but no `<body>` element anywhere in the file (the whole file is left as written).
- Regions you mark with a [`muxt:fmt` directive](../../how-to/keep-template-formatting.md).

A list of siblings is treated as a unit in one respect. When any gap in it is left as written because `muxt fmt` is unsure about white-space (unknown CSS, an unresolved caller), every gap in that list is left as written, including trimmed ones, and so is everything inside it. A `pre`, `script`, custom element or SVG does not do this: such an element is an island, and its siblings are still re-indented. A branch of `{{if}}`, `{{with}}` or `{{range}}` whose body is left as written keeps its own keyword lines (the opener, `{{else}}` and `{{end}}`) as written too. That freeze reaches outward through any enclosing branch bodies, keyword lines included, and stops at the nearest element, `{{block}}` body or define body, where the frozen branch is an island and its siblings are still re-indented. An unsure element that spans several lines freezes its list wherever it starts on a line.

When `muxt fmt` cannot tell whether whitespace matters, it does not format that part ([reference_static_files_fmt_unresolved.txt](../../../cmd/muxt/testdata/reference_static_files_fmt_unresolved.txt)).

## Where CSS comes from

CSS decides whether an element is block or inline and whether it preserves whitespace. `muxt fmt` reads author CSS from the package that embeds the file, in three places:

1. `<style>` elements in the templates of the file's own package.
2. `style` attributes.
3. Stylesheets that a `<link rel="stylesheet">` or an `@import` reaches through the static assets route of `muxt generate`.

The route is recorded in the header of the generated file. A `href` under the `--output-static-assets-path`, written out or as `{{.Path.ReadStatic}}app.css`, is read from the `--input-static-assets-directory`, and so is each sheet it imports from there ([Static assets](generate.md#static-assets), [reference_static_files_fmt.txt](../../../cmd/muxt/testdata/reference_static_files_fmt.txt)).

A stylesheet anywhere else cannot be read: another site, a path no static assets route serves, or a file that is not there. Then author CSS is unknown for the whole template set, and every element that a rule could affect is left as written.

The templates of a package are all read, even when only one is named, because a layout may link the sheet that styles a partial. They are the files the package embeds with `//go:embed` whose names end in `.gohtml`, `.html`, `.htm`, `.tmpl` or `.gotmpl` ([reference_static_files_fmt_html_context.txt](../../../cmd/muxt/testdata/reference_static_files_fmt_html_context.txt)), and the other files of the run in the same directory. Other embedded files are not read. Files under the `--input-static-assets-directory` are read only as stylesheets, never as templates, even when the package embeds them ([reference_static_files_fmt_assets_not_templates.txt](../../../cmd/muxt/testdata/reference_static_files_fmt_assets_not_templates.txt)). When one of them cannot be read or parsed, author CSS is unknown, the named file is left as written, and the unreadable one is reported with exit status `1` ([reference_static_files_fmt_unreadable_context.txt](../../../cmd/muxt/testdata/reference_static_files_fmt_unreadable_context.txt), [reference_static_files_fmt_broken_context.txt](../../../cmd/muxt/testdata/reference_static_files_fmt_broken_context.txt)).

When the package itself loads with errors, such as a `//go:embed` pattern that matches no file, the templates it embeds may be incomplete. Author CSS is unknown, and the load error is reported once, with exit status `1` ([err_fmt_package_load.txt](../../../cmd/muxt/testdata/err_fmt_package_load.txt)).

CSS is per package. A template in another directory belongs to another package, and its `<style>` elements and links do not apply, even when both directories are formatted in one run ([reference_fmt_package_per_directory.txt](../../../cmd/muxt/testdata/reference_fmt_package_per_directory.txt)).

## Which package a file belongs to

A file belongs to the nearest Go package, starting in its directory and going up, that embeds it. Templates in `templates/` that are embedded by `//go:embed templates/*.gohtml` in the parent directory belong to the parent's package, so that package's `<style>` elements, defines and static assets apply to them ([reference_fmt_package_subdirectory.txt](../../../cmd/muxt/testdata/reference_fmt_package_subdirectory.txt), [reference_fmt_package_subdirectory_static_assets.txt](../../../cmd/muxt/testdata/reference_fmt_package_subdirectory_static_assets.txt)). When more than one package embeds a file, the nearest one wins ([reference_fmt_package_subdirectory_parent_wins.txt](../../../cmd/muxt/testdata/reference_fmt_package_subdirectory_parent_wins.txt)). Standard input has no package.

## Define bodies and call sites

([define_call_sites.txtar](../../../internal/template/format/testdata/define_call_sites.txtar), [document_template_calls.txtar](../../../internal/template/format/testdata/document_template_calls.txtar))

A `{{define}}` body is formatted according to where it is called. For each `{{template}}` or `{{block}}` call in the package, `fmt` looks at the element that holds the call and takes the most preserving answer across all call sites. A body called only from collapsing contexts is re-indented. A body called from inside `<pre>`, a `white-space: pre` element, an attribute value, `<textarea>`, or a raw text element is left as written, and a body called from `pre-line` keeps its line break count.

The limits:

- A define that is a route (its name is an HTTP pattern), a define nothing calls, and a define called only from another Go package are all assumed to be called from collapsing contexts. If you call one from `<pre>` somewhere `fmt` cannot see, re-indenting it can change what the browser shows. Mark such a define with a `muxt:fmt` directive.
- A `{{template}}` call whose define `fmt` cannot find (it lives in a file not in the run) in `<head>` or in a table section is assumed to emit what belongs there only when every element that could own the text that follows collapses whitespace. Otherwise the call is treated as possibly writing text, and the surrounding gaps are left as written.

## Generated files

When walking a directory, `muxt fmt` skips a file whose first non-blank line is a single template comment or HTML comment matching Go's convention, `Code generated ... DO NOT EDIT.`, as in `{{/* Code generated by a tool. DO NOT EDIT. */}}`. The comment must be on its own line and must be one comment. A generated file named on the command line is formatted ([reference_fmt_generated_skip.txt](../../../cmd/muxt/testdata/reference_fmt_generated_skip.txt)).

## Migrating a project

`muxt fmt` can only trust CSS it can read. A project that serves its stylesheets by hand (an `http.FileServerFS` registered on the mux beside `TemplateRoutes`) hides them from `fmt`: every `<link rel="stylesheet">` is a sheet it cannot resolve, so author CSS is unknown and every element is possibly restyled. All of the file's markup is left as written.

To fix that, let `muxt generate` serve the directory:

```bash
muxt generate --input-static-assets-directory=assets --output-static-assets-path=/static
```

Then remove the hand-written file server for that directory and link the sheets under the new path, as `{{.Path.ReadStatic}}app.css` or written out as `/static/app.css`. The generated file's header records both flags, and `fmt` reads them back. See [Static assets](generate.md#static-assets). Run `muxt fmt -l .` before and after: files that were unchanged because their CSS was unknown now get formatted.

## Limits

- CSS is read per package. When the page shell that holds the `<link>` lives in another Go package, the templates of this package see no author CSS beyond their own `<style>` elements.
- A define that is a route, is never called, or is called only from another package is assumed to be called from a collapsing context.
- Prose is never re-wrapped. There is no line width.
