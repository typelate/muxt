# MUXT lets you register HTTP routes from your Go HTML Templates [![Go Reference](https://pkg.go.dev/badge/github.com/crhntr/muxt.svg)](https://pkg.go.dev/github.com/crhntr/muxt) [![Go](https://github.com/crhntr/muxt/actions/workflows/go.yml/badge.svg)](https://github.com/crhntr/muxt/actions/workflows/go.yml)

This is especially helpful when you are writing HTMX.

Given the following files

### main.go

```go
package main

import (
    "html/template"
    "log"
    "net/http"
)

//go:embed *.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*"))

type Backend struct {}

func main() {
	mux := http.NewServeMux()
	muxt := Routes(mux, templates)
	
	
}
```

### index.gohtml
```html
<!DOCTYPE html>
<html lang="en">
{{block "head" "example"}}
<head>
    <meta charset='UTF-8'/>
    <title>{{.}}</title>
    <script src='https://unpkg.com/htmx.org@2.0.1' integrity='sha384-QWGpdj554B4ETpJJC9z+ZHJcA/i59TyjxEPXiiUgN2WmTyV5OEZWCD6gQhgkdpB/' crossorigin='anonymous'></script>
    <script src='https://unpkg.com/htmx-ext-response-targets@2.0.0/response-targets.js'></script>

    <link rel='stylesheet' href='https://cdn.jsdelivr.net/npm/@picocss/pico@2/css/pico.min.css'>
</head>
{{end}}
<body hx-ext='response-targets'>
<main class='container'>
    <table>
        <thead>
        <tr>
            <th>Fruit</th>
            <th>Count</th>
        </tr>
        </thead>
        <tbody>

        {{- block "fruit row" . -}}
        <tr>
            <td>{{ .Fruit }}</td>
            <td hx-get='/fruits/{{.Fruit}}/edit' hx-include='this' hx-swap='outerHTML' hx-target='closest tr'>{{ .Count }}
                <input type='hidden' name='count' value='{{.Count}}'>
            </td>
        </tr>
        {{- end -}}
        
        {{- define "GET /{} List(ctx)" -}}
            {{template "index.gohtml" .}}
        {{- end -}}

        {{- define "GET /fruits/{fruit}/edit" -}}
        <tr>
            <td>{{ .PathValue "fruit" }}</td>
            <td>
                <form hx-patch='/fruits/{{.PathValue "fruit" }}' hx-target-error="#error">
                    <input aria-label='Count' type='number' name='count' value='{{ .FormValue "count" }}' step='1' min='0'>
                    <input type='submit' value='Update'>
                </form>
                <p id='error'></p>
            </td>
        </tr>
        {{- end -}}

        {{- define "PATCH /fruits/{fruit} EditRow(response, request, fruit)" }}
        {{template "fruit row" .}}
        {{ end -}}

        </tbody>
    </table>
</main>
</body>
</html>

{{define "GET /help"}}
<!DOCTYPE html>
<html lang='us-en'>
{{template "head" "Help"}}
<body>
<main class='container'>
    Hello, help!
</main>
</body>
</html>
{{end}}
```
