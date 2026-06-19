# Go HTML Template Syntax Highlighting

TextMate grammar for Go `html/template` files with muxt route pattern support.

## Features

- Go template actions: `{{if}}`, `{{range}}`, `{{with}}`, `{{define}}`, `{{block}}`
- Built-in functions: `and`, `or`, `eq`, `len`, `printf`, etc.
- Variables (`$var`), fields (`.Field`), and context (`.`)
- Strings, numbers, and escape sequences
- Muxt route patterns in template names:
  - HTTP methods: `GET`, `POST`, `PUT`, `DELETE`
  - Path parameters: `{id}`, `{path...}`, `{$}`
  - Status codes: `201`, `http.StatusCreated`
  - Method calls: `GetUser(ctx, id)`

## File Extensions

| Extension | Language |
|-----------|----------|
| `.gohtml` | Go HTML Template |
| `.gotmpl`, `.tmpl` | Go Template |

## Installation

### VS Code

Copy the extension to your VS Code extensions directory:

```bash
# Linux
cp -r editor ~/.vscode/extensions/muxt-gohtml-0.1.0

# macOS
cp -r editor ~/.vscode/extensions/muxt-gohtml-0.1.0

# Windows (PowerShell)
Copy-Item -Recurse editor $env:USERPROFILE\.vscode\extensions\muxt-gohtml-0.1.0
```

Restart VS Code. Files with `.gohtml` extension will automatically use the grammar.

### Zed

Zed uses TextMate grammars. Add to your Zed configuration:

1. Create the grammar directory:
   ```bash
   mkdir -p ~/.config/zed/languages/gohtml
   ```

2. Copy the grammar files:
   ```bash
   cp editor/syntaxes/*.json ~/.config/zed/languages/gohtml/
   ```

3. Add to `~/.config/zed/settings.json`:
   ```json
   {
     "languages": {
       "Go HTML Template": {
         "grammar": "source.gotemplate",
         "path_suffixes": ["gohtml"]
       }
     }
   }
   ```

### Sublime Text

1. Find your Sublime Text packages directory:
   - Linux: `~/.config/sublime-text/Packages/`
   - macOS: `~/Library/Application Support/Sublime Text/Packages/`
   - Windows: `%APPDATA%\Sublime Text\Packages\`

2. Create a new package:
   ```bash
   mkdir -p "Packages/GoHTML"
   cp editor/syntaxes/*.json "Packages/GoHTML/"
   ```

3. Rename the files to `.tmLanguage` (Sublime prefers this extension):
   ```bash
   cd "Packages/GoHTML"
   mv gotemplate.tmLanguage.json gotemplate.tmLanguage
   mv gohtml.tmLanguage.json gohtml.tmLanguage
   ```

### TextMate

1. Open TextMate preferences
2. Go to Bundles
3. Create a new bundle named "GoHTML"
4. Import the grammar files from `editor/syntaxes/`

### Other Editors

Most modern editors support TextMate grammars (`.tmLanguage.json` or `.tmLanguage`). Consult your editor's documentation for loading custom grammars.

The grammar files are:
- `syntaxes/gotemplate.tmLanguage.json` - Core Go template syntax
- `syntaxes/gohtml.tmLanguage.json` - HTML + Go templates + muxt patterns

## Scope Names

Use these scope names when configuring your editor:

| Scope | Description |
|-------|-------------|
| `source.gotemplate` | Go template language |
| `text.html.gohtml` | HTML with embedded Go templates |

## Example

```gohtml
{{define "GET /users/{id} GetUser(ctx, id)"}}
<!DOCTYPE html>
<html>
<head>
    <title>User {{.Result.Name}}</title>
</head>
<body>
    {{if .Err}}
        <p>Error: {{.Err}}</p>
    {{else}}
        <h1>{{.Result.Name}}</h1>
        <p>ID: {{.Path "id"}}</p>
    {{end}}
</body>
</html>
{{end}}
```

## Credits

Grammar structure inspired by [casualjim/vscode-gotemplate](https://github.com/casualjim/vscode-gotemplate).
