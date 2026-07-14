# Muxt Go Template Syntax Highlighting for VSCode

A VSCode extension that highlights Go `html/template` files (`.gohtml`) — both the HTML and the template actions — with special treatment for [Muxt](https://github.com/typelate/muxt) route names in `{{define}}`, `{{block}}`, and `{{template}}` actions.

## Features

- **Full HTML highlighting** — delegates to VSCode's built-in HTML grammar, so tags, attributes, entities, and embedded CSS/JS work as usual.
- **Template actions** — `{{if}}`, `{{range}}`, `{{with}}`, `{{end}}`, pipelines (`|`), variables (`$x`), field access (`.Field`), built-in functions (`printf`, `len`, `eq`, …), strings, numbers, and trim markers (`{{-` / `-}}`).
- **Template comments** — `{{/* ... */}}`, including trim-marker forms (`{{- /* ... */ -}}`). `Cmd+/` (toggle block comment) inserts `{{/* */}}`.
- **Actions inside HTML attributes** — `href="{{.URL}}"` and `hx-get="/x/{{.ID}}"` highlight correctly, because template actions are injected into every HTML scope, not just top-level text.
- **Muxt route names** — when a `define`/`block`/`template` name matches Muxt's `templateNameMux` pattern, each part gets its own scope:

  ```gotemplate
  {{define "GET /articles/{id} 200 GetArticle(ctx, id)"}}{{end}}
            └┬┘ └────────┬───┘ └┬┘ └────────┬────────┘
          METHOD        PATH  STATUS       CALL
  ```

  - HTTP method (`GET`, `POST`, `PUT`, `PATCH`, `DELETE`); any other all-caps method is marked `invalid.illegal` since `muxt generate` rejects it
  - Host prefix (e.g. `example.com/`)
  - Path segments, with `{param}` and `{path...}` wildcards highlighted as parameters
  - HTTP status (`200` or `http.StatusCreated`)
  - Handler call: function name, and arguments — the well-known identifiers `ctx`, `request`, `response`, `form`, and `err` are distinguished from path-derived arguments

  Names that don't match the route pattern (e.g. `{{define "footer"}}`) are highlighted as plain strings.

See [`examples/example.gohtml`](examples/example.gohtml) for a file exercising every feature.

## Installation

Download the build artifact from [a recent build](https://github.com/typelate/muxt/actions/workflows/vsix.yml). Unzip it then install it with this command:

```bash
code --install-extension muxt-template-syntax-0.1.1.vsix
```
Or in the VSCode extentions panel, click the `...` button on the top right and click "Install from VSIX".

## Configuration

### File associations

The extension registers the `gohtml` language for `.gohtml` files. To also use it for `.html` templates (or any other pattern), add to your `settings.json`:

```json
{
  "files.associations": {
    "**/templates/**/*.html.template": "gohtml",
    "*.html.tmpl": "gohtml"
  }
}
```
