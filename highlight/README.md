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

### Option 1: symlink into your extensions directory (development)

```bash
ln -s "$(pwd)/highlight" ~/.vscode/extensions/typelate.muxt-template-syntax-0.1.0
```

Then run **Developer: Reload Window** in VSCode (`Cmd+Shift+P` → "Reload Window").

### Option 2: package and install a `.vsix`

```bash
npm install -g @vscode/vsce
cd highlight
vsce package
code --install-extension muxt-template-syntax-0.1.0.vsix
```

## Configuration

### File associations

The extension registers the `gohtml` language for `.gohtml` files. To also use it for `.html` templates (or any other pattern), add to your `settings.json`:

```json
{
  "files.associations": {
    "**/templates/**/*.html": "gohtml",
    "*.html.tmpl": "gohtml"
  }
}
```

### Customizing colors

Every token has a TextMate scope you can target with `editor.tokenColorCustomizations`. The Muxt-specific scopes:

| Scope | Highlights |
|---|---|
| `keyword.control.http-method.muxt` | `GET`, `POST`, `PUT`, `PATCH`, `DELETE` |
| `invalid.illegal.http-method.muxt` | Methods muxt rejects (e.g. `YOLO`) |
| `entity.name.namespace.host.muxt` | Host prefix before the path |
| `string.other.path.muxt` | Path segment text |
| `punctuation.separator.path.muxt` | `/` separators |
| `variable.parameter.path.muxt` | `{id}`, `{path...}`, `{$}` parameter names |
| `keyword.operator.variadic.path.muxt` | The `...` in `{path...}` |
| `constant.numeric.http-status.muxt` | Numeric status like `200` |
| `support.constant.http-status.muxt` | Named status like `http.StatusCreated` |
| `entity.name.function.call.muxt` | Handler function name |
| `variable.language.call-arg.muxt` | `ctx`, `request`, `response`, `form`, `err` |
| `variable.parameter.call-arg.muxt` | Path-derived call arguments |

General template scopes: `comment.block.gotemplate`, `keyword.control.gotemplate`, `support.function.builtin.gotemplate`, `variable.other.gotemplate` (`$vars`), `variable.other.member.gotemplate` (`.Fields`), `string.quoted.double.gotemplate`, `punctuation.section.embedded.begin.gotemplate` / `...end.gotemplate` (`{{` / `}}`), `keyword.operator.trim.gotemplate` (`-` trim markers), `keyword.operator.pipe.gotemplate` (`|`).

Example — make routes pop:

```json
{
  "editor.tokenColorCustomizations": {
    "textMateRules": [
      {
        "scope": "keyword.control.http-method.muxt",
        "settings": { "foreground": "#C586C0", "fontStyle": "bold" }
      },
      {
        "scope": "variable.parameter.path.muxt",
        "settings": { "foreground": "#9CDCFE", "fontStyle": "italic" }
      },
      {
        "scope": "entity.name.function.call.muxt",
        "settings": { "foreground": "#DCDCAA" }
      }
    ]
  }
}
```

Use **Developer: Inspect Editor Tokens and Scopes** (`Cmd+Shift+P`) to see the scopes under the cursor while tweaking.

## How it works

Two TextMate grammars:

- [`syntaxes/gohtml.tmLanguage.json`](syntaxes/gohtml.tmLanguage.json) — the main `text.html.gohtml` grammar; it just includes VSCode's built-in `text.html.basic` grammar.
- [`syntaxes/gotemplate-injection.tmLanguage.json`](syntaxes/gotemplate-injection.tmLanguage.json) — an [injection grammar](https://code.visualstudio.com/api/language-extensions/syntax-highlight-guide#injection-grammars) (`injectionSelector: "L:text.html.gohtml ..."`) that inserts template-action rules ahead of the HTML rules at every nesting level. Injection — rather than plain inclusion — is what makes `{{...}}` highlight inside attribute-value strings, where an included rule would never be reached.

The route-name rule mirrors `templateNameMux` from `internal/muxt/definition.go`:

```
^(?P<pattern>((?P<METHOD>[A-Z]+)\s+)?(?P<HOST>([^/])*)(?P<PATH>(/(\S)*)))(\s+(?P<HTTP_STATUS>(\d|http\.Status)\S+))?(?P<CALL>.*)?$
```

adapted for TextMate matching: it is anchored to the quoted name with `\G`...`"` instead of `^`...`$`, and `\S`/`.` are narrowed to exclude `"` so the match cannot escape the string literal. If muxt's pattern changes, update the `template-name` rule to match.
