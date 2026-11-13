# How to share template fragments between packages

You are splitting a hypertext package into sub-packages, and they should keep one layout and one set of partials. The steps below use a `fragments` package, plus `workspace` and `billing` as the sub-packages.

1. Create `fragments/layout.gohtml`:

   ```gotmpl
   {{define "layout"}}<main data-layout="shared"><h1>{{upper .Title}}</h1>{{.Body}}</main>{{end}}
   ```

2. Create `fragments/fragments.go`. It exports the `embed.FS` and the function the layout calls:

   ```go
   package fragments

   import (
   	"embed"
   	"strings"
   )

   //go:embed layout.gohtml
   var TemplateSource embed.FS

   func Upper(s string) string { return strings.ToUpper(s) }
   ```

3. In `workspace/workspace.go`, parse the shared FS first and the package's own files after it. Register `Upper` in a `FuncMap` literal; `muxt` reads the literal, not a variable of another package:

   ```go
   //go:embed *.gohtml
   var templateSource embed.FS

   var templates = template.Must(template.Must(template.New("").
   	Funcs(template.FuncMap{"upper": fragments.Upper}).
   	ParseFS(fragments.TemplateSource, "*.gohtml")).
   	ParseFS(templateSource, "*.gohtml"))
   ```

   Passing `Funcs(fragments.Funcs)` instead fails with `to share functions across packages, export them and write template.FuncMap{"name": fragments.Func}`.

4. In `workspace/page.gohtml`, call the shared template:

   ```gotmpl
   {{define "GET /{$} Show()"}}{{template "layout" .Result}}{{end}}
   ```

5. Repeat steps 3 and 4 in `billing`, then generate both packages from the top of the module ([muxt.json](muxt-json.md)) and check each one:

   ```bash
   muxt generate
   cd workspace && muxt check
   ```

   ```text
   ok: 1 template
   ```

   The shared FS must be an exported `embed.FS` of a package in the same module.

## If a shared file holds a route

A route template in `fragments` becomes a route of every package that parses it. Two such packages on one mux panic when the routes are registered:

```text
panic: pattern "GET /about" (registered at .../billing/template_routes.go:30) conflicts with pattern "GET /about" (registered at .../workspace/template_routes.go:30)
```

Keep routes in each package's own files, or give each package its own mux.

## Replace a shared template in one package

Define the same name in the package's own file, and parse the shared FS first. The later definition wins, as in `html/template`. Bare `muxt` in that package lists it:

```text
Overrides:
  - layout
      replaced: /home/me/app/fragments/layout.gohtml:1:1
      kept:     /home/me/app/workspace/over.gohtml:1:1
```

Parsing the shared FS after the package's own files, so a shared file replaces the package's own template, is an error in every command:

```text
/home/me/app/workspace/over.gohtml:1:1: template "layout" is replaced by /home/me/app/fragments/layout.gohtml:1:1; parse the shared embed.FS before this package's own files
```

The same module, run end to end: [howto_shared_template_fragments.txt](../../cmd/muxt/testdata/howto_shared_template_fragments.txt). Rules, errors and which shared templates `muxt check` reports unused: [Templates Variable](../reference/templates-variable.md#sharing-another-packages-files).
