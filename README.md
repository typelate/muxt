# Muxt [![Go Reference](https://pkg.go.dev/badge/github.com/crhntr/muxt.svg)](https://pkg.go.dev/github.com/crhntr/muxt) [![Go](https://github.com/crhntr/muxt/actions/workflows/go.yml/badge.svg)](https://github.com/crhntr/muxt/actions/workflows/go.yml)

**Muxt** is a Go code generator that helps you build server-side rendered web apps with minimal overhead, leveraging Go
1.22’s improved `http.ServeMux` and standard `html/template` features.
No extra runtime dependencies are required—just plain Go code.

- It allows you to register HTTP routes from [HTML templates](https://pkg.go.dev/html/template)
- It generates handler functions and registers them on an [`*http.ServeMux`](https://pkg.go.dev/net/http#ServeMux)
- It generates code in handler functions to parse path parameters and form fields
- It generates a receiver interface to represent the boundary between your app code and HTTP/HTML
	- Use this to mock out your server and test the view layer of your application

## Examples

For more complete examples see:
- [muxt-example-htmx-sortable](http://github.com/crhntr/muxt-example-htmx-sortable) _**(NEW)**_
- [muxt-template-module-htmx](https://github.com/crhntr/muxt-template-module-htmx)

## Documentation

### Introduction
- [Getting Started](./docs/getting_started.md)
- [Notes on Integration with Existing Projects](./docs/integrating.md)
- [Writing Template Names](./docs/template_names.md)

### Reference
- [Call Parameters](./docs/call_parameters.md)
- [Call Results](./docs/call_results.md)
- [Templates Variable](./docs/templates_variable.md)
- [Template Action Type-Checking](./docs/action_type_checking.md)
- [Known Issues](./docs/known_issues.md)

### Testing
- [Testing_Hypertext](./docs/testing_hypertext.md)
- [Testing_the_Receiver](./docs/testing_the_receiver.md)

### Philosophy & Vision
- [Manifesto](./docs/manifesto.md)
- [Motivation](./docs/motivation.md)
- Goals:
  [see enhancement issues](https://github.com/crhntr/muxt/issues?q=is%3Aissue%20state%3Aopen%20label%3Aenhancement)

