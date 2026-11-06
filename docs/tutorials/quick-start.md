# Quick Start

Build a small web server whose routes muxt generates from template names. It serves a page, reads the request in a template, and swaps a fragment with no JavaScript of your own.

Prerequisite: Go 1.27 or later. Step 1 adds muxt to the module as a tool, so nothing is installed on your `PATH`.

## Step 1: Create the module

```bash
mkdir hello-muxt && cd hello-muxt
go mod init example.com/hello-muxt
go get -tool github.com/typelate/muxt
```

`go get -tool` records the muxt version in `go.mod`. The `go:generate` directive in step 2 runs that version with `go tool muxt`.

## Step 2: Create main.go

```go
package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
)

//go:embed *.gohtml
var templateFiles embed.FS

//go:generate go tool muxt generate
var templates = template.Must(template.ParseFS(templateFiles, "*.gohtml"))

func main() {
	mux := http.NewServeMux()
	TemplateRoutes(mux, nil)
	log.Fatal(http.ListenAndServe(":8080", mux))
}
```

Muxt reads the package-level [`templates` variable](../reference/templates-variable.md). `TemplateRoutes`, which step 4 generates, takes the receiver whose methods routes call. No route here calls a method, so `RoutesReceiver` is empty and `nil` satisfies it.

## Step 3: Create index.gohtml

```gotmpl
{{define "GET /"}}
<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Hello</title></head>
<body>
  <h1>Hello from Muxt!</h1>
</body>
</html>
{{end}}
```

The template name is the route: `GET` is the method, `/` the path.

## Step 4: Generate and run

```bash
go generate && go tool muxt check && go run .
```

`go generate` writes `template_routes.go`. Search it for `HandleFunc`: one call per route template, the template name as the pattern. `go tool muxt check` type-checks each template against its handler ([check](../reference/commands/check.md)). Open <http://localhost:8080>, then stop the server with Ctrl-C.

## Step 5: Read the request in a template

The template data carries the request as `.Request`. Append to `index.gohtml`:

```gotmpl
{{define "GET /hello/{name}"}}
<h1>Hello, {{.Request.PathValue "name"}}!</h1>
{{with .Request.URL.Query.Get "from"}}<p>Sent from {{.}}.</p>{{end}}
{{end}}
```

```bash
go generate && go tool muxt check && go run .
```

- <http://localhost:8080/hello/world> renders "Hello, world!"
- <http://localhost:8080/hello/world?from=Go> adds "Sent from Go."

Stop the server with Ctrl-C. `template_routes.go` also gained `ReadHelloByName`, a URL builder ([`.Path`](../reference/call-results.md#templatedata)).

## Step 6: Swap a fragment with htmz

Append a fragment route to `index.gohtml`:

```gotmpl
{{define "GET /number"}}
<div id="big-number">
  <p style="font-size:8rem;text-align:center">{{.Request.URL.Query.Get "n"}}</p>
</div>
{{end}}
```

Replace the `GET /` template in the same file:

```gotmpl
{{define "GET /"}}
<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Hello</title></head>
<body>
  <h1>Hello from Muxt!</h1>

  <form method="get" action="/number#big-number" target="htmz">
    <input type="number" name="n" value="42">
    <button>Show</button>
  </form>

  <div id="big-number">
    <p style="font-size:8rem;text-align:center">?</p>
  </div>

  <iframe hidden name=htmz onload="setTimeout(()=>document.querySelector(contentWindow.location.hash||null)?.replaceWith(...contentDocument.body.childNodes))"></iframe>
</body>
</html>
{{end}}
```

`target="htmz"` names the iframe, and the `#big-number` fragment names the element [htmz](https://leanrada.com/htmz/) replaces with the response's markup.

```bash
go generate && go tool muxt check && go run .
```

Open <http://localhost:8080>, type a number, click Show. The `GET /number` handler in `template_routes.go` has the same shape as the `GET /` handler.

## Edge cases

- **A binary install.** A binary installed with `go install github.com/typelate/muxt@latest` also works, but it can fall behind the project and then fails with errors such as `unknown command`.
- **Another directory.** Run `go tool muxt check` from the package directory, or pass `-C <dir>`.

## Next

- [Template names](../reference/template-names.md): methods, status codes, and the full syntax.
- [Find untested template behavior](find-untested-template-behavior.md): which parts of the HTML your tests hold you to.
- [Structure a project for testing](../how-to/receiver-package-and-testing.md): keep the receiver out of `package main` so tests can fake it.
