# Getting Started

This guide walks you through installing Muxt and generating your first routes from HTML templates.

## 1. Quick Overview

- **Code Generator, Not a Framework**  
  Muxt scans your `.gohtml` files for route definitions (like `GET /`, `POST /signup`, etc.) and **generates** Go code
  to register handlers on `*http.ServeMux`.
- **Minimal & Testable**  
  Muxt avoids large, complex dependencies. Your final program is just Go code. That means you can test each handler
  easily.
- **(Optional) Template Type Checking**  
  Muxt can also **statically analyze** template call signatures—helping you catch mistakes early (e.g., passing the
  wrong argument types to your route methods).

## 2. Installation

You do not need to import `muxt` into your module unless you want it as a [dev tool dependency (when Go 1.24 comes out)](https://tip.golang.org/doc/modules/managing-dependencies#tools).

For a global-install run:

```bash
go install github.com/typelate/muxt/cmd/muxt@latest
```

## 3. Generating Your First Routes

In this example, `muxt` will generate a function registering a handler for the HTTP request `GET /`
It will return a response with the text "Hello, world!".

### Create the "html/template" entrypoint

Add the following code to a new or existing Go source file. I usually call this file "templates.go"

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
	// TemplateRoutes(mux, Server{}) // un-comment this after you run `muxt generate`
	log.Fatal(http.ListenAndServe(":"+cmp.Or(os.Getenv("PORT"), "8080"), mux))
}

type Server struct{}

func (Server) F() string {
	return "Hello, world!"
}

```

### Create a "Hello, world!" page template

Create a file with the extention ".gohtml".

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

### Generate and Run

Now run `go generate`

*See [the Go blog article on Go generate](https://go.dev/blog/generate) to learn more.*

Un-comment the line `// TemplateRoutes(mux, Server{})`

Run `go run .`

Access the server at `http://localhost:8080`.

## Reading the generated code

*Note, this may change in patch releases of muxt. I will do my best to keep this updated.*

```go
package server

import (
  "bytes"
  "net/http"
)

type RoutesReceiver interface {
  F() any
}

func TemplateRoutes(mux *http.ServeMux, receiver RoutesReceiver) {
  mux.HandleFunc("GET /", func(response http.ResponseWriter, request *http.Request) {
    result := receiver.F()
    buf := bytes.NewBuffer(nil)
    rd := newTemplateData(result, request)
    if err := templates.ExecuteTemplate(buf, "GET / F()", rd); err != nil {
      http.Error(response, err.Error(), http.StatusInternalServerError)
      return
    }
    response.Header().Set("content-type", "text/html; charset=utf-8")
    response.WriteHeader(http.StatusOK)
    _, _ = buf.WriteTo(response)
  })
}

type TemplateData[T any] struct {
  Request *http.Request
  Result  T
}

func newTemplateData[T any](result T, request *http.Request) TemplateData[T] {
  return TemplateData[T]{Result: result, Request: request}
}
```

Starting with the `package main`, muxt will generate the template_routes file in the current directory in the non-test
package.

The 2 standard library `import`s here are minimal.
The generated routes function uses net/http.
The (optionally) generated execute function uses the byte buffer.

The named empty interface RoutesReceiver has one method `F() string`.
The method signature was discovered by muxt by iterating over the methods on the named receiver `type Server`.

`func TemplateRoutes` is where generated (inline) http.HandlerFunc closures are mapped to http routes on the
multiplexer.
It receives a pointer to the `http.ServeMux` if you have any route collisions from routes added on mux before
or after calling `TemplateRoutes`, `mux.HandleFunc` will panic.
The endpoint string `GET /` is cut out of the template name.
Inside the http handler func, the named method is called.
The result is then passed to execute.
