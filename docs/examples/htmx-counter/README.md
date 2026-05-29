# Using HTMX

I originally wrote muxt while using HTMX.
I copied these from my `hypertext` package.
These methods on `TemplateData` let you use [HTMX headers](https://htmx.org/reference/#headers) from your templates.
Using these methods in templates may let you increase locality of behavior of your hypermedia API.

## Usage

Copy `htmx.go` and `htmx_test.go` into whatever package you have your generated `template_routes.go` file.
