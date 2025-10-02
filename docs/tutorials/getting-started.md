# Tutorial: Your First Muxt Application

Build a "Hello, world!" web app with Muxt. Should take about 10 minutes.

By the end, you'll have a working server that displays "Hello, world!" using generated HTTP handlers.

## Prerequisites

- Go 1.22 or later
- You can write basic Go and HTML

## Step 1: Install Muxt

Install Muxt globally:

```bash
go install github.com/typelate/muxt@latest
```

Verify the installation:

```bash
muxt --help
```

## Step 2: Create a New Project

Create a new directory for your project:

```bash
mkdir hello-muxt
cd hello-muxt
go mod init example.com/hello
```

## Step 3: Write Your First Template

Create a file named `index.gohtml`:

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

**What's happening here:**
- `GET /` tells Muxt this template handles GET requests to the root path
- `F()` specifies which method provides the data for this template
- `{{.}}` will be replaced with the result from the `F()` method

## Step 4: Create the Go Entry Point

Create a file named `main.go`:

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

//go:generate muxt generate --receiver-type=Server
var templates = template.Must(template.ParseFS(templateFS, "*.gohtml"))

func main() {
	mux := http.NewServeMux()
	// TemplateRoutes(mux, Server{}) // You'll uncomment this after generating
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8080"), mux))
}

type Server struct{}

func (Server) F() string {
	return "Hello, world!"
}
```

## Step 5: Generate the Handler Code

Run code generation:

```bash
go generate
```

Muxt will create a file named `template_routes.go` containing the generated HTTP handlers.

## Step 6: Wire Up the Generated Routes

In `main.go`, uncomment the `TemplateRoutes` line:

```go
func main() {
	mux := http.NewServeMux()
	TemplateRoutes(mux, Server{})  // Uncommented!
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8080"), mux))
}
```

## Step 7: Run Your Application

Start the server:

```bash
go run .
```

Open your browser to `http://localhost:8080`. You should see an H1 heading displaying "Hello, world!".

## What Just Happened

You wrote a template. Muxt generated the HTTP handler. You shipped a working server.

No framework. No boilerplate. No magic.

The template name `"GET / F()"` told Muxt:
- Handle GET requests to `/`
- Call the `F()` method
- Pass the result to the template

Everything else is just normal Go and normal HTML.

## Next Steps

Now that you've got the basics, you can:
- [Integrate Muxt into an existing project](../how-to/integrate-existing-project.md)
- Learn the [template name syntax](../reference/template-names.md) to handle paths, parameters, and status codes
- Read about [writing receiver methods](../how-to/write-receiver-methods.md) for real-world data

Or just start building. The best way to learn is to ship something.