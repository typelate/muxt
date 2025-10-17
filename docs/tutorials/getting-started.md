# Tutorial: Your First Muxt Application

Build "Hello, world!" with Muxt in 10 minutes.

**Prerequisites:** Go 1.25+, basic Go and HTML knowledge

## Step 1: Install

```bash
go install github.com/typelate/muxt/cmd/muxt@latest
muxt version  # Verify
```

## Step 2: Create Project

```bash
mkdir hello-muxt && cd hello-muxt
go mod init example.com/hello
```

## Step 3: Write Template

Create `index.gohtml`:

```gotemplate
{{define "GET / F()" -}}
<!DOCTYPE html>
<html lang='en'>
<head>
    <meta charset='UTF-8'/>
    <title>Hello!</title>
</head>
<body>
<h1>{{.}}</h1>
</body>
</html>
{{- end}}
```

The template name `"GET / F()"` means: Handle GET requests to `/`, call method `F()`, pass result to template.

## Step 4: Create Main File

Create `main.go`:

```go
package main

import (
	"cmp"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
)

//go:embed *.gohtml
var templateFS embed.FS

//go:generate muxt generate --find-receiver-type=Server
var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))

func main() {
	mux := http.NewServeMux()
	// TemplateRoutes(mux, Server{})  // Uncomment after step 5
	addr := cmp.Or(os.Getenv("ADDR"), ":"+cmp.Or(os.Getenv("PORT"), "8080"))
	log.Fatal(http.ListenAndServe(addr, mux))
}

type Server struct{}

func (Server) F() string {
	return "Hello, world!"
}
```

## Step 5: Generate Handlers

```bash
go generate
```

This creates `template_routes.go` and `index_template_routes_gen.go`.

## Step 6: Wire Routes

Uncomment the `TemplateRoutes` line in `main.go`:

```go
func main() {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})  // Uncommented
	addr := cmp.Or(os.Getenv("ADDR"), ":"+cmp.Or(os.Getenv("PORT"), "8080"))
	log.Fatal(http.ListenAndServe(addr, mux))
}
```

## Step 7: Run

```bash
go run .
```

Visit `http://localhost:8080` to see "Hello, world!"

## What Happened

Template name → Muxt generates handler → Ship working server.

No framework. No boilerplate. Just Go and HTML.

## Next

- [Integrate into existing project](../how-to/integrate-existing-project.md)
- [Template name syntax](../reference/template-names.md) - paths, parameters, status codes
- [Write receiver methods](../how-to/write-receiver-methods.md) - real-world data